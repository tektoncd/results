/*
Copyright 2026 The Tekton Authors

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package main

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"google.golang.org/grpc/balancer/roundrobin"
	"knative.dev/pkg/controller"
)

func TestResyncPeriodDefault(t *testing.T) {
	if *resyncPeriod != controller.DefaultResyncPeriod {
		t.Errorf("resync_period default = %v, want %v", *resyncPeriod, controller.DefaultResyncPeriod)
	}
	if *resyncPeriod != 10*time.Hour {
		t.Errorf("resync_period default = %v, want 10h", *resyncPeriod)
	}
}

func TestDefaultServiceConfig(t *testing.T) {
	var config struct {
		LoadBalancingConfig []map[string]interface{} `json:"loadBalancingConfig"`
	}

	if err := json.Unmarshal([]byte(defaultServiceConfig), &config); err != nil {
		t.Fatalf("defaultServiceConfig is not valid JSON: %v", err)
	}

	if len(config.LoadBalancingConfig) == 0 {
		t.Fatal("defaultServiceConfig must contain at least one load balancing policy")
	}

	foundRoundRobin := false
	for _, policy := range config.LoadBalancingConfig {
		if _, ok := policy[roundrobin.Name]; ok {
			foundRoundRobin = true
			break
		}
	}

	if !foundRoundRobin {
		t.Errorf("defaultServiceConfig must include the round_robin load balancing policy, got: %v", config.LoadBalancingConfig)
	}
}

func TestWithResyncPeriod(t *testing.T) {
	tests := []struct {
		name    string
		period  time.Duration
		wantErr bool
	}{
		{name: "default", period: controller.DefaultResyncPeriod},
		{name: "positive override", period: 30 * time.Minute},
		{name: "zero disables resync", period: 0},
		{name: "negative rejected", period: -time.Second, wantErr: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ctx, err := withResyncPeriod(context.Background(), tc.period)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("withResyncPeriod(%v) error = nil, want error", tc.period)
				}
				return
			}
			if err != nil {
				t.Fatalf("withResyncPeriod(%v) unexpected error: %v", tc.period, err)
			}
			if got := controller.GetResyncPeriod(ctx); got != tc.period {
				t.Errorf("GetResyncPeriod() = %v, want %v", got, tc.period)
			}
		})
	}
}
