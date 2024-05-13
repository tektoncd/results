/*
Copyright 2023 The Tekton Authors

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package log

import (
	"path/filepath"

	"golang.org/x/oauth2/google"

	"context"
	"fmt"
	"io"
	"os"

	server "github.com/tektoncd/results/pkg/api/server/config"
	"github.com/tektoncd/results/pkg/apis/v1alpha3"

	"gocloud.dev/blob/gcsblob"
	"gocloud.dev/gcp"
)

type gcsStream struct {
	ctx    context.Context
	config *server.Config
	key    string
	client *gcp.HTTPClient
}

// NewGCSStream returns a log streamer for the GCS storage type.
func NewGCSStream(ctx context.Context, log *v1alpha3.Log, config *server.Config) (Stream, error) {
	if log.Status.Path == "" {
		filePath, err := FilePath(log)
		if err != nil {
			return nil, err
		}
		log.Status.Path = filePath
	}

	filePath := filepath.Join(config.LOGS_PATH, log.Status.Path)

	if config.STORAGE_EMULATOR_HOST != "" {
		os.Setenv("STORAGE_EMULATOR_HOST", config.STORAGE_EMULATOR_HOST)
	}
	client, err := getGCSClient(ctx, config)
	if err != nil {
		return nil, err
	}

	gcs := &gcsStream{
		ctx:    ctx,
		config: config,
		key:    filePath,
		client: client,
	}

	return gcs, nil
}

func getGCSClient(ctx context.Context, cfg *server.Config) (*gcp.HTTPClient, error) {
	var creds *google.Credentials

	if cfg.STORAGE_EMULATOR_HOST != "" {
		creds, _ = google.CredentialsFromJSON(ctx, []byte(`{"type": "service_account", "project_id": "my-project-id"}`))
	} else {
		var err error
		creds, err = gcp.DefaultCredentials(ctx)
		if err != nil {
			return nil, err
		}
	}

	return gcp.NewHTTPClient(
		gcp.DefaultTransport(),
		gcp.CredentialsTokenSource(creds))
}

func (*gcsStream) Type() string {
	return string(v1alpha3.GCSLogType)
}

func (gcs *gcsStream) WriteTo(w io.Writer) (int64, error) {
	bucket, err := gcsblob.OpenBucket(gcs.ctx, gcs.client, gcs.config.GCS_BUCKET_NAME, nil)
	if err != nil {
		return 0, fmt.Errorf("could not open bucket: %v", err)
	}
	defer bucket.Close()

	r, err := bucket.NewReader(gcs.ctx, gcs.key, nil)
	if err != nil {
		return 0, fmt.Errorf("could not create bucket reader: %v for the key: %s", err, gcs.key)
	}
	n, err := r.WriteTo(w)
	if err != nil {
		return 0, fmt.Errorf("could not read data from bucket: %v for the key: %s", err, gcs.key)
	}
	return n, nil
}

func (gcs *gcsStream) ReadFrom(r io.Reader) (n int64, err error) {
	bucket, err := gcsblob.OpenBucket(gcs.ctx, gcs.client, gcs.config.GCS_BUCKET_NAME, nil)
	if err != nil {
		return 0, fmt.Errorf("could not open bucket: %v for the key: %s", err, gcs.key)
	}
	defer bucket.Close()

	w, err := bucket.NewWriter(gcs.ctx, gcs.key, nil)
	if err != nil {
		return 0, fmt.Errorf("could not create bucket writer: %v for the key: %s", err, gcs.key)
	}
	defer func() {
		err = w.Close()
		if err != nil {
			err = fmt.Errorf("could not flush data to bucket: %v for the key: %s", err, gcs.key)
		}
	}()

	n, err = w.ReadFrom(r)
	if err != nil {
		return 0, fmt.Errorf("could not write data to bucket: %v for the key: %s", err, gcs.key)
	}
	return n, nil
}

func (gcs *gcsStream) Flush() error {
	return nil
}

func (gcs *gcsStream) Delete() error {
	bucket, err := gcsblob.OpenBucket(gcs.ctx, gcs.client, gcs.config.GCS_BUCKET_NAME, nil)
	if err != nil {
		return fmt.Errorf("could not open bucket: %v for the key: %s", err, gcs.key)
	}
	defer bucket.Close()

	if err := bucket.Delete(gcs.ctx, gcs.key); err != nil {
		return fmt.Errorf("could not delete bucket data: %v for the key: %s", err, gcs.key)
	}
	return nil
}
