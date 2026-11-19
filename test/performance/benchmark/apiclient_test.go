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
	"errors"
	"testing"

	"github.com/tektoncd/results/test/e2e/client"
)

type closingGRPCClient struct {
	client.GRPCClient
	closed   bool
	closeErr error
}

func (c *closingGRPCClient) Close() error {
	c.closed = true
	return c.closeErr
}

type closingRESTClient struct {
	client.RESTClient
	closed bool
}

func (c *closingRESTClient) CloseIdleConnections() {
	c.closed = true
}

func TestAPIClientsClose(t *testing.T) {
	tests := []struct {
		name     string
		closeErr error
	}{
		{name: "success"},
		{name: "gRPC close failure", closeErr: errors.New("close failed")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gc := &closingGRPCClient{closeErr: tt.closeErr}
			rc := &closingRESTClient{}
			clients := &APIClients{GRPC: gc, REST: rc}
			if err := clients.Close(); !errors.Is(err, tt.closeErr) {
				t.Errorf("Close() = %v, want %v", err, tt.closeErr)
			}
			if !gc.closed || !rc.closed {
				t.Errorf("cleanup: gRPC closed = %t, REST closed = %t, want both closed", gc.closed, rc.closed)
			}
		})
	}
}
