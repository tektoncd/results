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
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"log"
	"net"
	"net/http"
	"path"

	"go.uber.org/zap"

	prometheus "github.com/grpc-ecosystem/go-grpc-prometheus"
	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/spf13/viper"
	v1alpha2 "github.com/tektoncd/results/pkg/api/server/v1alpha2"
	"github.com/tektoncd/results/pkg/api/server/v1alpha2/auth"
	v1alpha2pb "github.com/tektoncd/results/proto/v1alpha2/results_go_proto"
	_ "go.uber.org/automaxprocs"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/reflection"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
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
	LOGS_API              bool   `mapstructure:"LOGS_API"`
}

func main() {

	viper.AddConfigPath("./config/env")
	viper.AddConfigPath("/etc/config/server")
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
		log.Fatalf("Failed to open the results.db: %v", err)
	}

	// Create k8s client
	config, err := rest.InClusterConfig()
	if err != nil {
		log.Fatal("Error getting kubernetes client config:", err)
	}
	k8s, err := kubernetes.NewForConfig(config)
	if err != nil {
		log.Fatal("Error creating kubernetes clientset:", err)
	}

	// Load TLS cert for server
	creds, tlsError := credentials.NewServerTLSFromFile(path.Join(configFile.TLS_PATH, "tls.crt"), path.Join(configFile.TLS_PATH, "tls.key"))
	if tlsError != nil {
		log.Infof("Error loading TLS key pair for server: %v", tlsError)
		log.Info("Creating server without TLS")
		creds = insecure.NewCredentials()
	}

	// Register API server(s)
	v1a2, err := v1alpha2.New(db, v1alpha2.WithAuth(auth.NewRBAC(k8s)))
	if err != nil {
		log.Fatalf("Failed to create server: %v", err)
	}
	s := grpc.NewServer(
		grpc.Creds(creds),
		grpc.StreamInterceptor(prometheus.StreamServerInterceptor),
		grpc.UnaryInterceptor(prometheus.UnaryServerInterceptor),
	)
	v1alpha2pb.RegisterResultsServer(s, v1a2)
	if configFile.LOGS_API {
		v1alpha2pb.RegisterLogsServer(s, v1a2)
	}

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
		log.Infof("Prometheus server listening on: %s", configFile.PROMETHEUS_PORT)
		if err := http.ListenAndServe(":"+configFile.PROMETHEUS_PORT, promhttp.Handler()); err != nil {
			log.Fatalf("Error running Prometheus HTTP handler: %v", err)
		}
	}()

	// Start gRPC server
	lis, err := net.Listen("tcp", ":"+configFile.GRPC_PORT)
	if err != nil {
		log.Fatalf("Failed to listen: %v", err)
	}
	go func() {
		log.Infof("gRPC server listening on: %s", configFile.GRPC_PORT)
		log.Fatal(s.Serve(lis))
	}()

	// Load REST client TLS cert to connect to the gRPC server
	if tlsError == nil {
		creds, err = credentials.NewClientTLSFromFile(path.Join(configFile.TLS_PATH, "tls.crt"), configFile.TLS_HOSTNAME_OVERRIDE)
		if err != nil {
			log.Fatalf("Error loading TLS certificate for REST: %v", err)
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
		log.Fatal("Error registering gRPC server endpoint: ", err)
	}

	// Start REST proxy server
	log.Infof("REST server Listening on: %s", configFile.REST_PORT)
	if tlsError != nil {
		log.Fatal(http.ListenAndServe(":"+configFile.REST_PORT, mux))
	} else {
		log.Fatal(http.ListenAndServeTLS(":"+configFile.REST_PORT, path.Join(configFile.TLS_PATH, "tls.crt"), path.Join(configFile.TLS_PATH, "tls.key"), mux))
	}

}

func getLogger(config ConfigFile) (*zap.SugaredLogger, zap.Config) {
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

	return zapLog.Sugar(), zapConf
}
