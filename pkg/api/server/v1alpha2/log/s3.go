package log

import (
	"bufio"
	"bytes"
	"path/filepath"

	v4 "github.com/aws/aws-sdk-go-v2/aws/signer/v4"

	"context"
	"fmt"
	"io"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	server "github.com/tektoncd/results/pkg/api/server/config"
	"github.com/tektoncd/results/pkg/apis/v1alpha2"
)

const (
	defaultS3MultiPartSize = 1024 * 1024 * 5
)

type s3Client interface {
	AbortMultipartUpload(context.Context, *s3.AbortMultipartUploadInput, ...func(*s3.Options)) (*s3.AbortMultipartUploadOutput, error)
	CompleteMultipartUpload(context.Context, *s3.CompleteMultipartUploadInput, ...func(*s3.Options)) (*s3.CompleteMultipartUploadOutput, error)
	CreateMultipartUpload(context.Context, *s3.CreateMultipartUploadInput, ...func(*s3.Options)) (*s3.CreateMultipartUploadOutput, error)
	DeleteObject(context.Context, *s3.DeleteObjectInput, ...func(*s3.Options)) (*s3.DeleteObjectOutput, error)
	GetObject(context.Context, *s3.GetObjectInput, ...func(*s3.Options)) (*s3.GetObjectOutput, error)
	UploadPart(context.Context, *s3.UploadPartInput, ...func(*s3.Options)) (*s3.UploadPartOutput, error)
}

type s3Stream struct {
	config        *server.Config
	ctx           context.Context
	size          int
	buffer        bytes.Buffer
	client        s3Client
	bucket        string
	key           string
	partNumber    int32
	partSize      int64
	uploadID      string
	parts         []types.CompletedPart
	multiPartSize int64
}

// NewS3Stream returns a log streamer for the S3 log storage type.
func NewS3Stream(ctx context.Context, log *v1alpha2.Log, config *server.Config) (Stream, error) {
	if log.Status.Path == "" {
		filePath, err := FilePath(log)
		if err != nil {
			return nil, err
		}
		log.Status.Path = filePath
	}

	filePath := filepath.Join(config.LOGS_PATH, log.Status.Path)

	client, err := initConfig(ctx, config)
	if err != nil {
		return nil, err
	}

	multipartUpload, err := client.CreateMultipartUpload(ctx,
		&s3.CreateMultipartUploadInput{
			Bucket: &config.S3_BUCKET_NAME,
			Key:    &filePath,
		},
	)
	if err != nil {
		return nil, err
	}

	multiPartSize := config.S3_MULTI_PART_SIZE
	if multiPartSize == 0 {
		multiPartSize = defaultS3MultiPartSize
	}

	size := config.LOGS_BUFFER_SIZE
	if size < 1 {
		size = DefaultBufferSize
	}

	s3s := &s3Stream{
		config:        config,
		ctx:           ctx,
		size:          size,
		bucket:        config.S3_BUCKET_NAME,
		key:           filePath,
		buffer:        bytes.Buffer{},
		uploadID:      *multipartUpload.UploadId,
		client:        client,
		partNumber:    1,
		multiPartSize: multiPartSize,
	}

	return s3s, nil
}

func initConfig(ctx context.Context, cfg *server.Config) (*s3.Client, error) {
	credentialsOpt := config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(cfg.S3_ACCESS_KEY_ID, cfg.S3_SECRET_ACCESS_KEY, ""))

	var awsConfig aws.Config
	var err error
	if len(cfg.S3_ENDPOINT) > 0 {
		customResolver := aws.EndpointResolverWithOptionsFunc(func(service, region string, options ...any) (aws.Endpoint, error) {
			if region == cfg.S3_REGION {
				return aws.Endpoint{
					URL:               cfg.S3_ENDPOINT,
					SigningRegion:     cfg.S3_REGION,
					HostnameImmutable: cfg.S3_HOSTNAME_IMMUTABLE,
				}, nil
			}
			return aws.Endpoint{}, &aws.EndpointNotFoundError{}
		})
		awsConfig, err = config.LoadDefaultConfig(ctx, config.WithRegion(cfg.S3_REGION), credentialsOpt, config.WithEndpointResolverWithOptions(customResolver))
	} else {
		awsConfig, err = config.LoadDefaultConfig(ctx, config.WithRegion(cfg.S3_REGION), credentialsOpt)
	}

	if err != nil {
		return nil, err
	}

	return s3.NewFromConfig(awsConfig), nil
}

func (*s3Stream) Type() string {
	return string(v1alpha2.S3LogType)
}

func (s3s *s3Stream) WriteTo(w io.Writer) (n int64, err error) {
	outPut, err := s3s.client.GetObject(s3s.ctx, &s3.GetObjectInput{
		Bucket: &s3s.bucket,
		Key:    &s3s.key,
	})
	if err != nil {
		return 0, fmt.Errorf(err.Error())
	}

	defer outPut.Body.Close()

	reader := bufio.NewReaderSize(outPut.Body, s3s.size)
	n, err = reader.WriteTo(w)
	if err != nil {
		return 0, fmt.Errorf(err.Error())
	}
	return
}

func (s3s *s3Stream) ReadFrom(r io.Reader) (int64, error) {
	n, err := s3s.buffer.ReadFrom(r)
	if err != nil {
		return 0, err
	}

	size := s3s.partSize + n
	if size >= s3s.multiPartSize {
		err = s3s.uploadMultiPart(&s3s.buffer, s3s.partNumber, n)
		if err != nil {
			return 0, err
		}
		s3s.partSize = 0
		s3s.buffer.Reset()
	} else {
		s3s.partSize = size
	}

	return s3s.partSize, err
}

func (s3s *s3Stream) uploadMultiPart(reader io.Reader, partNumber int32, partSize int64) error {
	part, err := s3s.client.UploadPart(s3s.ctx, &s3.UploadPartInput{
		UploadId:      &s3s.uploadID,
		Bucket:        &s3s.bucket,
		Key:           &s3s.key,
		PartNumber:    partNumber,
		Body:          reader,
		ContentLength: partSize,
	}, s3.WithAPIOptions(
		v4.SwapComputePayloadSHA256ForUnsignedPayloadMiddleware,
	))

	if err != nil {
		s3s.client.AbortMultipartUpload(s3s.ctx, &s3.AbortMultipartUploadInput{ //nolint:errcheck
			Bucket:   &s3s.bucket,
			Key:      &s3s.key,
			UploadId: &s3s.uploadID,
		})
		return err
	}

	s3s.parts = append(s3s.parts, types.CompletedPart{PartNumber: partNumber, ETag: part.ETag})
	s3s.partNumber++

	return err
}

func (s3s *s3Stream) Flush() error {
	if err := s3s.uploadMultiPart(&s3s.buffer, s3s.partNumber, int64(s3s.buffer.Len())); err != nil {
		return err
	}

	_, err := s3s.client.CompleteMultipartUpload(s3s.ctx, &s3.CompleteMultipartUploadInput{
		Bucket:   &s3s.bucket,
		Key:      &s3s.key,
		UploadId: &s3s.uploadID,
		MultipartUpload: &types.CompletedMultipartUpload{
			Parts: s3s.parts,
		},
	})
	return err
}

func (s3s *s3Stream) Delete() error {
	_, err := s3s.client.DeleteObject(s3s.ctx, &s3.DeleteObjectInput{
		Bucket: &s3s.bucket,
		Key:    &s3s.key,
	})
	return err
}
