/*
Copyright 2020 The Tekton Authors

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
	"crypto/tls"
	"fmt"
	"net/http"
	"path"
	"strings"
	"time"

	"github.com/tektoncd/results/pkg/api/server/v1alpha2/auth/impersonation"
	"golang.org/x/net/http2"
	"golang.org/x/net/http2/h2c"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"

	"github.com/golang-jwt/jwt/v4"
	grpc_middleware "github.com/grpc-ecosystem/go-grpc-middleware"
	grpc_auth "github.com/grpc-ecosystem/go-grpc-middleware/auth"
	grpc_zap "github.com/grpc-ecosystem/go-grpc-middleware/logging/zap"
	"github.com/grpc-ecosystem/go-grpc-middleware/logging/zap/ctxzap"
	grpc_ctxtags "github.com/grpc-ecosystem/go-grpc-middleware/tags"
	"github.com/grpc-ecosystem/go-grpc-middleware/v2/interceptors/recovery"
	prometheus "github.com/grpc-ecosystem/go-grpc-prometheus"
	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/tektoncd/results/pkg/api/server/config"
	"github.com/tektoncd/results/pkg/api/server/logger"
	v1alpha2 "github.com/tektoncd/results/pkg/api/server/v1alpha2"
	"github.com/tektoncd/results/pkg/api/server/v1alpha2/auth"
	v1alpha2pb "github.com/tektoncd/results/proto/v1alpha2/results_go_proto"
	_ "go.uber.org/automaxprocs"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/reflection"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
	"k8s.io/apimachinery/pkg/util/wait"
)

func main() {
	serverConfig := config.Get()

	log := logger.Get(serverConfig.LOG_LEVEL)
	// This defer statement will be executed at the end of the application lifecycle, so we do not lose
	// any data in the event of an unhandled error.
	defer log.Sync() //nolint:errcheck

	// Load server TLS
	certFile := path.Join(serverConfig.TLS_PATH, "tls.crt")
	keyFile := path.Join(serverConfig.TLS_PATH, "tls.key")
	creds, tlsError := credentials.NewServerTLSFromFile(certFile, keyFile)
	if tlsError != nil {
		log.Errorf("Error loading server TLS: %v", tlsError)
		log.Warn("TLS will be disabled")
		creds = insecure.NewCredentials()
	}

	if serverConfig.DB_USER == "" || serverConfig.DB_PASSWORD == "" {
		log.Fatal("Must provide both DB_USER and DB_PASSWORD")
	}

	// From all available sslmodes, "require", "verify-ca" and "verify-full" require CA cert
	// configured on the client side. We check and fail early if one is not provided.
	if (serverConfig.DB_SSLMODE == "require" || serverConfig.DB_SSLMODE == "verify-ca" || serverConfig.DB_SSLMODE == "verify-full") && serverConfig.DB_SSLROOTCERT == "" {
		log.Fatalf("DB_SSLROOTCERT can't be empty when DB_SSLMODE=%s", serverConfig.DB_SSLMODE)
	}

	// Connect to the database.
	// DSN derived from https://pkg.go.dev/gorm.io/driver/postgres

	var db *gorm.DB
	var err error

	dbURI := fmt.Sprintf("host=%s user=%s password=%s dbname=%s port=%s sslmode=%s sslrootcert=%s", serverConfig.DB_HOST, serverConfig.DB_USER, serverConfig.DB_PASSWORD, serverConfig.DB_NAME, serverConfig.DB_PORT, serverConfig.DB_SSLMODE, serverConfig.DB_SSLROOTCERT)
	gormConfig := &gorm.Config{}
	if log.Level() != zap.DebugLevel {
		gormConfig.Logger = gormlogger.Default.LogMode(gormlogger.Silent)
	}
	// Retry database connection, sometimes the database is not ready to accept connection
	err = wait.PollImmediate(10*time.Second, 2*time.Minute, func() (bool, error) {
		db, err = gorm.Open(postgres.Open(dbURI), gormConfig)
		if err != nil {
			log.Warnf("Error connecting to database (retrying in 10s): %v", err)
			return false, nil
		}
		return true, nil
	})
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}

	// Create the authorization authCheck
	var authCheck auth.Checker
	var serverMuxOptions []runtime.ServeMuxOption
	if serverConfig.AUTH_DISABLE {
		log.Warn("Kubernetes RBAC authorization check disabled - all requests will be allowed by the API server")
		authCheck = &auth.AllowAll{}
	} else {
		log.Info("Kubernetes RBAC authorization check enabled")
		// Create k8s client
		k8sConfig, err := rest.InClusterConfig()
		if err != nil {
			log.Fatal("Error getting kubernetes client config:", err)
		}
		k8s, err := kubernetes.NewForConfig(k8sConfig)
		if err != nil {
			log.Fatal("Error creating kubernetes clientset:", err)
		}

		if serverConfig.AUTH_IMPERSONATE {
			log.Info("Kubernetes RBAC impersonation enabled")
			serverMuxOptions = append(serverMuxOptions, runtime.WithIncomingHeaderMatcher(impersonation.HeaderMatcher))
		}
		authCheck = auth.NewRBAC(k8s, auth.WithImpersonation(serverConfig.AUTH_IMPERSONATE))
	}

	// Register API server(s)
	v1a2, err := v1alpha2.New(serverConfig, log, db, v1alpha2.WithAuth(authCheck))
	if err != nil {
		log.Fatalf("Failed to create server: %v", err)
	}

	// Shared options for the logger, with a custom gRPC code to log level function.
	zapOpts := []grpc_zap.Option{
		grpc_zap.WithDecider(func(fullMethodName string, err error) bool {
			return fullMethodName != healthpb.Health_Check_FullMethodName
		}),
		grpc_zap.WithDurationField(func(duration time.Duration) zapcore.Field {
			return zap.Int64("grpc.time_duration_in_ms", duration.Milliseconds())
		}),
	}

	// Customize logger, so it can be passed to the gRPC interceptors
	grpcLogger := log.Desugar().With(zap.Bool("grpc.auth_disabled", serverConfig.AUTH_DISABLE))

	gs := grpc.NewServer(
		grpc.Creds(creds),
		grpc_middleware.WithUnaryServerChain(
			// The grpc_ctxtags context updater should be before everything else
			grpc_ctxtags.UnaryServerInterceptor(grpc_ctxtags.WithFieldExtractor(grpc_ctxtags.CodeGenRequestFieldExtractor)),
			grpc_zap.UnaryServerInterceptor(grpcLogger, zapOpts...),
			grpc_auth.UnaryServerInterceptor(determineAuth),
			prometheus.UnaryServerInterceptor,
			recovery.UnaryServerInterceptor(recovery.WithRecoveryHandler(recoveryHandler)),
		),
		grpc_middleware.WithStreamServerChain(
			// The grpc_ctxtags context updater should be before everything else
			grpc_ctxtags.StreamServerInterceptor(grpc_ctxtags.WithFieldExtractor(grpc_ctxtags.CodeGenRequestFieldExtractor)),
			grpc_zap.StreamServerInterceptor(grpcLogger, zapOpts...),
			grpc_auth.StreamServerInterceptor(determineAuth),
			prometheus.StreamServerInterceptor,
			recovery.StreamServerInterceptor(recovery.WithRecoveryHandler(recoveryHandler)),
		),
	)
	v1alpha2pb.RegisterResultsServer(gs, v1a2)
	if serverConfig.LOGS_API {
		v1alpha2pb.RegisterLogsServer(gs, v1a2)
	}

	// Allow service reflection - required for grpc_cli ls to work.
	reflection.Register(gs)

	// Set up health checks.
	hs := health.NewServer()
	hs.SetServingStatus("tekton.results.v1alpha2.Results", healthpb.HealthCheckResponse_SERVING)
	if serverConfig.LOGS_API {
		hs.SetServingStatus("tekton.results.v1alpha2.Logs", healthpb.HealthCheckResponse_SERVING)
	}
	healthpb.RegisterHealthServer(gs, hs)

	// Start prometheus metrics server
	if serverConfig.PROMETHEUS_HISTOGRAM {
		prometheus.EnableHandlingTimeHistogram()
	}
	prometheus.Register(gs)
	http.Handle("/metrics", promhttp.Handler())
	go func() {
		log.Infof("Prometheus server listening on: %s", serverConfig.PROMETHEUS_PORT)
		if err := http.ListenAndServe(":"+serverConfig.PROMETHEUS_PORT, promhttp.Handler()); err != nil {
			log.Fatalf("Error running Prometheus HTTP handler: %v", err)
		}
	}()

	// Load client TLS to dial gRPC
	if tlsError == nil {
		// This is an internal client to proxy request from the REST listener to gRPC listener.
		// So we don't need certificate verification here.
		creds = credentials.NewTLS(&tls.Config{InsecureSkipVerify: true})
	}

	// Setup gRPC gateway to proxy request to gRPC health checks
	clientConn, err := grpc.Dial(":"+serverConfig.SERVER_PORT, grpc.WithTransportCredentials(creds), grpc.WithNoProxy())
	if err != nil {
		log.Fatalf("Error dialing gRPC endpoint: %v", err)
	}
	serverMuxOptions = append(serverMuxOptions, runtime.WithHealthzEndpoint(healthpb.NewHealthClient(clientConn)))

	// Create server for gRPC gateway
	ctx := context.Background()
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	httpMux := runtime.NewServeMux(serverMuxOptions...)
	opts := []grpc.DialOption{
		grpc.WithTransportCredentials(creds),
		grpc.WithDefaultCallOptions(grpc.MaxCallRecvMsgSize(100 * 1024 * 1024)),
		grpc.WithNoProxy(),
	}

	// Register gRPC server endpoint to gRPC gateway
	err = v1alpha2pb.RegisterResultsHandlerFromEndpoint(ctx, httpMux, ":"+serverConfig.SERVER_PORT, opts)
	if err != nil {
		log.Fatal("Error registering gRPC server endpoint for Results API: ", err)
	}

	if serverConfig.LOGS_API {
		err = v1alpha2pb.RegisterLogsHandlerFromEndpoint(ctx, httpMux, ":"+serverConfig.SERVER_PORT, opts)
		if err != nil {
			log.Fatal("Error registering gRPC server endpoints for Logs API: ", err)
		}
	}

	// Start server with gRPC and REST handler
	log.Infof("gRPC and REST server listening on: %s", serverConfig.SERVER_PORT)
	if tlsError != nil {
		log.Fatal(http.ListenAndServe(":"+serverConfig.SERVER_PORT, grpcHandler(gs, httpMux)))
	}
	log.Fatal(http.ListenAndServeTLS(":"+serverConfig.SERVER_PORT, certFile, keyFile, grpcHandler(gs, httpMux)))
}

// grpcHandler forwards the request to gRPC server based on the Content-Type header.
func grpcHandler(grpcServer *grpc.Server, httpHandler http.Handler) http.Handler {
	return h2c.NewHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.ProtoMajor == 2 && strings.HasPrefix(r.Header.Get("Content-Type"), "application/grpc") {
			grpcServer.ServeHTTP(w, r)
		} else {
			httpHandler.ServeHTTP(w, r)
		}
	}), &http2.Server{})
}

// recoveryHandler returns custom messages when server panics
func recoveryHandler(p any) error {
	return status.Errorf(codes.Unknown, "Error: %v", p)
}

func determineAuth(ctx context.Context) (context.Context, error) {
	// This code is used to extract values
	// it is not doing any form of verification.

	tokenString, err := grpc_auth.AuthFromMD(ctx, "bearer")
	if err != nil {
		ctxzap.AddFields(ctx,
			zap.String("grpc.user", "unknown"),
		)
		return ctx, nil
	}

	token, _, err := new(jwt.Parser).ParseUnverified(tokenString, jwt.MapClaims{})
	if err != nil {
		ctxzap.AddFields(ctx,
			zap.String("grpc.user", "unknown"),
		)
		return ctx, nil
	}

	if claims, ok := token.Claims.(jwt.MapClaims); ok {
		sub := fmt.Sprint(claims["sub"])
		iss := fmt.Sprint(claims["iss"])
		ctxzap.AddFields(ctx,
			zap.String("grpc.user", sub),
			zap.String("grpc.issuer", iss),
		)
	}
	return ctx, nil
}
