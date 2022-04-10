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
	"context"
	"reflect"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	federationv1 "github.com/cloudfunny/kubefederation/api/v1"
	"github.com/go-logr/logr"
)

// FederatedClusterReconciler reconciles a FederatedCluster object
type FederatedClusterReconciler struct {
	client.Client
	Scheme *runtime.Scheme
	Log    logr.Logger
}

//+kubebuilder:rbac:groups=federation.example.com,resources=federatedclusters,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=federation.example.com,resources=federatedclusters/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=federation.example.com,resources=federatedclusters/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the FederatedCluster object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.11.0/pkg/reconcile
func (r *FederatedClusterReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := r.Log.WithValues("FederatedCluster", req.NamespacedName)
	federatedCluster := &federationv1.FederatedCluster{}
	if err := r.Get(ctx, req.NamespacedName, federatedCluster); err != nil {
		log.Info("Failed to get FederatedCluster", "cluster", req.NamespacedName)
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	clusterClient, err := NewClusterClientSet(federatedCluster, r.Client, time.Second*5)
	if err != nil {
		log.Info("Failed to generate new clusterset", "cluster", req.NamespacedName)
		return ctrl.Result{}, err
	}

	clusterStatus, err := clusterClient.GetClusterHealthStatus()
	if err != nil {
		log.Info("Failed to get cluster status", "cluster", req.NamespacedName)
	}

	if !reflect.DeepEqual(clusterStatus, federatedCluster.Status) {
		federatedCluster.Status = *clusterStatus
		if err := r.Status().Update(ctx, federatedCluster, &client.UpdateOptions{}); err != nil {
			if apierrors.IsConflict(err) {
				return ctrl.Result{Requeue: true}, nil
			}
			log.Info("Failed to update cluster status", "cluster", req.NamespacedName)
		}
		log.Info("Success to update cluster status", "cluster", req.NamespacedName)
	}
	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *FederatedClusterReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&federationv1.FederatedCluster{}).
		Complete(r)
}
