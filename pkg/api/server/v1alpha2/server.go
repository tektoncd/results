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

	"github.com/google/uuid"
	cw "github.com/jonboulle/clockwork"
	resultscel "github.com/tektoncd/results/pkg/api/server/cel"
	pb "github.com/tektoncd/results/proto/v1alpha2/results_go_proto"
	"gorm.io/gorm"
)

var (
	uid = func() string {
		return uuid.New().String()
	}
	clock cw.Clock = cw.NewRealClock()
)

// Server with implementation of API server
type Server struct {
	pb.UnimplementedResultsServer
	env *cel.Env
	db  *gorm.DB

	// Converts result names -> IDs configurable to allow overrides for
	// testing.
	getResultID func(ctx context.Context, parent, result string) (string, error)
}

// New set up environment for the api server
func New(db *gorm.DB) (*Server, error) {
	env, err := resultscel.NewEnv()
	if err != nil {
		return nil, fmt.Errorf("failed to create CEL environment: %w", err)
	}
	srv := &Server{
		db:  db,
		env: env,
	}

	// Set default impls of overridable behavior
	srv.getResultID = srv.getResultIDImpl

	return srv, nil
}
