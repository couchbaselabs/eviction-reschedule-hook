package framework

import (
	"context"
	"testing"

	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// MustCreateCouchbaseCluster creates a simplified CouchbaseCluster resource in the test cluster.
func (tc *TestCluster) MustCreateCouchbaseCluster(t *testing.T, name string, inPlaceUpgrade bool) func() {
	tc.CreateCouchbaseClusterCRD(t)
	tc.AddClusterRolePermissions(t, "couchbase.com", "couchbaseclusters")

	upgradeProcess := "SwapRebalance"
	if inPlaceUpgrade {
		upgradeProcess = "InPlaceUpgrade"
	}

	obj := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "couchbase.com/v2",
			"kind":       "CouchbaseCluster",
			"metadata": map[string]interface{}{
				"name":      name,
				"namespace": tc.namespace,
			},
			"spec": map[string]interface{}{
				"upgradeProcess": upgradeProcess,
			},
		},
	}

	// Create the CR instance
	_, err := tc.dynamicClient.Resource(CouchbaseClusterGVR).Namespace(tc.namespace).Create(context.TODO(), obj, metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("Failed to create CouchbaseCluster: %v", err)
	}

	// Return a cleanup method that will delete the CouchbaseCluster resource.
	return func() {
		err := tc.dynamicClient.Resource(CouchbaseClusterGVR).Namespace(tc.namespace).Delete(context.TODO(), name, metav1.DeleteOptions{})
		if err != nil {
			t.Fatalf("Failed to delete CouchbaseCluster: %v", err)
		}
	}
}

func (tc *TestCluster) ValidateCouchbaseClusterHasAnnotations(t *testing.T, name string, expectedAnnotations map[string]string) {
	obj, err := tc.dynamicClient.Resource(CouchbaseClusterGVR).Namespace(tc.namespace).Get(context.TODO(), name, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("Failed to get CouchbaseCluster: %v", err)
	}

	validateHasAnnotations(t, obj, expectedAnnotations)
}

func (tc *TestCluster) ValidateCouchbaseClusterDoesNotHaveAnnotations(t *testing.T, name string, expectedAnnotations map[string]string) {
	obj, err := tc.dynamicClient.Resource(CouchbaseClusterGVR).Namespace(tc.namespace).Get(context.TODO(), name, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("Failed to get CouchbaseCluster: %v", err)
	}

	validateDoesNotHaveAnnotations(t, obj, expectedAnnotations)
}

func (tc *TestCluster) ValidateNamespaceHasAnnotations(t *testing.T, name string, expectedAnnotations map[string]string) {
	obj, err := tc.dynamicClient.Resource(NamespaceGVR).Get(context.TODO(), name, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("Failed to get Namespace: %v", err)
	}

	validateHasAnnotations(t, obj, expectedAnnotations)
}

func (tc *TestCluster) ValidateNamespaceDoesNotHaveAnnotations(t *testing.T, name string, expectedAnnotations map[string]string) {
	obj, err := tc.dynamicClient.Resource(NamespaceGVR).Get(context.TODO(), name, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("Failed to get Namespace: %v", err)
	}

	validateDoesNotHaveAnnotations(t, obj, expectedAnnotations)
}

func (tc *TestCluster) CreateCouchbaseClusterCRD(t *testing.T) {
	crd := &apiextensionsv1.CustomResourceDefinition{
		ObjectMeta: metav1.ObjectMeta{
			Name: "couchbaseclusters.couchbase.com",
		},
		Spec: apiextensionsv1.CustomResourceDefinitionSpec{
			Group: "couchbase.com",
			Names: apiextensionsv1.CustomResourceDefinitionNames{
				Kind:       "CouchbaseCluster",
				Singular:   "couchbasecluster",
				Plural:     "couchbaseclusters",
				ShortNames: []string{"cbc"},
			},
			Scope: apiextensionsv1.NamespaceScoped,
			Versions: []apiextensionsv1.CustomResourceDefinitionVersion{
				{
					Name: "v2",
					Schema: &apiextensionsv1.CustomResourceValidation{
						OpenAPIV3Schema: &apiextensionsv1.JSONSchemaProps{
							Properties: map[string]apiextensionsv1.JSONSchemaProps{
								"apiVersion": {
									Type: "string",
								},
								"kind": {
									Type: "string",
								},
								"metadata": {
									Type: "object",
								},
								"spec": {
									Type: "object",
									Properties: map[string]apiextensionsv1.JSONSchemaProps{
										"upgradeProcess": {
											Type: "string",
										},
									},
								},
							},
							Required: []string{"spec"},
							Type:     "object",
						},
					},
					Served:  true,
					Storage: true,
				},
			},
		},
	}

	_, err := tc.crdClient.ApiextensionsV1().CustomResourceDefinitions().Create(context.TODO(), crd, metav1.CreateOptions{})
	if err != nil {
		if !errors.IsAlreadyExists(err) {
			t.Fatalf("Failed to create CouchbaseCluster CRD: %v", err)
		}

		// Get the existing CRD
		existingCRD, err := tc.crdClient.ApiextensionsV1().CustomResourceDefinitions().Get(context.TODO(), crd.Name, metav1.GetOptions{})
		if err != nil {
			t.Fatalf("Failed to get existing CouchbaseCluster CRD: %v", err)
		}

		// Set the resourceVersion from the existing CRD and update it
		crd.ResourceVersion = existingCRD.ResourceVersion
		_, err = tc.crdClient.ApiextensionsV1().CustomResourceDefinitions().Update(context.TODO(), crd, metav1.UpdateOptions{})
		if err != nil {
			t.Fatalf("Failed to update CouchbaseCluster CRD: %v", err)
		}
	}
}
