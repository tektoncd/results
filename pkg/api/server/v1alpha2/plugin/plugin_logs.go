package plugin

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httputil"
	"net/url"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

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
	typePipelineRun    = "tekton.dev/v1.PipelineRun"
	typeTaskRun        = "tekton.dev/v1.TaskRun"
	typeTaskRunV1Beta1 = "tekton.dev/v1beta1.TaskRun"

	legacyLogType = "v1alpha2LogType"
	// TODO: make this key configurable in a future release
	defaultBlobPathParams = "/%s/%s/%s/" // parent/result/record

	// TODO make these key configurable in a future release
	pipelineRunUIDKey = "kubernetes.labels.tekton_dev_pipelineRunUID"
	taskRunUIDKey     = "kubernetes.labels.tekton_dev_taskRunUID"
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

	logPath := map[string]string{}

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
				logPath[""] = filepath.Join(s.config.LOGS_PATH, log.Status.Path)
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
			logPath[obj.Key] = obj.Key
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
		logPath[""] = filepath.Join(s.config.LOGS_PATH, log.Status.Path)
	default:
		s.logger.Errorf("record type is invalid, record ID: %v, Name: %v, result Name: %v, result ID:  %v", rec.ID, rec.Name, rec.ResultName, rec.ResultID)
		return fmt.Errorf("record type is invalid %s", rec.Type)
	}

	for k, v := range logPath {
		err := func() error {
			s.logger.Debugf("logPath key: %s value: %s", k, v)
			_, file := filepath.Split(k)
			fmt.Fprint(writer, strings.TrimRight(file, ".log")+" :-\n")
			rc, err := bucket.NewReader(ctx, v, nil)
			if err != nil {
				s.logger.Errorf("error creating bucket reader: %s for log path: %s", err, logPath)
				return err
			}
			defer rc.Close()

			_, err = io.Copy(writer, rc)
			if err != nil {
				s.logger.Errorf("error copying the logs: %s", err)
				return err
			}
			fmt.Fprint(writer, "\n")
			return nil
		}()
		if err != nil {
			s.logger.Error(err)
			return err
		}
	}
	return nil
}

func (s *LogServer) setLogPlugin() bool {
	switch strings.ToLower(s.config.LOGS_TYPE) {
	case string(v1alpha3.LokiLogType):
		s.IsLogPluginEnabled = true
		s.getLog = getLokiLogs
	case string(v1alpha3.BlobLogType):
		s.IsLogPluginEnabled = true
		s.getLog = getBlobLogs
	default:
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
