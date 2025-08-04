package e2e

import (
	"net/http"
	"testing"

	"github.com/couchbaselabs/eviction-reschedule-hook/pkg/reschedule"
	"github.com/couchbaselabs/eviction-reschedule-hook/pkg/reschedule/tracking"
	"github.com/couchbaselabs/eviction-reschedule-hook/test/framework"
	corev1 "k8s.io/api/core/v1"
)

func TestEvictMultipleCouchbasePodsAddsAnnotationNoTracking(t *testing.T) {
	cluster := framework.SetupTestCluster(t, nil)

	cleanup := cluster.MustCreateCouchbaseCluster(t, "couchbase-cluster", false)
	defer cleanup()

	cbPod1 := cluster.MustCreateCouchbasePod(t, "couchbase-1", "couchbase-cluster")
	cbPod2 := cluster.MustCreateCouchbasePod(t, "couchbase-2", "couchbase-cluster")
	busyboxPod := cluster.MustCreatePod(t, "busybox", nil)

	responses := cluster.EvictPods(t, []corev1.Pod{*cbPod1, *cbPod2, *busyboxPod})

	// Validate that the eviction is denied with TooManyRequests for the couchbase pods, and allowed for the busybox pod
	framework.ValidateEvictionDenied(t, responses, http.StatusTooManyRequests, reschedule.RescheduleAnnotationAddedToPodMsg, cbPod1.Name)
	framework.ValidateEvictionDenied(t, responses, http.StatusTooManyRequests, reschedule.RescheduleAnnotationAddedToPodMsg, cbPod2.Name)
	framework.ValidateEvictionAllowed(t, responses, busyboxPod.Name)

	// Validate the couchbase pods have the reschedule annotation, and the busybox pod has been evicted
	cluster.ValidatePodHasAnnotation(t, cbPod1.Name, reschedule.DefaultRescheduleAnnotationKey, reschedule.DefaultRescheduleAnnotationValue)
	cluster.ValidatePodHasAnnotation(t, cbPod2.Name, reschedule.DefaultRescheduleAnnotationKey, reschedule.DefaultRescheduleAnnotationValue)
	cluster.ValidatePodHasBeenEvicted(t, busyboxPod.Name)

	// Validate the couchbase cluster does not have the tracking annotation
	cluster.ValidateCouchbaseClusterDoesNotHaveAnnotations(t, "couchbase-cluster", map[string]string{
		reschedule.TrackingResourceAnnotation(cbPod1.Name, cbPod1.Namespace): "true",
		reschedule.TrackingResourceAnnotation(cbPod2.Name, cbPod2.Namespace): "true",
	})
}

func TestEvictMultipleCouchbasePodsAddsAnnotationWithTracking(t *testing.T) {
	cluster := framework.SetupTestCluster(t, nil)

	cleanup := cluster.MustCreateCouchbaseCluster(t, "couchbase-cluster", true)
	defer cleanup()

	cbPod1 := cluster.MustCreateCouchbasePod(t, "couchbase-1", "couchbase-cluster")
	cbPod2 := cluster.MustCreateCouchbasePod(t, "couchbase-2", "couchbase-cluster")
	busyboxPod := cluster.MustCreatePod(t, "busybox", nil)

	responses := cluster.EvictPods(t, []corev1.Pod{*cbPod1, *cbPod2, *busyboxPod})

	framework.ValidateEvictionDenied(t, responses, http.StatusTooManyRequests, reschedule.RescheduleAnnotationAddedToPodMsg, cbPod1.Name)
	framework.ValidateEvictionDenied(t, responses, http.StatusTooManyRequests, reschedule.RescheduleAnnotationAddedToPodMsg, cbPod2.Name)
	framework.ValidateEvictionAllowed(t, responses, busyboxPod.Name)

	// Make sure the couchbase pods have the reschedule annotation, and the busybox pod has been evicted
	cluster.ValidatePodHasAnnotation(t, cbPod1.Name, reschedule.DefaultRescheduleAnnotationKey, reschedule.DefaultRescheduleAnnotationValue)
	cluster.ValidatePodHasAnnotation(t, cbPod2.Name, reschedule.DefaultRescheduleAnnotationKey, reschedule.DefaultRescheduleAnnotationValue)
	cluster.ValidatePodHasBeenEvicted(t, busyboxPod.Name)

	// Validate the couchbase cluster has the tracking annotation
	cluster.ValidateCouchbaseClusterHasAnnotations(t, "couchbase-cluster", map[string]string{
		reschedule.TrackingResourceAnnotation(cbPod1.Name, cbPod1.Namespace): "true",
		reschedule.TrackingResourceAnnotation(cbPod2.Name, cbPod2.Namespace): "true",
	})

	// By deleting and recreating the pods with the same name, like the operator would do when InPlaceUpgrade is enabled, we expect subsequent evictions (which the drain command will trigger) to be
	// rejected with a 404 and for the tracking annotations to be removed from the couchbase cluster
	cluster.MustDeletePod(t, cbPod1.Name, cbPod1.Namespace)
	cluster.MustDeletePod(t, cbPod2.Name, cbPod2.Namespace)

	cbPod1 = cluster.MustCreateCouchbasePod(t, "couchbase-1", "couchbase-cluster")
	cbPod2 = cluster.MustCreateCouchbasePod(t, "couchbase-2", "couchbase-cluster")

	responses = cluster.EvictPods(t, []corev1.Pod{*cbPod1, *cbPod2})

	// Check that the eviction is denied with a 404
	framework.ValidateEvictionDenied(t, responses, http.StatusNotFound, reschedule.PodRescheduledWithSameNameMsg, cbPod1.Name)
	framework.ValidateEvictionDenied(t, responses, http.StatusNotFound, reschedule.PodRescheduledWithSameNameMsg, cbPod2.Name)

	// Validate the tracking annotations have been removed from the couchbase cluster
	cluster.ValidateCouchbaseClusterDoesNotHaveAnnotations(t, "couchbase-cluster", map[string]string{
		reschedule.TrackingResourceAnnotation(cbPod1.Name, cbPod1.Namespace): "true",
		reschedule.TrackingResourceAnnotation(cbPod2.Name, cbPod2.Namespace): "true",
	})
}

func TestEvictCouchbasePodUsingNamespaceTrackingResource(t *testing.T) {
	config := reschedule.NewConfigBuilder().WithTrackingResource(tracking.ResourceTypeNamespace).Build()
	cluster := framework.SetupTestCluster(t, config)
	cluster.AddClusterRolePermissions(t, "", "namespaces")

	cbPod1 := cluster.MustCreateCouchbasePod(t, "couchbase-1", "couchbase-cluster")
	busyboxPod := cluster.MustCreatePod(t, "busybox", nil)

	responses := cluster.EvictPods(t, []corev1.Pod{*cbPod1, *busyboxPod})

	framework.ValidateEvictionDenied(t, responses, http.StatusTooManyRequests, reschedule.RescheduleAnnotationAddedToPodMsg, cbPod1.Name)
	framework.ValidateEvictionAllowed(t, responses, busyboxPod.Name)

	cluster.ValidatePodHasAnnotation(t, cbPod1.Name, reschedule.DefaultRescheduleAnnotationKey, reschedule.DefaultRescheduleAnnotationValue)
	cluster.ValidatePodHasBeenEvicted(t, busyboxPod.Name)

	cluster.ValidateNamespaceHasAnnotations(t, cluster.GetNamespace(), map[string]string{
		reschedule.TrackingResourceAnnotation(cbPod1.Name, cbPod1.Namespace): "true",
	})

	cluster.MustDeletePod(t, cbPod1.Name, cbPod1.Namespace)

	cbPod1 = cluster.MustCreateCouchbasePod(t, "couchbase-1", "couchbase-cluster")

	responses = cluster.EvictPods(t, []corev1.Pod{*cbPod1, *busyboxPod})

	framework.ValidateEvictionDenied(t, responses, http.StatusNotFound, reschedule.PodRescheduledWithSameNameMsg, cbPod1.Name)

	cluster.ValidateNamespaceDoesNotHaveAnnotations(t, cluster.GetNamespace(), map[string]string{
		reschedule.TrackingResourceAnnotation(cbPod1.Name, cbPod1.Namespace): "true",
	})
}

func TestEvictPodWithDifferentConfigValuesUsingNamespaceTrackingResource(t *testing.T) {
	config := reschedule.NewConfigBuilder().
		WithTrackingResource(tracking.ResourceTypeNamespace).
		WithPodLabelSelector("appLabel", "another_application").
		WithRescheduleAnnotation("rescheduleMe", "yes").
		Build()
	cluster := framework.SetupTestCluster(t, config)
	cluster.AddClusterRolePermissions(t, "", "namespaces")

	cbPod := cluster.MustCreateCouchbasePod(t, "couchbase-1", "couchbase-cluster")
	otherPod := cluster.MustCreatePod(t, "other-pod", map[string]string{"appLabel": "another_application"})

	responses := cluster.EvictPods(t, []corev1.Pod{*cbPod, *otherPod})

	framework.ValidateEvictionDenied(t, responses, http.StatusTooManyRequests, reschedule.RescheduleAnnotationAddedToPodMsg, otherPod.Name)
	framework.ValidateEvictionAllowed(t, responses, cbPod.Name)

	cluster.ValidatePodHasAnnotation(t, otherPod.Name, "rescheduleMe", "yes")
	cluster.ValidatePodHasBeenEvicted(t, cbPod.Name)

	cluster.ValidateNamespaceHasAnnotations(t, cluster.GetNamespace(), map[string]string{
		reschedule.TrackingResourceAnnotation(otherPod.Name, otherPod.Namespace): "true",
	})

	cluster.MustDeletePod(t, otherPod.Name, otherPod.Namespace)

	otherPod = cluster.MustCreatePod(t, "other-pod", map[string]string{"appLabel": "another_application"})

	responses = cluster.EvictPods(t, []corev1.Pod{*cbPod, *otherPod})

	framework.ValidateEvictionDenied(t, responses, http.StatusNotFound, reschedule.PodRescheduledWithSameNameMsg, otherPod.Name)

	cluster.ValidateNamespaceDoesNotHaveAnnotations(t, cluster.GetNamespace(), map[string]string{
		reschedule.TrackingResourceAnnotation(cbPod.Name, cbPod.Namespace): "true",
	})
}

func TestEvicPodsWithDryRunDoesNotMutateResources(t *testing.T) {
	cluster := framework.SetupTestCluster(t, nil)

	cleanup := cluster.MustCreateCouchbaseCluster(t, "couchbase-cluster", false)
	defer cleanup()

	cbPod1 := cluster.MustCreateCouchbasePod(t, "couchbase-1", "couchbase-cluster")
	cbPod2 := cluster.MustCreateCouchbasePod(t, "couchbase-2", "couchbase-cluster")
	busyboxPod := cluster.MustCreatePod(t, "busybox", nil)

	responses := cluster.EvictPodsDryRun(t, []corev1.Pod{*cbPod1, *cbPod2, *busyboxPod})

	// Validate that the eviction is denied with TooManyRequests for the couchbase pods, and allowed for the busybox pod
	framework.ValidateEvictionDenied(t, responses, http.StatusTooManyRequests, reschedule.RescheduleAnnotationAddedToPodMsg, cbPod1.Name)
	framework.ValidateEvictionDenied(t, responses, http.StatusTooManyRequests, reschedule.RescheduleAnnotationAddedToPodMsg, cbPod2.Name)
	framework.ValidateEvictionAllowed(t, responses, busyboxPod.Name)

	// Validate the couchbase pods do not have the reschedule annotation as they should not have been mutated during a dry run
	cluster.ValidatePodDoesNotHaveAnnotation(t, cbPod1.Name, reschedule.DefaultRescheduleAnnotationKey, reschedule.DefaultRescheduleAnnotationValue)
	cluster.ValidatePodDoesNotHaveAnnotation(t, cbPod2.Name, reschedule.DefaultRescheduleAnnotationKey, reschedule.DefaultRescheduleAnnotationValue)

	// Validate the couchbase cluster does not have the tracking annotation
	cluster.ValidateCouchbaseClusterDoesNotHaveAnnotations(t, "couchbase-cluster", map[string]string{
		reschedule.TrackingResourceAnnotation(cbPod1.Name, cbPod1.Namespace): "true",
		reschedule.TrackingResourceAnnotation(cbPod2.Name, cbPod2.Namespace): "true",
	})
}
