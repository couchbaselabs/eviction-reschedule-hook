package reschedule

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"slices"
	"syscall"
	"time"

	admissionv1 "k8s.io/api/admission/v1"
	policyv1 "k8s.io/api/policy/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	CouchbasePodLabelKey   = "app"
	CouchbasePodLabelValue = "couchbase"
	TrackRescheduledPods   = true
)

func tlsConfig() *tls.Config {
	// TODO We should be using a config here instead of env/static values for these. This should be ok for now.
	certFile := os.Getenv("TLS_CERT_FILE")
	keyFile := os.Getenv("TLS_KEY_FILE")

	if certFile == "" {
		certFile = "/etc/webhook/certs/tls.crt"
	}

	if keyFile == "" {
		keyFile = "/etc/webhook/certs/tls.key"
	}

	cert, err := tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		slog.Error("Unable to load TLS certificate", "error", err)
		os.Exit(1)
	}

	return &tls.Config{
		Certificates: []tls.Certificate{cert},
	}
}

func Serve() {
	mux := http.NewServeMux()
	mux.HandleFunc("/", serveDefault)
	mux.HandleFunc("/readyz", serveReadiness)
	mux.HandleFunc("/eviction", serveEviction)

	tlsConfig := tlsConfig()
	server := &http.Server{
		Addr:         ":8443",
		TLSConfig:    tlsConfig,
		Handler:      mux,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  30 * time.Second,
	}

	go func() {
		slog.Info("Starting reschedule hook server")
		if err := server.ListenAndServeTLS("", ""); !errors.Is(err, http.ErrServerClosed) {
			slog.Error("Server failed to start", "error", err)
		}
	}()

	// Gracefully handle server shutdown
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)

	<-stop
	slog.Info("Shutting down reschedule hook server")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		slog.Error("Server shutdown failed", "error", err)
	}

	slog.Info("Server exited")
}

func serveReadiness(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
}

func serveDefault(w http.ResponseWriter, r *http.Request) {
	slog.Error("Unexpected request", "path", r.URL.String())
	w.WriteHeader(http.StatusNotFound)
}

func serveEviction(w http.ResponseWriter, r *http.Request) {
	var body []byte
	if r.Body != nil {
		if data, err := io.ReadAll(r.Body); err == nil {
			body = data
		} else {
			slog.Error("Failed to read request body", "error", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
	}

	// Ensure the content is JSON before decoding it
	contentType := r.Header.Get("Content-Type")
	if contentType != "application/json" {
		slog.Error("Unsupported Content-Type", "content-type", contentType)
		w.WriteHeader(http.StatusUnsupportedMediaType)
		return
	}

	// Decode the request body into an admission review request
	var reviewRequest admissionv1.AdmissionReview
	if err := json.Unmarshal(body, &reviewRequest); err != nil {
		slog.Error("Failed to decode admission review", "error", err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	// Decode the review body into an eviction request
	var eviction policyv1.Eviction
	if err := json.Unmarshal(reviewRequest.Request.Object.Raw, &eviction); err != nil {
		slog.Error("Failed to decode eviction request", "error", err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	// Initialise the Kubernetes client
	client, err := NewClient()
	if err != nil {
		slog.Error("Failed to create Kubernetes client", "error", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	// Handle the eviction request
	response := handleEviction(eviction, client)
	// Set the UID of the response to the UID of the request
	response.UID = reviewRequest.Request.UID

	// Create the admission review response
	review := admissionv1.AdmissionReview{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "admission.k8s.io/v1",
			Kind:       "AdmissionReview",
		},
		Response: response,
	}

	// Marshal to JSON and write the response
	resp, err := json.Marshal(review)
	if err != nil {
		slog.Error("Failed to encode admission review response", "error", err)
	}

	w.Header().Set("Content-Type", "application/json")
	if _, err := w.Write(resp); err != nil {
		slog.Error("Failed to write admission review response", "error", err)
	}
}

func handleEviction(eviction policyv1.Eviction, client Client) *admissionv1.AdmissionResponse {
	slog.Info("Handling eviction request",
		"pod", eviction.Name,
		"namespace", eviction.Namespace)

	pod, err := client.GetPod(eviction.Namespace, eviction.Name)
	// If the pod doesn't exist, we can assume that it has already been recreated with a different name since the drain request
	if err != nil {
		if k8serrors.IsNotFound(err) {
			slog.Info("Pod has been rescheduled", "pod", eviction.Name, "namespace", eviction.Namespace)
			return allowEviction()
		}

		slog.Error("Failed to get pod", "error", err)
		return denyEviction(http.StatusInternalServerError, metav1.StatusReasonInternalError, "Failed to get pod")
	}

	if pod.Labels[CouchbasePodLabelKey] != CouchbasePodLabelValue {
		slog.Info("Pod is not a couchbase node", "pod", pod.Name, "namespace", pod.Namespace)
		return allowEviction()
	}

	// If the pod has already been marked for rescheduling, we can exit here but deny the eviction to keep the drain command
	// in a loop until the pod no longer exists.
	if reschedule, exists := pod.GetAnnotations()[RescheduleAnnotation]; exists && reschedule == RescheduleTrue {
		slog.Info("Couchbase pod awaiting reschedule", "pod", pod.Name, "namespace", pod.Namespace)
		return denyEviction(http.StatusTooManyRequests, metav1.StatusReasonTooManyRequests, "Couchbase pod awaiting reschedule")
	}

	// If the pod has not been marked for rescheduling, it's possible it has already been rescheduled with the same name.
	// We can track pods that have been already been marked for rescheduling using an annotation on the CouchbaseCluster
	// resource the pod (couchbase node) belongs to. We will only add to this list if the cluster is using InPlaceUpgrade, but
	// will always attempt to remove the pod from the list.
	// Skip this step by setting the TrackRescheduledPods flag to false.
	if TrackRescheduledPods {
		clusterName := pod.Labels[CouchbaseClusterLabelKey]

		response := handlePreviouslyRescheduledPodsList(pod.Name, clusterName, eviction.Namespace, client, true)
		if response != nil {
			return response
		}
	}

	// At this point, we can assume the pod has not already been rescheduled and should therefore be marked for rescheduling.
	slog.Info("Adding reschedule annotation to pod", "pod", pod.Name, "namespace", pod.Namespace)
	err = client.ReschedulePod(pod)
	if err != nil {
		slog.Error("Failed to add reschedule annotation to pod", "error", err)
		return denyEviction(http.StatusInternalServerError, metav1.StatusReasonInternalError, "Failed to add reschedule annotation to pod")
	}

	// By denying the eviction with StatusReasonTooManyRequests, the drain command will continue attempting to evict
	// the pod every 5 seconds until it has been rescheduled correctly.
	return denyEviction(http.StatusTooManyRequests, metav1.StatusReasonTooManyRequests, "Reschedule annotation added to pod")
}

// handlePreviouslyRescheduledPodsList handles situations where a pod may have been rescheduled with the same name. This method will
// check an annotation on the couchbase cluster resource for a list of pods that have been rescheduled.
// When calling this method, we know that the pod does not have the reschedule annotation. Therefore, if it is present in the list,
// we can assume that it has already been rescheduled and allow the eviction to proceed. If it is not present in the list
// and the cluster is using InPlaceUpgrade, the pod will be added to the list, in preparation for the next eviction attempt.
func handlePreviouslyRescheduledPodsList(pod, cluster, namespace string, client Client, shouldUpdateList bool) *admissionv1.AdmissionResponse {
	clusterInfo, err := client.GetClusterInfo(cluster, namespace)
	if err != nil {
		slog.Error("error fetching required cluster info", "error", err)
		return denyEviction(http.StatusInternalServerError, metav1.StatusReasonInternalError, "Failed to get required cluster info")
	}

	if slices.Contains(clusterInfo.rescheduleHookPodsList, pod) {
		slog.Info("Pod has already been rescheduled, removing from tracking list if needed", "pod", pod, "namespace", namespace)
		err = client.PatchRescheduleHookPodsList(cluster, namespace, slices.DeleteFunc(clusterInfo.rescheduleHookPodsList, func(s string) bool {
			return s == pod
		}))

		if err != nil {
			slog.Error("Failed to remove pod from tracking list", "error", err)
			return denyEviction(http.StatusInternalServerError, metav1.StatusReasonInternalError, "Failed to remove pod from tracking list")
		}

		return allowEviction()
	}

	if shouldUpdateList {
		err = client.PatchRescheduleHookPodsList(cluster, namespace, append(clusterInfo.rescheduleHookPodsList, pod))

		if err != nil {
			slog.Error("Failed to update tracking list", "error", err)
			return denyEviction(http.StatusInternalServerError, metav1.StatusReasonInternalError, "Failed to update rescheduled pods tracking list")
		}
	}

	return nil
}

func denyEviction(code int32, reason metav1.StatusReason, message string) *admissionv1.AdmissionResponse {
	return &admissionv1.AdmissionResponse{
		Allowed: false,
		Result: &metav1.Status{
			Status:  "Failure",
			Message: message,
			Reason:  metav1.StatusReasonInvalid,
			Code:    code,
		},
	}
}

func allowEviction() *admissionv1.AdmissionResponse {
	return &admissionv1.AdmissionResponse{
		Allowed: true,
	}
}
