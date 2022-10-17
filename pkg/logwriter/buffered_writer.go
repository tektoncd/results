package log

import (
	"bytes"

	pb "github.com/tektoncd/results/proto/v1alpha2/results_go_proto"
)

const (
	// DefaultMaxLogChunkSize is the default maximum log chunk size. This based on the recommended
	// gRPC message size for streamed content, which ranges from 16 to 64 KiB. Choosing 32 KiB as a
	// middle ground between the two.
	DefaultMaxLogChunkSize = 32 * 1024
)

type Sender interface {
	Send(chunk *pb.Log) error
}

type bufferedLog struct {
	sender     Sender
	recordName string
	chunkSize  int
	buffer     bytes.Buffer
}

// NewBufferedLogWriter returns an io.Writer that writes log chunk messages to the gRPC sender for the
// named Tekton result. The chunk size determines the maximum size of a single sent message - if
// less than zero, this defaults to DefaultMaxLogChunkSize.
func NewBufferedLogWriter(sender Sender, recordName string, chunkSize int) *bufferedLog {
	if chunkSize < 1 {
		chunkSize = DefaultMaxLogChunkSize
	}
	return &bufferedLog{
		sender:     sender,
		recordName: recordName,
		chunkSize:  chunkSize,
		buffer:     *bytes.NewBuffer(make([]byte, 0)),
	}
}

func (w *bufferedLog) Write(p []byte) (n int, err error) {
	allBts := make([]byte, 0)
	allBts = append(allBts, w.buffer.Bytes()...)
	allBts = append(allBts, p...)

	btsLength := len(allBts)
	remainBytes := btsLength % w.chunkSize

	amountChunks := (btsLength - remainBytes) / w.chunkSize

	for i := 0; i < amountChunks; i++ {
		offSet := i * w.chunkSize
		_, err = w.sendBytes(allBts[offSet : offSet+w.chunkSize])
		if err != nil {
			return 0, err
		}
	}

	b := []byte{}
	if remainBytes > 0 {
		b = allBts[(amountChunks * w.chunkSize):]
	}

	w.buffer.Reset()

	if _, err = w.buffer.Write(b); err != nil {
		return 0, err
	}

	return len(p), err
}

func (w *bufferedLog) WriteRemain() (int, error) {
	if len(w.buffer.Bytes()) > 0 {
		return w.sendBytes(w.buffer.Bytes())
	}
	return 0, nil
}

// sendBytes sends the provided byte array over gRPC.
func (w *bufferedLog) sendBytes(p []byte) (int, error) {
	log := &pb.Log{
		Name: w.recordName,
		Data: p,
	}
	err := w.sender.Send(log)
	if err != nil {
		return 0, err
	}
	return len(p), nil
}
