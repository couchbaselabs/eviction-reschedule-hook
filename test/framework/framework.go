package framework

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	admissionregistrationv1 "k8s.io/api/admissionregistration/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apiextensionsclient "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

type TestFramework struct {
	client    *kubernetes.Clientset
	crdClient *apiextensionsclient.Clientset
	namespace string
}

// TestCluster represents a test environment with its own namespace
func SetupFramework() (*TestFramework, error) {
	clientConfig, err := clientConfig()
	if err != nil {
		return nil, err
	}

	client, err := kubernetes.NewForConfig(clientConfig)
	if err != nil {
		return nil, err
	}

	crdClient, err := apiextensionsclient.NewForConfig(clientConfig)
	if err != nil {
		return nil, err
	}

	tf := &TestFramework{
		client:    client,
		crdClient: crdClient,
		namespace: defaultNamespace,
	}

	// Delete any existing resources
	tf.TearDown()

	caCert, err := createWebhookSecret(tf.client, secretName, svcName, tf.namespace)
	if err != nil {
		return nil, fmt.Errorf("failed to create webhook secret: %v", err)
	}

	steps := []struct {
		name string
		fn   func() error
	}{
		{"create webhook config", func() error {
			return createWebhookConfig(tf.client, svcName, webhookConfigName, tf.namespace, caCert)
		}},
		{"create service account", func() error {
			return createRescheduleHookServerServiceAccount(tf.client, saName, tf.namespace)
		}},
		{"create cluster role", func() error {
			return createRescheduleHookServerClusterRole(tf.client, crName, tf.namespace)
		}},
		{"create cluster role binding", func() error {
			return createRescheduleHookServerClusterRoleBinding(tf.client, crbName, saName, tf.namespace, crName)
		}},
		{"create service", func() error {
			return createRescheduleHookServerService(tf.client, svcName, tf.namespace)
		}},
	}

	for _, step := range steps {
		if err := step.fn(); err != nil {
			return nil, fmt.Errorf("failed to %s: %v", step.name, err)
		}
	}

	return tf, nil
}

func (tf *TestFramework) TearDown() {
	slog.Info("Tearing down test framework", "namespace", tf.namespace)                                                                     //nolint:errcheck
	tf.client.CoreV1().ServiceAccounts(tf.namespace).Delete(context.TODO(), saName, metav1.DeleteOptions{})                                 //nolint:errcheck
	tf.client.RbacV1().ClusterRoles().Delete(context.TODO(), crName, metav1.DeleteOptions{})                                                //nolint:errcheck
	tf.client.RbacV1().ClusterRoleBindings().Delete(context.TODO(), crbName, metav1.DeleteOptions{})                                        //nolint:errcheck
	tf.client.CoreV1().Secrets(tf.namespace).Delete(context.TODO(), secretName, metav1.DeleteOptions{})                                     //nolint:errcheck
	tf.client.CoreV1().Services(tf.namespace).Delete(context.TODO(), svcName, metav1.DeleteOptions{})                                       //nolint:errcheck
	tf.client.AdmissionregistrationV1().ValidatingWebhookConfigurations().Delete(context.TODO(), webhookConfigName, metav1.DeleteOptions{}) //nolint:errcheck
}

func createWebhookConfig(client *kubernetes.Clientset, svcName, configName, namespace string, caCert []byte) error {
	slog.Info("Creating webhook config", "namespace", namespace, "svcName", svcName, "configName", configName)

	webhookConfig := &admissionregistrationv1.ValidatingWebhookConfiguration{
		ObjectMeta: metav1.ObjectMeta{Name: configName},
		Webhooks: []admissionregistrationv1.ValidatingWebhook{
			{
				Name: svcName + "." + namespace + ".svc",
				Rules: []admissionregistrationv1.RuleWithOperations{
					{
						Operations: []admissionregistrationv1.OperationType{"CREATE"},
						Rule: admissionregistrationv1.Rule{
							APIGroups:   []string{""},
							APIVersions: []string{"v1"},
							Resources:   []string{"pods/eviction"},
						},
					},
				},
				ClientConfig: admissionregistrationv1.WebhookClientConfig{
					Service: &admissionregistrationv1.ServiceReference{
						Name:      svcName,
						Namespace: namespace,
						Path:      stringPtr("/eviction"),
					},
					CABundle: caCert,
				},
				AdmissionReviewVersions: []string{"v1"},
				SideEffects:             (*admissionregistrationv1.SideEffectClass)(stringPtr("None")),
			},
		},
	}

	_, err := client.AdmissionregistrationV1().ValidatingWebhookConfigurations().Create(context.TODO(), webhookConfig, metav1.CreateOptions{})
	if err != nil {
		if !errors.IsAlreadyExists(err) {
			return err
		}
		// If webhook config exists, update it
		_, err = client.AdmissionregistrationV1().ValidatingWebhookConfigurations().Update(context.TODO(), webhookConfig, metav1.UpdateOptions{})
	}
	return err
}

func createRescheduleHookServerService(client *kubernetes.Clientset, svcName, namespace string) error {
	slog.Info("Creating reschedule hook server service", "namespace", namespace, "svcName", svcName)

	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: svcName},
		Spec: corev1.ServiceSpec{
			Selector: map[string]string{
				"app": svcName,
			},
			Ports: []corev1.ServicePort{
				{
					Port:       443,
					TargetPort: intstr.FromString("webhook-api"),
				},
			},
		},
	}

	_, err := client.CoreV1().Services(namespace).Create(context.TODO(), service, metav1.CreateOptions{})
	if err != nil {
		if !errors.IsAlreadyExists(err) {
			return err
		}
		// If service exists, update it
		_, err = client.CoreV1().Services(namespace).Update(context.TODO(), service, metav1.UpdateOptions{})
	}
	return err
}

func createRescheduleHookServerServiceAccount(client *kubernetes.Clientset, saName, namespace string) error {
	slog.Info("Creating reschedule hook server service account", "namespace", namespace, "saName", saName)

	serviceAccount := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{Name: saName},
	}

	_, err := client.CoreV1().ServiceAccounts(namespace).Create(context.TODO(), serviceAccount, metav1.CreateOptions{})
	if err != nil {
		if !errors.IsAlreadyExists(err) {
			return err
		}
		// If service account exists, update it
		_, err = client.CoreV1().ServiceAccounts(namespace).Update(context.TODO(), serviceAccount, metav1.UpdateOptions{})
	}
	return err
}

func createRescheduleHookServerClusterRole(client *kubernetes.Clientset, crName, namespace string) error {
	slog.Info("Creating reschedule hook server cluster role", "namespace", namespace, "crName", crName)

	clusterRole := &rbacv1.ClusterRole{
		ObjectMeta: metav1.ObjectMeta{Name: crName},
		Rules: []rbacv1.PolicyRule{
			{
				APIGroups: []string{""},
				Resources: []string{"pods"},
				Verbs:     []string{"get", "patch"},
			},
		},
	}

	_, err := client.RbacV1().ClusterRoles().Create(context.TODO(), clusterRole, metav1.CreateOptions{})
	if err != nil {
		if !errors.IsAlreadyExists(err) {
			return err
		}
	}
	return nil
}

func createRescheduleHookServerClusterRoleBinding(client *kubernetes.Clientset, crbName, saName, namespace, crName string) error {
	slog.Info("Creating reschedule hook server cluster role binding", "namespace", namespace, "crbName", crbName, "saName", saName, "crName", crName)

	clusterRoleBinding := &rbacv1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{Name: crbName},
		Subjects: []rbacv1.Subject{
			{
				Kind:      "ServiceAccount",
				Name:      saName,
				Namespace: namespace,
			},
		},
		RoleRef: rbacv1.RoleRef{
			Kind: "ClusterRole",
			Name: crName,
		},
	}

	_, err := client.RbacV1().ClusterRoleBindings().Create(context.TODO(), clusterRoleBinding, metav1.CreateOptions{})
	if err != nil {
		if !errors.IsAlreadyExists(err) {
			return err
		}
	}
	return nil
}

func createWebhookSecret(client *kubernetes.Clientset, secretName, serviceName, namespace string) ([]byte, error) {
	slog.Info("Creating webhook secret", "namespace", namespace, "secretName", secretName, "serviceName", serviceName)

	caCertPEM, caKeyPEM, err := GenerateSelfSignedCA("webhook-test-ca")
	if err != nil {
		return nil, err
	}

	certPEM, keyPEM, err := GenerateServingCert(
		caCertPEM, caKeyPEM,
		[]string{
			serviceName + "." + namespace + ".svc",
			serviceName + "." + namespace + ".svc.cluster.local",
		},
		serviceName+"."+namespace+".svc",
	)
	if err != nil {
		return nil, err
	}

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: secretName},
		Type:       corev1.SecretTypeTLS,
		Data: map[string][]byte{
			corev1.TLSCertKey:       certPEM,
			corev1.TLSPrivateKeyKey: keyPEM,
		},
	}

	_, err = client.CoreV1().Secrets(namespace).Create(context.TODO(), secret, metav1.CreateOptions{})
	if err != nil {
		if !errors.IsAlreadyExists(err) {
			return nil, err
		}
		// If secret exists, update it
		_, err = client.CoreV1().Secrets(namespace).Update(context.TODO(), secret, metav1.UpdateOptions{})
		if err != nil {
			return nil, err
		}
	}

	return caCertPEM, nil
}

func clientConfig() (*rest.Config, error) {
	config, err := rest.InClusterConfig()
	if err != nil {
		// If in-cluster config fails, try local kubeconfig
		kubeconfig := filepath.Join(os.Getenv("HOME"), ".kube", "config")
		config, err = clientcmd.BuildConfigFromFlags("", kubeconfig)
		if err != nil {
			return nil, err
		}
	}

	return config, nil
}

func stringPtr(s string) *string {
	return &s
}

func boolPtr(b bool) *bool {
	return &b
}
