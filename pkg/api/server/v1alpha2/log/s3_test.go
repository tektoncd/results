package log

import (
	"bytes"
	"context"
	"io"
	"strconv"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/s3"
	server "github.com/tektoncd/results/pkg/api/server/config"
)

type mockS3Client struct {
	bucket     string
	key        string
	body       []byte
	uploadID   string
	partNumber int32
	t          *testing.T
}

func (m *mockS3Client) AbortMultipartUpload(ctx context.Context, params *s3.AbortMultipartUploadInput, optFns ...func(*s3.Options)) (*s3.AbortMultipartUploadOutput, error) { //nolint:revive
	m.checkParams(params.Bucket, params.Key)
	return &s3.AbortMultipartUploadOutput{}, nil
}

func (m *mockS3Client) CompleteMultipartUpload(ctx context.Context, params *s3.CompleteMultipartUploadInput, optFns ...func(*s3.Options)) (*s3.CompleteMultipartUploadOutput, error) { //nolint:revive
	m.checkParams(params.Bucket, params.Key)
	return &s3.CompleteMultipartUploadOutput{}, nil
}

func (m *mockS3Client) CreateMultipartUpload(ctx context.Context, params *s3.CreateMultipartUploadInput, optFns ...func(*s3.Options)) (*s3.CreateMultipartUploadOutput, error) { //nolint:revive
	m.checkParams(params.Bucket, params.Key)
	return &s3.CreateMultipartUploadOutput{UploadId: &m.uploadID}, nil
}

func (m *mockS3Client) DeleteObject(ctx context.Context, params *s3.DeleteObjectInput, optFns ...func(*s3.Options)) (*s3.DeleteObjectOutput, error) { //nolint:revive
	m.checkParams(params.Bucket, params.Key)
	dm := true
	return &s3.DeleteObjectOutput{DeleteMarker: &dm}, nil
}

func (m *mockS3Client) GetObject(ctx context.Context, params *s3.GetObjectInput, optFns ...func(*s3.Options)) (*s3.GetObjectOutput, error) { //nolint:revive
	m.checkParams(params.Bucket, params.Key)
	return &s3.GetObjectOutput{
		Body: io.NopCloser(bytes.NewReader(m.body)),
	}, nil
}

func (m *mockS3Client) UploadPart(ctx context.Context, params *s3.UploadPartInput, optFns ...func(*s3.Options)) (*s3.UploadPartOutput, error) { //nolint:revive
	buffer := bytes.Buffer{}
	_, err := buffer.ReadFrom(params.Body)
	if err != nil {
		m.t.Errorf("error uploading part: %d", params.PartNumber)
	}
	m.body = append(m.body, buffer.Bytes()...)
	e := strconv.Itoa(int(m.partNumber))
	return &s3.UploadPartOutput{ETag: &e}, nil
}

func (m *mockS3Client) checkParams(bucket, key *string) {
	m.t.Helper()
	if bucket == nil {
		m.t.Fatalf("bucket cannot be nil")
	}
	if *bucket != m.bucket {
		m.t.Fatalf("bucket not found! want: %s, got: %s", m.bucket, *bucket)
	}
	if key == nil {
		m.t.Fatalf("key cannot be nil")
	}
	if *key != m.key {
		m.t.Fatalf("key not found! want: %s, got: %s", m.key, *key)
	}
}

func TestS3Stream_WriteTo(t *testing.T) {
	want := "test data"
	c := &server.Config{
		S3_BUCKET_NAME: "test-bucket",
	}
	filePath := "test"
	s := &s3Stream{
		config: c,
		bucket: c.S3_BUCKET_NAME,
		key:    filePath,
		client: &mockS3Client{
			t:      t,
			bucket: c.S3_BUCKET_NAME,
			key:    filePath,
			body:   []byte(want),
		},
	}

	buffer := &bytes.Buffer{}
	_, err := s.WriteTo(buffer)
	if err != nil {
		t.Fatal(err)
	}
	if got := buffer.String(); got != want {
		t.Errorf("want: %s, got: %s", want, got)
	}
}

func TestS3Stream_ReadFrom(t *testing.T) {
	want := "test body of multi-part upload"
	const DefaultBufferSize = 10
	c := &server.Config{
		S3_BUCKET_NAME: "test-bucket",
	}
	filePath := "test"
	s := &s3Stream{
		config:     c,
		bucket:     c.S3_BUCKET_NAME,
		key:        filePath,
		size:       c.LOGS_BUFFER_SIZE,
		buffer:     bytes.Buffer{},
		partNumber: 1,
		client: &mockS3Client{
			t:          t,
			bucket:     c.S3_BUCKET_NAME,
			key:        filePath,
			partNumber: 1,
			uploadID:   "test-upload-id",
		},
	}

	u, err := s.client.CreateMultipartUpload(context.Background(), &s3.CreateMultipartUploadInput{
		Bucket: &s.config.S3_BUCKET_NAME,
		Key:    &filePath,
	})
	if err != nil {
		t.Fatal(err)
	}
	s.uploadID = *u.UploadId

	data := []byte(want)
	offset := 0
	for i := 1; i <= len(data)/DefaultBufferSize; i++ {
		part := data[offset:(i * DefaultBufferSize)]
		_, err = s.ReadFrom(bytes.NewReader(part))
		if err != nil {
			t.Fatal(err)
		}
		offset = i * DefaultBufferSize
	}
	err = s.Flush()
	if err != nil {
		t.Fatal(err)
	}

	out, err := s.client.GetObject(context.Background(), &s3.GetObjectInput{
		Bucket: &c.S3_BUCKET_NAME,
		Key:    &filePath,
	})
	if err != nil {
		t.Fatal(err)
	}
	buffer := bytes.Buffer{}
	_, err = buffer.ReadFrom(out.Body)
	if err != nil {
		t.Fatal(err)
	}
	got := buffer.String()
	if got != want {
		t.Errorf("want: %v, got: %v", want, got)
	}
}

func TestS3Stream_Delete(t *testing.T) {
	c := &server.Config{
		S3_BUCKET_NAME: "test-bucket",
	}
	filePath := "test"
	s := &s3Stream{
		config: c,
		bucket: c.S3_BUCKET_NAME,
		key:    filePath,
		client: &mockS3Client{
			t:      t,
			bucket: c.S3_BUCKET_NAME,
			key:    filePath,
		},
	}

	err := s.Delete()
	if err != nil {
		t.Error(err)
	}
}
