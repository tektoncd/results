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
	"os"

	v1alpha2 "github.com/tektoncd/results/pkg/api/server/v1alpha2"
	v1alpha2pb "github.com/tektoncd/results/proto/v1alpha2/results_go_proto"

	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

func main() {
	flag.Parse()

	user, pass := os.Getenv("DB_USER"), os.Getenv("DB_PASSWORD")
	if user == "" || pass == "" {
		log.Fatal("Must provide both DB_USER and DB_PASSWORD")
	}
	// Connect to the MySQL database.
	// DSN derived from https://github.com/go-sql-driver/mysql#dsn-data-source-name
	dbURI := fmt.Sprintf("%s:%s@%s(%s)/%s?parseTime=true", user, pass, os.Getenv("DB_PROTOCOL"), os.Getenv("DB_ADDR"), os.Getenv("DB_NAME"))
	db, err := gorm.Open(mysql.Open(dbURI), &gorm.Config{})
	if err != nil {
		log.Fatalf("failed to open the results.db: %v", err)
	}

	s := grpc.NewServer()

	// Register API server(s)
	v1a2, err := v1alpha2.New(db)
	if err != nil {
		log.Fatalf("failed to create server: %v", err)
	}
	v1alpha2pb.RegisterResultsServer(s, v1a2)

	// Allow service reflection - required for grpc_cli ls to work.
	reflection.Register(s)

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
