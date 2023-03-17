// Copyright 2020 The Tekton Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package server

import (
	"context"
	"fmt"

	"github.com/google/cel-go/cel"
	"github.com/tektoncd/results/pkg/api/server/config"
	"go.uber.org/zap"

	"github.com/google/uuid"
	cw "github.com/jonboulle/clockwork"
	resultscel "github.com/tektoncd/results/pkg/api/server/cel"
	model "github.com/tektoncd/results/pkg/api/server/db"
	"github.com/tektoncd/results/pkg/api/server/v1alpha2/auth"
	pb "github.com/tektoncd/results/proto/v1alpha2/results_go_proto"
	"gorm.io/gorm"
)

var (
	uid = func() string {
		return uuid.New().String()
	}
	clock cw.Clock = cw.NewRealClock()
)

type getResultID func(ctx context.Context, parent, result string) (string, error)

// Server with implementation of API server
type Server struct {
	pb.UnimplementedResultsServer
	pb.UnimplementedLogsServer
	config     *config.Config
	logger     *zap.SugaredLogger
	env        *cel.Env
	resultsEnv *cel.Env
	recordsEnv *cel.Env
	db         *gorm.DB
	auth       auth.Checker

	// testing.
	getResultID getResultID
}

// New set up environment for the api server
func New(config *config.Config, logger *zap.SugaredLogger, db *gorm.DB, opts ...Option) (*Server, error) {
	env, err := resultscel.NewEnv()
	if err != nil {
		return nil, fmt.Errorf("failed to create CEL environment: %w", err)
	}
	// TODO: turn the func into a MustX that should panic on error.
	resultsEnv, err := resultscel.NewResultsEnv()
	if err != nil {
		return nil, err
	}
	recordsEnv, err := resultscel.NewRecordsEnv()
	if err != nil {
		return nil, err
	}

	srv := &Server{
		db:         db,
		env:        env,
		resultsEnv: resultsEnv,
		recordsEnv: recordsEnv,
		config:     config,
		logger:     logger,
		// Default open auth for easier testing.
		auth: auth.AllowAll{},
	}

	// Set default impls of overridable behavior
	srv.getResultID = srv.getResultIDImpl

	for _, o := range opts {
		o(srv)
	}

	if config.DB_ENABLE_AUTO_MIGRATION {
		if err := db.AutoMigrate(&model.Result{}, &model.Record{}); err != nil {
			return nil, fmt.Errorf("error automigrating DB: %w", err)
		}
	}

	return srv, nil
}

type Option func(*Server)

func WithAuth(c auth.Checker) Option {
	return func(s *Server) {
		s.auth = c
	}
}

func withGetResultID(f getResultID) Option {
	return func(s *Server) {
		s.getResultID = f
	}
}
