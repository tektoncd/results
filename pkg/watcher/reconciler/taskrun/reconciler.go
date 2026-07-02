package taskrun

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/tektoncd/results/pkg/apis/config"
	"github.com/tektoncd/results/pkg/metrics"
	"github.com/tektoncd/results/pkg/taskrunmetrics"

	pipelinev1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1"
	"github.com/tektoncd/pipeline/pkg/client/clientset/versioned"
	taskrunreconciler "github.com/tektoncd/pipeline/pkg/client/injection/reconciler/pipeline/v1/taskrun"
	v1 "github.com/tektoncd/pipeline/pkg/client/listers/pipeline/v1"
	"github.com/tektoncd/results/pkg/watcher/reconciler"

	resultsannotation "github.com/tektoncd/results/pkg/watcher/reconciler/annotation"
	"github.com/tektoncd/results/pkg/watcher/reconciler/client"
	"github.com/tektoncd/results/pkg/watcher/reconciler/dynamic"
	"github.com/tektoncd/results/pkg/watcher/results"
	pb "github.com/tektoncd/results/proto/v1alpha2/results_go_proto"
	"go.uber.org/zap"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
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
	taskRunLister  v1.TaskRunLister
	pipelineClient versioned.Interface
	cfg            *reconciler.Config
	metrics        *metrics.Recorder
	taskRunMetrics *taskrunmetrics.Recorder
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

	taskRunClient := &client.TaskRunClient{
		TaskRunInterface: r.pipelineClient.TektonV1().TaskRuns(tr.Namespace),
	}

	dyn := dynamic.NewDynamicReconciler(r.kubeClientSet, r.resultsClient, r.logsClient, taskRunClient, r.cfg)
	dyn.AfterDeletion = func(ctx context.Context, object results.Object) error {
		tr, ok := object.(*pipelinev1.TaskRun)
		if !ok {
			return fmt.Errorf("expected TaskRun, got %T", object)
		}
		if err := r.taskRunMetrics.DurationAndCountDeleted(ctx, r.configStore.Load().Metrics, tr); err != nil {
			// Log but don't fail reconciliation for metrics issues
			logging.FromContext(ctx).Warnf("Failed to record taskrun deletion metrics: %v", err)
		}
		return nil
	}
	dyn.AfterStorage = func(ctx context.Context, o results.Object, _ bool) error {
		tr, ok := o.(*pipelinev1.TaskRun)
		if !ok {
			return fmt.Errorf("expected TaskRun, got %T", o)
		}
		return r.metrics.RecordStorageLatency(ctx, tr)
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
	if rerr != nil {
		// Keep requeue semantics in finalize() while ensuring this reconcile error is always visible.
		logging.FromContext(ctx).Warnw("reconcile during taskrun finalization returned error",
			zap.Error(rerr))
	}

	return r.finalize(ctx, tr, rerr)
}

func (r *Reconciler) finalize(ctx context.Context, tr *pipelinev1.TaskRun, rerr error) (result knativereconciler.Event) {
	// MIGRATION: When finalize decides to allow deletion (returns nil), check if the
	// finalizer was added via merge patch by the old controller version. SSA cannot
	// remove finalizers it doesn't own, so we remove it via merge patch ourselves.
	// This can be removed once all pre-SSA resources are deleted.
	defer func() {
		if result != nil {
			return
		}
		if !r.isFinalizerOwnedByMergePatch(tr) {
			return
		}
		logging.FromContext(ctx).Infof("Removing merge-patch finalizer on %s/%s for SSA migration",
			tr.Namespace, tr.Name)
		if err := r.removeFinalizerViaMergePatch(ctx, tr); err != nil {
			logging.FromContext(ctx).Warnw("Failed to remove finalizer via merge patch",
				zap.Error(err))
			result = controller.NewRequeueAfter(r.cfg.FinalizerRequeueInterval)
		}
	}()

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
				if err := metrics.CountRunNotStored(ctx, tr.Namespace, "TaskRun"); err != nil {
					logging.FromContext(ctx).Errorf("error counting TaskRun as not stored: %w", err)
				}
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
	if rerr != nil {
		return controller.NewRequeueAfter(r.cfg.FinalizerRequeueInterval)
	}
	if stored != "true" {
		logging.FromContext(ctx).Debugf("stored annotation is not true on taskrun %s/%s, now: %s, storeDeadline: %s",
			tr.Namespace, tr.Name, now.String(), storeDeadline.String())
		return controller.NewRequeueAfter(r.cfg.FinalizerRequeueInterval)
	}

	return nil
}

// isFinalizerOwnedByMergePatch checks if the finalizer was added via merge patch (Update operation).
// MIGRATION: This is a temporary migration feature to handle the upgrade scenario where
// in-flight TaskRuns have finalizers set via merge patch by the old controller version.
// Kubernetes SSA treats (manager, Update) and (manager, Apply) as different owners, so we need
// to detect and handle the old ownership pattern.
// This function can be removed once all resources from the pre-SSA version are deleted.
func (r *Reconciler) isFinalizerOwnedByMergePatch(tr *pipelinev1.TaskRun) bool {
	for _, mf := range tr.ManagedFields {
		// Check if this is from the old merge patch operation
		if mf.Operation == metav1.ManagedFieldsOperationUpdate {
			// Parse FieldsV1 to check if it owns the finalizers field
			// FieldsV1 is a JSON structure, we need to check if it contains f:metadata.f:finalizers
			if mf.FieldsV1 != nil && mf.FieldsV1.Raw != nil {
				// Check if this managed field entry owns finalizers AND specifically our finalizer
				if bytes.Contains(mf.FieldsV1.Raw, []byte(`"f:finalizers"`)) &&
					bytes.Contains(mf.FieldsV1.Raw, []byte(`v:\"results.tekton.dev/taskrun\"`)) {
					return true
				}
			}
		}
	}
	return false
}

// removeFinalizerViaMergePatch removes the finalizer using merge patch.
// MIGRATION: This is a temporary migration feature to handle the upgrade scenario where
// in-flight TaskRuns have finalizers set via merge patch by the old controller version.
// This uses merge patch to remove finalizers that cannot be removed via SSA due to different
// ownership (manager, Update) vs (manager, Apply).
// This function can be removed once all resources from the pre-SSA version are deleted.
func (r *Reconciler) removeFinalizerViaMergePatch(ctx context.Context, tr *pipelinev1.TaskRun) error {
	// Remove our finalizer from the list
	var newFinalizers []string
	for _, f := range tr.Finalizers {
		if f != "results.tekton.dev/taskrun" {
			newFinalizers = append(newFinalizers, f)
		}
	}

	mergePatch := map[string]interface{}{
		"metadata": map[string]interface{}{
			"finalizers":      newFinalizers,
			"resourceVersion": tr.ResourceVersion,
		},
	}

	patch, err := json.Marshal(mergePatch)
	if err != nil {
		return err
	}

	_, err = r.pipelineClient.TektonV1().TaskRuns(tr.Namespace).Patch(
		ctx, tr.Name, types.MergePatchType, patch, metav1.PatchOptions{})
	return err
}
