package logs

import (
	"bytes"

	pb "github.com/tektoncd/results/proto/v1alpha2/results_go_proto"
	"google.golang.org/genproto/googleapis/api/httpbody"
)

const (
	// DefaultBufferSize is the default buffer size. This based on the recommended
	// gRPC message size for streamed content, which ranges from 16 to 64 KiB. Choosing 32 KiB as a
	// middle ground between the two.
	DefaultBufferSize = 64 * 1024
)

// LogSender is an interface that defines the contract for sending log data.
type LogSender interface {
	Send(*pb.Log) error
}

// HTTPSender is an interface that defines the contract for sending log data in HTTP body.
type HTTPSender interface {
	Send(*httpbody.HttpBody) error
}

// BufferedLog is in memory buffered log sender.
type BufferedLog struct {
	sender any
	name   string
	size   int
	buffer bytes.Buffer
}

// NewBufferedWriter returns an io.Writer that writes log chunk messages to the gRPC sender for the
// named Tekton result. The chunk size determines the maximum size of a single sent message - if
// less than zero, this defaults to DefaultBufferSize.
func NewBufferedWriter(sender LogSender, name string, size int) *BufferedLog {
	if size < 1 {
		size = DefaultBufferSize
	}

	return &BufferedLog{
		sender: sender,
		name:   name,
		size:   size,
		buffer: *bytes.NewBuffer(make([]byte, 0)),
	}
}

// NewBufferedHTTPWriter returns an io.Writer that writes log chunk messages to the gRPC sender for the
// named Tekton result. The chunk size determines the maximum size of a single sent message - if
// less than zero, this defaults to DefaultBufferSize.
func NewBufferedHTTPWriter(sender HTTPSender, name string, size int) *BufferedLog {
	if size < 1 {
		size = DefaultBufferSize
	}

	return &BufferedLog{
		sender: sender,
		name:   name,
		size:   size,
		buffer: *bytes.NewBuffer(make([]byte, 0)),
	}
}

// Write sends bytes to the buffer and/or consumer (e.g., gRPC stream).
// This method combines the bytes from the buffer with a new portion of p bytes in memory.
// Bytes larger than the buffer size will be truncated and sent to the consumer,
// while the remaining bytes will be stored in the buffer.
func (w *BufferedLog) Write(p []byte) (n int, err error) {
	allBts := w.buffer.Bytes()
	allBts = append(allBts, p...)

	btsLength := len(allBts)
	remainBytes := btsLength % w.size

	amountChunks := (btsLength - remainBytes) / w.size

	for i := 0; i < amountChunks; i++ {
		offSet := i * w.size
		_, err = w.sendBytes(allBts[offSet : offSet+w.size])
		if err != nil {
			return 0, err
		}
	}

	var b []byte
	if remainBytes > 0 {
		b = allBts[(amountChunks * w.size):]
	}

	w.buffer.Reset()

	if _, err = w.buffer.Write(b); err != nil {
		return 0, err
	}

	return len(p), err
}

// Flush sends all remaining bytes in the buffer to consumer.
func (w *BufferedLog) Flush() (int, error) {
	if len(w.buffer.Bytes()) > 0 {
		return w.sendBytes(w.buffer.Bytes())
	}
	return 0, nil
}

// sendBytes sends the provided byte array over gRPC.
func (w *BufferedLog) sendBytes(p []byte) (int, error) {
	var err error
	switch t := w.sender.(type) {
	case HTTPSender:
		err = t.Send(&httpbody.HttpBody{
			ContentType: "text/plain",
			Data:        p,
		})
	case LogSender:
		err = t.Send(&pb.Log{
			Name: w.name,
			Data: p,
		})
	}
	if err != nil {
		return 0, err
	}
	return len(p), nil
}
