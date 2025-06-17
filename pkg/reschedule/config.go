package reschedule

import (
	"os"
	"strconv"

	"github.com/couchbase/couchbase-reschedule-hook/pkg/reschedule/tracking"
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
	return env
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
	return b
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

func (b *ConfigBuilder) Build() *Config {
	return &b.config
}
