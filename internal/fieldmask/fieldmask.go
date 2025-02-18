package fieldmask

import (
	"context"
	"log"
	"net/http"
	"os"
	"strings"
	"sync/atomic"

	jsoniter "github.com/json-iterator/go"
	"github.com/tidwall/gjson"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/known/fieldmaskpb"
)

const (
	key = "fields"

	errStr = "\033[35m" + "%s\n" + "\033[0m" + "\033[31m" + "[error] " + "\033[0m"
)

var (
	logger = newLogger()
)

// Logger logger interface
type Logger interface {
	Error(context.Context, string, ...interface{})
}

func newLogger() Logger {
	return &stdLogger{
		log: log.New(os.Stdout, "\r\n", log.LstdFlags),
	}
}

type stdLogger struct {
	log *log.Logger
}

func (l *stdLogger) Error(_ context.Context, msg string, data ...interface{}) {
	l.log.Printf(errStr+msg, data...)
}

// SetLogger sets the logger for the fieldmask package.
func SetLogger(l Logger) {
	logger = l
}

// FieldMask is recursive structure to define a path mask
//
// Reference: https://protobuf.dev/reference/protobuf/google.protobuf/#json-encoding-field-masks
//
// Reference: https://github.com/protocolbuffers/protobuf/blob/main/src/google/protobuf/field_mask.proto
//
// For example, given the message:
//
//	f {
//	  b {
//	    d: 1
//	    x: 2
//	  }
//	  c: [1]
//	}
//
// then if the path is:
//
// paths: ["f.b.d"]
//
// then the result will be:
//
//	f {
//	  b {
//	    d: 10
//	  }
//	}
type FieldMask map[string]FieldMask

// Build populates a FieldMask from the input array of paths recursively.
// The array should contain JSON paths with dot "." notation.
func (fm FieldMask) Build(paths []string) {
	if len(paths) == 0 {
		return
	}

	fields := strings.Split(paths[0], ".")
	m := fm
	for _, field := range fields {
		if _, ok := m[field]; !ok {
			m[field] = FieldMask{}
		}
		m = m[field]
	}

	fm.Build(paths[1:]) //nolint:gosec // disable G602
}

// Filter takes a Proto message as input and updates the message according to the FieldMask.
func (fm FieldMask) Filter(message proto.Message) {
	if len(fm) == 0 {
		return
	}

	reflect := message.ProtoReflect()
	reflect.Range(func(fd protoreflect.FieldDescriptor, v protoreflect.Value) bool {
		mask, ok := fm[string(fd.Name())]
		if !ok {
			reflect.Clear(fd)
		}

		if len(mask) == 0 {
			return true
		}

		switch {
		case fd.IsMap():
			m := reflect.Get(fd).Map()
			m.Range(func(k protoreflect.MapKey, v protoreflect.Value) bool {
				if fm, ok := mask[k.String()]; ok {
					if i, ok := v.Interface().(protoreflect.Message); ok && len(fm) > 0 {
						fm.Filter(i.Interface())
					}
				} else {
					m.Clear(k)
				}
				return true
			})
		case fd.IsList():
			list := reflect.Get(fd).List()
			for i := 0; i < list.Len(); i++ {
				mask.Filter(list.Get(i).Message().Interface())
			}
		case fd.Kind() == protoreflect.MessageKind:
			mask.Filter(reflect.Get(fd).Message().Interface())
		case fd.Kind() == protoreflect.BytesKind:
			if b := v.Bytes(); gjson.ValidBytes(b) {
				b, err := jsoniter.Marshal(mask.FilterJSON(b, []string{}))
				if err == nil {
					reflect.Set(fd, protoreflect.ValueOfBytes(b))
				}
			}
		default:
			// TODO: We can configure zap logger from main instead of using default logger.
			logger.Error(context.Background(), "unsupported field type: %s", fd.Kind())
		}
		return true
	})
}

// Paths return the dot "." JSON notation os all the paths in the FieldMask.
// Parameter root []string is used internally for recursion, but it can also be used for setting an initial root path.
func (fm FieldMask) Paths(path []string) (paths []string) {
	for k, v := range fm {
		path = append(path, k)
		if len(v) == 0 {
			paths = append(paths, strings.Join(path, "."))
		}
		paths = append(paths, v.Paths(path)...)
		path = path[:len(path)-1]
	}
	return
}

// FilterJSON takes a JSON as input and return a map of the filtered JSON according to the FieldMask.
func (fm FieldMask) FilterJSON(json []byte, path []string) (out map[string]any) {
	for k, v := range fm {
		if out == nil {
			out = make(map[string]interface{})
		}
		path = append(path, k)
		if len(v) == 0 {
			out[k] = gjson.GetBytes(json, strings.Join(path, ".")).Value()
		} else {
			out[k] = v.FilterJSON(json, path)
		}
		path = path[:len(path)-1]
	}
	return
}

// FromMetadata gets all the filter definitions from gRPC metadata.
func FromMetadata(md metadata.MD) FieldMask {
	fm := &fieldmaskpb.FieldMask{}
	masks := md.Get(key)
	for _, mask := range masks {
		paths := strings.Split(mask, ",")
		for _, path := range paths {
			fm.Paths = append(fm.Paths, strings.TrimSpace(path))
		}
	}
	fm.Normalize()
	m := FieldMask{}
	m.Build(fm.Paths)
	return m
}

// MetadataAnnotator injects key from query parameter to gRPC metadata (for REST client).
func MetadataAnnotator(_ context.Context, req *http.Request) metadata.MD {
	if err := req.ParseForm(); err == nil && req.Form.Has(key) {
		return metadata.Pairs(key, req.Form.Get(key))
	}
	return nil
}

// UnaryServerInterceptor updates the response message according to the FieldMask.
func UnaryServerInterceptor(enabled *atomic.Bool) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, _ *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		resp, err := handler(ctx, req)
		if err != nil || !enabled.Load() {
			return resp, err
		}

		message, ok := resp.(proto.Message)
		if !ok {
			return resp, err
		}

		md, ok := metadata.FromIncomingContext(ctx)
		if !ok {
			return resp, err
		}

		fm := FromMetadata(md)
		fm.Filter(message)
		return resp, err
	}
}
