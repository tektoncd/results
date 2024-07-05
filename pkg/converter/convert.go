/*
Copyright 2024 The Tekton Authors

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

package converter

import (
	"context"
	"encoding/json"

	"github.com/google/martian/v3/log"
	v1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1"
	"github.com/tektoncd/pipeline/pkg/apis/pipeline/v1beta1"
	"github.com/tektoncd/results/pkg/api/server/db"
	"github.com/tektoncd/results/pkg/api/server/db/errors"
	"go.uber.org/zap"
	"gorm.io/gorm"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"knative.dev/pkg/logging"
)

// New provides an instance of converter which converts v1beta1 to v1 API in db
// revive:disable:unexported-return
func New(logger *zap.SugaredLogger, db *gorm.DB, limit int) *convertor {
	return &convertor{
		db:     db,
		logger: logger,
		limit:  limit,
	}
}

type convertor struct {
	logger *zap.SugaredLogger
	db     *gorm.DB
	limit  int
}

func (c *convertor) Start(ctx context.Context) {
	c.convert(ctx, "tekton.dev/v1beta1.TaskRun")
	c.convert(ctx, "tekton.dev/v1beta1.PipelineRun")
}

func (c *convertor) convert(ctx context.Context, recType string) {
	logger := logging.FromContext(ctx)
	var conv func(ctx context.Context, record *db.Record) error
	switch recType {
	case "tekton.dev/v1beta1.TaskRun":
		conv = c.convertTR
	case "tekton.dev/v1beta1.PipelineRun":
		conv = c.convertPR
	default:
		logger.Error("Incorrect Type of record for conversion")
	}
	pending := true
	for pending {
		records, err := c.getRecords(ctx, recType)
		if err != nil {
			logger.Errorf("failed to fetch records", err)
			continue
		}
		if len(records) == 0 {
			log.Infof("No %s in db", recType)
			pending = false
		}
		for i := range records {
			err := conv(ctx, &records[i])
			if err != nil {
				logger.Errorf("failed to convert record name:%s id: %s  err: %v", records[i].Name, records[i].ID, err.Error())
			}
		}
		err = c.setRecords(ctx, records)
		if err != nil {
			logger.Errorf("failed to set records %v", err.Error())
			continue
		}
	}
}

func (c *convertor) convertTR(ctx context.Context, record *db.Record) error {
	var tr v1beta1.TaskRun //nolint:staticcheck
	err := json.Unmarshal(record.Data, &tr)
	if err != nil {
		return err
	}

	trV1 := &v1.TaskRun{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "tekton.dev/v1",
			Kind:       "TaskRun"},
	}

	if err := tr.ConvertTo(ctx, trV1); err != nil {
		return err
	}

	data, _ := json.Marshal(trV1)
	record.Data = data
	record.Type = "tekton.dev/v1.TaskRun"
	return nil
}

func (c *convertor) convertPR(ctx context.Context, record *db.Record) error {
	var pr v1beta1.PipelineRun //nolint:staticcheck
	err := json.Unmarshal(record.Data, &pr)
	if err != nil {
		return err
	}

	trV1 := &v1.PipelineRun{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "tekton.dev/v1",
			Kind:       "PipelineRun"},
	}

	if err := pr.ConvertTo(ctx, trV1); err != nil {
		return err
	}

	data, _ := json.Marshal(trV1)
	record.Data = data
	record.Type = "tekton.dev/v1.PipelineRun"
	return nil
}

func (c *convertor) getRecords(ctx context.Context, recType string) ([]db.Record, error) {
	txn := c.db.WithContext(ctx)
	records := []db.Record{}
	q := txn.Limit(c.limit).Find(&records, &db.Record{Type: recType})
	if err := errors.Wrap(q.Error); err != nil {
		return records, err
	}
	return records, nil
}

func (c *convertor) setRecords(ctx context.Context, records []db.Record) error {
	var err error
	txn := c.db.WithContext(ctx)
	transaction := txn.Begin()
	for i := range records {
		transaction.Model(&records[i]).Updates(db.Record{Data: records[i].Data, Type: records[i].Type})
	}
	err = transaction.Commit().Error
	return err

}
