package reschedule

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	admissionv1 "k8s.io/api/admission/v1"
	corev1 "k8s.io/api/core/v1"
	policyv1 "k8s.io/api/policy/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	PodWaitingForRescheduleMsg                        = "Pod waiting to be rescheduled"
	PodRescheduledMsg                                 = "Pod has been rescheduled"
	PodRescheduledWithSameNameMsg                     = "Pod has been rescheduled with the same name"
	RescheduleAnnotationAddedToPodMsg                 = "Reschedule annotation added to pod"
	FailedToAddRescheduleAnnotationMsg                = "Failed to add reschedule annotation to pod"
	FailedToGetTrackingResourceMsg                    = "Failed to get rescheduled pods tracking resource"
	FailedToRemoveRescheduleHookTrackingAnnotationMsg = "Failed to remove tracking annotation from rescheduled pods tracking resource"
	FailedToGetPodMsg                                 = "Failed to get pod"
	FailedToAddRescheduleHookTrackingAnnotationMsg    = "Failed to add annotation to rescheduled pods tracking resource"
)

func tlsConfig(config *Config) *tls.Config {
	cert, err := tls.LoadX509KeyPair(config.certFile, config.keyFile)
	if err != nil {
		slog.Error("Unable to load TLS certificate", "error", err)
		os.Exit(1)
	}

	return &tls.Config{
		Certificates: []tls.Certificate{cert},
	}
}

func Serve() {
	// Config is loaded from environment variables or default values if not set
	config := NewConfigBuilder().FromEnvironment().Build()

	mux := http.NewServeMux()
	mux.HandleFunc("/", serveDefault)
	mux.HandleFunc("/readyz", serveReadiness)
	mux.HandleFunc("/eviction", func(w http.ResponseWriter, r *http.Request) {
		serveEviction(w, r, config)
	})

	tlsConfig := tlsConfig(config)
	server := &http.Server{
		Addr:         ":8443",
		TLSConfig:    tlsConfig,
		Handler:      mux,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  30 * time.Second,
	}

	go func() {
		slog.Info("Reschedule hook server started")
		config.Print()
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

func serveEviction(w http.ResponseWriter, r *http.Request, config *Config) {
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
	client, err := NewClient(config)
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

	pod, err := client.GetPod(eviction.Name, eviction.Namespace)
	// If the pod doesn't exist, we can assume that it has already been evicted
	if err != nil {
		if k8serrors.IsNotFound(err) {
			slog.Info("Pod no longer exists", "pod", eviction.Name, "namespace", eviction.Namespace)
			return denyEviction(http.StatusNotFound, metav1.StatusReasonNotFound, PodRescheduledMsg)
		}

		slog.Error("Failed to get pod", "error", err)
		return denyEviction(http.StatusInternalServerError, metav1.StatusReasonInternalError, FailedToGetPodMsg)
	}

	// If the pod does not have the correct label, we can allow the eviction immediately
	if pod.Labels[client.GetConfig().podLabelSelectorKey] != client.GetConfig().podLabelSelectorValue {
		slog.Info(fmt.Sprintf("Pod does not have the %s=%s label, eviction allowed", client.GetConfig().podLabelSelectorKey, client.GetConfig().podLabelSelectorValue), "pod", pod.Name, "namespace", pod.Namespace)
		return allowEviction()
	}

	// If the pod has already been marked for rescheduling, we can exit here but deny the eviction to keep the drain command
	// in a loop until the pod no longer exists
	if reschedule, exists := pod.GetAnnotations()[client.GetConfig().rescheduleAnnotationKey]; exists && reschedule == client.GetConfig().rescheduleAnnotationValue {
		slog.Info("Pod waiting to be rescheduled", "pod", pod.Name, "namespace", pod.Namespace)
		return denyEviction(http.StatusTooManyRequests, metav1.StatusReasonTooManyRequests, PodWaitingForRescheduleMsg)
	}

	// If the pod does not have the reschedule annotation, it's possible it has already been rescheduled with the same name.
	// When the TrackRescheduledPods config value has been enabled, we will use an annotation on another resource to track which pods have already been rescheduled
	// If the pod is missing the reschedule annotation, but is present in this tracking list, we can assume it has already been rescheduled with the same name
	if client.ShouldTrackRescheduledPods() {
		response := trackRescheduledPods(client, pod)
		if response != nil {
			return response
		}
	}

	// At this point, we can assume the pod has not already been rescheduled and should therefore be marked for rescheduling
	slog.Info("Adding reschedule annotation to pod", "pod", pod.Name, "namespace", pod.Namespace)
	err = client.ReschedulePod(pod)
	if err != nil {
		slog.Error("Failed to add reschedule annotation to pod", "error", err)
		return denyEviction(http.StatusInternalServerError, metav1.StatusReasonInternalError, FailedToAddRescheduleAnnotationMsg)
	}

	// By denying the eviction with StatusReasonTooManyRequests, the drain command will continue attempting to evict
	// the pod every 5 seconds until it has been rescheduled correctly
	return denyEviction(http.StatusTooManyRequests, metav1.StatusReasonTooManyRequests, RescheduleAnnotationAddedToPodMsg)
}

// trackRescheduledPods handles situations where a pod may have been rescheduled with the same name. This method will
// check for the existence of a tracking annotation on the tracking resource.
// If a tracking annotation already exists for the pod, it must have already been rescheduled with the same name.
// We can therefore remove the tracking annotation and return a 404.
// If the tracking resource does not have a tracking annotation for the pod and the pod will be rescheduled with the same name,
// we will add a tracking annotation before marking the pod for rescheduling.
func trackRescheduledPods(client Client, pod *corev1.Pod) *admissionv1.AdmissionResponse {
	trackingResourceInstance, err := client.GetTrackingResourceInstance(client.GetConfig().trackingResource.GetInstanceName(pod), pod.Namespace)
	if err != nil {
		slog.Error("Failed to get tracking resource", "error", err)
		return denyEviction(http.StatusInternalServerError, metav1.StatusReasonInternalError, FailedToGetTrackingResourceMsg)
	}

	if val, exists := trackingResourceInstance.GetAnnotations()[TrackingResourceAnnotation(pod.Name, pod.Namespace)]; exists && val == "true" {
		slog.Info("Pod has been rescheduled with the same name", "pod", pod.Name, "namespace", pod.Namespace)

		err = client.RemoveRescheduleHookTrackingAnnotation(pod.Name, pod.Namespace, trackingResourceInstance.GetName())
		if err != nil {
			slog.Error("Failed to remove tracking annotation", "error", err)
			return denyEviction(http.StatusInternalServerError, metav1.StatusReasonInternalError, FailedToRemoveRescheduleHookTrackingAnnotationMsg)
		}

		return denyEviction(http.StatusNotFound, metav1.StatusReasonNotFound, PodRescheduledWithSameNameMsg)
	}

	// If we want to track the rescheduled pods (this may be conditional on the tracking resource type), we can add an annotation to the tracking resource
	if client.ShouldAddTrackingAnnotation(trackingResourceInstance) {
		slog.Info("Pod will be rescheduled with the same name, adding annotation to tracking resource", "pod", pod.Name, "namespace", pod.Namespace, "trackingResource", trackingResourceInstance.GetName())
		err = client.AddRescheduleHookTrackingAnnotation(pod.Name, pod.Namespace, trackingResourceInstance.GetName())
		if err != nil {
			slog.Error("Failed to add tracking annotation", "error", err)
			return denyEviction(http.StatusInternalServerError, metav1.StatusReasonInternalError, FailedToAddRescheduleHookTrackingAnnotationMsg)
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
			Reason:  reason,
			Code:    code,
		},
	}
}

func allowEviction() *admissionv1.AdmissionResponse {
	return &admissionv1.AdmissionResponse{
		Allowed: true,
	}
}
