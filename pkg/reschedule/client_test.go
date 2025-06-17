package reschedule

import (
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

func TestGetTrackingResourceInstance(t *testing.T) {
	testcases := []struct {
		testname             string
		trackingResourceType string
		resourceStub         *unstructured.Unstructured
		expected             *unstructured.Unstructured
	}{
		{
			testname:             "CouchbaseCluster",
			trackingResourceType: "couchbasecluster",
			resourceStub:         couchbaseClusterStub("test-cluster", "default-namespace", true, nil),
		},
		{
			testname:             "Namespace",
			trackingResourceType: "namespace",
			resourceStub:         namespaceStub("test-namespace", nil),
		},
	}

	for _, testcase := range testcases {
		t.Run(testcase.testname, func(t *testing.T) {
			unstructuredStub, err := runtime.DefaultUnstructuredConverter.ToUnstructured(testcase.resourceStub)
			if err != nil {
				t.Fatalf("Failed to convert resource to unstructured: %v", err)
			}

			client := &ClientImpl{
				dynamicClient: fake.NewSimpleDynamicClient(runtime.NewScheme(), &unstructured.Unstructured{Object: unstructuredStub}),
				config:        NewConfigBuilder().FromEnvironment().WithTrackingResource(testcase.trackingResourceType).Build(),
			}

			trackingResourceInstance, err := client.GetTrackingResourceInstance(testcase.resourceStub.GetName(), "default-namespace")
			if err != nil {
				t.Fatalf("Failed to get tracking resource: %v", err)
			}

			if !reflect.DeepEqual(trackingResourceInstance, testcase.resourceStub) {
				t.Fatalf("Expected tracking resource to be %v, got %v", testcase.resourceStub, trackingResourceInstance)
			}
		})
	}
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
		config:        NewConfigBuilder().FromEnvironment().Build(),
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
	if updatedPod.Annotations[client.GetConfig().rescheduleAnnotationKey] != client.GetConfig().rescheduleAnnotationValue {
		t.Fatalf("Expected pod to have reschedule annotation, got %v", updatedPod.Annotations)
	}
}

func TestAddRescheduleHookTrackingAnnotation(t *testing.T) {
	testcases := []struct {
		testname             string
		trackingResourceType string
		namespace            string
		resourceStub         *unstructured.Unstructured
	}{
		{
			testname:             "CouchbaseCluster",
			trackingResourceType: "couchbasecluster",
			namespace:            "test-namespace",
			resourceStub:         couchbaseClusterStub("test-cluster", "test-namespace", true, nil),
		},
		{
			testname:             "Namespace",
			trackingResourceType: "namespace",
			namespace:            "test-namespace",
			resourceStub:         namespaceStub("test-namespace", nil),
		},
	}

	for _, testcase := range testcases {
		t.Run(testcase.testname, func(t *testing.T) {
			unstructuredStub, err := runtime.DefaultUnstructuredConverter.ToUnstructured(testcase.resourceStub)
			if err != nil {
				t.Fatalf("Failed to convert resource to unstructured: %v", err)
			}

			client := &ClientImpl{
				dynamicClient: fake.NewSimpleDynamicClient(runtime.NewScheme(), &unstructured.Unstructured{Object: unstructuredStub}),
				config:        NewConfigBuilder().FromEnvironment().WithTrackingResource(testcase.trackingResourceType).Build(),
			}

			podName := "test-pod"

			err = client.AddRescheduleHookTrackingAnnotation(podName, testcase.namespace, testcase.resourceStub.GetName())
			if err != nil {
				t.Fatalf("Failed to add reschedule hook tracking annotation: %v", err)
			}

			updatedResource, err := client.GetTrackingResourceInstance(testcase.resourceStub.GetName(), testcase.namespace)
			if err != nil {
				t.Fatalf("Failed to get updated resource: %v", err)
			}

			if updatedResource.GetAnnotations()[TrackingResourceAnnotation(podName, testcase.namespace)] != "true" {
				t.Fatalf("Expected resource to have reschedule hook tracking annotation, got %v", updatedResource.GetAnnotations())
			}
		})
	}
}

func TestRemoveRescheduleHookTrackingAnnotation(t *testing.T) {
	testcases := []struct {
		testname             string
		trackingResourceType string
		resourceStub         *unstructured.Unstructured
	}{
		{
			testname:             "CouchbaseCluster",
			trackingResourceType: "couchbasecluster",
			resourceStub: couchbaseClusterStub("test-cluster", "default-namespace", true, map[string]interface{}{
				TrackingResourceAnnotation("test-pod", "default-namespace"): "true",
			}),
		},
		{
			testname:             "Namespace",
			trackingResourceType: "namespace",
			resourceStub: namespaceStub("test-namespace", map[string]interface{}{
				TrackingResourceAnnotation("test-pod", "default-namespace"): "true",
			}),
		},
		{
			testname:             "No annotations exist on cluster",
			trackingResourceType: "couchbasecluster",
			resourceStub:         couchbaseClusterStub("test-cluster", "default-namespace", true, nil),
		},
	}

	for _, testcase := range testcases {
		t.Run(testcase.testname, func(t *testing.T) {
			unstructuredStub, err := runtime.DefaultUnstructuredConverter.ToUnstructured(testcase.resourceStub)
			if err != nil {
				t.Fatalf("Failed to convert tracking resource to unstructured: %v", err)
			}

			client := &ClientImpl{
				dynamicClient: fake.NewSimpleDynamicClient(runtime.NewScheme(), &unstructured.Unstructured{Object: unstructuredStub}),
				config:        NewConfigBuilder().FromEnvironment().WithTrackingResource(testcase.trackingResourceType).Build(),
			}

			podName := "test-pod"
			podNamespace := "default-namespace"
			err = client.RemoveRescheduleHookTrackingAnnotation(podName, podNamespace, testcase.resourceStub.GetName())
			if err != nil {
				t.Fatalf("Failed to remove reschedule hook tracking annotation: %v", err)
			}

			updatedResource, err := client.GetTrackingResourceInstance(testcase.resourceStub.GetName(), "default-namespace")
			if err != nil {
				t.Fatalf("Failed to get updated tracking resource: %v", err)
			}

			if updatedResource.GetAnnotations()[TrackingResourceAnnotation(podName, podNamespace)] != "" {
				t.Fatalf("Expected tracking resource to not have reschedule hook tracking annotation, got %v", updatedResource.GetAnnotations())
			}
		})
	}
}

func couchbaseClusterStub(clusterName, namespace string, inPlaceUpgrade bool, annotations map[string]interface{}) *unstructured.Unstructured {
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
				"upgradeProcess": upgradeProcess,
			},
		},
	}

	obj.SetKind("CouchbaseCluster")
	obj.SetAPIVersion("couchbase.com/v2")
	return obj
}

func namespaceStub(name string, annotations map[string]interface{}) *unstructured.Unstructured {
	metadata := map[string]interface{}{
		"name": name,
	}

	if annotations != nil {
		metadata["annotations"] = annotations
	}

	obj := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"metadata": metadata,
		},
	}

	obj.SetKind("Namespace")
	obj.SetAPIVersion("v1")
	return obj
}
