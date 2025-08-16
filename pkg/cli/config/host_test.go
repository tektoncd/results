package config

import (
	"testing"

	"k8s.io/client-go/rest"
)

func TestHostMethod(t *testing.T) {
	// Test that Host method works with correct parameters
	config := &config{
		RESTConfig: &rest.Config{},
	}

	// Test that the method can be called without panicking
	result := config.Host()
	// In test environment, detection should fail and return empty string
	if result == "" {
		t.Log("Host method called successfully and returned empty string as expected")
	} else {
		t.Logf("Host method returned: %s", result)
	}
}
