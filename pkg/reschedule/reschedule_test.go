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
	"k8s.io/apimachinery/pkg/runtime/schema"
)

type mockClient struct {
	pod         *corev1.Pod
	clusterInfo *ClusterInfo
}

func (m *mockClient) GetPod(name, namespace string) (*corev1.Pod, error) {
	if m.pod == nil {
		return nil, k8serrors.NewNotFound(schema.GroupResource{Group: "", Resource: "pods"}, name)
	}
	return m.pod, nil
}

func (m *mockClient) GetClusterInfo(name, namespace string) (*ClusterInfo, error) {
	return m.clusterInfo, nil
}

func (m *mockClient) ReschedulePod(pod *corev1.Pod) error {
	if pod.Annotations == nil {
		pod.Annotations = make(map[string]string)
	}

	pod.Annotations[RescheduleAnnotation] = RescheduleTrue
	m.pod = pod
	return nil
}

func (m *mockClient) PatchRescheduleHookPodsList(name, namespace string, list []string) error {
	m.clusterInfo.rescheduleHookPodsList = list
	return nil
}

func (m *mockClient) AddRescheduleHookTrackingAnnotation(podName, clusterName, clusterNamespace string) error {
	m.clusterInfo.clusterAnnotations[RescheduleHookTrackingAnnotationPrefix+podName] = RescheduleTrue
	return nil
}

func (m *mockClient) RemoveRescheduleHookTrackingAnnotation(podName, clusterName, clusterNamespace string) error {
	delete(m.clusterInfo.clusterAnnotations, RescheduleHookTrackingAnnotationPrefix+podName)
	return nil
}

func TestHandleEviction(t *testing.T) {
	testcases := []struct {
		testname            string
		evictedPodName      string
		mockClient          *mockClient
		expectedResult      *admissionv1.AdmissionResponse
		expectedPod         *corev1.Pod
		expectedClusterInfo *ClusterInfo
	}{
		{
			testname:       "Ignore non-existent/rescheduled pod",
			evictedPodName: "non-existent-pod",
			mockClient: &mockClient{
				pod: nil,
			},
			expectedResult: allowEviction(),
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
							CouchbasePodLabelKey: CouchbasePodLabelValue,
						},
						Annotations: map[string]string{
							RescheduleAnnotation: RescheduleTrue,
						},
					},
				},
			},
			expectedResult: denyEviction(http.StatusTooManyRequests, metav1.StatusReasonTooManyRequests, "Couchbase pod awaiting reschedule"),
		},
		{
			testname:       "Ignore pod if tracking annotation exists",
			evictedPodName: "pod1",
			mockClient: &mockClient{
				pod: &corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "pod1",
						Namespace: "default",
						Labels: map[string]string{
							CouchbasePodLabelKey:     CouchbasePodLabelValue,
							CouchbaseClusterLabelKey: "couchbase-cluster",
						},
					},
				},
				clusterInfo: &ClusterInfo{
					clusterAnnotations: map[string]string{
						RescheduleHookTrackingAnnotationPrefix + "pod1": RescheduleTrue,
					},
				},
			},
			expectedClusterInfo: &ClusterInfo{
				clusterAnnotations: map[string]string{},
			},
			expectedResult: allowEviction(),
		},
		{
			testname:       "Deny eviction with TooManyRequests if pod is to be rescheduled and track rescheduled pod on cluster with InPlaceUpgrade",
			evictedPodName: "pod2",
			mockClient: &mockClient{
				pod: &corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "pod2",
						Namespace: "default",
						Labels: map[string]string{
							CouchbasePodLabelKey:     CouchbasePodLabelValue,
							CouchbaseClusterLabelKey: "couchbase-cluster",
						},
					},
				},
				clusterInfo: &ClusterInfo{
					clusterAnnotations: map[string]string{
						RescheduleHookTrackingAnnotationPrefix + "pod1": RescheduleTrue,
					},
					inPlaceUpgrade: true,
				},
			},
			expectedPod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "pod2",
					Namespace: "default",
					Labels: map[string]string{
						CouchbasePodLabelKey:     CouchbasePodLabelValue,
						CouchbaseClusterLabelKey: "couchbase-cluster",
					},
					Annotations: map[string]string{
						RescheduleAnnotation: RescheduleTrue,
					},
				},
			},
			expectedClusterInfo: &ClusterInfo{
				clusterAnnotations: map[string]string{
					RescheduleHookTrackingAnnotationPrefix + "pod1": RescheduleTrue,
					RescheduleHookTrackingAnnotationPrefix + "pod2": RescheduleTrue,
				},
				inPlaceUpgrade: true,
			},
			expectedResult: denyEviction(http.StatusTooManyRequests, metav1.StatusReasonTooManyRequests, "Reschedule annotation added to pod"),
		},
		{
			testname:       "Deny eviction with TooManyRequests if pod is to be rescheduled with no tracking when InPlaceUpgrade is not enabled",
			evictedPodName: "pod2",
			mockClient: &mockClient{
				pod: &corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "pod2",
						Namespace: "default",
						Labels: map[string]string{
							CouchbasePodLabelKey:     CouchbasePodLabelValue,
							CouchbaseClusterLabelKey: "couchbase-cluster",
						},
					},
				},
				clusterInfo: &ClusterInfo{
					clusterAnnotations: map[string]string{
						RescheduleHookTrackingAnnotationPrefix + "pod1": RescheduleTrue,
					},
					inPlaceUpgrade: false,
				},
			},
			expectedPod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "pod2",
					Namespace: "default",
					Labels: map[string]string{
						CouchbasePodLabelKey:     CouchbasePodLabelValue,
						CouchbaseClusterLabelKey: "couchbase-cluster",
					},
					Annotations: map[string]string{
						RescheduleAnnotation: RescheduleTrue,
					},
				},
			},
			expectedClusterInfo: &ClusterInfo{
				clusterAnnotations: map[string]string{
					RescheduleHookTrackingAnnotationPrefix + "pod1": RescheduleTrue,
				},
			},
			expectedResult: denyEviction(http.StatusTooManyRequests, metav1.StatusReasonTooManyRequests, "Reschedule annotation added to pod"),
		},
	}

	for _, testcase := range testcases {
		t.Run(testcase.testname, func(t *testing.T) {
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

			if testcase.expectedClusterInfo != nil {
				clusterInfo, err := testcase.mockClient.GetClusterInfo("couchbase-cluster", "default")
				if err != nil {
					t.Errorf("Failed to get cluster info: %v", err)
				}

				if !reflect.DeepEqual(clusterInfo, testcase.expectedClusterInfo) {
					t.Errorf("Expected cluster info to be %v, got %v", testcase.expectedClusterInfo, clusterInfo)
				}
			}
		})
	}
}

func TestHandleEvictionList(t *testing.T) {
	testcases := []struct {
		testname       string
		evictedPodName string
		mockClient     *mockClient
		expectedResult *admissionv1.AdmissionResponse
	}{
		{
			testname:       "Ignore pod if it exists in the reschedule hook pods list",
			evictedPodName: "pod1",
			mockClient: &mockClient{
				pod: &corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "pod1",
						Namespace: "default",
						Labels: map[string]string{
							CouchbasePodLabelKey:     CouchbasePodLabelValue,
							CouchbaseClusterLabelKey: "couchbase-cluster",
						},
					},
				},
				clusterInfo: &ClusterInfo{
					rescheduleHookPodsList: []string{"pod1"},
					inPlaceUpgrade:         true,
				},
			},
			expectedResult: allowEviction(),
		},
		{
			testname:       "Deny eviction with TooManyRequests if pod is to be rescheduled",
			evictedPodName: "pod2",
			mockClient: &mockClient{
				pod: &corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "pod2",
						Namespace: "default",
						Labels: map[string]string{
							CouchbasePodLabelKey:     CouchbasePodLabelValue,
							CouchbaseClusterLabelKey: "couchbase-cluster",
						},
					},
				},
				clusterInfo: &ClusterInfo{
					rescheduleHookPodsList: []string{"pod1"},
					inPlaceUpgrade:         true,
				},
			},
			expectedResult: denyEviction(http.StatusTooManyRequests, metav1.StatusReasonTooManyRequests, "Reschedule annotation added to pod"),
		},
	}

	for _, testcase := range testcases {
		t.Run(testcase.testname, func(t *testing.T) {
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
		})
	}
}
