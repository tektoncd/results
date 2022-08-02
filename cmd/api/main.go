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
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"

	// Go runtime is unaware of CPU quota which means it will set GOMAXPROCS
	// to underlying host vm node. This high value means that GO runtime
	// scheduler assumes that it has more threads and does context switching
	// when it might work with fewer threads.
	// This doesn't happen# with our other controllers and services because
	// sharedmain already import this package for them.
	_ "go.uber.org/automaxprocs"

	grpc_prometheus "github.com/grpc-ecosystem/go-grpc-prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/spf13/viper"
	v1alpha2 "github.com/tektoncd/results/pkg/api/server/v1alpha2"
	"github.com/tektoncd/results/pkg/api/server/v1alpha2/auth"
	v1alpha2pb "github.com/tektoncd/results/proto/v1alpha2/results_go_proto"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/reflection"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

type ConfigFile struct {
	DB_USER     string `mapstructure:"DB_USER"`
	DB_PASSWORD string `mapstructure:"DB_PASSWORD"`
	DB_PROTOCOL string `mapstructure:"DB_PROTOCOL"`
	DB_ADDR     string `mapstructure:"DB_ADDR"`
	DB_NAME     string `mapstructure:"DB_NAME"`
	DB_SSLMODE  string `mapstructure:"DB_SSLMODE"`
}

func main() {

	viper.AddConfigPath("./env")
	viper.SetConfigName("config")
	viper.SetConfigType("env")

	viper.AutomaticEnv()

	err := viper.ReadInConfig()
	if err != nil {
		return
	}

	configFile := ConfigFile{}
	err = viper.Unmarshal(&configFile)

	if err != nil {
		log.Fatal("cannot load config:", err)
	}

	flag.Parse()

	user, pass := configFile.DB_USER, configFile.DB_PASSWORD
	if user == "" || pass == "" {
		log.Fatal("Must provide both DB_USER and DB_PASSWORD")
	}
	// Connect to the database.
	// DSN derived from https://pkg.go.dev/gorm.io/driver/postgres

	sslmode := configFile.DB_SSLMODE
	if sslmode == "" {
		sslmode = "disable"
	}

	dbURI := fmt.Sprintf("host=%s user=%s password=%s dbname=%s port=5432 sslmode=%s", configFile.DB_ADDR, user, pass, configFile.DB_NAME, configFile.DB_SSLMODE)
	db, err := gorm.Open(postgres.Open(dbURI), &gorm.Config{})
	if err != nil {
		log.Fatalf("failed to open the results.db: %v", err)
	}

	// Create k8s client
	config, err := rest.InClusterConfig()
	if err != nil {
		log.Fatal("error getting kubernetes client config:", err)
	}
	k8s, err := kubernetes.NewForConfig(config)
	if err != nil {
		log.Fatal("error creating kubernetes clientset:", err)
	}

	// Load TLS cert
	creds, err := credentials.NewServerTLSFromFile("/etc/tls/tls.crt", "/etc/tls/tls.key")
	if err != nil {
		log.Fatalf("error loading TLS key pair: %v", err)
	}

	// Register API server(s)
	v1a2, err := v1alpha2.New(db, v1alpha2.WithAuth(auth.NewRBAC(k8s)))
	if err != nil {
		log.Fatalf("failed to create server: %v", err)
	}
	s := grpc.NewServer(
		grpc.Creds(creds),
		grpc.StreamInterceptor(grpc_prometheus.StreamServerInterceptor),
		grpc.UnaryInterceptor(grpc_prometheus.UnaryServerInterceptor),
	)
	v1alpha2pb.RegisterResultsServer(s, v1a2)

	// Allow service reflection - required for grpc_cli ls to work.
	reflection.Register(s)

	// Set up health checks.
	hs := health.NewServer()
	hs.SetServingStatus("tekton.results.v1alpha2.Results", healthpb.HealthCheckResponse_SERVING)
	healthpb.RegisterHealthServer(s, hs)

	// Prometheus metrics
	grpc_prometheus.Register(s)
	http.Handle("/metrics", promhttp.Handler())
	go func() {
		if err := http.ListenAndServe(":8080", promhttp.Handler()); err != nil {
			log.Fatalf("error running Prometheus HTTP handler: %v", err)
		}
	}()

	// Listen on port and serve.
	port := os.Getenv("PORT")
	if port == "" {
		// Default gRPC server port to this value from tutorials (e.g., https://grpc.io/docs/guides/auth/#go)
		port = "50051"
	}
	lis, err := net.Listen("tcp", ":"+port)
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}
	log.Printf("Listening on :%s...", port)
	log.Fatal(s.Serve(lis))
}
