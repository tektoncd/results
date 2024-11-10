package plugin

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/tektoncd/results/pkg/api/server/config"
	"go.uber.org/zap"

	"github.com/tektoncd/results/pkg/api/server/v1alpha2/auth"
	"golang.org/x/oauth2"
	"gorm.io/gorm"

	pb3 "github.com/tektoncd/results/proto/v1alpha3/results_go_proto"

	"k8s.io/client-go/transport"
)

// LogServer is the server for the log plugin server
type LogServer struct {
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

	getLog getLog
}

// NewLogServer returns a plugin log server
func NewLogServer(config *config.Config, logger *zap.SugaredLogger, auth auth.Checker, db *gorm.DB) (*LogServer, error) {
	s := &LogServer{
		config: config,
		logger: logger,
		auth:   auth,
		db:     db,
	}

	s.logger.Debugf("LOGS_TYPE: %s", strings.ToLower(s.config.LOGS_TYPE))
	// If the logs type is not plugin supported,
	//  we don't need to set up the LogPluginServer
	if !s.setLogPlugin() {
		return s, nil
	}

	s.logger.Info("Setting up LogPluginServer")

	if s.config.LOGGING_PLUGIN_STATIC_LABELS != "" {
		labels := strings.Split(s.config.LOGGING_PLUGIN_STATIC_LABELS, ",")
		for _, v := range labels {
			label := strings.Split(v, "=")
			if len(label) != 2 {
				return nil, fmt.Errorf("incorrect format for LOGGING_STATIC_LABELS: %s", v)
			}
			s.staticLabels += label[0] + `="` + label[1] + `",`
		}
	}

	s.client = &http.Client{
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
		s.client.Transport.(*http.Transport).TLSClientConfig = &tls.Config{
			RootCAs: caCertPool, //nolint:gosec  // needed when we have our own CA
		}
	} else if s.config.LOGGING_PLUGIN_TLS_VERIFICATION_DISABLE {
		s.client.Transport.(*http.Transport).TLSClientConfig = &tls.Config{
			InsecureSkipVerify: true, //nolint:gosec  // needed for skipping tls verification
		}
	}

	s.forwarderDelayDuration = time.Duration(s.config.LOGGING_PLUGIN_FORWARDER_DELAY_DURATION) * time.Minute

	s.tokenSource = transport.NewCachedFileTokenSource(s.config.LOGGING_PLUGIN_TOKEN_PATH)
	s.queryLimit = s.config.LOGGING_PLUGIN_QUERY_LIMIT

	s.queryParams = map[string]string{}
	if s.config.LOGGING_PLUGIN_QUERY_PARAMS != "" {
		for _, v := range strings.Split(s.config.LOGGING_PLUGIN_QUERY_PARAMS, "&") {
			queryParam := strings.Split(v, "=")
			if len(queryParam) != 2 {
				return nil, fmt.Errorf("incorrect format for LOGGING_PLUGIN_QUERY_PARAMS: %s", v)
			}
			s.queryParams[queryParam[0]] = queryParam[1]
		}
	}

	return s, nil
}
