package reschedule

import (
	"context"
	"reflect"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/dynamic/fake"
)

func TestGetPod(t *testing.T) {
	stub := &corev1.Pod{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Pod",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pod",
			Namespace: "default-namespace",
		},
	}

	unstructuredStub, err := runtime.DefaultUnstructuredConverter.ToUnstructured(stub)
	if err != nil {
		t.Fatalf("Failed to convert pod to unstructured: %v", err)
	}

	client := &ClientImpl{
		dynamicClient: fake.NewSimpleDynamicClient(runtime.NewScheme(), &unstructured.Unstructured{Object: unstructuredStub}),
	}

	pod, err := client.GetPod("test-pod", "default-namespace")
	if err != nil {
		t.Fatalf("Failed to get pod: %v", err)
	}

	if !reflect.DeepEqual(pod, stub) {
		t.Fatalf("Expected pods to be %v, got %v", stub, pod)
	}
}

func TestGetClusterInfo(t *testing.T) {
	testcases := []struct {
		testname string
		stub     *unstructured.Unstructured
		expected *ClusterInfo
	}{
		{
			testname: "With InPlaceUpgrade",
			stub:     clusterStub("test-cluster", "default-namespace", true, nil),
			expected: &ClusterInfo{
				inPlaceUpgrade: true,
			},
		},
		{
			testname: "With SwapRebalance",
			stub:     clusterStub("test-cluster", "default-namespace", false, nil),
			expected: &ClusterInfo{
				inPlaceUpgrade: false,
			},
		},
		{
			testname: "With annotations and SwapRebalance",
			stub: clusterStub("test-cluster", "default-namespace", false, map[string]interface{}{
				RescheduleHookTrackingAnnotationPrefix + "pod1": RescheduleTrue,
				RescheduleHookTrackingAnnotationPrefix + "pod2": RescheduleTrue,
			}),
			expected: &ClusterInfo{
				inPlaceUpgrade: false,
				clusterAnnotations: map[string]string{
					RescheduleHookTrackingAnnotationPrefix + "pod1": RescheduleTrue,
					RescheduleHookTrackingAnnotationPrefix + "pod2": RescheduleTrue,
				},
			},
		},
	}

	for _, testcase := range testcases {
		t.Run(testcase.testname, func(t *testing.T) {
			unstructuredStub, err := runtime.DefaultUnstructuredConverter.ToUnstructured(testcase.stub)
			if err != nil {
				t.Fatalf("Failed to convert cluster to unstructured: %v", err)
			}

			client := &ClientImpl{
				dynamicClient: fake.NewSimpleDynamicClient(runtime.NewScheme(), &unstructured.Unstructured{Object: unstructuredStub}),
			}

			clusterInfo, err := client.GetClusterInfo("test-cluster", "default-namespace")
			if err != nil {
				t.Fatalf("Failed to get cluster info: %v", err)
			}

			if !reflect.DeepEqual(clusterInfo, testcase.expected) {
				t.Fatalf("Expected cluster info to be %v, got %v", testcase.expected, clusterInfo)
			}
		})
	}
}

func TestGetClusterInfoList(t *testing.T) {
	testcases := []struct {
		testname string
		stub     *unstructured.Unstructured
		expected *ClusterInfo
	}{
		{
			testname: "With pod reschedule list",
			stub: clusterStub("test-cluster", "default-namespace", true, map[string]interface{}{
				RescheduleHookPodsListAnnotation: `["pod1", "pod2"]`,
			}),
			expected: &ClusterInfo{
				rescheduleHookPodsList: []string{"pod1", "pod2"},
				inPlaceUpgrade:         true,
			},
		},
		{
			testname: "With empty pod reschedule list",
			stub: clusterStub("test-cluster", "default-namespace", true, map[string]interface{}{
				RescheduleHookPodsListAnnotation: "",
				"someOtherAnnotation/annotation": "some-value",
			}),
			expected: &ClusterInfo{
				rescheduleHookPodsList: []string{},
				inPlaceUpgrade:         false,
			},
		},
		{
			testname: "With a different annotation and SwapRebalance",
			stub: clusterStub("test-cluster", "default-namespace", false, map[string]interface{}{
				"someOtherAnnotation/annotation": "some-value",
			}),
			expected: &ClusterInfo{
				rescheduleHookPodsList: []string{},
				inPlaceUpgrade:         false,
			},
		},
		{
			testname: "Without annotation and SwapRebalance",
			stub:     clusterStub("test-cluster", "default-namespace", false, nil),
			expected: &ClusterInfo{
				rescheduleHookPodsList: []string{},
				inPlaceUpgrade:         false,
			},
		},
	}

	for _, testcase := range testcases {
		t.Run(testcase.testname, func(t *testing.T) {
			unstructuredStub, err := runtime.DefaultUnstructuredConverter.ToUnstructured(testcase.stub)
			if err != nil {
				t.Fatalf("Failed to convert cluster to unstructured: %v", err)
			}

			client := &ClientImpl{
				dynamicClient: fake.NewSimpleDynamicClient(runtime.NewScheme(), &unstructured.Unstructured{Object: unstructuredStub}),
			}

			clusterInfo, err := client.GetClusterInfo("test-cluster", "default-namespace")
			if err != nil {
				t.Fatalf("Failed to get cluster info: %v", err)
			}

			if !reflect.DeepEqual(clusterInfo, testcase.expected) {
				t.Fatalf("Expected cluster info to be %v, got %v", testcase.expected, clusterInfo)
			}
		})
	}
}

func clusterStub(clusterName, namespace string, inPlaceUpgrade bool, annotations map[string]interface{}) *unstructured.Unstructured {
	metadata := map[string]interface{}{
		"name":      clusterName,
		"namespace": namespace,
	}

	if annotations != nil {
		metadata["annotations"] = annotations
	}

	upgradeProcess := "SwapRebalance"
	if inPlaceUpgrade {
		upgradeProcess = "InPlaceUpgrade"
	}

	obj := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"metadata": metadata,
			"spec": map[string]interface{}{
				InPlaceUpgradeStrategyKey: upgradeProcess,
			},
		},
	}

	obj.SetKind("CouchbaseCluster")
	obj.SetAPIVersion("couchbase.com/v2")
	return obj
}

func TestReschedulePod(t *testing.T) {
	stub := &corev1.Pod{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Pod",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pod",
			Namespace: "default-namespace",
		},
	}

	unstructuredStub, err := runtime.DefaultUnstructuredConverter.ToUnstructured(stub)
	if err != nil {
		t.Fatalf("Failed to convert pod to unstructured: %v", err)
	}

	client := &ClientImpl{
		dynamicClient: fake.NewSimpleDynamicClient(runtime.NewScheme(), &unstructured.Unstructured{Object: unstructuredStub}),
	}

	err = client.ReschedulePod(stub)
	if err != nil {
		t.Fatalf("Failed to reschedule pod: %v", err)
	}

	updatedPod, err := client.GetPod("test-pod", "default-namespace")
	if err != nil {
		t.Fatalf("Failed to get pod: %v", err)
	}

	// Check that the updated pod has the reschedule annotation
	if updatedPod.Annotations[RescheduleAnnotation] != RescheduleTrue {
		t.Fatalf("Expected pod to have reschedule annotation, got %v", updatedPod.Annotations)
	}
}

func TestAddRescheduleHookTrackingAnnotation(t *testing.T) {
	stub := clusterStub("test-cluster", "default-namespace", true, nil)

	unstructuredStub, err := runtime.DefaultUnstructuredConverter.ToUnstructured(stub)
	if err != nil {
		t.Fatalf("Failed to convert pod to unstructured: %v", err)
	}

	client := &ClientImpl{
		dynamicClient: fake.NewSimpleDynamicClient(runtime.NewScheme(), &unstructured.Unstructured{Object: unstructuredStub}),
	}

	err = client.AddRescheduleHookTrackingAnnotation("test-pod", "test-cluster", "default-namespace")
	if err != nil {
		t.Fatalf("Failed to add reschedule hook tracking annotation: %v", err)
	}

	updatedCluster, err := client.dynamicClient.Resource(CouchbaseClusterResource).Namespace("default-namespace").Get(context.TODO(), "test-cluster", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("Failed to get updated cluster: %v", err)
	}

	if updatedCluster.GetAnnotations()[RescheduleHookTrackingAnnotationPrefix+"test-pod"] != RescheduleTrue {
		t.Fatalf("Expected cluster to have reschedule hook tracking annotation, got %v", updatedCluster.GetAnnotations())
	}
}

func TestRemoveRescheduleHookTrackingAnnotation(t *testing.T) {
	testcases := []struct {
		testname string
		stub     *unstructured.Unstructured
	}{
		{
			testname: "Tracking annotation exists for pod",
			stub: clusterStub("test-cluster", "default-namespace", true, map[string]interface{}{
				RescheduleHookTrackingAnnotationPrefix + "test-pod": RescheduleTrue,
			}),
		},
		{
			testname: "Tracking annotation does not exist for pod",
			stub: clusterStub("test-cluster", "default-namespace", true, map[string]interface{}{
				"someOtherAnnotation/annotation": "some-value",
			}),
		},
		{
			testname: "No annotations exist on cluster",
			stub:     clusterStub("test-cluster", "default-namespace", true, nil),
		},
	}

	for _, testcase := range testcases {
		t.Run(testcase.testname, func(t *testing.T) {
			unstructuredStub, err := runtime.DefaultUnstructuredConverter.ToUnstructured(testcase.stub)
			if err != nil {
				t.Fatalf("Failed to convert cluster to unstructured: %v", err)
			}

			client := &ClientImpl{
				dynamicClient: fake.NewSimpleDynamicClient(runtime.NewScheme(), &unstructured.Unstructured{Object: unstructuredStub}),
			}

			err = client.RemoveRescheduleHookTrackingAnnotation("test-pod", "test-cluster", "default-namespace")
			if err != nil {
				t.Fatalf("Failed to remove reschedule hook tracking annotation: %v", err)
			}

			updatedCluster, err := client.dynamicClient.Resource(CouchbaseClusterResource).Namespace("default-namespace").Get(context.TODO(), "test-cluster", metav1.GetOptions{})
			if err != nil {
				t.Fatalf("Failed to get updated cluster: %v", err)
			}

			if updatedCluster.GetAnnotations()[RescheduleHookTrackingAnnotationPrefix+"test-pod"] != "" {
				t.Fatalf("Expected cluster to not have reschedule hook tracking annotation, got %v", updatedCluster.GetAnnotations())
			}
		})
	}
}

func TestPatchRescheduleHookPodsList(t *testing.T) {
	testcases := []struct {
		testname               string
		clusterStub            *unstructured.Unstructured
		rescheduleHookPodsList []string
		expectedClusterStub    *unstructured.Unstructured
	}{
		{
			testname: "Add pod to existing reschedule hook pods list",
			clusterStub: clusterStub("test-cluster", "default-namespace", true, map[string]interface{}{
				RescheduleHookPodsListAnnotation: `["pod1","pod2"]`,
			}),
			rescheduleHookPodsList: []string{"pod1", "pod2", "pod3"},
			expectedClusterStub: clusterStub("test-cluster", "default-namespace", true, map[string]interface{}{
				RescheduleHookPodsListAnnotation: `["pod1","pod2","pod3"]`,
			}),
		},
		{
			testname: "Remove pod from existing reschedule hook pods list",
			clusterStub: clusterStub("test-cluster", "default-namespace", true, map[string]interface{}{
				RescheduleHookPodsListAnnotation: `["pod1","pod2"]`,
			}),
			rescheduleHookPodsList: []string{"pod2"},
			expectedClusterStub: clusterStub("test-cluster", "default-namespace", true, map[string]interface{}{
				RescheduleHookPodsListAnnotation: `["pod2"]`,
			}),
		},
		{
			testname:               "Add reschedule hook pods list",
			clusterStub:            clusterStub("test-cluster", "default-namespace", false, map[string]interface{}{}),
			rescheduleHookPodsList: []string{"pod1"},
			expectedClusterStub: clusterStub("test-cluster", "default-namespace", false, map[string]interface{}{
				RescheduleHookPodsListAnnotation: `["pod1"]`,
			}),
		},
		{
			testname: "Empty list will remove pod list annotation",
			clusterStub: clusterStub("test-cluster", "default-namespace", true, map[string]interface{}{
				RescheduleHookPodsListAnnotation: `["pod1"]`,
				"someOtherAnnotation/annotation": "some-value",
			}),
			rescheduleHookPodsList: []string{},
			expectedClusterStub: clusterStub("test-cluster", "default-namespace", true, map[string]interface{}{
				"someOtherAnnotation/annotation": "some-value",
			}),
		},
	}
	for _, testcase := range testcases {
		t.Run(testcase.testname, func(t *testing.T) {
			client := &ClientImpl{
				dynamicClient: fake.NewSimpleDynamicClient(runtime.NewScheme(), testcase.clusterStub),
			}
			err := client.PatchRescheduleHookPodsList("test-cluster", "default-namespace", testcase.rescheduleHookPodsList)
			if err != nil {
				t.Fatalf("Failed to patch reschedule hook pods list: %v", err)
			}

			updatedCluster, err := client.dynamicClient.Resource(CouchbaseClusterResource).Namespace("default-namespace").Get(context.TODO(), "test-cluster", metav1.GetOptions{})
			if err != nil {
				t.Fatalf("Failed to get updated cluster: %v", err)
			}

			if !reflect.DeepEqual(updatedCluster.Object, testcase.expectedClusterStub.Object) {
				t.Fatalf("Expected cluster to be %v, got %v", testcase.expectedClusterStub, updatedCluster)
			}
		})
	}
}
