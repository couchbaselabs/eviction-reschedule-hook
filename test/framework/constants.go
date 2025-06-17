package framework

import "k8s.io/apimachinery/pkg/runtime/schema"

const (
	defaultNamespace    = "default"
	saName              = "reschedule-hook-sa"
	crName              = "reschedule-hook-cr"
	crbName             = "reschedule-hook-crb"
	secretName          = "reschedule-hook-tls"
	svcName             = "reschedule-hook-server"
	webhookConfigName   = "reschedule-webhook-config"
	rescheduleHookImage = "couchbase/couchbase-reschedule-hook:latest"
)

var CouchbaseClusterGVR = schema.GroupVersionResource{
	Group:    "couchbase.com",
	Version:  "v2",
	Resource: "couchbaseclusters",
}

var NamespaceGVR = schema.GroupVersionResource{
	Version:  "v1",
	Resource: "namespaces",
}
