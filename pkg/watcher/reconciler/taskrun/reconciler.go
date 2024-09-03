package taskrun

import (
	"context"

	"github.com/tektoncd/results/pkg/apis/config"
	"github.com/tektoncd/results/pkg/taskrunmetrics"
	"github.com/tektoncd/results/pkg/watcher/results"

	pipelinev1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1"

	"github.com/tektoncd/pipeline/pkg/client/clientset/versioned"
	taskrunreconciler "github.com/tektoncd/pipeline/pkg/client/injection/reconciler/pipeline/v1/taskrun"
	v1 "github.com/tektoncd/pipeline/pkg/client/listers/pipeline/v1"
	"github.com/tektoncd/results/pkg/watcher/reconciler"
	"github.com/tektoncd/results/pkg/watcher/reconciler/dynamic"
	pb "github.com/tektoncd/results/proto/v1alpha2/results_go_proto"
	"go.uber.org/zap"
	"k8s.io/client-go/kubernetes"
	"knative.dev/pkg/logging"
	knativereconciler "knative.dev/pkg/reconciler"
)

// Reconciler represents taskRun watcher logic
type Reconciler struct {

	// kubeClientSet allows us to talk to the k8s for core APIs
	kubeClientSet kubernetes.Interface

	resultsClient  pb.ResultsClient
	logsClient     pb.LogsClient
	lister         v1.TaskRunLister
	pipelineClient versioned.Interface
	cfg            *reconciler.Config
	metrics        *taskrunmetrics.Recorder
	configStore    *config.Store
}

// Check that our Reconciler implements taskrunreconciler.Interface and taskrunreconciler.Finalizer
var _ taskrunreconciler.Interface = (*Reconciler)(nil)
var _ taskrunreconciler.Finalizer = (*Reconciler)(nil)

// ReconcileKind makes new watcher reconcile cycle to handle TaskRun.
func (r *Reconciler) ReconcileKind(ctx context.Context, tr *pipelinev1.TaskRun) knativereconciler.Event {
	logger := logging.FromContext(ctx).With(zap.String("results.tekton.dev/kind", "TaskRun"))

	taskRunClient := &dynamic.TaskRunClient{
		TaskRunInterface: r.pipelineClient.TektonV1().TaskRuns(tr.Namespace),
	}

	dyn := dynamic.NewDynamicReconciler(r.kubeClientSet, r.resultsClient, r.logsClient, taskRunClient, r.cfg)
	dyn.AfterDeletion = func(ctx context.Context, o results.Object) error {
		tr := o.(*pipelinev1.TaskRun)
		return r.metrics.DurationAndCountDeleted(ctx, r.configStore.Load().Metrics, tr)
	}
	return dyn.Reconcile(logging.WithLogger(ctx, logger), tr)
}

// FinalizeKind implements pipelinerunreconciler.Finalizer
// We utilize finalizers to ensure that we get a crack at storing every taskrun
// that we see flowing through the system.  If we don't add a finalizer, it could
// get cleaned up before we see the final state and store it.
func (r *Reconciler) FinalizeKind(ctx context.Context, tr *pipelinev1.TaskRun) knativereconciler.Event {
	return nil
}
