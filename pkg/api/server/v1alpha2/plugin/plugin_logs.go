package plugin

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"gorm.io/gorm"

	"gocloud.dev/blob"

	// Adding the driver for gcs.
	_ "gocloud.dev/blob/gcsblob"
	// Adding the driver for s3.
	_ "gocloud.dev/blob/s3blob"

	"github.com/tektoncd/results/pkg/api/server/db"
	dbErrors "github.com/tektoncd/results/pkg/api/server/db/errors"
	"github.com/tektoncd/results/pkg/api/server/v1alpha2/auth"
	"github.com/tektoncd/results/pkg/api/server/v1alpha2/log"
	"github.com/tektoncd/results/pkg/apis/v1alpha3"
	"github.com/tektoncd/results/pkg/logs"
	pb3 "github.com/tektoncd/results/proto/v1alpha3/results_go_proto"

	pipelinev1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1"
)

const (
	lokiQueryPath      = "/loki/api/v1/query_range"
	splunkQueryPath    = "/services/search/v2/jobs"
	typePipelineRun    = "tekton.dev/v1.PipelineRun"
	typeTaskRun        = "tekton.dev/v1.TaskRun"
	typeTaskRunV1Beta1 = "tekton.dev/v1beta1.TaskRun"

	legacyLogType = "v1alpha2LogType"
	// TODO: make this key configurable in a future release
	defaultBlobPathParams = "/%s/%s/%s/" // parent/result/record

	// TODO make these key configurable in a future release
	pipelineRunUIDKey         = "kubernetes.labels.tekton_dev_pipelineRunUID"
	taskRunUIDKey             = "kubernetes.labels.tekton_dev_taskRunUID"
	splunkPollInterval        = 5 * time.Second
	splunkPollTimeoutDuration = 1 * time.Minute
	splunkTokenEnv            = "SPLUNK_SEARCH_TOKEN"
	splunkOutputFormat        = "?output_mode=json"
)

var (
	openBucket = func(ctx context.Context, urlString string) (*blob.Bucket, error) {
		bucket, err := blob.OpenBucket(ctx, urlString)
		return bucket, err
	}
	clean = func(bucket *blob.Bucket, logger *zap.SugaredLogger) {
		err := bucket.Close()
		if err != nil {
			logger.Errorf("Got error while closing bucket %s", err)
		}
	}
)

type getLog func(s *LogServer, writer io.Writer, parent string, rec *db.Record) error

// GetLog streams log record by log request
func (s *LogServer) GetLog(req *pb3.GetLogRequest, srv pb3.Logs_GetLogServer) error {
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

	if rec == nil {
		s.logger.Errorf("records not found: parent: %s, result: %s, name: %s", parent, res, name)
		return status.Error(codes.Internal, "Error streaming log")
	}

	writer := logs.NewBufferedHTTPWriter(srv, req.GetName(), s.config.LOGS_BUFFER_SIZE)

	err = s.getLog(s, writer, parent, rec)
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

func getLokiLogs(s *LogServer, writer io.Writer, parent string, rec *db.Record) error {
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
			s.logger.Error(err)
			return err
		}

		if data.Status.StartTime == nil {
			err = errors.New("there's no startime in pipelinerun")
			s.logger.Error(err)
			return err
		}
		startTime = strconv.FormatInt(data.Status.StartTime.UTC().Unix(), 10)

		if data.Status.CompletionTime == nil {
			err = errors.New("there's no completion in pipelinerun")
			s.logger.Error(err)
			return err
		}
		endTime = strconv.FormatInt(data.Status.CompletionTime.Add(s.forwarderDelayDuration).UTC().Unix(), 10)

	case typeTaskRun:
		uidKey = taskRunUIDKey
		data := &pipelinev1.TaskRun{}
		err := json.Unmarshal(rec.Data, data)
		if err != nil {
			err = fmt.Errorf("failed to marshal taskrun data for fetching log, err: %s", err.Error())
			s.logger.Error(err)
			return err
		}
		if data.Status.StartTime == nil {
			err = errors.New("there's no startime in taskrun")
			s.logger.Error(err)
			return err
		}
		startTime = strconv.FormatInt(data.Status.StartTime.UTC().Unix(), 10)

		if data.Status.CompletionTime == nil {
			err = errors.New("there's no completion in taskrun")
			s.logger.Error(err)
			return err
		}
		endTime = strconv.FormatInt(data.Status.CompletionTime.Add(s.forwarderDelayDuration).UTC().Unix(), 10)

	default:
		s.logger.Errorf("record type is invalid, record ID: %v, Name: %v, result Name: %v, result ID:  %v", rec.ID, rec.Name, rec.ResultName, rec.ResultID)
		return errors.New("record type is invalid")
	}

	parameters := url.Values{}
	for k, v := range s.queryParams {
		parameters.Add(k, v)
	}
	query := `{ ` + s.staticLabels + s.config.LOGGING_PLUGIN_NAMESPACE_KEY + `="` + parent + `" }|json uid="` + uidKey + `", message="message" |uid="` + rec.Name + `"| line_format "{{.message}}"`
	if s.config.LOGGING_PLUGIN_CONTAINER_KEY != "" {
		query = `{ ` + s.staticLabels + s.config.LOGGING_PLUGIN_NAMESPACE_KEY + `="` + parent + `" }|json uid="` + uidKey + `", container="` + s.config.LOGGING_PLUGIN_CONTAINER_KEY + `", message="message" |uid="` + rec.Name + `"| line_format "container-{{.container}}: message={{.message}}"`
	}
	parameters.Add("query", query)
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
				Values [][]string `json:"values"`
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

	var logMessages [][]string
	for _, result := range lokiResponse.Data.Result {
		logMessages = append(logMessages, result.Values...)
	}

	sort.Slice(logMessages, func(i, j int) bool {
		if len(logMessages[i]) == 0 {
			return true
		}
		if len(logMessages[j]) == 0 {
			return false
		}
		return logMessages[i][0] < logMessages[j][0]
	})

	var orderMessages []string
	for _, val := range logMessages {
		if len(val) <= 1 {
			continue
		}
		orderMessages = append(orderMessages, val[1:]...)
	}

	formattedLogs := strings.Join(orderMessages, "\n")

	if _, err = writer.Write([]byte(formattedLogs)); err != nil {
		s.logger.Errorf("failed to write log data, err: %s", err.Error())
		return status.Error(codes.Internal, "Error streaming log")
	}

	return nil
}

func getBlobLogs(s *LogServer, writer io.Writer, parent string, rec *db.Record) error {
	u, err := url.Parse(s.config.LOGGING_PLUGIN_API_URL)
	if err != nil {
		s.logger.Error(err)
		return err
	}

	legacy := false
	queryParams := u.Query()

	for k, v := range s.queryParams {
		if k == legacyLogType && v == "true" {
			legacy = true
			continue
		}
		queryParams.Add(k, v)
	}
	u.RawQuery = queryParams.Encode()

	logPath := []string{}

	ctx := context.Background()
	s.logger.Debugf("blob bucket: %s", u.String())
	bucket, err := openBucket(ctx, u.String())
	if err != nil {
		s.logger.Errorf("error opening bucket: %s", err)
		return err
	}
	defer clean(bucket, s.logger)

	switch rec.Type {
	case typePipelineRun:
		err := errors.New("pipelinerun not supported, please use taskrun")
		s.logger.Error(err)
		return err
	case typeTaskRunV1Beta1:
		if legacy {
			logRec, err := getLogRecord(s.db, parent, rec.ResultID, rec.Name)
			if err != nil {
				s.logger.Debugf("error getting legacy log record: %s", err)
			}
			if logRec != nil {
				log := &v1alpha3.Log{}
				err := json.Unmarshal(logRec.Data, log)
				if err != nil {
					err = fmt.Errorf("could not decode Log record: %w", err)
					s.logger.Error(err)
					return err
				}
				logPath = append(logPath, filepath.Join(s.config.LOGS_PATH, log.Status.Path))
			}
		} else {
			s.logger.Errorf("record type is invalid %s", rec.Type)
			return fmt.Errorf("record type is invalid %s", rec.Type)
		}
	case typeTaskRun:
		s.logger.Debugf("taskrun type")
		iter := bucket.List(&blob.ListOptions{
			Prefix: strings.TrimPrefix(s.config.LOGS_PATH+fmt.Sprintf(defaultBlobPathParams, parent, rec.ResultName, rec.Name), "/"),
		})
		s.logger.Debugf("prefix: %s", strings.TrimPrefix(s.config.LOGS_PATH+fmt.Sprintf(defaultBlobPathParams, parent, rec.ResultName, rec.Name), "/"))
		// bucket.List returns the objects sorted alphabetically by key (name), we need that sorted by last modified time
		toSort := []*blob.ListObject{}
		for {
			obj, err := iter.Next(ctx)
			if err == io.EOF {
				break
			}
			if err != nil {
				err := fmt.Errorf("error listing log bucket objects: %w", err)
				s.logger.Error(err)
				return err
			}
			toSort = append(toSort, obj)
		}
		// S3 objects ModTime is rounded to the second (not milliseconds), so objects stored in the same second are still ordered alphabetically
		slices.SortFunc(toSort, func(a, b *blob.ListObject) int {
			return time.Time.Compare(a.ModTime, b.ModTime)
		})
		for _, obj := range toSort {
			logPath = append(logPath, obj.Key)
		}

		s.logger.Debugf("logPath: %v", logPath)
	case v1alpha3.LogRecordType, v1alpha3.LogRecordTypeV2:
		log := &v1alpha3.Log{}
		err := json.Unmarshal(rec.Data, log)
		if err != nil {
			err = fmt.Errorf("could not decode Log record: %w", err)
			s.logger.Error(err)
			return err
		}
		logPath = append(logPath, filepath.Join(s.config.LOGS_PATH, log.Status.Path))
	default:
		s.logger.Errorf("record type is invalid, record ID: %v, Name: %v, result Name: %v, result ID:  %v", rec.ID, rec.Name, rec.ResultName, rec.ResultID)
		return fmt.Errorf("record type is invalid %s", rec.Type)
	}

	regex := s.config.LOGGING_PLUGIN_MULTIPART_REGEX
	re, err := regexp.Compile(regex)
	if err != nil {
		s.logger.Errorf("failed to compile regexp: %s", err)
		return err
	}
	mergedLogParts := mergeLogParts(logPath, re)

	for _, parts := range mergedLogParts {
		baseName := re.ReplaceAllString(parts[0], "")
		s.logger.Debugf("mergedLogParts key: %s value: %v", baseName, parts)
		_, file := filepath.Split(baseName)
		fmt.Fprint(writer, strings.TrimRight(file, ".log")+" :-\n")
		for _, part := range parts {
			err := func() error {
				rc, err := bucket.NewReader(ctx, part, nil)
				if err != nil {
					s.logger.Errorf("error creating bucket reader: %s for log part: %s", err, part)
					return err
				}
				defer rc.Close()

				_, err = rc.WriteTo(writer)
				if err != nil {
					s.logger.Errorf("error writing the logs: %s", err)
				}
				return nil
			}()
			if err != nil {
				s.logger.Error(err)
				return err
			}
			fmt.Fprint(writer, "\n")
		}
	}

	return nil
}

// mergeLogParts organizes in groups objects part of the same log
func mergeLogParts(logPath []string, re *regexp.Regexp) [][]string {
	merged := [][]string{}
	// use extra mapping [log_base_name:index_of_slice_of_parts] to preserve the order of elements
	baseNameIndexes := map[string]int{}
	index := 0
	for _, log := range logPath {
		baseName := re.ReplaceAllString(log, "")
		if existingIndex, ok := baseNameIndexes[baseName]; ok {
			merged[existingIndex] = append(merged[existingIndex], log)
		} else {
			baseNameIndexes[baseName] = index
			merged = append(merged, []string{log})
			index++
		}
	}
	return merged
}

// getSplunkLogs retrieves logs for a given record from a Splunk backend and writes them to the provided writer.
//
// It constructs a Splunk search job using the record's UID and namespace, submits the job, polls for completion, and fetches the resulting logs.
// The function requires a valid Splunk API URL, a search token from the environment, and an "index" query parameter.
// Returns an error if any step in the process fails, including job creation, polling, or log retrieval.
func getSplunkLogs(s *LogServer, writer io.Writer, parent string, rec *db.Record) error {
	URL, err := url.Parse(s.config.LOGGING_PLUGIN_API_URL)
	if err != nil {
		s.logger.Error(err)
		return err
	}
	URL.Path = path.Join(URL.Path, splunkQueryPath)

	token := os.Getenv(splunkTokenEnv)
	if token == "" {
		s.logger.Error("splunk token not set SPLUNK_SEARCH_TOKEN")
		return errors.New("splunk token not set")

	}

	var uidKey string
	switch rec.Type {
	case typePipelineRun:
		uidKey = pipelineRunUIDKey
	case typeTaskRun:
		uidKey = taskRunUIDKey
	default:
		s.logger.Errorf("record type is invalid, record ID: %v, Name: %v, result Name: %v, result ID:  %v, rec Type: %v", rec.ID, rec.Name, rec.ResultName, rec.ResultID, rec.Type)
		return errors.New("record type is invalid")
	}
	index, ok := s.queryParams["index"]
	if !ok {
		s.logger.Errorf("index not specified in queryParams: %v\n", s.queryParams)
		return errors.New("index not specified in query parameters")

	}
	s.logger.Debugf("splunk request url:%s", URL.String()+splunkOutputFormat)

	query := fmt.Sprintf(`search index=%s %s=%q %s=%q | table message structured.msg %s`,
		index, uidKey, rec.Name, s.config.LOGGING_PLUGIN_NAMESPACE_KEY, parent,
		s.config.LOGGING_PLUGIN_CONTAINER_KEY)

	queryData := url.Values{}
	queryData.Set("search", query)

	req, err := http.NewRequest("POST", URL.String()+splunkOutputFormat, bytes.NewReader([]byte(queryData.Encode())))
	if err != nil {
		s.logger.Errorf("new request to splunk failed, err: %s:", err.Error())
		return err
	}

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := s.client.Do(req)
	if err != nil {
		s.logger.Errorf("request to splunk failed, err: %s, req: %v", err.Error(), req)
		return status.Error(codes.Internal, "Error streaming log")
	}

	if resp == nil {
		s.logger.Errorf("request to splunk failed, received nil response")
		s.logger.Debugf("splunk request url:%s", URL.String())
		return status.Error(codes.Internal, "Error streaming log")
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		s.logger.Errorf("Splunk Job Creation API request failed with HTTP status code: %d", resp.StatusCode)
		return status.Error(codes.Internal, "Error fetching log data - search job creation failed")
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		s.logger.Errorf("failed to read response body, err: %s", err.Error())
		return status.Error(codes.Internal, "Error streaming log")
	}

	var searchResponse struct {
		SID string `json:"sid"`
	}

	if err := json.Unmarshal(data, &searchResponse); err != nil {
		s.logger.Errorf("failed to unmarshal Splunk Search response, err: %s, data: %s", err.Error(), string(data))
		return status.Error(codes.Internal, fmt.Sprintf("Error processing fetched log data, err: %s, data: %s",
			err.Error(), string(data)))
	}

	if err := pollSplunkJobStatus(s, URL.String()+"/"+searchResponse.SID+splunkOutputFormat, token); err != nil {
		s.logger.Errorf("failed to poll splunk job status, err: %v", err)
		return status.Error(codes.Internal, fmt.Sprintf("Error failed to poll splunk search job status, err: %s", err.Error()))
	}

	req, err = http.NewRequest("GET", URL.String()+"/"+searchResponse.SID+"/results?output_mode=json_rows&count=0", nil)
	if err != nil {
		s.logger.Errorf("new request to splunk failed, err: %s:", err.Error())
		return err
	}

	req.Header.Set("Authorization", "Bearer "+token)
	lresp, err := s.client.Do(req)
	if err != nil {
		s.logger.Errorf("request to fetch log from  splunk failed, err: %s, req: %v", err.Error(), req)
		return status.Error(codes.Internal, "Error streaming log")
	}

	if lresp == nil {
		s.logger.Errorf("request to splunk failed, received nil response")
		s.logger.Debugf("splunk request url:%s", URL.String())
		return status.Error(codes.Internal, "Error streaming log")
	}
	defer lresp.Body.Close()

	if lresp.StatusCode != http.StatusOK {
		s.logger.Errorf("Splunk Fetch Log API request failed with HTTP status code: %d", resp.StatusCode)
		return status.Error(codes.Internal, "Error fetching log data - fetch log api failed")
	}

	data, err = io.ReadAll(lresp.Body)
	if err != nil {
		s.logger.Errorf("failed to read response body, err: %s", err.Error())
		return status.Error(codes.Internal, "Error streaming log")
	}

	var logData struct {
		Rows [][]string `json:"rows"`
	}
	if err := json.Unmarshal(data, &logData); err != nil {
		s.logger.Errorf("failed to unmarshal Splunk log data, err: %s, data: %s", err.Error(), string(data))
		return fmt.Errorf("failed to unmarshal Splunk log data, err: %s", err.Error())
	}

	var logMessages []string
	step := ""
	for _, msg := range logData.Rows {
		if len(msg) != 3 {
			s.logger.Errorf("mismatch in column data, should be 3 received: %v, %v", len(msg), msg)
			continue
		}
		if step != msg[2] {
			step = msg[2]
			logMessages = append(logMessages, step+":-")
		}
		logMessages = append(logMessages, msg[0]+msg[1])
	}

	formattedLogs := strings.Join(logMessages, "\n")
	if _, err = writer.Write([]byte(formattedLogs)); err != nil {
		s.logger.Errorf("failed to write log data, err: %s", err.Error())
		return status.Error(codes.Internal, "Error streaming log")
	}

	return nil
}

// pollSplunkJobStatus polls the status of a Splunk search job until it is complete, fails, or times out.
// It sends periodic GET requests to the provided Splunk job status URL using the given token.
// Returns an error if the job fails, is canceled, or does not complete within the timeout period.
func pollSplunkJobStatus(s *LogServer, url, token string) error {
	ticker := time.NewTicker(splunkPollInterval)
	defer ticker.Stop()

	timeout := time.After(splunkPollTimeoutDuration)

	errSplunkJobFailed := errors.New("splunk job failed")

	for {
		select {
		case <-timeout:
			s.logger.Errorf("timeout reached for splunk search job, url: %v", url)
			return fmt.Errorf("timeout reached for splunk search job, url: %v", url)
		case <-ticker.C:
			err := func() error {
				req, err := http.NewRequest("GET", url, nil)
				if err != nil {
					s.logger.Errorf("new request to splunk failed, err: %s:", err.Error())
					return fmt.Errorf("new request to splunk failed: err: %s", err.Error())
				}

				req.Header.Set("Authorization", "Bearer "+token)
				resp, err := s.client.Do(req)
				if err != nil {
					s.logger.Errorf("request to splunk failed, err: %s, req: %v", err.Error(), req)
					return fmt.Errorf("request to splunk failed, err: %s, req: %v", err.Error(), req)
				}

				if resp == nil {
					s.logger.Errorf("request to splunk failed, received nil response,: %s", url)
					return fmt.Errorf("request to splunk failed, received nil response,: %s", url)
				}
				defer resp.Body.Close()
				if resp.StatusCode != http.StatusOK {
					s.logger.Errorf("Splunk Job Creation API request failed with HTTP status code: %d", resp.StatusCode)
					return fmt.Errorf("splunk job creation API request failed with HTTP status code: %d", resp.StatusCode)
				}

				data, err := io.ReadAll(resp.Body)
				if err != nil {
					s.logger.Errorf("failed to read response body, err: %s", err.Error())
					return fmt.Errorf("failed to read response body, err: %s", err.Error())
				}

				var jobResp struct {
					Entry []struct {
						Content struct {
							DispatchState string `json:"dispatchState"`
						} `json:"content"`
					} `json:"entry"`
				}

				if err := json.Unmarshal(data, &jobResp); err != nil {
					s.logger.Errorf("failed to unmarshal Splunk Search response, err: %s, data: %s", err.Error(), string(data))
					return fmt.Errorf("failed to unmarshal Splunk Search response, err: %s, data: %s", err.Error(), string(data))
				}
				if len(jobResp.Entry) == 0 {
					s.logger.Errorf("empty Splunk Search response entry, data: %s", string(data))
					return fmt.Errorf("empty Splunk Search response entry, data: %s", string(data))
				}

				switch jobResp.Entry[0].Content.DispatchState {
				case "DONE":
					return nil
				case "INTERNAL_CANCEL", "USER_CANCEL", "BAD_INPUT_CANCEL", "QUIT", "FAILED":
					return fmt.Errorf("%w: state: %s", errSplunkJobFailed, jobResp.Entry[0].Content.DispatchState)
				default:
					return fmt.Errorf("waiting for job to be done: current state %s", jobResp.Entry[0].Content.DispatchState)
				}
			}()
			if err != nil {
				if errors.Is(err, errSplunkJobFailed) {
					return err
				}
				continue
			}
			return nil
		}
	}
}

func (s *LogServer) setLogPlugin() bool {
	switch strings.ToLower(s.config.LOGS_TYPE) {
	case string(v1alpha3.LokiLogType):
		s.IsLogPluginEnabled = true
		s.getLog = getLokiLogs
	case string(v1alpha3.BlobLogType):
		s.IsLogPluginEnabled = true
		s.getLog = getBlobLogs
	case string(v1alpha3.SplunkLogType):
		s.IsLogPluginEnabled = true
		s.getLog = getSplunkLogs
	default:
		// TODO(xinnjie) when s.config.LOGS_TYPE is File also show this error log
		s.IsLogPluginEnabled = false
		s.logger.Warnf("Plugin Logs API Disable: unsupported type of logs given for plugin, " +
			"legacy logging system might work")
	}
	return s.IsLogPluginEnabled
}

// LogMux returns a http.Handler that serves the log plugin server
func (s *LogServer) LogMux() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// TODO: Create a new log handler
		ctx := r.Context()
		md := metadata.MD(r.Header)
		ctx = metadata.NewIncomingContext(ctx, md)
		parent := r.PathValue("parent")
		recID := r.PathValue("recordID")
		res := r.PathValue("resultID")
		s.logger.Debugf("recordID: %s resultID: %s name: %s md: %+v", recID, res, parent, r.Header)
		if err := s.auth.Check(ctx, parent, auth.ResourceLogs, auth.PermissionGet); err != nil {
			s.logger.Error(err)
			http.Error(w, "Not Authorized", http.StatusUnauthorized)
			return
		}
		rec, err := getRecord(s.db, parent, res, recID)
		if err != nil {
			s.logger.Error(err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		if rec == nil {
			s.logger.Errorf("records not found: parent: %s, result: %s, recID: %s", parent, res, recID)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		err = s.getLog(s, w, parent, rec)
		if err != nil {
			s.logger.Error(err)
			http.Error(w, "Failed to stream logs err: "+err.Error(), http.StatusInternalServerError)
			return
		}
	})
}

func getRecord(txn *gorm.DB, parent, result, name string) (*db.Record, error) {
	store := &db.Record{}
	q := txn.
		Where(&db.Record{Parent: parent, ResultName: result, Name: name}).
		First(store)
	if err := dbErrors.Wrap(q.Error); err != nil {
		return nil, err
	}
	return store, nil
}

func getLogRecord(txn *gorm.DB, parent, resultID, name string) (*db.Record, error) {
	store := &db.Record{}
	q := txn.
		Where(&db.Record{Parent: parent, ResultID: resultID}).
		Where("data -> 'spec' -> 'resource' ->> 'uid' =  ?", name).
		First(store)
	if err := dbErrors.Wrap(q.Error); err != nil {
		return nil, err
	}
	return store, nil
}
