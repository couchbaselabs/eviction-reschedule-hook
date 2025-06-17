package tracking

import (
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
)

// NamespaceTrackingResource is a TrackingResource implementation for tracking rescheduled pods using namespace annotations
type NamespaceTrackingResource struct {
	GroupVersionResource schema.GroupVersionResource
	InstanceName         string
}

func (t *NamespaceTrackingResource) GetResourceType() string {
	return ResourceTypeNamespace
}

func (t *NamespaceTrackingResource) GetInstanceName(pod *corev1.Pod) string {
	return pod.Namespace
}

func (t *NamespaceTrackingResource) ShouldTrack(resourceInstance *unstructured.Unstructured) bool {
	return true
}

func (t *NamespaceTrackingResource) GetResourceInterface(client dynamic.Interface, namespace string) dynamic.ResourceInterface {
	return client.Resource(schema.GroupVersionResource{
		Version:  "v1",
		Resource: "namespaces",
	})
}
