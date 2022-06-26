package taskrun

import (
	"context"
	"time"

	"github.com/tektoncd/pipeline/pkg/client/clientset/versioned"
	"github.com/tektoncd/pipeline/pkg/client/listers/pipeline/v1beta1"
	"github.com/tektoncd/results/pkg/watcher/reconciler"
	"github.com/tektoncd/results/pkg/watcher/reconciler/dynamic"
	pb "github.com/tektoncd/results/proto/v1alpha2/results_go_proto"
	"go.uber.org/zap"
	"k8s.io/client-go/tools/cache"
	"knative.dev/pkg/logging"
)

type Reconciler struct {
	client         pb.ResultsClient
	lister         v1beta1.TaskRunLister
	pipelineClient versioned.Interface
	cfg            *reconciler.Config
	enqueue        func(interface{}, time.Duration)
}

func (r *Reconciler) Reconcile(ctx context.Context, key string) error {
	log := logging.FromContext(ctx)
	log.With(zap.String("key", key))

	namespace, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		log.Errorf("invalid resource key: %s", key)
		return nil
	}
	pr, err := r.lister.TaskRuns(namespace).Get(name)
	if err != nil {
		return err
	}

	k8sclient := &dynamic.TaskRunClient{
		TaskRunInterface: r.pipelineClient.TektonV1beta1().TaskRuns(namespace),
	}

	dyn := dynamic.NewDynamicReconciler(r.client, k8sclient, r.cfg, r.enqueue)
	if err := dyn.Reconcile(ctx, pr); err != nil {
		return err
	}

	return nil
}
