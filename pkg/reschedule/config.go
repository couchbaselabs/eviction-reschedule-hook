package reschedule

import (
	"log/slog"
	"os"
	"strconv"
	"strings"

	"github.com/couchbaselabs/eviction-reschedule-hook/pkg/reschedule/tracking"
)

const (
	DefaultRescheduleAnnotationKey   = "cao.couchbase.com/reschedule"
	DefaultRescheduleAnnotationValue = "true"
	DefaultPodLabelSelectorKey       = "app"
	DefaultPodLabelSelectorValue     = "couchbase"
	DefaultCertFile                  = "/etc/webhook/certs/tls.crt"
	DefaultKeyFile                   = "/etc/webhook/certs/tls.key"
	DefaultTrackRescheduledPods      = "true"
	DefaultTrackingResourceType      = tracking.ResourceTypeCouchbaseCluster
	DefaultEnableNodeConditionCheck  = "false"
	DefaultNodeFailureConditionTypes = ""
	DefaultNodeFailureTaintKeys      = ""
)

// Config holds the configuration for the reschedule hook
type Config struct {
	rescheduleAnnotationValue string
	rescheduleAnnotationKey   string
	trackRescheduledPods      bool
	podLabelSelectorKey       string
	podLabelSelectorValue     string
	certFile                  string
	keyFile                   string
	trackingResource          tracking.TrackingResource
	enableNodeConditionCheck  bool
	nodeFailureConditionTypes []string
	nodeFailureTaintKeys      []string
}

func (c *Config) ToEnvironment() map[string]string {
	env := map[string]string{}
	env["POD_LABEL_SELECTOR_KEY"] = c.podLabelSelectorKey
	env["POD_LABEL_SELECTOR_VALUE"] = c.podLabelSelectorValue
	env["TLS_CERT_FILE"] = c.certFile
	env["TLS_KEY_FILE"] = c.keyFile
	env["RESCHEDULE_ANNOTATION_KEY"] = c.rescheduleAnnotationKey
	env["RESCHEDULE_ANNOTATION_VALUE"] = c.rescheduleAnnotationValue
	env["TRACK_RESCHEULED_PODS"] = strconv.FormatBool(c.trackRescheduledPods)
	env["TRACKING_RESOURCE_TYPE"] = c.trackingResource.GetResourceType()
	env["ENABLE_NODE_CONDITION_CHECK"] = strconv.FormatBool(c.enableNodeConditionCheck)
	env["NODE_FAILURE_CONDITION_TYPES"] = strings.Join(c.nodeFailureConditionTypes, ",")
	env["NODE_FAILURE_TAINT_KEYS"] = strings.Join(c.nodeFailureTaintKeys, ",")
	return env
}

func (c *Config) Print() {
	slog.Info("Config loaded",
		"rescheduleAnnotationKey", c.rescheduleAnnotationKey,
		"rescheduleAnnotationValue", c.rescheduleAnnotationValue,
		"podLabelSelectorKey", c.podLabelSelectorKey,
		"podLabelSelectorValue", c.podLabelSelectorValue,
		"trackRescheduledPods", c.trackRescheduledPods,
		"trackingResource", c.trackingResource.GetResourceType(),
		"enableNodeConditionCheck", c.enableNodeConditionCheck,
		"nodeFailureConditionTypes", c.nodeFailureConditionTypes,
		"nodeFailureTaintKeys", c.nodeFailureTaintKeys)
}

// ConfigBuilder helps construct a Config with validation
type ConfigBuilder struct {
	config Config
}

// NewConfigBuilder creates a new ConfigBuilder with default values
func NewConfigBuilder() *ConfigBuilder {
	return &ConfigBuilder{
		config: Config{
			rescheduleAnnotationKey:   DefaultRescheduleAnnotationKey,
			rescheduleAnnotationValue: DefaultRescheduleAnnotationValue,
			podLabelSelectorKey:       DefaultPodLabelSelectorKey,
			podLabelSelectorValue:     DefaultPodLabelSelectorValue,
			certFile:                  DefaultCertFile,
			keyFile:                   DefaultKeyFile,
			trackRescheduledPods:      true,
			trackingResource:          tracking.GetTrackingResource(DefaultTrackingResourceType),
			enableNodeConditionCheck:  false,
			nodeFailureConditionTypes: []string{},
			nodeFailureTaintKeys:      []string{},
		},
	}
}

// FromEnvironment loads configuration from environment variables
func (b *ConfigBuilder) FromEnvironment() *ConfigBuilder {
	if val := os.Getenv("POD_LABEL_SELECTOR_KEY"); val != "" {
		b.config.podLabelSelectorKey = val
	}
	if val := os.Getenv("POD_LABEL_SELECTOR_VALUE"); val != "" {
		b.config.podLabelSelectorValue = val
	}
	if val := os.Getenv("TLS_CERT_FILE"); val != "" {
		b.config.certFile = val
	}
	if val := os.Getenv("TLS_KEY_FILE"); val != "" {
		b.config.keyFile = val
	}
	if val := os.Getenv("RESCHEDULE_ANNOTATION_KEY"); val != "" {
		b.config.rescheduleAnnotationKey = val
	}
	if val := os.Getenv("RESCHEDULE_ANNOTATION_VALUE"); val != "" {
		b.config.rescheduleAnnotationValue = val
	}
	if val := os.Getenv("TRACK_RESCHEULED_PODS"); val != "" {
		b.config.trackRescheduledPods, _ = strconv.ParseBool(val)
	}
	if val := os.Getenv("TRACKING_RESOURCE_TYPE"); val != "" {
		b.config.trackingResource = tracking.GetTrackingResource(val)
	}
	if val := os.Getenv("ENABLE_NODE_CONDITION_CHECK"); val != "" {
		b.config.enableNodeConditionCheck, _ = strconv.ParseBool(val)
	}
	if val := os.Getenv("NODE_FAILURE_CONDITION_TYPES"); val != "" {
		b.config.nodeFailureConditionTypes = splitAndTrim(val)
	}
	if val := os.Getenv("NODE_FAILURE_TAINT_KEYS"); val != "" {
		b.config.nodeFailureTaintKeys = splitAndTrim(val)
	}
	return b
}

func splitAndTrim(s string) []string {
	var result []string
	for _, item := range strings.Split(s, ",") {
		trimmed := strings.TrimSpace(item)
		if trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}

func (b *ConfigBuilder) WithPodLabelSelector(key, value string) *ConfigBuilder {
	b.config.podLabelSelectorKey = key
	b.config.podLabelSelectorValue = value
	return b
}

func (b *ConfigBuilder) WithRescheduleAnnotation(key, value string) *ConfigBuilder {
	b.config.rescheduleAnnotationKey = key
	b.config.rescheduleAnnotationValue = value
	return b
}

func (b *ConfigBuilder) WithTrackRescheduledPods(track bool) *ConfigBuilder {
	b.config.trackRescheduledPods = track
	return b
}

func (b *ConfigBuilder) WithTrackingResource(resourceType string) *ConfigBuilder {
	b.config.trackingResource = tracking.GetTrackingResource(resourceType)
	return b
}

func (b *ConfigBuilder) WithEnableNodeConditionCheck(enabled bool) *ConfigBuilder {
	b.config.enableNodeConditionCheck = enabled
	return b
}

func (b *ConfigBuilder) WithNodeFailureConditionTypes(types []string) *ConfigBuilder {
	b.config.nodeFailureConditionTypes = types
	return b
}

func (b *ConfigBuilder) WithNodeFailureTaintKeys(keys []string) *ConfigBuilder {
	b.config.nodeFailureTaintKeys = keys
	return b
}

func (b *ConfigBuilder) Build() *Config {
	return &b.config
}
