package reschedule

import (
	"context"
	"encoding/json"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/rest"
)

var podResource = schema.GroupVersionResource{Group: "", Version: "v1", Resource: "pods"}

var CouchbaseClusterResource = schema.GroupVersionResource{Group: "couchbase.com", Version: "v2", Resource: "couchbaseclusters"}

const (
	RescheduleHookPodsListAnnotation       = "reschedule.hook/rescheduledPod"
	RescheduleHookTrackingAnnotationPrefix = "reschedule.hook/"
	RescheduleTrue                         = "true"
	RescheduleAnnotation                   = "cao.couchbase.com/reschedule"
	CouchbaseClusterLabelKey               = "couchbase_cluster"
	InPlaceUpgradeStrategyKey              = "upgradeProcess"
	InPlaceUpgradeStrategyValue            = "InPlaceUpgrade"
)

type Client interface {
	GetPod(name, namespace string) (*corev1.Pod, error)
	GetClusterInfo(name, namespace string) (*ClusterInfo, error)
	ReschedulePod(pod *corev1.Pod) error
	PatchRescheduleHookPodsList(name, namespace string, list []string) error
	AddRescheduleHookTrackingAnnotation(podName, clusterName, clusterNamespace string) error
	RemoveRescheduleHookTrackingAnnotation(podName, clusterName, clusterNamespace string) error
}

type ClientImpl struct {
	dynamicClient dynamic.Interface
}

type ClusterInfo struct {
	clusterAnnotations     map[string]string
	rescheduleHookPodsList []string
	inPlaceUpgrade         bool
}

func NewClient() (Client, error) {
	kubeConfig, err := rest.InClusterConfig()
	if err != nil {
		return nil, err
	}

	dynamicClient, err := dynamic.NewForConfig(kubeConfig)
	if err != nil {
		return nil, err
	}

	return &ClientImpl{
		dynamicClient: dynamicClient,
	}, nil
}

func (c *ClientImpl) GetPod(name, namespace string) (*corev1.Pod, error) {
	podUnstructured, err := c.dynamicClient.Resource(podResource).Namespace(namespace).Get(context.TODO(), name, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}

	pod := &corev1.Pod{}
	err = runtime.DefaultUnstructuredConverter.FromUnstructured(podUnstructured.Object, pod)
	if err != nil {
		return nil, fmt.Errorf("failed to convert unstructured to Pod: %w", err)
	}

	return pod, nil
}

func (c *ClientImpl) GetClusterInfo(name, namespace string) (*ClusterInfo, error) {
	cluster, err := c.dynamicClient.Resource(CouchbaseClusterResource).Namespace(namespace).Get(context.TODO(), name, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}

	upgradeStrategy, found, err := unstructured.NestedString(cluster.Object, "spec", InPlaceUpgradeStrategyKey)
	if err != nil || !found {
		return nil, fmt.Errorf("error reading cluster upgrade strategy: %w", err)
	}

	return &ClusterInfo{
		clusterAnnotations: cluster.GetAnnotations(),
		inPlaceUpgrade:     upgradeStrategy == InPlaceUpgradeStrategyValue,
	}, nil
}

func (c *ClientImpl) GetClusterInfoList(name, namespace string) (*ClusterInfo, error) {
	cluster, err := c.dynamicClient.Resource(CouchbaseClusterResource).Namespace(namespace).Get(context.TODO(), name, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}

	upgradeStrategy, found, err := unstructured.NestedString(cluster.Object, "spec", InPlaceUpgradeStrategyKey)
	if err != nil || !found {
		return nil, fmt.Errorf("error reading cluster upgrade strategy: %w", err)
	}

	rescheduleJsonString, exists := cluster.GetAnnotations()[RescheduleHookPodsListAnnotation]
	if !exists || rescheduleJsonString == "" {
		rescheduleJsonString = "[]"
	}

	var list []string
	if err := json.Unmarshal([]byte(rescheduleJsonString), &list); err != nil {
		return nil, fmt.Errorf("error parsing cluster annotations: %w", err)
	}

	return &ClusterInfo{
		clusterAnnotations:     cluster.GetAnnotations(),
		rescheduleHookPodsList: list,
		inPlaceUpgrade:         upgradeStrategy == InPlaceUpgradeStrategyValue,
	}, nil
}

func (c *ClientImpl) AddRescheduleHookTrackingAnnotation(podName, clusterName, clusterNamespace string) error {
	return c.addResourceAnnotation(clusterName, clusterNamespace, RescheduleHookTrackingAnnotationPrefix+podName, RescheduleTrue, CouchbaseClusterResource)
}

func (c *ClientImpl) RemoveRescheduleHookTrackingAnnotation(podName, clusterName, clusterNamespace string) error {
	return c.removeResourceAnnotation(clusterName, clusterNamespace, RescheduleHookTrackingAnnotationPrefix+podName, CouchbaseClusterResource)
}

func (c *ClientImpl) PatchRescheduleHookPodsList(clusterName, clusterNamespace string, list []string) error {
	if len(list) == 0 {
		return c.removeResourceAnnotation(clusterName, clusterNamespace, RescheduleHookPodsListAnnotation, CouchbaseClusterResource)
	}

	jsonBytes, err := json.Marshal(list)
	if err != nil {
		return err
	}

	jsonString := string(jsonBytes)

	return c.addResourceAnnotation(clusterName, clusterNamespace, RescheduleHookPodsListAnnotation, jsonString, CouchbaseClusterResource)
}

func (c *ClientImpl) ReschedulePod(pod *corev1.Pod) error {
	return c.addResourceAnnotation(pod.Name, pod.Namespace, RescheduleAnnotation, RescheduleTrue, podResource)
}

func (c *ClientImpl) addResourceAnnotation(name, namespace string, annotation string, value string, resourceType schema.GroupVersionResource) error {
	patch := map[string]interface{}{
		"metadata": map[string]interface{}{
			"annotations": map[string]interface{}{
				annotation: value,
			},
		},
	}

	payload, err := json.Marshal(patch)
	if err != nil {
		return err
	}

	_, err = c.dynamicClient.Resource(resourceType).Namespace(namespace).Patch(context.TODO(), name, types.MergePatchType, payload, metav1.PatchOptions{})
	return err
}

func (c *ClientImpl) removeResourceAnnotation(name, namespace string, annotation string, resourceType schema.GroupVersionResource) error {
	patch := map[string]interface{}{
		"metadata": map[string]interface{}{
			"annotations": map[string]interface{}{
				annotation: nil,
			},
		},
	}

	payload, err := json.Marshal(patch)
	if err != nil {
		return err
	}

	_, err = c.dynamicClient.Resource(resourceType).Namespace(namespace).Patch(context.TODO(), name, types.MergePatchType, payload, metav1.PatchOptions{})
	return err
}
