package server

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"strings"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"k8s.io/client-go/transport"

	"github.com/tektoncd/results/pkg/api/server/v1alpha2/auth"
	"github.com/tektoncd/results/pkg/api/server/v1alpha2/log"
	"github.com/tektoncd/results/pkg/apis/v1alpha3"
	"github.com/tektoncd/results/pkg/logs"
	pb3 "github.com/tektoncd/results/proto/v1alpha3/results_go_proto"
)

const (
	lokiQueryPath = "/loki/api/v1/query_range"
)

// GetLog streams log record by log request
func (s *LogPluginServer) GetLog(req *pb3.GetLogRequest, srv pb3.Logs_GetLogServer) error {
	parent, res, name, err := log.ParseName(req.GetName())
	if err != nil {
		s.logger.Error(err)
		return status.Error(codes.InvalidArgument, "Invalid Name")
	}

	if err := s.auth.Check(srv.Context(), parent, auth.ResourceLogs, auth.PermissionGet); err != nil {
		s.logger.Error(err)
		// unauthenticated status code and debug message produced by Check
		return err
	}

	rec, err := getRecord(s.db, parent, res, name)
	if err != nil {
		s.logger.Error(err)
		return err
	}

	writer := logs.NewBufferedHTTPWriter(srv, req.GetName(), s.config.LOGS_BUFFER_SIZE)

	err = s.getPluginLogs(writer, parent, rec.Name)
	if err != nil {
		s.logger.Error(err)
	}
	_, err = writer.Flush()
	if err != nil {
		s.logger.Error(err)
		return status.Error(codes.Internal, "Error streaming log")
	}
	return nil
}

func (s *LogPluginServer) getPluginLogs(writer *logs.BufferedLog, parent, id string) error {
	switch strings.ToLower(s.config.LOGS_TYPE) {
	case string(v1alpha3.LokiLogType):
		return s.getLokiLogs(writer, parent, id)
	default:
		s.logger.Errorf("unsupported type of logs given for plugin")
		return fmt.Errorf("unsupported type of logs given for plugin")
	}
}

func (s *LogPluginServer) getLokiLogs(writer *logs.BufferedLog, parent, id string) error {
	URL, err := url.Parse(s.config.LOGGING_PLUGIN_API_URL)
	if err != nil {
		s.logger.Error(err)
		return err
	}
	URL.Path = path.Join(URL.Path, s.config.LOGGING_PLUGIN_PROXY_PATH, lokiQueryPath)

	now := time.Now()
	parameters := url.Values{}
	parameters.Add("query", `{ `+s.staticLabels+s.config.LOGGING_PLUGIN_NAMESPACE_KEY+`="`+parent+`" }|json|="`+id+`"| line_format "{{.message}}"`)
	parameters.Add("end", strconv.FormatInt(now.UTC().Unix(), 10))
	parameters.Add("start", strconv.FormatInt(now.Add(-s.MaxRetention).UTC().Unix(), 10))

	URL.RawQuery = parameters.Encode()

	req, err := http.NewRequest("GET", URL.String(), nil)
	if err != nil {
		s.logger.Errorf("new request to loki failed, err: %s:", err.Error())
		return err
	}

	token, err := transport.NewCachedFileTokenSource(s.config.LOGGING_PLUGIN_TOKEN_PATH).Token()
	if err != nil {
		s.logger.Error(err)
		return err
	}

	req.Header.Set("Authorization", "Bearer "+token.AccessToken)
	resp, err := s.client.Do(req)
	if err != nil {
		s.logger.Errorf("request to loki failed, err: %s, req: %v", err.Error(), req)
		return status.Error(codes.Internal, "Error streaming log")
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		s.logger.Errorf("failed to read response body, err: %s", err.Error())
		return status.Error(codes.Internal, "Error streaming log")
	}

	var lokiResponse struct {
		Data struct {
			Result []struct {
				Stream struct {
					Message string `json:"message"`
				} `json:"stream"`
			} `json:"result"`
		} `json:"data"`
	}

	if err := json.Unmarshal(data, &lokiResponse); err != nil {
		s.logger.Errorf("failed to unmarshal Loki response, err: %s, data: %s", err.Error(), string(data))
		return status.Error(codes.Internal, "Error processing log data")
	}

	var logMessages []string
	for _, result := range lokiResponse.Data.Result {
		logMessages = append(logMessages, result.Stream.Message)
	}

	formattedLogs := strings.Join(logMessages, "\n")

	if _, err = writer.Write([]byte(formattedLogs)); err != nil {
		s.logger.Errorf("failed to write log data, err: %s", err.Error())
		return status.Error(codes.Internal, "Error streaming log")
	}

	return nil

}
