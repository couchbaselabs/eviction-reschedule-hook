package tracking

import (
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
)

// CouchbaseClusterTrackingResource is a TrackingResource implementation for tracking rescheduled pods using annotations on the CouchbaseCluster resource
type CouchbaseClusterTrackingResource struct {
	GroupVersionResource schema.GroupVersionResource
	InstanceName         string
}

func (t *CouchbaseClusterTrackingResource) GetResourceType() string {
	return ResourceTypeCouchbaseCluster
}

// ShouldTrack checks if the resource instance is an InPlaceUpgrade cluster
func (t *CouchbaseClusterTrackingResource) ShouldTrack(resourceInstance *unstructured.Unstructured) bool {
	upgradeStrategy, found, err := unstructured.NestedString(resourceInstance.Object, "spec", "upgradeProcess")
	if err != nil || !found {
		return false
	}

	return upgradeStrategy == "InPlaceUpgrade"
}

func (t *CouchbaseClusterTrackingResource) GetInstanceName(pod *corev1.Pod) string {
	return pod.Labels["couchbase_cluster"]
}

func (t *CouchbaseClusterTrackingResource) GetResourceInterface(client dynamic.Interface, namespace string) dynamic.ResourceInterface {
	return client.Resource(schema.GroupVersionResource{
		Group:    "couchbase.com",
		Version:  "v2",
		Resource: "couchbaseclusters",
	}).Namespace(namespace)
}
