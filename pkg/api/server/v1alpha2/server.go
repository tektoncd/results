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
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/google/cel-go/cel"
	"github.com/tektoncd/results/pkg/api/server/config"
	"go.uber.org/zap"
	"k8s.io/client-go/transport"

	"github.com/tektoncd/results/pkg/apis/v1alpha3"

	"github.com/google/uuid"
	cw "github.com/jonboulle/clockwork"
	resultscel "github.com/tektoncd/results/pkg/api/server/cel"
	model "github.com/tektoncd/results/pkg/api/server/db"
	"github.com/tektoncd/results/pkg/api/server/v1alpha2/auth"
	pb "github.com/tektoncd/results/proto/v1alpha2/results_go_proto"
	pb3 "github.com/tektoncd/results/proto/v1alpha3/results_go_proto"
	"golang.org/x/oauth2"
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
	config          *config.Config
	logger          *zap.SugaredLogger
	env             *cel.Env
	resultsEnv      *cel.Env
	recordsEnv      *cel.Env
	db              *gorm.DB
	auth            auth.Checker
	LogPluginServer *LogPluginServer

	// testing.
	getResultID getResultID
}

// LogPluginServer is the server for the log plugin server
type LogPluginServer struct {
	pb3.UnimplementedLogsServer

	IsLogPluginEnabled bool
	staticLabels       string

	config *config.Config
	logger *zap.SugaredLogger
	auth   auth.Checker
	db     *gorm.DB
	client *http.Client

	forwarderDelayDuration time.Duration

	queryLimit uint

	queryParams map[string]string

	// TODO: In future add support for non Oauth support
	tokenSource oauth2.TokenSource
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

	if err := srv.createLogPluginServer(); err != nil {
		return nil, fmt.Errorf("failed to create log plugin server: %w", err)
	}

	return srv, nil
}

// Option is customization for server configuration.
type Option func(*Server)

// WithAuth is an option to enable auth checker for Server
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

func (s *Server) createLogPluginServer() error {
	s.LogPluginServer = &LogPluginServer{
		config: s.config,
		logger: s.logger,
		auth:   s.auth,
		db:     s.db,
	}

	s.logger.Debugf("LOGS_TYPE: %s", strings.ToLower(s.config.LOGS_TYPE))
	// If the logs type is not Loki, we don't need to set up the LogPluginServer
	// In future, we can add support for other logging APIs and we will need to
	// check the value of LOGS_TYPE in a switch statement.
	if strings.ToLower(s.config.LOGS_TYPE) != string(v1alpha3.LokiLogType) {
		return nil
	}

	s.logger.Info("Setting up LogPluginServer")

	// Set the LogPluginServer to enabled because we are using Loki
	s.LogPluginServer.IsLogPluginEnabled = true

	labels := strings.Split(s.config.LOGGING_PLUGIN_STATIC_LABELS, ",")
	for _, v := range labels {
		label := strings.Split(v, "=")
		if len(label) != 2 {
			return fmt.Errorf("incorrect format for LOGGING_STATIC_LABELS: %s", v)
		}
		s.LogPluginServer.staticLabels += label[0] + `="` + label[1] + `",`
	}

	s.LogPluginServer.client = &http.Client{
		Transport: &http.Transport{
			Dial: (&net.Dialer{
				Timeout:   5 * time.Minute,
				KeepAlive: 10 * time.Minute,
			}).Dial,
			TLSHandshakeTimeout:   10 * time.Second,
			ResponseHeaderTimeout: 10 * time.Second,
			ExpectContinueTimeout: 1 * time.Second,
		},
	}

	if s.config.LOGGING_PLUGIN_CA_CERT != "" {
		caCertPool := x509.NewCertPool()
		caCertPool.AppendCertsFromPEM([]byte(s.config.LOGGING_PLUGIN_CA_CERT))
		// #nosec G402
		s.LogPluginServer.client.Transport.(*http.Transport).TLSClientConfig = &tls.Config{
			RootCAs: caCertPool, //nolint:gosec  // needed when we have our own CA
		}
	} else if s.config.LOGGING_PLUGIN_TLS_VERIFICATION_DISABLE {
		s.LogPluginServer.client.Transport.(*http.Transport).TLSClientConfig = &tls.Config{
			InsecureSkipVerify: true, //nolint:gosec  // needed for skipping tls verification
		}
	}

	s.LogPluginServer.forwarderDelayDuration = time.Duration(s.config.LOGGING_PLUGIN_FORWARDER_DELAY_DURATION) * time.Minute

	s.LogPluginServer.tokenSource = transport.NewCachedFileTokenSource(s.config.LOGGING_PLUGIN_TOKEN_PATH)
	s.LogPluginServer.queryLimit = s.config.LOGGING_PLUGIN_QUERY_LIMIT

	s.LogPluginServer.queryParams = map[string]string{}
	if s.config.LOGGING_PLUGIN_QUERY_PARAMS != "" {
		for _, v := range strings.Split(s.config.LOGGING_PLUGIN_QUERY_PARAMS, "&") {
			queryParam := strings.Split(v, "=")
			if len(queryParam) != 2 {
				return fmt.Errorf("incorrect format for LOGGING_PLUGIN_QUERY_PARAMS: %s", v)
			}
			s.LogPluginServer.queryParams[queryParam[0]] = queryParam[1]
		}
	}

	return nil
}
