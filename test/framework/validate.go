package framework

import (
	"context"
	"strings"
	"testing"

	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func ValidateEvictionDenied(t *testing.T, responses map[string]error, expectedCode int32, expectedMessage, podName string) {
	if statusErr, ok := responses[podName].(*errors.StatusError); ok {
		if statusErr.Status().Code != expectedCode {
			t.Fatalf("Expected code %d, got %d for pod %s, with message %s", expectedCode, statusErr.Status().Code, podName, statusErr.Status().Message)
		}
		if !strings.Contains(statusErr.Status().Message, expectedMessage) {
			t.Fatalf("Expected message %s, got %s for pod %s", expectedMessage, statusErr.Status().Message, podName)
		}
	} else {
		t.Fatalf("Expected error to be a StatusError, got %T", responses[podName])
	}
}

func ValidateEvictionAllowed(t *testing.T, responses map[string]error, podName string) {
	if err, ok := responses[podName]; ok && err != nil {
		t.Fatalf("Expected no error, got %v for pod %s", err, podName)
	}
}

// ValidatePodHasRescheduleAnnotation asserts the pod has the reschedule annotation.
func (tc *TestCluster) ValidatePodHasAnnotation(t *testing.T, podName string, annotationKey, annotationValue string) {
	tc.validatePodAnnotation(t, podName, annotationKey, annotationValue, true)
}

// ValidatePodDoesNotHaveRescheduleAnnotation asserts the pod does not have the reschedule annotation.
func (tc *TestCluster) ValidatePodDoesNotHaveAnnotation(t *testing.T, podName string, annotationKey, annotationValue string) {
	tc.validatePodAnnotation(t, podName, annotationKey, annotationValue, false)
}

// ValidatePodRescheduleAnnotation checks if a pod has (or does not have) the reschedule annotation.
func (tc *TestCluster) validatePodAnnotation(t *testing.T, podName string, annotationKey, annotationValue string, expectPresent bool) {
	pod := tc.MustGetPod(t, podName)
	val := pod.Annotations[annotationKey]
	if expectPresent && val != annotationValue {
		t.Fatalf("Expected pod %s to have annotation %s with value %s", podName, annotationKey, annotationValue)
	}
	if !expectPresent && val == annotationValue {
		t.Fatalf("Expected pod %s not to have annotation %s with value %s", podName, annotationKey, annotationValue)
	}
}

// ValidatePodHasBeenEvicted asserts the pod has been evicted, that being it no longer exists or is terminating.
func (tc *TestCluster) ValidatePodHasBeenEvicted(t *testing.T, podName string) {
	pod, err := tc.client.CoreV1().Pods(tc.GetNamespace()).Get(context.TODO(), podName, metav1.GetOptions{})
	if err != nil {
		if !errors.IsNotFound(err) {
			t.Fatalf("Unexpected error getting pod %s: %v", podName, err)
		}
		return
	}

	// If pod exists, check if it's terminating
	if pod.DeletionTimestamp == nil {
		t.Fatalf("Expected pod %s to either not exist or be terminating", podName)
	}
}
