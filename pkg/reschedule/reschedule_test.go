package reschedule

import (
	"net/http"
	"reflect"
	"testing"

	admissionv1 "k8s.io/api/admission/v1"
	corev1 "k8s.io/api/core/v1"
	policyv1 "k8s.io/api/policy/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

type mockClient struct {
	pod                         *corev1.Pod
	config                      *Config
	trackingResourceAnnotations map[string]string
	shouldTrackRescheduledPods  bool
	shouldAddTrackingAnnotation bool
}

func (m *mockClient) GetPod(name, namespace string) (*corev1.Pod, error) {
	if m.pod == nil {
		return nil, k8serrors.NewNotFound(schema.GroupResource{Group: "", Resource: "pods"}, name)
	}
	return m.pod, nil
}

func (m *mockClient) ReschedulePod(pod *corev1.Pod) error {
	if pod.Annotations == nil {
		pod.Annotations = make(map[string]string)
	}

	pod.Annotations[m.config.rescheduleAnnotationKey] = m.config.rescheduleAnnotationValue
	m.pod = pod
	return nil
}

func (m *mockClient) GetTrackingResourceInstance(name, namespace string) (*unstructured.Unstructured, error) {
	return &unstructured.Unstructured{Object: map[string]interface{}{
		"metadata": map[string]interface{}{
			"annotations": stringMapToInterfaceMap(m.trackingResourceAnnotations),
		},
	}}, nil
}

func stringMapToInterfaceMap(in map[string]string) map[string]interface{} {
	out := make(map[string]interface{}, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func (m *mockClient) GetConfig() *Config {
	return m.config
}

func (m *mockClient) AddRescheduleHookTrackingAnnotation(podName, podNamespace, trackingResourceName string) error {
	if m.trackingResourceAnnotations == nil {
		m.trackingResourceAnnotations = make(map[string]string)
	}
	m.trackingResourceAnnotations[TrackingResourceAnnotation(podName, podNamespace)] = "true"
	return nil
}

func (m *mockClient) RemoveRescheduleHookTrackingAnnotation(podName, podNamespace, trackingResourceName string) error {
	delete(m.trackingResourceAnnotations, TrackingResourceAnnotation(podName, podNamespace))
	return nil
}

func (m *mockClient) ShouldTrackRescheduledPods() bool {
	return m.shouldTrackRescheduledPods
}

func (m *mockClient) ShouldAddTrackingAnnotation(trackingResourceInstance *unstructured.Unstructured) bool {
	return m.shouldAddTrackingAnnotation
}

func TestHandleEviction(t *testing.T) {
	testcases := []struct {
		testname                            string
		evictedPodName                      string
		mockClient                          *mockClient
		expectedResult                      *admissionv1.AdmissionResponse
		expectedPod                         *corev1.Pod
		expectedTrackingResourceAnnotations map[string]string
	}{
		{
			testname:       "Ignore non-existent/rescheduled pod",
			evictedPodName: "non-existent-pod",
			mockClient: &mockClient{
				pod: nil,
			},
			expectedResult: denyEviction(http.StatusNotFound, metav1.StatusReasonNotFound, PodRescheduledMsg),
		},
		{
			testname:       "Ignore non-couchbase pods",
			evictedPodName: "non-couchbase-pod",
			mockClient: &mockClient{
				pod: &corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "non-couchbase-pod",
						Namespace: "default",
						Labels: map[string]string{
							"app": "not-couchbase",
						},
					},
				},
			},
			expectedResult: allowEviction(),
		},
		{
			testname:       "Deny eviction with TooManyRequests if pod has reschedule annotation",
			evictedPodName: "rescheduled-pod",
			mockClient: &mockClient{
				pod: &corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "rescheduled-pod",
						Namespace: "default",
						Labels: map[string]string{
							"app": "couchbase",
						},
						Annotations: map[string]string{
							"cao.couchbase.com/reschedule": "true",
						},
					},
				},
			},
			expectedResult: denyEviction(http.StatusTooManyRequests, metav1.StatusReasonTooManyRequests, PodWaitingForRescheduleMsg),
		},
		{
			testname:       "Deny eviction with TooManyRequests, track reschedule and add reschedule annotation to pod",
			evictedPodName: "pod2",
			mockClient: &mockClient{
				pod: &corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "pod2",
						Namespace: "default",
						Labels: map[string]string{
							"app": "couchbase",
						},
					},
				},
				shouldTrackRescheduledPods:  true,
				shouldAddTrackingAnnotation: true,
			},
			expectedPod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "pod2",
					Namespace: "default",
					Labels: map[string]string{
						"app": "couchbase",
					},
					Annotations: map[string]string{
						"cao.couchbase.com/reschedule": "true",
					},
				},
			},
			expectedTrackingResourceAnnotations: map[string]string{
				TrackingResourceAnnotation("pod2", "default"): "true",
			},
			expectedResult: denyEviction(http.StatusTooManyRequests, metav1.StatusReasonTooManyRequests, RescheduleAnnotationAddedToPodMsg),
		},
		{
			testname:       "Allow eviction if pod is tracked but missing reschedule annotation",
			evictedPodName: "pod2",
			mockClient: &mockClient{
				pod: &corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "pod2",
						Namespace: "default",
						Labels: map[string]string{
							"app": "couchbase",
						},
					},
				},
				trackingResourceAnnotations: map[string]string{
					TrackingResourceAnnotation("pod2", "default"): "true",
				},
				shouldTrackRescheduledPods:  true,
				shouldAddTrackingAnnotation: true,
			},
			expectedTrackingResourceAnnotations: map[string]string{},
			expectedResult:                      denyEviction(http.StatusNotFound, metav1.StatusReasonNotFound, PodRescheduledWithSameNameMsg),
		},
		{
			testname:       "Deny eviction with TooManyRequests if different pod is tracked, but this pod is missing reschedule annotation",
			evictedPodName: "pod2",
			mockClient: &mockClient{
				pod: &corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "pod2",
						Namespace: "default",
						Labels: map[string]string{
							"app": "couchbase",
						},
					},
				},
				trackingResourceAnnotations: map[string]string{
					TrackingResourceAnnotation("pod1", "default"): "true",
				},
				shouldTrackRescheduledPods:  true,
				shouldAddTrackingAnnotation: true,
			},
			expectedTrackingResourceAnnotations: map[string]string{
				TrackingResourceAnnotation("pod1", "default"): "true",
				TrackingResourceAnnotation("pod2", "default"): "true",
			},
			expectedResult: denyEviction(http.StatusTooManyRequests, metav1.StatusReasonTooManyRequests, RescheduleAnnotationAddedToPodMsg),
		},
		{
			testname:       "Deny eviction with TooManyRequests, add reschedule annotation to pod",
			evictedPodName: "pod2",
			mockClient: &mockClient{
				pod: &corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "pod2",
						Namespace: "default",
						Labels: map[string]string{
							"app": "couchbase",
						},
					},
				},
			},
			expectedPod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "pod2",
					Namespace: "default",
					Labels: map[string]string{
						"app": "couchbase",
					},
					Annotations: map[string]string{
						"cao.couchbase.com/reschedule": "true",
					},
				},
			},
			expectedResult: denyEviction(http.StatusTooManyRequests, metav1.StatusReasonTooManyRequests, RescheduleAnnotationAddedToPodMsg),
		},
	}

	for _, testcase := range testcases {
		t.Run(testcase.testname, func(t *testing.T) {
			testcase.mockClient.config = NewConfigBuilder().FromEnvironment().Build()
			eviction := policyv1.Eviction{
				ObjectMeta: metav1.ObjectMeta{
					Name:      testcase.evictedPodName,
					Namespace: "default",
				},
			}

			result := handleEviction(eviction, testcase.mockClient)

			if !reflect.DeepEqual(result, testcase.expectedResult) {
				t.Errorf("Expected response to be %v, got %v", testcase.expectedResult, result)
			}

			if testcase.expectedPod != nil {
				pod, err := testcase.mockClient.GetPod(testcase.evictedPodName, "default")
				if err != nil {
					t.Errorf("Failed to get pod: %v", err)
				}

				if !reflect.DeepEqual(pod, testcase.expectedPod) {
					t.Errorf("Expected pod to be %v, got %v", testcase.expectedPod, pod)
				}
			}

			if !reflect.DeepEqual(testcase.mockClient.trackingResourceAnnotations, testcase.expectedTrackingResourceAnnotations) {
				t.Errorf("Expected tracking resource annotations to be %v, got %v", testcase.expectedTrackingResourceAnnotations, testcase.mockClient.trackingResourceAnnotations)
			}
		})
	}
}
