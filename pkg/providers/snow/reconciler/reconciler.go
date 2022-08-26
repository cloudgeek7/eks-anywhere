package reconciler

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	anywherev1 "github.com/aws/eks-anywhere/pkg/api/v1alpha1"
	"github.com/aws/eks-anywhere/pkg/clients/kubernetes"
	"github.com/aws/eks-anywhere/pkg/cluster"
	"github.com/aws/eks-anywhere/pkg/controller"
	"github.com/aws/eks-anywhere/pkg/controller/clientutil"
	"github.com/aws/eks-anywhere/pkg/controller/serverside"
	"github.com/aws/eks-anywhere/pkg/providers/snow"
)

type CNIReconciler interface {
	Reconcile(ctx context.Context, logger logr.Logger, client client.Client, spec *cluster.Spec) (controller.Result, error)
}

type Reconciler struct {
	client        client.Client
	cniReconciler CNIReconciler
	*serverside.ObjectApplier
}

func New(client client.Client, cniReconciler CNIReconciler) *Reconciler {
	return &Reconciler{
		client:        client,
		cniReconciler: cniReconciler,
		ObjectApplier: serverside.NewObjectApplier(client),
	}
}

func (r *Reconciler) Reconcile(ctx context.Context, log logr.Logger, c *anywherev1.Cluster) (controller.Result, error) {
	log = log.WithValues("provider", "snow")
	clusterSpec, err := cluster.BuildSpec(ctx, clientutil.NewKubeClient(r.client), c)
	if err != nil {
		return controller.Result{}, err
	}

	return controller.NewPhaseRunner().Register(
		r.ValidateMachineConfigs,
		r.ReconcileWorkers,
	).Run(ctx, log, clusterSpec)
}

func (r *Reconciler) ValidateMachineConfigs(ctx context.Context, log logr.Logger, clusterSpec *cluster.Spec) (controller.Result, error) {
	log = log.WithValues("phase", "validateMachineConfigs")
	for _, machineConfig := range clusterSpec.SnowMachineConfigs {
		if !machineConfig.Status.SpecValid {
			failureMessage := fmt.Sprintf("SnowMachineConfig %s is invalid", machineConfig.Name)
			if machineConfig.Status.FailureMessage != nil {
				failureMessage += ": " + *machineConfig.Status.FailureMessage
			}

			log.Error(nil, failureMessage)
			clusterSpec.Cluster.Status.FailureMessage = &failureMessage
			return controller.Result{}, nil
		}
	}

	return controller.Result{}, nil
}

func (s *Reconciler) ReconcileWorkers(ctx context.Context, log logr.Logger, clusterSpec *cluster.Spec) (controller.Result, error) {
	log = log.WithValues("phase", "reconcileWorkers")
	log.Info("Applying worker CAPI objects")

	return s.Apply(ctx, func() ([]kubernetes.Object, error) {
		return snow.WorkersObjects(ctx, clusterSpec, clientutil.NewKubeClient(s.client))
	})
}
