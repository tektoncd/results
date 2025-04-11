package config

import (
	"testing"

	"k8s.io/apimachinery/pkg/runtime"
)

// TestExtensionBasic tests the basic functionality of Extension
func TestExtensionBasic(t *testing.T) {
	tests := []struct {
		name                string
		ext                 *Extension
		wantAPIPath         string
		wantHost            string
		wantToken           string
		wantTimeout         string
		wantInsecureSkipTLS string
		wantAPIVersion      string
		wantKind            string
	}{
		{
			name: "valid extension",
			ext: &Extension{
				TypeMeta: runtime.TypeMeta{
					APIVersion: "results.tekton.dev/v1alpha2",
					Kind:       "Client",
				},
				APIPath:               "/api/v1",
				Host:                  "https://example.com",
				Token:                 "test-token",
				Timeout:               "30s",
				InsecureSkipTLSVerify: "true",
			},
			wantAPIPath:         "/api/v1",
			wantHost:            "https://example.com",
			wantToken:           "test-token",
			wantTimeout:         "30s",
			wantInsecureSkipTLS: "true",
			wantAPIVersion:      "results.tekton.dev/v1alpha2",
			wantKind:            "Client",
		},
		{
			name:                "empty extension",
			ext:                 &Extension{},
			wantAPIPath:         "",
			wantHost:            "",
			wantToken:           "",
			wantTimeout:         "",
			wantInsecureSkipTLS: "",
			wantAPIVersion:      "",
			wantKind:            "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Verify fields are set correctly
			if tt.ext.APIPath != tt.wantAPIPath {
				t.Errorf("Expected APIPath to be %q, got %q", tt.wantAPIPath, tt.ext.APIPath)
			}
			if tt.ext.Host != tt.wantHost {
				t.Errorf("Expected Host to be %q, got %q", tt.wantHost, tt.ext.Host)
			}
			if tt.ext.Token != tt.wantToken {
				t.Errorf("Expected Token to be %q, got %q", tt.wantToken, tt.ext.Token)
			}
			if tt.ext.Timeout != tt.wantTimeout {
				t.Errorf("Expected Timeout to be %q, got %q", tt.wantTimeout, tt.ext.Timeout)
			}
			if tt.ext.InsecureSkipTLSVerify != tt.wantInsecureSkipTLS {
				t.Errorf("Expected InsecureSkipTLSVerify to be %q, got %q", tt.wantInsecureSkipTLS, tt.ext.InsecureSkipTLSVerify)
			}
			if tt.ext.APIVersion != tt.wantAPIVersion {
				t.Errorf("Expected APIVersion to be %q, got %q", tt.wantAPIVersion, tt.ext.APIVersion)
			}
			if tt.ext.Kind != tt.wantKind {
				t.Errorf("Expected Kind to be %q, got %q", tt.wantKind, tt.ext.Kind)
			}
		})
	}
}

// TestExtensionDeepCopy tests the deep copy functionality
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
