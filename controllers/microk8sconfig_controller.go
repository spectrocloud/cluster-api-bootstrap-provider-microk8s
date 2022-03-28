/*
Copyright 2022.

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

package controllers

import (
	v1alpha4 "cluster-api-bootstrap-provider-microk8s/api/v1alpha4"
	"context"
	"fmt"

	"github.com/pkg/errors"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	bsutil "sigs.k8s.io/cluster-api/bootstrap/util"
	expv1 "sigs.k8s.io/cluster-api/exp/api/v1beta1"
	"sigs.k8s.io/cluster-api/feature"
	"sigs.k8s.io/cluster-api/util"
	"sigs.k8s.io/cluster-api/util/annotations"
	"sigs.k8s.io/cluster-api/util/predicates"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/source"

	bootstrapclusterxk8siov1alpha4 "cluster-api-bootstrap-provider-microk8s/api/v1alpha4"
)

// MicroK8sConfigReconciler reconciles a MicroK8sConfig object
type MicroK8sConfigReconciler struct {
	client.Client
	Scheme *runtime.Scheme
	// WatchFilterValue is the label value used to filter events prior to reconciliation.
	WatchFilterValue string
}

//+kubebuilder:rbac:groups=bootstrap.cluster.x-k8s.io.bootstrap.cluster.x-k8s.io,resources=microk8sconfigs,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=bootstrap.cluster.x-k8s.io.bootstrap.cluster.x-k8s.io,resources=microk8sconfigs/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=bootstrap.cluster.x-k8s.io.bootstrap.cluster.x-k8s.io,resources=microk8sconfigs/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the MicroK8sConfig object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.11.0/pkg/reconcile
func (r *MicroK8sConfigReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := log.FromContext(ctx)

	// Lookup the kubeadm config
	config := &v1alpha4.MicroK8sConfig{}
	if err := r.Client.Get(ctx, req.NamespacedName, config); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		log.Error(err, "Failed to get config")
		return ctrl.Result{}, err
	}

	// Look up the owner of this kubeadm config if there is one
	configOwner, err := bsutil.GetConfigOwner(ctx, r.Client, config)
	if apierrors.IsNotFound(err) {
		// Could not find the owner yet, this is not an error and will rereconcile when the owner gets set.
		return ctrl.Result{}, nil
	}
	if err != nil {
		log.Error(err, "Failed to get owner")
		return ctrl.Result{}, err
	}
	if configOwner == nil {
		return ctrl.Result{}, nil
	}
	log = log.WithValues("kind", configOwner.GetKind(), "version", configOwner.GetResourceVersion(), "name", configOwner.GetName())

	// Lookup the cluster the config owner is associated with
	cluster, err := util.GetClusterByName(ctx, r.Client, configOwner.GetNamespace(), configOwner.ClusterName())
	if err != nil {
		if errors.Cause(err) == util.ErrNoCluster {
			log.Info(fmt.Sprintf("%s does not belong to a cluster yet, waiting until it's part of a cluster", configOwner.GetKind()))
			return ctrl.Result{}, nil
		}

		if apierrors.IsNotFound(err) {
			log.Info("Cluster does not exist yet, waiting until it is created")
			return ctrl.Result{}, nil
		}
		log.Error(err, "Could not get cluster with metadata")
		return ctrl.Result{}, err
	}

	if annotations.IsPaused(cluster, config) {
		log.Info("Reconciliation is paused for this object")
		return ctrl.Result{}, nil
	}

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *MicroK8sConfigReconciler) SetupWithManager(ctx context.Context, mgr ctrl.Manager) error {
	b := ctrl.NewControllerManagedBy(mgr).
		For(&bootstrapclusterxk8siov1alpha4.MicroK8sConfig{}).
		Watches(&source.Kind{Type: &clusterv1.Machine{}},
			handler.EnqueueRequestsFromMapFunc(r.MachineToBootstrapMapFunc)).WithEventFilter(predicates.ResourceNotPausedAndHasFilterLabel(ctrl.LoggerFrom(ctx), r.WatchFilterValue))

	c, err := b.Build(r)
	if err != nil {
		return errors.Wrap(err, "failed setting up with a controller manager")
	}
	err = c.Watch(
		&source.Kind{Type: &clusterv1.Cluster{}},
		handler.EnqueueRequestsFromMapFunc(r.ClusterToMicroK8sConfigs),
		predicates.All(ctrl.LoggerFrom(ctx),
			predicates.ClusterUnpausedAndInfrastructureReady(ctrl.LoggerFrom(ctx)),
			predicates.ResourceHasFilterLabel(ctrl.LoggerFrom(ctx), r.WatchFilterValue),
		),
	)
	if err != nil {
		return errors.Wrap(err, "failed adding Watch for Clusters to controller manager")
	}

	return nil
}

func (r *MicroK8sConfigReconciler) ClusterToMicroK8sConfigs(o client.Object) []ctrl.Request {
	result := []ctrl.Request{}

	c, ok := o.(*clusterv1.Cluster)
	if !ok {
		panic(fmt.Sprintf("Expected a Cluster but got a %T", o))
	}

	selectors := []client.ListOption{
		client.InNamespace(c.Namespace),
		client.MatchingLabels{
			clusterv1.ClusterLabelName: c.Name,
		},
	}

	machineList := &clusterv1.MachineList{}
	if err := r.Client.List(context.TODO(), machineList, selectors...); err != nil {
		return nil
	}

	for _, m := range machineList.Items {
		if m.Spec.Bootstrap.ConfigRef != nil &&
			m.Spec.Bootstrap.ConfigRef.GroupVersionKind().GroupKind() == v1alpha4.GroupVersion.WithKind("MicroK8sConfig").GroupKind() {
			name := client.ObjectKey{Namespace: m.Namespace, Name: m.Spec.Bootstrap.ConfigRef.Name}
			result = append(result, ctrl.Request{NamespacedName: name})
		}
	}

	if feature.Gates.Enabled(feature.MachinePool) {
		machinePoolList := &expv1.MachinePoolList{}
		if err := r.Client.List(context.TODO(), machinePoolList, selectors...); err != nil {
			return nil
		}

		for _, mp := range machinePoolList.Items {
			if mp.Spec.Template.Spec.Bootstrap.ConfigRef != nil &&
				mp.Spec.Template.Spec.Bootstrap.ConfigRef.GroupVersionKind().GroupKind() == v1alpha4.GroupVersion.WithKind("MicroK8sConfig").GroupKind() {
				name := client.ObjectKey{Namespace: mp.Namespace, Name: mp.Spec.Template.Spec.Bootstrap.ConfigRef.Name}
				result = append(result, ctrl.Request{NamespacedName: name})
			}
		}
	}

	return result
}

func (r *MicroK8sConfigReconciler) MachineToBootstrapMapFunc(o client.Object) []ctrl.Request {
	m, ok := o.(*clusterv1.Machine)
	if !ok {
		panic(fmt.Sprintf("Expected a Machine but got a %T", o))
	}

	result := []ctrl.Request{}
	if m.Spec.Bootstrap.ConfigRef != nil && m.Spec.Bootstrap.ConfigRef.GroupVersionKind() == v1alpha4.GroupVersion.WithKind("MicroK8sConfig") {
		name := client.ObjectKey{Namespace: m.Namespace, Name: m.Spec.Bootstrap.ConfigRef.Name}
		result = append(result, ctrl.Request{NamespacedName: name})
	}
	return result
}
