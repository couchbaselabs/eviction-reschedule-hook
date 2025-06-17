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

const (
	RescheduledPodsTrackingKeyPrefix = "reschedule.hook/"
)

type Client interface {
	GetPod(name, namespace string) (*corev1.Pod, error)
	ReschedulePod(pod *corev1.Pod) error
	GetTrackingResourceInstance(name, namespace string) (*unstructured.Unstructured, error)
	AddRescheduleHookTrackingAnnotation(podName, podNamespace, resourceInstanceName string) error
	RemoveRescheduleHookTrackingAnnotation(podName, podNamespace, resourceInstanceName string) error
	ShouldTrackRescheduledPods() bool
	ShouldAddTrackingAnnotation(trackingResourceInstance *unstructured.Unstructured) bool
	GetConfig() *Config
}

type ClientImpl struct {
	config        *Config
	dynamicClient dynamic.Interface
}

func NewClient(config *Config) (Client, error) {
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
		config:        config,
	}, nil
}

func (c *ClientImpl) GetConfig() *Config {
	return c.config
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

func (c *ClientImpl) GetTrackingResourceInstance(name, namespace string) (*unstructured.Unstructured, error) {
	return c.config.trackingResource.GetResourceInterface(c.dynamicClient, namespace).Get(context.TODO(), name, metav1.GetOptions{})
}

// AddRescheduleHookTrackingAnnotation adds an annotation to the tracking resource, marking that a pod has had the reschedule annotation added to it.
func (c *ClientImpl) AddRescheduleHookTrackingAnnotation(podName, podNamespace, trackingResourceName string) error {
	return c.addResourceAnnotation(trackingResourceName, TrackingResourceAnnotation(podName, podNamespace), "true", c.config.trackingResource.GetResourceInterface(c.dynamicClient, podNamespace))
}

// RemoveRescheduleHookTrackingAnnotation removes the tracking annotation from the tracking resource if it is present
func (c *ClientImpl) RemoveRescheduleHookTrackingAnnotation(podName, podNamespace, trackingResourceName string) error {
	return c.removeResourceAnnotation(trackingResourceName, TrackingResourceAnnotation(podName, podNamespace), c.config.trackingResource.GetResourceInterface(c.dynamicClient, podNamespace))
}

func (c *ClientImpl) ReschedulePod(pod *corev1.Pod) error {
	return c.addResourceAnnotation(pod.Name, c.config.rescheduleAnnotationKey, c.config.rescheduleAnnotationValue, c.dynamicClient.Resource(podResource).Namespace(pod.Namespace))
}

func (c *ClientImpl) ShouldTrackRescheduledPods() bool {
	return c.config.trackRescheduledPods
}

func (c *ClientImpl) ShouldAddTrackingAnnotation(trackingResourceInstance *unstructured.Unstructured) bool {
	return c.config.trackingResource.ShouldTrack(trackingResourceInstance)
}

func (c *ClientImpl) addResourceAnnotation(name, annotation string, value string, resourceInterface dynamic.ResourceInterface) error {
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

	_, err = resourceInterface.Patch(context.TODO(), name, types.MergePatchType, payload, metav1.PatchOptions{})
	return err
}

func (c *ClientImpl) removeResourceAnnotation(name, annotation string, resourceInterface dynamic.ResourceInterface) error {
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

	_, err = resourceInterface.Patch(context.TODO(), name, types.MergePatchType, payload, metav1.PatchOptions{})
	return err
}

func TrackingResourceAnnotation(podName, podNamespace string) string {
	return RescheduledPodsTrackingKeyPrefix + podNamespace + "." + podName
}
