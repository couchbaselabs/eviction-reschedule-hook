package framework

import (
	"context"
	"fmt"
	"sync"
	"testing"

	corev1 "k8s.io/api/core/v1"
	policyv1 "k8s.io/api/policy/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

// Common pod container spec for mock pods
var mockContainerSpec = corev1.Container{
	Name:    "busybox",
	Image:   "busybox:1.28",
	Command: []string{"sh", "-c", "while true; do sleep 1; done"},
}

// MustGetPod waits for the pod with the given name to exist and returns it
func (tc *TestCluster) MustGetPod(t *testing.T, name string) *corev1.Pod {
	return retryFetch(t, name, func() (interface{}, error) {
		return tc.client.CoreV1().Pods(tc.GetNamespace()).Get(context.TODO(), name, metav1.GetOptions{})
	}).(*corev1.Pod)
}

// MustCreatePod creates a basic pod with the given name and waits for it to be running
func (tc *TestCluster) MustCreatePod(t *testing.T, name string, labels map[string]string) *corev1.Pod {
	return createBasicPod(t, tc.client, name, tc.GetNamespace(), labels)
}

// MustCreateCouchbasePod creates a basic pod with couchbase labels and waits for it to be running
func (tc *TestCluster) MustCreateCouchbasePod(t *testing.T, name, cbCluster string) *corev1.Pod {
	labels := map[string]string{
		"app":               "couchbase",
		"couchbase_cluster": cbCluster,
	}
	return createBasicPod(t, tc.client, name, tc.GetNamespace(), labels)
}

func (tc *TestCluster) EvictPodsDryRun(t *testing.T, pods []corev1.Pod) map[string]error {
	return tc.sendConcurrentEvictionRequests(t, pods, &metav1.DeleteOptions{DryRun: []string{metav1.DryRunAll}})
}

func (tc *TestCluster) EvictPods(t *testing.T, pods []corev1.Pod) map[string]error {
	return tc.sendConcurrentEvictionRequests(t, pods, nil)
}

// EvictPods sends eviction requests concurrently to the given pods and returns a map of pod names to errors
// The concurrent eviction of pods is more similar to the drain command
func (tc *TestCluster) sendConcurrentEvictionRequests(t *testing.T, pods []corev1.Pod, deleteOptions *metav1.DeleteOptions) map[string]error {
	var wg sync.WaitGroup
	ch := make(chan map[string]error, len(pods))
	for _, pod := range pods {
		wg.Add(1)
		go func(pod corev1.Pod, ch chan map[string]error) {
			defer wg.Done()
			err := tc.EvictPod(t, pod.Name, pod.Namespace, deleteOptions)
			ch <- map[string]error{pod.Name: err}
		}(pod, ch)
	}

	wg.Wait()
	close(ch)

	responses := make(map[string]error)
	for resp := range ch {
		for podName, err := range resp {
			responses[podName] = err
		}
	}
	return responses
}

// EvictPod sends an eviction request for a single pod
func (tc *TestCluster) EvictPod(t *testing.T, name, namespace string, deleteOptions *metav1.DeleteOptions) error {
	eviction := &policyv1.Eviction{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		DeleteOptions: deleteOptions,
	}

	return tc.client.CoreV1().Pods(tc.GetNamespace()).EvictV1(context.TODO(), eviction)
}

// MustDeletePod deletes the pod with the given name and waits for it to no longer exist
func (tc *TestCluster) MustDeletePod(t *testing.T, name, namespace string) {
	if err := tc.client.CoreV1().Pods(namespace).Delete(context.TODO(), name, metav1.DeleteOptions{}); err != nil {
		if !errors.IsNotFound(err) {
			t.Fatalf("Failed to delete pod: %v", err)
		}
	}

	retryFetch(t, name, func() (interface{}, error) {
		if _, err := tc.client.CoreV1().Pods(namespace).Get(context.TODO(), name, metav1.GetOptions{}); err != nil && errors.IsNotFound(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("pod %s still exists", name)
	})
}

// createBasicPod creates a basic busybox pod with the given name and labels, and waits for it to be scheduled and running
func createBasicPod(t *testing.T, client *kubernetes.Clientset, name, namespace string, labels map[string]string) *corev1.Pod {
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:   name,
			Labels: labels,
		},
		Spec: corev1.PodSpec{
			Containers:                    []corev1.Container{mockContainerSpec},
			TerminationGracePeriodSeconds: &[]int64{0}[0],
		},
	}

	return createPodAndWait(t, client, name, namespace, pod)
}

func createPodAndWait(t *testing.T, client *kubernetes.Clientset, name, namespace string, pod *corev1.Pod) *corev1.Pod {
	_, err := client.CoreV1().Pods(namespace).Create(context.TODO(), pod, metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("Failed to create pod: %v", err)
	}

	fetchPod := func() (interface{}, error) {
		pod, err := client.CoreV1().Pods(namespace).Get(context.TODO(), name, metav1.GetOptions{})
		if err != nil {
			return nil, err
		}

		if pod.Status.Phase != corev1.PodRunning || !pod.Status.ContainerStatuses[0].Ready {
			return nil, fmt.Errorf("pod %s is not running or not ready, current phase: %s, ready: %t", name, pod.Status.Phase, pod.Status.ContainerStatuses[0].Ready)
		}

		return pod, nil
	}
	return retryFetch(t, name, fetchPod).(*corev1.Pod)
}
