/*
Copyright 2022 domechn.

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

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	irsav1alpha1 "domc.me/irsa-controller/api/v1alpha1"
	"domc.me/irsa-controller/pkg/aws"
	"domc.me/irsa-controller/pkg/utils/slices"
)

// IamRoleServiceAccountReconciler reconciles a IamRoleServiceAccount object
type IamRoleServiceAccountReconciler struct {
	client.Client
	Scheme *runtime.Scheme

	IamRoleClient aws.IamRoleClient
}

//+kubebuilder:rbac:groups=irsa.domc.me,resources=iamroleserviceaccounts,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=irsa.domc.me,resources=iamroleserviceaccounts/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=irsa.domc.me,resources=iamroleserviceaccounts/finalizers,verbs=update
//+kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=serviceaccount,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=serviceaccount/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the IamRoleServiceAccount object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.11.0/pkg/reconcile
func (r *IamRoleServiceAccountReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	l := log.FromContext(ctx)

	irsa := new(irsav1alpha1.IamRoleServiceAccount)

	err := r.Client.Get(ctx, req.NamespacedName, irsa)
	if err != nil {
		if errors.IsNotFound(err) {
			// means irsa is deleted, ignore this event cause we using finalizers to handle deleting events
			l.Info("IRSA is deleted, ignore events")
			err = nil
		}
		return ctrl.Result{}, err
	}

	myFinalizerName := "iamRole.finalizer.irsa.domc.me"

	// check whether irsa has been deleted
	if irsa.ObjectMeta.DeletionTimestamp.IsZero() {
		// irsa has not been deleted, add finalizer to irsa
		if !slices.ContainsString(irsa.ObjectMeta.Finalizers, myFinalizerName) {
			irsa.ObjectMeta.Finalizers = append(irsa.ObjectMeta.Finalizers, myFinalizerName)
			if err := r.Update(context.Background(), irsa); err != nil {
				l.Error(err, "update finalizer failed")
				return ctrl.Result{}, err
			}
		}
	} else {
		// irsa is being deleted
		if slices.ContainsString(irsa.ObjectMeta.Finalizers, myFinalizerName) {
			l.Info("IRSA is being deleted, cleaning aws iam role")
			roleArn := irsa.Status.RoleArn
			if roleArn != "" {
				l = l.WithValues("roleArn", roleArn)
				// clean aws iam role
				if err := r.IamRoleClient.Delete(ctx, roleArn); err != nil {
					l.Error(err, "delete aws iam role failed", irsa.Status.RoleArn)
					return ctrl.Result{}, err
				}
			}

			// clean finalizers when aws iam role has been deleted successfully
			irsa.ObjectMeta.Finalizers = slices.RemoveString(irsa.ObjectMeta.Finalizers, myFinalizerName)
			if err := r.Update(context.Background(), irsa); err != nil {
				return ctrl.Result{}, err
			}
		}

		l.Info("Delete IRSA successfully")

		return ctrl.Result{}, nil
	}

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *IamRoleServiceAccountReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&irsav1alpha1.IamRoleServiceAccount{}).
		Complete(r)
}
