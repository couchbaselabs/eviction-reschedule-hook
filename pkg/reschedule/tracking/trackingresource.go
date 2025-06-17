package tracking

import (
	"log/slog"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/dynamic"
)

// TrackingResource is an in\rface that defines the methods for tracking rescheduled pods on another resource. To add a new tracking resource, implement this interface and register it
// in the init function. The tracking resource is determined by the TRACKING_RESOURCE_TYPE environment variable
type TrackingResource interface {
	// GetResourceType returns the type of the tracking resource. This is used to determine the type of the tracking resource to create.
	GetResourceType() string
	// GetInstanceName returns the name of the instance of the tracking resource that the pod belongs to. During eviction
	// requests, we only have access to the pod
	GetInstanceName(pod *corev1.Pod) string
	// ShouldTrack can be used to check a conditional on the tracking resource. For example, we only want to track rescheduled pods on
	// CouchbaseClusters that have InPlaceUpgrade enabled as this determines whether pods will be recreated with the same name
	ShouldTrack(resourceInstance *unstructured.Unstructured) bool
	// GetResourceInterface returns the resource interface for the tracking resource. This is used to get the tracking resource using
	// the dynamic client. It is needed as some tracking resources may not be namespaces.
	GetResourceInterface(client dynamic.Interface, namespace string) dynamic.ResourceInterface
}

// ResourceType constants for tracking resources
const (
	ResourceTypeNamespace        = "namespace"
	ResourceTypeCouchbaseCluster = "couchbasecluster"
)

// trackingResourceRegistry holds all registered tracking resource types
var trackingResourceRegistry = map[string]TrackingResource{
	ResourceTypeNamespace:        &NamespaceTrackingResource{},
	ResourceTypeCouchbaseCluster: &CouchbaseClusterTrackingResource{},
}

// Init registers each of the possible tracking resources
func init() {
	trackingResourceRegistry[ResourceTypeNamespace] = &NamespaceTrackingResource{}
	trackingResourceRegistry[ResourceTypeCouchbaseCluster] = &CouchbaseClusterTrackingResource{}
}

// GetTrackingResource returns the TrackingResource implementation for the given resource type. If the resource type is not found, it will return the default
// tracking resource
func GetTrackingResource(resourceType string) TrackingResource {
	if resource, exists := trackingResourceRegistry[resourceType]; exists {
		return resource
	}

	slog.Warn("Unknown tracking resource type, defaulting to couchbasecluster", "type", resourceType)
	return trackingResourceRegistry[ResourceTypeCouchbaseCluster]
}
