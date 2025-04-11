package config

import (
	"testing"

	"k8s.io/client-go/rest"
)

// TestGetRoutes tests the getRoutes function
func TestGetRoutes(t *testing.T) {
	tests := []struct {
		name       string
		config     *rest.Config
		wantErr    bool
		wantRoutes bool
	}{
		{
			name: "no services found",
			config: &rest.Config{
				Host: "test-host",
			},
			wantErr:    true,
			wantRoutes: false,
		},
		{
			name: "invalid config",
			config: &rest.Config{
				Host: "",
			},
			wantErr:    true,
			wantRoutes: false,
		},
		{
			name: "invalid host",
			config: &rest.Config{
				Host: "http://invalid.host:8080",
			},
			wantErr:    true,
			wantRoutes: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			routes, err := getRoutes(tt.config)
			if tt.wantErr {
				if err == nil {
					t.Error("Expected error, got nil")
				}
			} else {
				if err != nil {
					t.Errorf("Expected no error, got %v", err)
				}
			}
			if tt.wantRoutes {
				if routes == nil {
					t.Error("Expected routes, got nil")
				}
			} else {
				if routes != nil {
					t.Error("Expected nil routes, got non-nil")
				}
			}
		})
	}
}

// TestGetRoutesWithNilConfig tests getRoutes with nil config
func TestGetRoutesWithNilConfig(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("Expected panic with nil config")
		}
	}()

	_, _ = getRoutes(nil)
}
