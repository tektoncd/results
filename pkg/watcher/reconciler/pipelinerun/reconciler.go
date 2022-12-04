package pipelinerun

import (
	"context"

	"github.com/tektoncd/pipeline/pkg/client/clientset/versioned"
	"github.com/tektoncd/pipeline/pkg/client/listers/pipeline/v1beta1"
	"github.com/tektoncd/results/pkg/watcher/reconciler"
	"github.com/tektoncd/results/pkg/watcher/reconciler/dynamic"
	pb "github.com/tektoncd/results/proto/v1alpha2/results_go_proto"
	"go.uber.org/zap"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/cache"
	"knative.dev/pkg/controller"
	"knative.dev/pkg/logging"
	knativereconciler "knative.dev/pkg/reconciler"
)

type Reconciler struct {
	// Inline LeaderAwareFuncs to support leader election.
	knativereconciler.LeaderAwareFuncs

	resultsClient pb.ResultsClient
	logsClient    pb.LogsClient
	lister        v1beta1.PipelineRunLister
	k8sclient     versioned.Interface
	cfg           *reconciler.Config
}

// Check that our Reconciler is LeaderAware.
var _ knativereconciler.LeaderAware = (*Reconciler)(nil)

func (r *Reconciler) Reconcile(ctx context.Context, key string) error {
	log := logging.FromContext(ctx).With(zap.String("results.tekton.dev/kind", "PipelineRun"))
	log.Info("Reconciling PipelineRun")

	namespace, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		log.Errorf("invalid resource key: %s", key)
		return nil
	}

	if !r.IsLeaderFor(types.NamespacedName{Namespace: namespace, Name: name}) {
		log.Debug("Skipping PipelineRun key because this instance isn't its leader")
		return controller.NewSkipKey(key)
	}

	pr, err := r.lister.PipelineRuns(namespace).Get(name)
	if err != nil {
		return err
	}

	k8sclient := &dynamic.PipelineRunClient{
		PipelineRunInterface: r.k8sclient.TektonV1beta1().PipelineRuns(namespace),
	}

	dyn := dynamic.NewDynamicReconciler(r.resultsClient, k8sclient, r.cfg)
	if err := dyn.Reconcile(logging.WithLogger(ctx, log), pr); err != nil {
		return err
	}

	return nil
}
