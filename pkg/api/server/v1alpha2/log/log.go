package log

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"path/filepath"
	"regexp"

	"github.com/tektoncd/results/pkg/api/server/config"
	"github.com/tektoncd/results/pkg/api/server/db"
	"github.com/tektoncd/results/pkg/apis/v1alpha3"
	pb "github.com/tektoncd/results/proto/v1alpha2/results_go_proto"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const (
	// DefaultBufferSize is the default buffer size. This based on the recommended
	// gRPC message size for streamed content, which ranges from 16 to 64 KiB. Choosing 32 KiB as a
	// middle ground between the two.
	DefaultBufferSize = 32 * 1024
)

var (
	// NameRegex matches valid name specs for a Result.
	NameRegex = regexp.MustCompile("(^[a-z0-9_-]{1,63})/results/([a-z0-9_-]{1,63})/logs/([a-z0-9_-]{1,63}$)")
)

// ParseName splits a full Result name into its individual (parent, result, name)
// components.
func ParseName(raw string) (parent, result, name string, err error) {
	s := NameRegex.FindStringSubmatch(raw)
	if len(s) != 4 {
		return "", "", "", status.Errorf(codes.InvalidArgument, "name must match %s", NameRegex.String())
	}
	return s[1], s[2], s[3], nil
}

// FormatName takes in a parent ("a/results/b") and record name ("c") and
// returns the full resource name ("a/results/b/logs/c").
func FormatName(parent, name string) string {
	return fmt.Sprintf("%s/logs/%s", parent, name)
}

// Stream is an interface that defines the behavior of a streaming log service.
type Stream interface {
	io.ReaderFrom
	io.WriterTo
	Type() string
	Delete() error
	Flush() error
}

// NewStream returns a LogStreamer for the given Log.
// LogStreamers do the following:
//
// 1. Write log data from their respective source to an io.Writer interface.
// 2. Read log data from a source, and store it in the respective backend if that behavior is supported.
//
// All LogStreamers support writing log data to an io.Writer from the provided source.
// LogStreamers do not need to receive and store data from the provided source.
//
// NewStream may mutate the Log object's status, to provide implementation information
// for reading and writing files.
func NewStream(ctx context.Context, log *v1alpha3.Log, config *config.Config) (Stream, error) {
	switch log.Spec.Type {
	case v1alpha3.FileLogType:
		return NewFileStream(ctx, log, config)
	case v1alpha3.S3LogType:
		return NewS3Stream(ctx, log, config)
	case v1alpha3.GCSLogType:
		return NewGCSStream(ctx, log, config)
	}
	return nil, fmt.Errorf("log streamer type %s is not supported", log.Spec.Type)
}

// ToStorage converts log record to marshaled json bytes
func ToStorage(record *pb.Record, config *config.Config) ([]byte, error) {
	log := &v1alpha3.Log{}
	if len(record.GetData().Value) > 0 {
		err := json.Unmarshal(record.GetData().Value, log)
		if err != nil {
			return nil, err
		}
	}
	log.Default()

	if log.Spec.Type == "" {
		log.Spec.Type = v1alpha3.LogType(config.LOGS_TYPE)
		if len(log.Spec.Type) == 0 {
			return nil, fmt.Errorf("failed to set up log storage type to spec")
		}
	}
	return json.Marshal(log)
}

// ToStream returns three arguments.
// First one is a new log streamer created by log record.
// Second one is log API resource retrieved from log record.
// Third argument is an error.
func ToStream(ctx context.Context, record *db.Record, config *config.Config) (Stream, *v1alpha3.Log, error) {
	if record.Type != v1alpha3.LogRecordType && record.Type != v1alpha3.LogRecordTypeV2 {
		return nil, nil, fmt.Errorf("record type %s cannot stream logs", record.Type)
	}
	log := &v1alpha3.Log{}
	err := json.Unmarshal(record.Data, log)
	if err != nil {
		return nil, nil, fmt.Errorf("could not decode Log record: %w", err)
	}
	stream, err := NewStream(ctx, log, config)
	return stream, log, err
}

// FilePath returns file path to store log. This file path can be
// path in the real file system or virtual value depending on storage type.
func FilePath(log *v1alpha3.Log) (string, error) {
	filePath := filepath.Join(log.GetNamespace(), string(log.GetUID()), log.Name)
	if filePath == "" {
		return "", fmt.Errorf("invalid file path")
	}
	return filePath, nil
}
