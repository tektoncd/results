package config

import (
	"testing"

	"k8s.io/apimachinery/pkg/runtime"
)

// TestExtensionDeepCopy tests the deep copy functionality (kept from original)
func TestExtensionDeepCopy(t *testing.T) {
	// Create an original Extension
	original := &Extension{
		TypeMeta: runtime.TypeMeta{
			APIVersion: "results.tekton.dev/v1alpha2",
			Kind:       "Client",
		},
		APIPath:               "/api/v1",
		Host:                  "https://example.com",
		Token:                 "test-token",
		Timeout:               "30s",
		InsecureSkipTLSVerify: "true",
	}

	// Test DeepCopy
	deepCopy := original.DeepCopy()
	if deepCopy == original {
		t.Error("DeepCopy returned the same object, expected a new object")
	}
	if deepCopy.APIPath != original.APIPath {
		t.Errorf("Expected APIPath to be '%s', got '%s'", original.APIPath, deepCopy.APIPath)
	}

	// Test DeepCopyObject
	copyObj := original.DeepCopyObject()
	copy2, ok := copyObj.(*Extension)
	if !ok {
		t.Fatal("DeepCopyObject did not return an Extension")
	}
	if copy2.APIPath != original.APIPath {
		t.Errorf("Expected APIPath to be '%s', got '%s'", original.APIPath, copy2.APIPath)
	}

	// Test DeepCopyInto
	target := &Extension{}
	original.DeepCopyInto(target)
	if target.APIPath != original.APIPath {
		t.Errorf("Expected APIPath to be '%s', got '%s'", original.APIPath, target.APIPath)
	}

	// Verify independence
	deepCopy.APIPath = "/api/v2"
	if original.APIPath == deepCopy.APIPath {
		t.Error("Modifying the copy affected the original")
	}
}

// TestExtensionEmpty tests the behavior of Extension with empty fields
func TestExtensionEmpty(t *testing.T) {
	// Create an empty Extension
	ext := &Extension{}

	// Verify fields are empty
	if ext.APIPath != "" {
		t.Errorf("Expected empty APIPath, got '%s'", ext.APIPath)
	}
	if ext.Host != "" {
		t.Errorf("Expected empty Host, got '%s'", ext.Host)
	}
	if ext.Token != "" {
		t.Errorf("Expected empty Token, got '%s'", ext.Token)
	}
	if ext.Timeout != "" {
		t.Errorf("Expected empty Timeout, got '%s'", ext.Timeout)
	}
	if ext.InsecureSkipTLSVerify != "" {
		t.Errorf("Expected empty InsecureSkipTLSVerify, got '%s'", ext.InsecureSkipTLSVerify)
	}

	// Test DeepCopy with empty Extension
	deepCopy := ext.DeepCopy()
	if deepCopy == ext {
		t.Error("DeepCopy returned the same object, expected a new object")
	}
	if deepCopy.APIPath != ext.APIPath {
		t.Errorf("Expected APIPath to be '%s', got '%s'", ext.APIPath, deepCopy.APIPath)
	}

	// Test DeepCopyObject with empty Extension
	copyObj := ext.DeepCopyObject()
	copy2, ok := copyObj.(*Extension)
	if !ok {
		t.Fatal("DeepCopyObject did not return an Extension")
	}
	if copy2.APIPath != ext.APIPath {
		t.Errorf("Expected APIPath to be '%s', got '%s'", ext.APIPath, copy2.APIPath)
	}

	// Test DeepCopyInto with empty Extension
	target := &Extension{}
	ext.DeepCopyInto(target)
	if target.APIPath != ext.APIPath {
		t.Errorf("Expected APIPath to be '%s', got '%s'", ext.APIPath, target.APIPath)
	}
}
