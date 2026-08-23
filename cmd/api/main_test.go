package main

import (
	"bytes"
	"context"
	"io"
	"strings"
	"testing"

	"github.com/golang-jwt/jwt/v4"
	"github.com/grpc-ecosystem/go-grpc-middleware/logging/zap/ctxzap"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"google.golang.org/grpc/metadata"
)

func Test_determineAuth(t *testing.T) {
	validToken, _ := jwt.NewWithClaims(jwt.SigningMethodHS256, &jwt.RegisteredClaims{
		Subject: "sayan",
		Issuer:  "redhat",
	}).SignedString([]byte("secret"))

	invalidToken := "invalid-token-format"

	tests := []struct {
		name string
		md   metadata.MD
		want string
	}{
		{
			name: "missing token",
			md:   metadata.MD{},
			want: "\"grpc.user\":\"unknown\"",
		},
		{
			name: "invalid token",
			md:   metadata.Pairs("authorization", "Bearer "+invalidToken),
			want: "\"grpc.user\":\"unknown\"",
		},
		{
			name: "valid token",
			md:   metadata.Pairs("authorization", "Bearer "+validToken),
			want: "\"grpc.user\":\"sayan\",\"grpc.issuer\":\"redhat\"",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var b bytes.Buffer
			ctx := contextWithLogger(&b)
			_, err := determineAuth(metadata.NewIncomingContext(ctx, tc.md))
			l := ctxzap.Extract(ctx)
			l.Info(tc.name)
			if err != nil {
				t.Fatalf("No error expected, but received error: %v", err)
			}
			if !strings.Contains(b.String(), tc.want) {
				t.Fatalf("Log doesn't contain the string: %s", tc.want)
			}
		})
	}
}

func contextWithLogger(w io.Writer) context.Context {
	encoder := zapcore.NewJSONEncoder(zap.NewDevelopmentConfig().EncoderConfig)
	writer := zapcore.AddSync(w)
	logger := zap.New(zapcore.NewCore(encoder, writer, zapcore.DebugLevel))
	return ctxzap.ToContext(context.Background(), logger)
}

func Test_profilingServerAddress(t *testing.T) {
	tests := []struct {
		name string
		port string
		want string
	}{
		{
			name: "default port",
			port: "6060",
			want: "127.0.0.1:6060",
		},
		{
			name: "custom port",
			port: "9999",
			want: "127.0.0.1:9999",
		},
		{
			name: "empty port",
			port: "",
			want: "127.0.0.1:",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := profilingServerAddress(tc.port)
			if got != tc.want {
				t.Errorf("profilingServerAddress(%q) = %q, want %q", tc.port, got, tc.want)
			}
			// Security check: verify the address is bound to loopback only
			if !strings.HasPrefix(got, "127.0.0.1:") {
				t.Errorf("profilingServerAddress(%q) = %q, must start with '127.0.0.1:' for security", tc.port, got)
			}
		})
	}
}
