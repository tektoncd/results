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
	"fmt"
	"go.uber.org/zap/zapcore"
	"log"
	"net"
	"net/http"
	"path"
	"time"

	"github.com/golang-jwt/jwt/v4"
	grpc_middleware "github.com/grpc-ecosystem/go-grpc-middleware"
	grpc_auth "github.com/grpc-ecosystem/go-grpc-middleware/auth"
	grpc_zap "github.com/grpc-ecosystem/go-grpc-middleware/logging/zap"
	"github.com/grpc-ecosystem/go-grpc-middleware/logging/zap/ctxzap"
	grpc_ctxtags "github.com/grpc-ecosystem/go-grpc-middleware/tags"
	prometheus "github.com/grpc-ecosystem/go-grpc-prometheus"
	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/spf13/viper"
	v1alpha2 "github.com/tektoncd/results/pkg/api/server/v1alpha2"
	"github.com/tektoncd/results/pkg/api/server/v1alpha2/auth"
	v1alpha2pb "github.com/tektoncd/results/proto/v1alpha2/results_go_proto"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/reflection"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

type ConfigFile struct {
	DB_USER               string `mapstructure:"DB_USER"`
	DB_PASSWORD           string `mapstructure:"DB_PASSWORD"`
	DB_HOST               string `mapstructure:"DB_HOST"`
	DB_PORT               string `mapstructure:"DB_PORT"`
	DB_NAME               string `mapstructure:"DB_NAME"`
	DB_SSLMODE            string `mapstructure:"DB_SSLMODE"`
	GRPC_PORT             string `mapstructure:"GRPC_PORT"`
	REST_PORT             string `mapstructure:"REST_PORT"`
	PROMETHEUS_PORT       string `mapstructure:"PROMETHEUS_PORT"`
	LOG_LEVEL             string `mapstructure:"LOG_LEVEL"`
	TLS_HOSTNAME_OVERRIDE string `mapstructure:"TLS_HOSTNAME_OVERRIDE"`
	TLS_PATH              string `mapstructure:"TLS_PATH"`
}

func main() {
	viper.AddConfigPath("./env")
	viper.AddConfigPath("/env")
	viper.SetConfigName("config")
	viper.SetConfigType("env")

	viper.AutomaticEnv()

	err := viper.ReadInConfig()
	if err != nil {
		log.Fatalf("Error reading config: %v", err)
	}

	configFile := ConfigFile{}
	err = viper.Unmarshal(&configFile)

	if err != nil {
		log.Fatal("Cannot load config:", err)
	}

	log, logConf := getLogger(configFile)
	defer log.Sync()

	if configFile.DB_USER == "" || configFile.DB_PASSWORD == "" {
		log.Fatal("Must provide both DB_USER and DB_PASSWORD")
	}
	// Connect to the database.
	// DSN derived from https://pkg.go.dev/gorm.io/driver/postgres

	dbURI := fmt.Sprintf("host=%s user=%s password=%s dbname=%s port=%s sslmode=%s", configFile.DB_HOST, configFile.DB_USER, configFile.DB_PASSWORD, configFile.DB_NAME, configFile.DB_PORT, configFile.DB_SSLMODE)

	gormConf := &gorm.Config{}
	if logConf.Level.Level() != zap.DebugLevel {
		gormConf.Logger = logger.Default.LogMode(logger.Silent)
	}
	db, err := gorm.Open(postgres.Open(dbURI), gormConf)
	if err != nil {
		log.Fatal("Failed to open the results.db",
			zap.String("Error: ", err.Error()))
	}

	// Create k8s client
	config, err := rest.InClusterConfig()
	if err != nil {
		log.Fatal("Error getting kubernetes client config",
			zap.String("Error: ", err.Error()))
	}
	k8s, err := kubernetes.NewForConfig(config)
	if err != nil {
		log.Fatal("Error creating kubernetes clientset",
			zap.String("Error: ", err.Error()))
	}

	// Load TLS cert for server
	creds, tlsError := credentials.NewServerTLSFromFile(path.Join(configFile.TLS_PATH, "tls.crt"), path.Join(configFile.TLS_PATH, "tls.key"))
	if tlsError != nil {
		log.Info("Error loading TLS key pair for server",
			zap.String("Error: ", tlsError.Error()))
		log.Info("Creating server without TLS")
		creds = insecure.NewCredentials()
	}

	// Register API server(s)
	v1a2, err := v1alpha2.New(db, v1alpha2.WithAuth(auth.NewRBAC(k8s)))
	if err != nil {
		log.Fatal("Failed to create server",
			zap.String("Error: ", err.Error()))
	}

	// Shared options for the logger, with a custom gRPC code to log level function.
	zapOpts := []grpc_zap.Option{
		grpc_zap.WithDurationField(func(duration time.Duration) zapcore.Field {
			return zap.Int64("grpc.time_duration_in_ns", duration.Nanoseconds())
		}),
	}

	// Make sure that log statements internal to gRPC library are logged using the zapLogger as well.
	//grpc_zap.ReplaceGrpcLoggerV2WithVerbosity(zapLogger, 1)
	s := grpc.NewServer(
		grpc.Creds(creds),
		grpc_middleware.WithUnaryServerChain(
			grpc_ctxtags.UnaryServerInterceptor(grpc_ctxtags.WithFieldExtractor(grpc_ctxtags.CodeGenRequestFieldExtractor)),
			grpc_zap.UnaryServerInterceptor(log, zapOpts...),
			grpc_auth.UnaryServerInterceptor(determineAuth),
			prometheus.UnaryServerInterceptor,
		),
		grpc_middleware.WithStreamServerChain(
			grpc_ctxtags.StreamServerInterceptor(grpc_ctxtags.WithFieldExtractor(grpc_ctxtags.CodeGenRequestFieldExtractor)),
			grpc_zap.StreamServerInterceptor(log, zapOpts...),
			grpc_auth.StreamServerInterceptor(determineAuth),
			prometheus.StreamServerInterceptor,
		),
	)
	v1alpha2pb.RegisterResultsServer(s, v1a2)

	// Allow service reflection - required for grpc_cli ls to work.
	reflection.Register(s)

	// Set up health checks.
	hs := health.NewServer()
	hs.SetServingStatus("tekton.results.v1alpha2.Results", healthpb.HealthCheckResponse_SERVING)
	healthpb.RegisterHealthServer(s, hs)

	// Start prometheus metrics server
	prometheus.Register(s)
	http.Handle("/metrics", promhttp.Handler())
	go func() {
		log.Info("Prometheus server listening",
			zap.String("Port : ", configFile.PROMETHEUS_PORT))
		if err := http.ListenAndServe(":"+configFile.PROMETHEUS_PORT, promhttp.Handler()); err != nil {
			log.Fatal("Error running Prometheus HTTP handler: %v",
				zap.String("Error: ", err.Error()))
		}
	}()

	// Start gRPC server
	lis, err := net.Listen("tcp", ":"+configFile.GRPC_PORT)
	if err != nil {
		log.Fatal("Failed to listen",
			zap.String("Error: ", err.Error()))
	}
	go func() {
		log.Info("gRPC server listening",
			zap.String("Port: ", configFile.GRPC_PORT))
		log.Fatal("",
			zap.String("Error: ", s.Serve(lis).Error()))
	}()

	// Load REST client TLS cert to connect to the gRPC server
	if tlsError == nil {
		creds, err = credentials.NewClientTLSFromFile(path.Join(configFile.TLS_PATH, "tls.crt"), configFile.TLS_HOSTNAME_OVERRIDE)
		if err != nil {
			log.Fatal("Error loading TLS certificate for REST",
				zap.String("Error: ", err.Error()))
		}
	}

	opts := []grpc.DialOption{grpc.WithTransportCredentials(creds)}

	// Register gRPC server endpoint for gRPC gateway
	ctx := context.Background()
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	mux := runtime.NewServeMux()
	err = v1alpha2pb.RegisterResultsHandlerFromEndpoint(ctx, mux, ":"+configFile.GRPC_PORT, opts)
	if err != nil {
		log.Fatal("Error registering gRPC server endpoint: ",
			zap.String("Error: ", err.Error()))
	}

	// Start REST proxy server
	log.Info("REST server Listening",
		zap.String("Port:", configFile.REST_PORT))

	if tlsError != nil {
		log.Fatal("",
			zap.String("Error: ", http.ListenAndServe(":"+configFile.REST_PORT, mux).Error()))
	} else {
		log.Fatal("",
			zap.String("Error: ", http.ListenAndServeTLS(":"+configFile.REST_PORT, path.Join(configFile.TLS_PATH, "tls.crt"), path.Join(configFile.TLS_PATH, "tls.key"), mux).Error()))
	}
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

func getLogger(config ConfigFile) (*zap.Logger, zap.Config) {
	zapConf := zap.NewProductionConfig()
	if len(config.LOG_LEVEL) > 0 {
		var err error
		if zapConf.Level, err = zap.ParseAtomicLevel(config.LOG_LEVEL); err != nil {
			log.Fatalf("Failed to parse log level from config: %v", err)
		}
	}

	zapLog, err := zapConf.Build()
	if err != nil {
		log.Fatalf("Failed to initialize zap logger: %v", err)
	}

	return zapLog, zapConf
}
