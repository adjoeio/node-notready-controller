package notready

import (
	"context"
	"fmt"
	"time"

	controllerruntime "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	nodeclaimutil "sigs.k8s.io/karpenter/pkg/utils/nodeclaim"

	corev1 "k8s.io/api/core/v1"

	v1 "sigs.k8s.io/karpenter/pkg/apis/v1"
)

type Controller struct {
	kubeClient         client.Client
	unreachableTimeout time.Duration
}

func NewController(kubeClient client.Client, unreachableTimeout time.Duration) *Controller {
	return &Controller{
		kubeClient:         kubeClient,
		unreachableTimeout: unreachableTimeout,
	}
}

func (c *Controller) Reconcile(ctx context.Context, nodeClaim *v1.NodeClaim) (reconcile.Result, error) {
	node, err := nodeclaimutil.NodeForNodeClaim(ctx, c.kubeClient, nodeClaim)
	if err != nil {
		return reconcile.Result{}, nodeclaimutil.IgnoreDuplicateNodeError(nodeclaimutil.IgnoreNodeNotFoundError(err))
	}

	var zero int64 = 0
	deletOpts := client.DeleteOptions{
		GracePeriodSeconds: &zero,
	}

	log.FromContext(ctx).V(0).Info("Checking Node taints")
	for _, taint := range node.Spec.Taints {
		if taint.Key != corev1.TaintNodeUnreachable {
			continue
		}
		if taint.TimeAdded != nil {
			durationSinceTaint := time.Since(taint.TimeAdded.Time)
			if durationSinceTaint < c.unreachableTimeout {
				// If the node is unreachable and the time since it became unreachable is less than the configured timeout,
				// we requeue to prevent the node from remaining in an unreachable state indefinitely
				log.FromContext(ctx).V(0).Info("Node has been unreachable for less than unreachableTimeout, requeueing", "node", node.Name)
				return reconcile.Result{RequeueAfter: c.unreachableTimeout + 1*time.Minute}, nil
			}
			// if node is unreachable for too long, delete the nodeclaim
			if err := c.kubeClient.Delete(ctx, nodeClaim, &deletOpts); err != nil {
				log.FromContext(ctx).V(0).Error(err, "Failed to delete NodeClaim", "node", node.Name)
				return reconcile.Result{}, err
			}
			log.FromContext(ctx).V(0).Info("Deleted NodeClaim because the node has been unreachable for more than unreachableTimeout", "node", node.Name)
		} else {
			log.FromContext(ctx).V(0).Error(fmt.Errorf("Missing TimeAdded on unreachable taint"), "node", node.Name)
		}
	}

	return reconcile.Result{}, nil
}

func (c *Controller) Register(_ context.Context, m manager.Manager) error {
	if err := m.GetFieldIndexer().IndexField(
		context.Background(),
		&corev1.Node{},
		"spec.providerID",
		func(o client.Object) []string {
			node := o.(*corev1.Node)
			return []string{node.Spec.ProviderID}
		}); err != nil {
		return fmt.Errorf("failed to index Node spec.providerID field: %w", err)
	}
	builder := controllerruntime.NewControllerManagedBy(m)
	return builder.
		Named("nodeclaim.notready").
		For(&v1.NodeClaim{}).
		Watches(
			&corev1.Node{},
			nodeclaimutil.NodeEventHandler(c.kubeClient),
		).
		WithOptions(controller.Options{MaxConcurrentReconciles: 100}).
		Complete(reconcile.AsReconciler(m.GetClient(), c))
}
