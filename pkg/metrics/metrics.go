package metrics

import (
	"context"
	"fmt"
	"sync"
	"time"

	"go.opencensus.io/stats"
	"go.opencensus.io/stats/view"
	"go.opencensus.io/tag"
	"go.uber.org/zap"
	"knative.dev/pkg/metrics"
)

var (
	registerMutex sync.Mutex = sync.Mutex{}

	registeredAt       *time.Time
	runsNotStoredCount = stats.Int64("runs_not_stored_count", "total number of runs which were deleted without being stored", stats.UnitDimensionless)
	runsNotStoredView  *view.View

	namespaceTag = tag.MustNewKey("namespace")
	kindTag      = tag.MustNewKey("kind")
)

func registerViews(logger *zap.SugaredLogger) error {
	runsNotStoredView = &view.View{
		Description: runsNotStoredCount.Description(),
		TagKeys:     []tag.Key{kindTag, namespaceTag},
		Measure:     runsNotStoredCount,
		Aggregation: view.Count(),
	}
	logger.Debug("registering shared metrics views")
	return view.Register(runsNotStoredView)
}

func unregisterViews(logger *zap.SugaredLogger) {
	logger.Debug("unregistering pipelinerun metrics view")
	if registeredAt != nil {
		view.Unregister(runsNotStoredView)
		registeredAt = nil
	}
}

// IdempotentRegisterViews Ensures all shared views are registered.
// Does not unregister views if they have already been registered.
func IdempotentRegisterViews(logger *zap.SugaredLogger) {
	registerMutex.Lock()
	defer registerMutex.Unlock()
	if registeredAt != nil {
		return
	}
	unregisterViews(logger)
	if err := registerViews(logger); err != nil {
		logger.Errorf("Failed to register View %v ", err)
	} else {
		now := time.Now()
		registeredAt = &now
	}
}

func CountRunNotStored(ctx context.Context, namespace, kind string) error {
	ctx, err := tag.New(
		ctx,
		tag.Insert(kindTag, kind),
		tag.Insert(namespaceTag, namespace),
	)
	if err != nil {
		return fmt.Errorf("unable to create tags for %s metric: %w", runsNotStoredCount.Name(), err)
	}

	metrics.Record(ctx, runsNotStoredCount.M(1))
	return nil
}
