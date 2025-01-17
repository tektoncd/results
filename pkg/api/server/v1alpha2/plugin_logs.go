package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httputil"
	"net/url"
	"path"
	"strconv"
	"strings"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/tektoncd/results/pkg/api/server/db"
	"github.com/tektoncd/results/pkg/api/server/v1alpha2/auth"
	"github.com/tektoncd/results/pkg/api/server/v1alpha2/log"
	"github.com/tektoncd/results/pkg/apis/v1alpha3"
	"github.com/tektoncd/results/pkg/logs"
	pb3 "github.com/tektoncd/results/proto/v1alpha3/results_go_proto"

	pipelinev1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1"
)

const (
	lokiQueryPath   = "/loki/api/v1/query_range"
	typePipelineRun = "tekton.dev/v1.PipelineRun"
	typeTaskRun     = "tekton.dev/v1.TaskRun"

	// TODO make these key configurable in a future release
	pipelineRunUIDKey = "kubernetes.labels.tekton_dev_pipelineRunUID"
	taskRunUIDKey     = "kubernetes.labels.tekton_dev_taskRunUID"
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

	err = s.getPluginLogs(writer, parent, rec)
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

func (s *LogPluginServer) getPluginLogs(writer *logs.BufferedLog, parent string, rec *db.Record) error {
	switch strings.ToLower(s.config.LOGS_TYPE) {
	case string(v1alpha3.LokiLogType):
		return s.getLokiLogs(writer, parent, rec)
	default:
		s.logger.Errorf("unsupported type of logs given for plugin")
		return fmt.Errorf("unsupported type of logs given for plugin")
	}
}

func (s *LogPluginServer) getLokiLogs(writer *logs.BufferedLog, parent string, rec *db.Record) error {
	URL, err := url.Parse(s.config.LOGGING_PLUGIN_API_URL)
	if err != nil {
		s.logger.Error(err)
		return err
	}
	URL.Path = path.Join(URL.Path, s.config.LOGGING_PLUGIN_PROXY_PATH, lokiQueryPath)

	var startTime, endTime, uidKey string
	switch rec.Type {
	case typePipelineRun:
		uidKey = pipelineRunUIDKey
		data := &pipelinev1.PipelineRun{}
		err := json.Unmarshal(rec.Data, data)
		if err != nil {
			err = fmt.Errorf("failed to marshal pipelinerun data for fetching log, err: %s", err.Error())
			s.logger.Error(err.Error())
			return err
		}

		if data.Status.StartTime == nil {
			err = errors.New("there's no startime in pipelinerun")
			s.logger.Error(err.Error())
			return err
		}
		startTime = strconv.FormatInt(data.Status.StartTime.UTC().Unix(), 10)

		if data.Status.CompletionTime == nil {
			err = errors.New("there's no completion in pipelinerun")
			s.logger.Error(err.Error())
			return err
		}
		endTime = strconv.FormatInt(data.Status.CompletionTime.Add(s.forwarderDelayDuration).UTC().Unix(), 10)

	case typeTaskRun:
		uidKey = taskRunUIDKey
		data := &pipelinev1.TaskRun{}
		err := json.Unmarshal(rec.Data, data)
		if err != nil {
			err = fmt.Errorf("failed to marshal taskrun data for fetching log, err: %s", err.Error())
			s.logger.Error(err.Error())
			return err
		}

		if data.Status.StartTime == nil {
			err = errors.New("there's no startime in taskrun")
			s.logger.Error(err.Error())
			return err
		}
		startTime = strconv.FormatInt(data.Status.StartTime.UTC().Unix(), 10)

		if data.Status.CompletionTime == nil {
			err = errors.New("there's no completion in taskrun")
			s.logger.Error(err.Error())
			return err
		}
		endTime = strconv.FormatInt(data.Status.CompletionTime.Add(s.forwarderDelayDuration).UTC().Unix(), 10)

	default:
		s.logger.Error("record type is invalid")
		return errors.New("record type is invalid")
	}

	parameters := url.Values{}
	for k, v := range s.queryParams {
		parameters.Add(k, v)
	}
	parameters.Add("query", `{ `+s.staticLabels+s.config.LOGGING_PLUGIN_NAMESPACE_KEY+`="`+parent+`" }|json uid="`+uidKey+`", message="message" |uid="`+rec.Name+`"| line_format "{{.message}}"`)
	parameters.Add("end", endTime)
	parameters.Add("start", startTime)
	parameters.Add("limit", strconv.FormatUint(uint64(s.queryLimit), 10))

	URL.RawQuery = parameters.Encode()
	s.logger.Debugf("loki request url:%s", URL.String())

	req, err := http.NewRequest("GET", URL.String(), nil)
	if err != nil {
		s.logger.Errorf("new request to loki failed, err: %s:", err.Error())
		return err
	}

	token, err := s.tokenSource.Token()
	if err != nil {
		s.logger.Error("failed to fetch token", err)
		return err
	}

	req.Header.Set("Authorization", "Bearer "+token.AccessToken)
	resp, err := s.client.Do(req)
	if err != nil {
		dump, derr := httputil.DumpRequest(req, true)
		if derr == nil {
			s.logger.Debugf("Request Dump***:\n %q\n", dump)
		}
		s.logger.Errorf("request to loki failed, err: %s, req: %v", err.Error(), req)
		return status.Error(codes.Internal, "Error streaming log")
	}

	if resp == nil {
		dump, err := httputil.DumpRequest(req, true)
		if err == nil {
			s.logger.Debugf("Request Dump***:\n %q\n", dump)
		}
		s.logger.Errorf("request to loki failed, received nil response")
		s.logger.Debugf("loki request url:%s", URL.String())
		return status.Error(codes.Internal, "Error streaming log")
	}

	if resp.StatusCode != http.StatusOK {
		s.logger.Errorf("Loki API request failed with HTTP status code: %d", resp.StatusCode)
		dump, err := httputil.DumpRequest(req, true)
		if err == nil {
			s.logger.Debugf("Request Dump***:\n %q\n", dump)
		}
		dump, err = httputil.DumpResponse(resp, true)
		if err == nil {
			s.logger.Debugf("Response Dump***:\n %q\n", dump)
		}
		return status.Error(codes.Internal, "Error fetching log data")
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		s.logger.Errorf("failed to read response body, err: %s", err.Error())
		return status.Error(codes.Internal, "Error streaming log")
	}

	var lokiResponse struct {
		Status string `json:"status"`
		Error  string `json:"error"`
		Data   struct {
			Result []struct {
				Stream struct {
					Message string `json:"message"`
				} `json:"stream"`
			} `json:"result"`
			Stats map[string]interface{} `json:"stats"`
		} `json:"data"`
	}

	if err := json.Unmarshal(data, &lokiResponse); err != nil {
		s.logger.Errorf("failed to unmarshal Loki response, err: %s, data: %s", err.Error(), string(data))
		return status.Error(codes.Internal, fmt.Sprintf("Error processing fetched log data, err: %s, data: %s",
			err.Error(), string(data)))
	}

	s.logger.Debugf("stats.summary %v", lokiResponse.Data.Stats["summary"])

	if lokiResponse.Status != "success" {
		s.logger.Errorf("Loki API request failed with status: %s, error: %s", lokiResponse.Status, lokiResponse.Error)
		return status.Error(codes.Internal, "Error fetching log data from Loki")
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
