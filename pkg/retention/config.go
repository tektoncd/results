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
	"fmt"
	"log"
	"sync"

	"github.com/tektoncd/results/pkg/apis/config"
	"go.uber.org/zap"
	"gorm.io/gorm"

	_ "knative.dev/pkg/client/injection/kube/client"

	"github.com/robfig/cron/v3"
	"knative.dev/pkg/controller"
	"knative.dev/pkg/injection"
	"knative.dev/pkg/injection/sharedmain"
	"knative.dev/pkg/logging"
	"knative.dev/pkg/signals"
)

const (
	// ResultsRetentionPolicyAgent is the name of the logger for the retention policy agent cmd
	ResultsRetentionPolicyAgent = "results-retention-policy-agent"
)

type Agent struct {
	config.RetentionPolicy

	mutex sync.Mutex

	Logger *zap.SugaredLogger

	db *gorm.DB

	cron *cron.Cron
}

func NewAgent(db *gorm.DB) (*Agent, error) {
	cfg := injection.ParseAndGetRESTConfigOrDie()
	ctx := signals.NewContext()
	ctx = injection.WithConfig(ctx, cfg)
	ctx, informers := injection.Default.SetupInformers(ctx, cfg)
	if err := controller.StartInformers(ctx.Done(), informers...); err != nil {
		log.Fatal("failed to start informers:", err)
	}
	fmt.Println("test 1")
	logger := logging.FromContext(ctx)
	fmt.Println("test 3")
	ctx = logging.WithLogger(ctx, logger)
	fmt.Println("test 5")

	cmw := sharedmain.SetupConfigMapWatchOrDie(ctx, logger)
	fmt.Println("test 7")
	agent := Agent{
		Logger: logger,
		db:     db,
	}
	configStore := config.NewStore(logger.Named("config-store"), agent.AgentOnStore(logger))
	configStore.WatchConfigs(cmw)
	if err := cmw.Start(ctx.Done()); err != nil {
		logger.Fatalw("Failed to start configuration manager", zap.Error(err))
	}
	logger.Infof("%+v", agent)
	return &agent, nil
}

// AgentOnStore returns a function that checks if agent are configured for a config.Store, and registers it if so
func (a *Agent) AgentOnStore(logger *zap.SugaredLogger) func(name string,
	value interface{}) {
	return func(name string, value interface{}) {
		if name == config.GetRetentionPolicyConfigName() {
			cfg, ok := value.(*config.RetentionPolicy)
			if !ok {
				logger.Error("Failed to do type insertion for extracting retention policy config")
				return
			}
			a.setAgentConfig(cfg)
			a.stop()
			a.start()
			logger.Infof("%+v", a)
		}
	}
}

func (a *Agent) setAgentConfig(cfg *config.RetentionPolicy) {
	a.mutex.Lock()
	defer a.mutex.Unlock()
	a.RetentionPolicy = *cfg
}
