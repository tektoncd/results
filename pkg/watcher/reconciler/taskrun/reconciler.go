package taskrun

import (
	"context"
	"time"

	"github.com/tektoncd/results/pkg/apis/config"
	"github.com/tektoncd/results/pkg/taskrunmetrics"
	"github.com/tektoncd/results/pkg/watcher/results"

	pipelinev1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1"

	"github.com/tektoncd/pipeline/pkg/client/clientset/versioned"
	taskrunreconciler "github.com/tektoncd/pipeline/pkg/client/injection/reconciler/pipeline/v1/taskrun"
	v1 "github.com/tektoncd/pipeline/pkg/client/listers/pipeline/v1"
	"github.com/tektoncd/results/pkg/watcher/reconciler"
	resultsannotation "github.com/tektoncd/results/pkg/watcher/reconciler/annotation"
	"github.com/tektoncd/results/pkg/watcher/reconciler/dynamic"
	pb "github.com/tektoncd/results/proto/v1alpha2/results_go_proto"
	"go.uber.org/zap"
	"k8s.io/client-go/kubernetes"
	"knative.dev/pkg/controller"
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

	if r.cfg.DisableStoringIncompleteRuns {
		// Skip if taskrun is not done
		if !tr.IsDone() {
			logger.Debugf("taskrun %s/%s is not done and incomplete runs are disabled, skipping storing", tr.Namespace, tr.Name)
			return nil
		}

		// Skip if taskrun is already stored
		if tr.Annotations != nil && tr.Annotations[resultsannotation.Stored] == "true" {
			logger.Debugf("taskrun %s/%s is already stored, skipping", tr.Namespace, tr.Name)
			return nil
		}
	}

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
	// Reconcile the taskrun to ensure that it is stored in the database
	rerr := r.ReconcileKind(ctx, tr)

	return r.finalize(ctx, tr, rerr)
}

func (r *Reconciler) finalize(ctx context.Context, tr *pipelinev1.TaskRun, rerr error) knativereconciler.Event {
	// If logsClient isn't nil, it means we have logging storage enabled
	// and we can't use finalizers to coordinate deletion.
	if r.logsClient != nil {
		return nil
	}

	// If annotation update is disabled, we can't use finalizers to coordinate deletion.
	if r.cfg.DisableAnnotationUpdate {
		return nil
	}

	// Check the TaskRun has finished.
	if !tr.IsDone() {
		logging.FromContext(ctx).Debugf("taskrun %s/%s is still running", tr.Namespace, tr.Name)
		return nil
	}

	now := time.Now().UTC()

	// Check if the forwarding buffer is configured and passed
	if r.cfg.ForwardBuffer != nil {
		if tr.Status.CompletionTime == nil {
			logging.FromContext(ctx).Infof("removing finalizer without wait, no completion time set for taskrun %s/%s",
				tr.Namespace, tr.Name)
			return nil
		}
		buffer := tr.Status.CompletionTime.UTC().Add(*r.cfg.ForwardBuffer)
		if !now.After(buffer) {
			logging.FromContext(ctx).Debugf("log forwarding buffer wait for taskrun %s/%s", tr.Namespace, tr.Name)
			return controller.NewRequeueAfter(r.cfg.FinalizerRequeueInterval)
		}
	}

	var storeDeadline time.Time

	// Check if the store deadline is configured
	if r.cfg.StoreDeadline != nil {
		if tr.Status.CompletionTime == nil {
			logging.FromContext(ctx).Infof("removing finalizer without wait, no completion time set for taskrun %s/%s",
				tr.Namespace, tr.Name)
			return nil
		}
		storeDeadline = tr.Status.CompletionTime.UTC().Add(*r.cfg.StoreDeadline)
		if now.After(storeDeadline) {
			logging.FromContext(ctx).Debugf("store deadline: %s now: %s, completion time: %s", storeDeadline.String(), now.String(),
				tr.Status.CompletionTime.UTC().String())
			logging.FromContext(ctx).Debugf("store deadline has passed for taskrun %s/%s", tr.Namespace, tr.Name)
			_, ok := tr.Annotations[resultsannotation.Stored]
			if !ok {
				logging.FromContext(ctx).Errorf("taskrun not stored: %s/%s, uid: %s,",
					tr.Namespace, tr.Name, tr.UID)
			}
			return nil // Proceed with deletion
		}
	}

	if tr.Annotations == nil {
		logging.FromContext(ctx).Debugf("taskrun %s/%s annotations are missing, now: %s, storeDeadline: %s",
			tr.Namespace, tr.Name, now.String(), storeDeadline.String())
		return controller.NewRequeueAfter(r.cfg.FinalizerRequeueInterval)
	}

	stored, ok := tr.Annotations[resultsannotation.Stored]
	if !ok {
		logging.FromContext(ctx).Debugf("stored annotation is missing on taskrun %s/%s, now: %s, storeDeadline: %s",
			tr.Namespace, tr.Name, now.String(), storeDeadline.String())
		return controller.NewRequeueAfter(r.cfg.FinalizerRequeueInterval)
	}
	if rerr != nil || stored != "true" {
		logging.FromContext(ctx).Debugf("stored annotation is not true on taskrun %s/%s, now: %s, storeDeadline: %s",
			tr.Namespace, tr.Name, now.String(), storeDeadline.String())
		return controller.NewRequeueAfter(r.cfg.FinalizerRequeueInterval)
	}

	return nil
}
