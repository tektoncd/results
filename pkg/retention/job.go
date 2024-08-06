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

package retention

import (
	"time"

	"github.com/robfig/cron/v3"
	"github.com/tektoncd/results/pkg/api/server/db"
	"github.com/tektoncd/results/pkg/api/server/db/errors"
)

func (a *Agent) start() {
	c := cron.New()
	_, err := c.AddFunc(a.RunAt, a.job)
	if err != nil {
		a.Logger.Fatalf("failed to add function for cronjob %s", err.Error())
	}
	a.cron = c
	a.cron.Start()
}

func (a *Agent) stop() {
	if a.cron == nil {
		return
	}
	a.cron.Stop()
}

func (a *Agent) job() {
	now := time.Now().Add(-a.MaxRetention)
	a.Logger.Infof("retention job started at: %s, deleting data older than %s, retention policy: %s",
		time.Now().String(), now, a.RetentionPolicy)

	q := a.db.Unscoped().
		Where("updated_time < ?", now).
		Delete(&db.Record{})
	if err := errors.Wrap(q.Error); err != nil {
		a.Logger.Errorf("failed to delete record %s", err.Error())
	}
	q = a.db.Unscoped().
		Where("updated_time < ?", now).
		Delete(&db.Result{})
	if err := errors.Wrap(q.Error); err != nil {
		a.Logger.Errorf("failed to delete result %s", err.Error())
	}

	a.Logger.Infof("retention job finished at: %s", time.Now().String())

}
