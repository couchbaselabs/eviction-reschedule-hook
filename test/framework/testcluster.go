package framework

import (
	"context"
	"fmt"
	"log/slog"
	"os/exec"
	"testing"
	"time"

	reschedule "github.com/couchbase/couchbase-reschedule-hook/pkg/reschedule"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apiextensionsclient "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
)

type TestCluster struct {
	namespace     string
	client        *kubernetes.Clientset
	dynamicClient *dynamic.DynamicClient
	crdClient     *apiextensionsclient.Clientset
}

func (tc *TestCluster) GetNamespace() string {
	return tc.namespace
}

// SetupTestCluster creates a new namespace for a test and returns a TestCluster instance
// This should be called at the start of each test. Set serverConfig to nil to use the default config.
func SetupTestCluster(t *testing.T, serverConfig *reschedule.Config) *TestCluster {
	clientConfig, err := clientConfig()
	if err != nil {
		t.Fatalf("Failed to create client config: %v", err)
	}

	client, err := kubernetes.NewForConfig(clientConfig)
	if err != nil {
		t.Fatalf("Failed to create kubernetes client: %v", err)
	}

	dynamicClient, err := dynamic.NewForConfig(clientConfig)
	if err != nil {
		t.Fatalf("Failed to create dynamic client: %v", err)
	}

	crdClient, err := apiextensionsclient.NewForConfig(clientConfig)
	if err != nil {
		t.Fatalf("Failed to create CRD client: %v", err)
	}

	// Create a unique namespace for the test
	testNamespace := fmt.Sprintf("test-%d", time.Now().UnixNano())
	_, err = client.CoreV1().Namespaces().Create(context.TODO(), &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: testNamespace,
		},
	}, metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("Failed to create namespace: %v", err)
	}

	tc := &TestCluster{
		namespace:     testNamespace,
		client:        client,
		dynamicClient: dynamicClient,
		crdClient:     crdClient,
	}

	// We need to recreate reschedule hook server inside each test as the withTrackingResource flag is determined by the test.
	// For now, this is created in the default namespace, but at some point it'd be nice to create
	// this inside the test namespace, with a validating webhook pointing to it for pod evictions
	// that occur in the same namespace. This would then allow for test parallelism.
	tc.CreateRescheduleHookServer(t, svcName, defaultNamespace, secretName, serverConfig)

	// Register the cleanup function to run after the test
	t.Cleanup(func() {
		// Delete the reschedule hook server. We need to block until this occurs as the server
		// is recreated with the same name for each test.
		tc.MustDeletePod(t, defaultNamespace, svcName)

		// Delete the namespace. This might take a while, but we don't need to block until it occurs.
		if err := client.CoreV1().Namespaces().Delete(context.TODO(), tc.namespace, metav1.DeleteOptions{}); err != nil {
			slog.Error("Failed to delete namespace", "error", err, "namespace", tc.namespace)
		}
	})

	return tc
}

// retryGet waits up to a minute for a function to return a nil error value, with the fetch method running every second until it does.
func retryFetch(t *testing.T, name string, fetchResource func() (interface{}, error)) interface{} {
	timeout := time.After(1 * time.Minute)
	tick := time.Tick(1 * time.Second)

	for {
		select {
		case <-timeout:
			t.Fatalf("Timed out waiting for resource %s", name)
		case <-tick:
			resource, err := fetchResource()
			if err == nil {
				return resource
			}
		}
	}
}

// DrainNode runs kubectl drain command on the specified node
func (tc *TestCluster) DrainNode(t *testing.T, nodeName string) error {
	cmd := exec.Command("kubectl", "drain", nodeName, "--ignore-daemonsets", "--delete-emptydir-data", "--force")
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to drain node %s: %v\nOutput: %s", nodeName, err, string(output))
	}
	return nil
}

func (tc *TestCluster) CreateRescheduleHookServer(t *testing.T, rescheduleHookServerName, namespace string, secretName string, config *reschedule.Config) {
	slog.Info("Creating reschedule hook server for test", "testNamespace", tc.namespace, "namespace", namespace, "svcName", svcName, "secretName", secretName)

	envVars := []corev1.EnvVar{}
	if config != nil {
		for k, v := range config.ToEnvironment() {
			envVars = append(envVars, corev1.EnvVar{Name: k, Value: v})
		}
	}

	// Make sure the pod has been deleted by the last test.
	tc.MustDeletePod(t, rescheduleHookServerName, namespace)

	server := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name: rescheduleHookServerName,
			Labels: map[string]string{
				"app": svcName,
			},
		},
		Spec: corev1.PodSpec{
			SecurityContext: &corev1.PodSecurityContext{
				RunAsNonRoot: boolPtr(true),
			},
			ServiceAccountName: "reschedule-hook-sa",
			Containers: []corev1.Container{
				{
					Name:            rescheduleHookServerName,
					Image:           "couchbase/couchbase-reschedule-hook:latest",
					ImagePullPolicy: corev1.PullIfNotPresent,
					Ports: []corev1.ContainerPort{
						{
							ContainerPort: 8443,
							Name:          "webhook-api",
						},
					},
					VolumeMounts: []corev1.VolumeMount{
						{
							Name:      "webhook-certs",
							MountPath: "/etc/webhook/certs",
							ReadOnly:  true,
						},
					},
					Env: envVars,
				},
			},
			Volumes: []corev1.Volume{
				{
					Name: "webhook-certs",
					VolumeSource: corev1.VolumeSource{
						Secret: &corev1.SecretVolumeSource{
							SecretName: secretName,
						},
					},
				},
			},
		},
	}

	createPodAndWait(t, tc.client, rescheduleHookServerName, namespace, server)
}

// AddClusterRolePermissions adds get and patch permissions to the cluster role for the given group and resource.
// If the rule already exists, it will not be added again. The additional permissions will also not be removed from the role after
// the TestCluster is deleted.
func (tc *TestCluster) AddClusterRolePermissions(t *testing.T, group, resource string) {
	cr, err := tc.client.RbacV1().ClusterRoles().Get(context.TODO(), crName, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("Failed to get cluster role: %v", err)
	}

	// Check if the rule already exists
	for _, rule := range cr.Rules {
		if len(rule.APIGroups) == 1 && rule.APIGroups[0] == group &&
			len(rule.Resources) == 1 && rule.Resources[0] == resource &&
			len(rule.Verbs) == 2 && rule.Verbs[0] == "get" && rule.Verbs[1] == "patch" {
			// Rule already exists, no need to add it again
			return
		}
	}

	// Add the new rule if it doesn't exist
	cr.Rules = append(cr.Rules, rbacv1.PolicyRule{
		APIGroups: []string{group},
		Resources: []string{resource},
		Verbs:     []string{"get", "patch"},
	})

	_, err = tc.client.RbacV1().ClusterRoles().Update(context.TODO(), cr, metav1.UpdateOptions{})
	if err != nil {
		t.Fatalf("Failed to update cluster role: %v", err)
	}
}

// ValidateCouchbaseClusterDoesNotHaveAnnotations validates that the CouchbaseCluster resource does not have the given annotations
func (tc *TestCluster) ValidateResourceDoesNotHaveAnnotations(t *testing.T, resourceName string, resourceGVR schema.GroupVersionResource, expectedAnnotations map[string]string) {
	obj, err := tc.dynamicClient.Resource(resourceGVR).Namespace(tc.namespace).Get(context.TODO(), resourceName, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("Failed to get %s %s: %v", resourceGVR.Resource, resourceName, err)
	}

	annotations := obj.GetAnnotations()
	for key := range expectedAnnotations {
		if _, exists := annotations[key]; exists {
			t.Fatalf("Expected annotation %s to not exist on %s %s", key, resourceGVR.Resource, resourceName)
		}
	}
}

// ValidateCouchbaseClusterHasAnnotations validates that the CouchbaseCluster resource has the given annotations
func (tc *TestCluster) ValidateResourceHasAnnotations(t *testing.T, resourceName string, resourceGVR schema.GroupVersionResource, expectedAnnotations map[string]string) {
	obj, err := tc.dynamicClient.Resource(resourceGVR).Namespace(tc.namespace).Get(context.TODO(), resourceName, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("Failed to get %s %s: %v", resourceGVR.Resource, resourceName, err)
	}

	annotations := obj.GetAnnotations()
	for key, expectedValue := range expectedAnnotations {
		actualValue, exists := annotations[key]
		if !exists {
			t.Fatalf("Expected annotation %s to exist on %s %s", key, resourceGVR.Resource, resourceName)
		}
		if actualValue != expectedValue {
			t.Fatalf("Expected annotation %s to have value %s, got %s on %s %s",
				key, expectedValue, actualValue, resourceGVR.Resource, resourceName)
		}
	}
}

func validateDoesNotHaveAnnotations(t *testing.T, obj *unstructured.Unstructured, expectedAnnotations map[string]string) {
	annotations := obj.GetAnnotations()
	for key := range expectedAnnotations {
		if _, exists := annotations[key]; exists {
			t.Fatalf("Expected annotation %s to not exist on %s %s", key, obj.GetKind(), obj.GetName())
		}
	}
}

func validateHasAnnotations(t *testing.T, obj *unstructured.Unstructured, expectedAnnotations map[string]string) {
	annotations := obj.GetAnnotations()
	for key, expectedValue := range expectedAnnotations {
		actualValue, exists := annotations[key]
		if !exists {
			t.Fatalf("Expected annotation %s to exist on %s %s", key, obj.GetKind(), obj.GetName())
		}
		if actualValue != expectedValue {
			t.Fatalf("Expected annotation %s to have value %s, got %s on %s %s",
				key, expectedValue, actualValue, obj.GetKind(), obj.GetName())
		}
	}
}
