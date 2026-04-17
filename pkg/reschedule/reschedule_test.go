package reschedule

import (
	"fmt"
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
	node                        *corev1.Node
	nodeErr                     error
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

func (m *mockClient) GetNode(name string) (*corev1.Node, error) {
	if m.nodeErr != nil {
		return nil, m.nodeErr
	}
	if m.node == nil {
		return nil, k8serrors.NewNotFound(schema.GroupResource{Group: "", Resource: "nodes"}, name)
	}
	return m.node, nil
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
			expectedResult: denyEviction(http.StatusNotFound, metav1.StatusReasonNotFound, PodNoLongerExistsMsg),
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
		{
			testname:       "Allow eviction when node has matching failure condition (Status=True)",
			evictedPodName: "pod-on-failing-node",
			mockClient: &mockClient{
				pod: &corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "pod-on-failing-node",
						Namespace: "default",
						Labels: map[string]string{
							"app": "couchbase",
						},
					},
					Spec: corev1.PodSpec{
						NodeName: "node1",
					},
				},
				node: &corev1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name: "node1",
					},
					Status: corev1.NodeStatus{
						Conditions: []corev1.NodeCondition{
							{
								Type:   corev1.NodeDiskPressure,
								Status: corev1.ConditionTrue,
							},
						},
					},
				},
				config: NewConfigBuilder().
					WithEnableNodeConditionCheck(true).
					WithNodeFailureConditionTypes([]string{"DiskPressure"}).
					Build(),
			},
			expectedResult: allowEviction(),
		},
		{
			testname:       "Deny eviction when node condition is present but Status=False",
			evictedPodName: "pod-on-healthy-node",
			mockClient: &mockClient{
				pod: &corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "pod-on-healthy-node",
						Namespace: "default",
						Labels: map[string]string{
							"app": "couchbase",
						},
					},
					Spec: corev1.PodSpec{
						NodeName: "node1",
					},
				},
				node: &corev1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name: "node1",
					},
					Status: corev1.NodeStatus{
						Conditions: []corev1.NodeCondition{
							{
								Type:   corev1.NodeDiskPressure,
								Status: corev1.ConditionFalse,
							},
						},
					},
				},
				config: NewConfigBuilder().
					WithEnableNodeConditionCheck(true).
					WithNodeFailureConditionTypes([]string{"DiskPressure"}).
					Build(),
			},
			expectedResult: denyEviction(http.StatusTooManyRequests, metav1.StatusReasonTooManyRequests, RescheduleAnnotationAddedToPodMsg),
		},
		{
			testname:       "Fail closed (deny) when GetNode errors",
			evictedPodName: "pod-node-error",
			mockClient: &mockClient{
				pod: &corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "pod-node-error",
						Namespace: "default",
						Labels: map[string]string{
							"app": "couchbase",
						},
					},
					Spec: corev1.PodSpec{
						NodeName: "node1",
					},
				},
				nodeErr: fmt.Errorf("connection refused"),
				config: NewConfigBuilder().
					WithEnableNodeConditionCheck(true).
					WithNodeFailureConditionTypes([]string{"DiskPressure"}).
					Build(),
			},
			expectedResult: denyEviction(http.StatusTooManyRequests, metav1.StatusReasonTooManyRequests, RescheduleAnnotationAddedToPodMsg),
		},
		{
			testname:       "Skip node check when feature disabled via config",
			evictedPodName: "pod-check-disabled",
			mockClient: &mockClient{
				pod: &corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "pod-check-disabled",
						Namespace: "default",
						Labels: map[string]string{
							"app": "couchbase",
						},
					},
					Spec: corev1.PodSpec{
						NodeName: "node1",
					},
				},
				node: &corev1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name: "node1",
					},
					Status: corev1.NodeStatus{
						Conditions: []corev1.NodeCondition{
							{
								Type:   corev1.NodeDiskPressure,
								Status: corev1.ConditionTrue,
							},
						},
					},
				},
				config: NewConfigBuilder().
					WithEnableNodeConditionCheck(false).
					WithNodeFailureConditionTypes([]string{"DiskPressure"}).
					Build(),
			},
			expectedResult: denyEviction(http.StatusTooManyRequests, metav1.StatusReasonTooManyRequests, RescheduleAnnotationAddedToPodMsg),
		},
		{
			testname:       "Allow eviction when node has matching taint key",
			evictedPodName: "pod-tainted-node",
			mockClient: &mockClient{
				pod: &corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "pod-tainted-node",
						Namespace: "default",
						Labels: map[string]string{
							"app": "couchbase",
						},
					},
					Spec: corev1.PodSpec{
						NodeName: "node1",
					},
				},
				node: &corev1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name: "node1",
					},
					Spec: corev1.NodeSpec{
						Taints: []corev1.Taint{
							{
								Key:    "node.example.com/pending-eviction",
								Effect: corev1.TaintEffectNoSchedule,
							},
						},
					},
				},
				config: NewConfigBuilder().
					WithEnableNodeConditionCheck(true).
					WithNodeFailureTaintKeys([]string{"node.example.com/pending-eviction"}).
					Build(),
			},
			expectedResult: allowEviction(),
		},
		{
			testname:       "Allow eviction with OR logic - condition matches, no taint",
			evictedPodName: "pod-or-logic",
			mockClient: &mockClient{
				pod: &corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "pod-or-logic",
						Namespace: "default",
						Labels: map[string]string{
							"app": "couchbase",
						},
					},
					Spec: corev1.PodSpec{
						NodeName: "node1",
					},
				},
				node: &corev1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name: "node1",
					},
					Status: corev1.NodeStatus{
						Conditions: []corev1.NodeCondition{
							{
								Type:   corev1.NodeDiskPressure,
								Status: corev1.ConditionTrue,
							},
						},
					},
				},
				config: NewConfigBuilder().
					WithEnableNodeConditionCheck(true).
					WithNodeFailureConditionTypes([]string{"DiskPressure"}).
					WithNodeFailureTaintKeys([]string{"some-taint-key"}).
					Build(),
			},
			expectedResult: allowEviction(),
		},
	}

	for _, testcase := range testcases {
		t.Run(testcase.testname, func(t *testing.T) {
			if testcase.mockClient.config == nil {
				testcase.mockClient.config = NewConfigBuilder().FromEnvironment().Build()
			}
			eviction := policyv1.Eviction{
				ObjectMeta: metav1.ObjectMeta{
					Name:      testcase.evictedPodName,
					Namespace: "default",
				},
			}

			result := handleEviction(eviction, testcase.mockClient, CreateLogger(eviction.Name, eviction.Namespace, false))

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
