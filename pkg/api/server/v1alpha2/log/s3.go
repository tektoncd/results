package log

import (
	"bufio"
	"bytes"
	"path/filepath"

	"context"
	"fmt"
	"io"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/feature/s3/manager"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/emicklei/go-restful/log"
	"github.com/tektoncd/results/pkg/apis/v1alpha2"
	"github.com/tektoncd/results/pkg/conf"
)

const (
	DefaultS3MultiPartSize = 1024 * 1024 * 5
)

type S3LogStreamer struct {
	LogStreamer
	io.WriterTo
	io.ReaderFrom
	Flushable

	conf *conf.ConfigFile
	ctx  context.Context

	bufSize       int
	contentBuffer bytes.Buffer

	client          *s3.Client
	uploader        *manager.Uploader
	virtFilePath    string
	amountBytes     int64
	multiPartNumber int32 // no more then 10000 S3 api limitation.
	uploadId        *string
	parts           []types.CompletedPart
	multiPartSize   int64
}

func NewS3LogStreamer(trl *v1alpha2.TaskRunLog, bufSize int, conf *conf.ConfigFile, logDataDir string, ctx context.Context) (LogStreamer, error) {
	if trl.Status.File == nil {
		trl.Status.S3Log = &v1alpha2.S3LogStatus{
			Path: defaultFilePath(trl),
		}
	}

	virtualFilePath := filepath.Join(logDataDir, trl.Status.S3Log.Path)
	client, err := initConfig(conf, ctx)
	if err != nil {
		return nil, err
	}
	uploader := manager.NewUploader(client)
	out, err := uploader.S3.CreateMultipartUpload(ctx,
		&s3.CreateMultipartUploadInput{
			Bucket: &conf.S3_BUCKET_NAME,
			Key:    &virtualFilePath,
		},
	)
	if err != nil {
		return nil, err
	}
	uploadId := out.UploadId

	multiPartSize := conf.S3_MULTI_PART_SIZE
	if multiPartSize == 0 {
		multiPartSize = DefaultS3MultiPartSize
	}

	ls := &S3LogStreamer{
		conf:            conf,
		ctx:             ctx,
		bufSize:         bufSize,
		virtFilePath:    virtualFilePath,
		contentBuffer:   bytes.Buffer{},
		uploadId:        uploadId,
		client:          client,
		uploader:        uploader,
		multiPartNumber: 1,
		multiPartSize:   multiPartSize,
	}

	return ls, nil
}

func initConfig(conf *conf.ConfigFile, ctx context.Context) (*s3.Client, error) {
	credentialsOpt := config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(conf.S3_ACCESS_KEY_ID, conf.S3_SECRET_ACCESS_KEY, ""))

	var cfg aws.Config
	var err error
	if len(conf.S3_ENDPOINT) > 0 {
		customResolver := aws.EndpointResolverWithOptionsFunc(func(service, region string, options ...interface{}) (aws.Endpoint, error) {
			if region == conf.S3_REGION {
				return aws.Endpoint{
					URL:               conf.S3_ENDPOINT,
					SigningRegion:     conf.S3_REGION,
					HostnameImmutable: conf.S3_HOSTNAME_IMMUTABLE,
				}, nil
			}
			return aws.Endpoint{}, &aws.EndpointNotFoundError{}
		})
		cfg, err = config.LoadDefaultConfig(ctx, config.WithRegion(conf.S3_REGION), credentialsOpt, config.WithEndpointResolverWithOptions(customResolver))
	} else {
		cfg, err = config.LoadDefaultConfig(ctx, config.WithRegion(conf.S3_REGION), credentialsOpt)
	}

	if err != nil {
		return nil, err
	}

	return s3.NewFromConfig(cfg), nil
}

func (*S3LogStreamer) Type() string {
	return string(v1alpha2.S3LogType)
}

func (ls *S3LogStreamer) WriteTo(w io.Writer) (n int64, err error) {
	outPut, err := ls.client.GetObject(ls.ctx, &s3.GetObjectInput{
		Bucket: &ls.conf.S3_BUCKET_NAME,
		Key:    &ls.virtFilePath,
	})
	if err != nil {
		return 0, fmt.Errorf(err.Error())
	}

	defer outPut.Body.Close()

	reader := bufio.NewReaderSize(outPut.Body, ls.bufSize)
	n, err = reader.WriteTo(w)
	if err != nil {
		return 0, fmt.Errorf(err.Error())
	}
	return
}

func (ls *S3LogStreamer) ReadFrom(r io.Reader) (int64, error) {
	n, err := ls.contentBuffer.ReadFrom(r)
	if err != nil {
		return 0, err
	}

	totalBytes := ls.amountBytes + n
	if totalBytes >= ls.multiPartSize {
		err = ls.SendMultiPart(&ls.contentBuffer, ls.multiPartNumber)
		if err != nil {
			return 0, err
		}
		// Reset buffer for new bytes portion
		ls.amountBytes = 0
		ls.contentBuffer.Reset()
	} else {
		ls.amountBytes = totalBytes
	}

	return ls.amountBytes, err
}

func (ls *S3LogStreamer) SendMultiPart(reader io.Reader, partNumber int32) error {
	part, err := ls.uploader.S3.UploadPart(ls.ctx,
		&s3.UploadPartInput{
			UploadId:   ls.uploadId,
			Bucket:     &ls.conf.S3_BUCKET_NAME,
			Key:        &ls.virtFilePath,
			PartNumber: partNumber,
			Body:       reader,
		})

	if err != nil {
		log.Printf("failed to upload part number %d with id: %s. Let's try to cancel it.", partNumber, ls.uploadId)
		_, err = ls.uploader.S3.AbortMultipartUpload(ls.ctx,
			&s3.AbortMultipartUploadInput{
				Bucket:   &ls.conf.S3_BUCKET_NAME,
				Key:      &ls.virtFilePath,
				UploadId: ls.uploadId,
			})
		return err
	}
	ls.multiPartNumber += 1
	ls.parts = append(ls.parts, types.CompletedPart{PartNumber: partNumber, ETag: part.ETag})
	return err
}

func (ls *S3LogStreamer) Flush() error {
	// send remaining bytes
	if err := ls.SendMultiPart(&ls.contentBuffer, ls.multiPartNumber); err != nil {
		return err
	}

	_, err := ls.uploader.S3.CompleteMultipartUpload(ls.ctx,
		&s3.CompleteMultipartUploadInput{
			Bucket:   &ls.conf.S3_BUCKET_NAME,
			Key:      &ls.virtFilePath,
			UploadId: ls.uploadId,
			MultipartUpload: &types.CompletedMultipartUpload{
				Parts: ls.parts,
			},
		},
	)
	return err
}
