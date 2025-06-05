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
	"syscall"
	"time"

	admissionv1 "k8s.io/api/admission/v1"
	policyv1 "k8s.io/api/policy/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

const (
	RescheduleAnnotation         = "cao.couchbase.com/reschedule"
	RescheduleHookFlagAnnotation = "reschedule.hook/added"
	RescheduleTrue               = "true"
	CouchbasePodLabelKey         = "app"
	CouchbasePodLabelValue       = "couchbase"
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

func getKubeClient() (kubernetes.Interface, error) {
	kubeConfig, err := rest.InClusterConfig()
	if err != nil {
		return nil, err
	}

	clientset, err := kubernetes.NewForConfig(kubeConfig)
	if err != nil {
		return nil, err
	}

	return clientset, nil
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
	slog.Info("Pod eviction request received")

	// Read the POST body content
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
	client, err := getKubeClient()
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

func handleEviction(eviction policyv1.Eviction, client kubernetes.Interface) *admissionv1.AdmissionResponse {
	slog.Info("Handling eviction request",
		"pod", eviction.Name,
		"namespace", eviction.Namespace)

	pod, err := client.CoreV1().Pods(eviction.Namespace).Get(context.TODO(), eviction.Name, metav1.GetOptions{})

	// TODO It's possible that the pod has been rescheduled, therefore if the pod does not exist, we should allow drain to continue
	// IF an inplaceupgrade has taken place, we need to consider that the pod may have the same name but already rescheduled
	if err != nil {
		if k8serrors.IsNotFound(err) {
			slog.Info("Pod has been rescheduled", "pod", eviction.Name, "namespace", eviction.Namespace)
			return allowEviction()
		}
		slog.Error("Failed to get pod", "error", err)
		return denyEviction(http.StatusInternalServerError, metav1.StatusReasonInternalError, "Failed to get pod")
	}

	if pod.Labels[CouchbasePodLabelKey] != CouchbasePodLabelValue {
		slog.Info("Pod is not a couchbase pod", "pod", pod.Name, "namespace", pod.Namespace)
		return allowEviction()
	}

	if reschedule, exists := pod.GetAnnotations()[RescheduleAnnotation]; exists && reschedule == RescheduleTrue {
		slog.Info("Couchbase pod awaiting reschedule", "pod", pod.Name, "namespace", pod.Namespace)
		return denyEviction(http.StatusTooManyRequests, metav1.StatusReasonTooManyRequests, "Couchbase pod awaiting reschedule")
	} else if !exists {
		// TODO We need to add a second annotation here to mark that we have added a reschedule annotation.
		slog.Info("Adding annotations to pod", "pod", pod.Name, "namespace", pod.Namespace)
		// If both annotations are present, we can quit
		// If only the mark annotation is present, we can assume the pod has been rescheduled and skip/return to break the kubectl for loop to allow the eviction
		annotations := pod.GetAnnotations()
		if annotations == nil {
			annotations = make(map[string]string)
		}

		annotations[RescheduleAnnotation] = RescheduleTrue
		pod.SetAnnotations(annotations)

		_, err = client.CoreV1().Pods(eviction.Namespace).Update(context.TODO(), pod, metav1.UpdateOptions{})
		if err != nil {
			slog.Error("Failed to add reschedule annotation to pod", "error", err)
			return denyEviction(http.StatusInternalServerError, metav1.StatusReasonInternalError, "Failed to add reschedule annotation to pod")
		}

		// TODO If the pod has already been evicted, we need to exit the drain request loop as the pod no longer exists but kubectl will continue to try to evict it as it isn't refreshed
		// TODO Potentially, we only need to add the reschedule annotation if the pod is on a cordoned node?
		return denyEviction(http.StatusTooManyRequests, metav1.StatusReasonTooManyRequests, "Reschedule annotation added to pod")
	}

	// If we get to this point, we can assume all is fine and dandy and allow the eviction request to continue
	return allowEviction()
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
