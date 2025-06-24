package e2e

import (
	"log/slog"
	"os"
	"testing"

	"github.com/couchbaselabs/eviction-reschedule-hook/test/framework"
)

// TestMain is the entry point for the test suite that handles setup and cleanup of test manifests
func TestMain(m *testing.M) {
	tf, err := framework.SetupFramework()
	if err != nil {
		slog.Error("Failed to setup testing environment", "error", err)
		tf.TearDown()
		os.Exit(1)
	}

	// Run tests
	code := m.Run()

	tf.TearDown()

	os.Exit(code)
}
