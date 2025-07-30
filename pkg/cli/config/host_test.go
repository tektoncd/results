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
	result := config.Host("tekton-pipelines")
	if result == nil {
		t.Log("Host method called successfully")
	}
}
