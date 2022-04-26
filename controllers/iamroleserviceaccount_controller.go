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

	// check whether irsa has been deleted
	if irsa.ObjectMeta.DeletionTimestamp.IsZero() {
		// irsa has not been deleted, add finalizer to irsa
		updated, err := r.finalizer(ctx, irsa, false)
		if err != nil {
			l.Error(err, "Update finalizer failed")
			return ctrl.Result{}, err
		}
		// only do one thing in once Reconcile
		if updated {
			return ctrl.Result{}, nil
		}

		// begin to sync status
		if err := r.reconcile(ctx, irsa); err != nil {
			l.Error(err, "Reconcile irsa failed")
			return ctrl.Result{}, err
		}
	} else {
		// irsa is being deleted
		if _, err := r.finalizer(ctx, irsa, true); err != nil {
			l.Error(err, "Delete aws iam role failed", irsa.Status.RoleArn)
			return ctrl.Result{}, err
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

func (r *IamRoleServiceAccountReconciler) reconcile(ctx context.Context, irsa *irsav1alpha1.IamRoleServiceAccount) error {
	l := log.FromContext(ctx)
	l.Info("Syncing the status of irsa")

	// irsa is just created
	if irsa.Status.Condition == irsav1alpha1.IrsaSubmitted {
		r.updateConditionStatus(ctx, irsa, irsav1alpha1.IrsaPending)
		return nil
	}

	// creating iam role
	if irsa.Status.Condition == irsav1alpha1.IrsaPending {
		l.Info("pending")
		arn, err := r.IamRoleClient.Create(ctx)
		if err != nil {
			irsa.Status.Reason = err.Error()
			r.updateConditionStatus(ctx, irsa, irsav1alpha1.IrsaFailed)
			return err
		}
		irsa.Status.RoleArn = arn
		r.updateConditionStatus(ctx, irsa, irsav1alpha1.IrsaOK)
		return nil
	}

	return nil
}

func (r *IamRoleServiceAccountReconciler) finalizer(ctx context.Context, irsa *irsav1alpha1.IamRoleServiceAccount, deleted bool) (bool, error) {
	myFinalizerName := "iamRole.finalizer.irsa.domc.me"
	l := log.FromContext(ctx)
	updated := true
	// irsa is being deleted and finalizer has not been handled
	if deleted && slices.ContainsString(irsa.ObjectMeta.Finalizers, myFinalizerName) {
		l.Info("IRSA is being deleted, cleaning aws iam role")
		if err := r.deleteExternalResources(ctx, irsa); err != nil {
			return updated, err
		}

		// clean finalizers when aws iam role has been deleted successfully
		irsa.ObjectMeta.Finalizers = slices.RemoveString(irsa.ObjectMeta.Finalizers, myFinalizerName)
		return updated, r.Update(ctx, irsa)
	}
	// irsa is not being deleted and finalizer has not been handled
	if !deleted && !slices.ContainsString(irsa.ObjectMeta.Finalizers, myFinalizerName) {
		irsa.ObjectMeta.Finalizers = append(irsa.ObjectMeta.Finalizers, myFinalizerName)
		return updated, r.Update(ctx, irsa)
	}
	updated = false
	return updated, nil
}

func (r *IamRoleServiceAccountReconciler) deleteExternalResources(ctx context.Context, irsa *irsav1alpha1.IamRoleServiceAccount) error {
	l := log.FromContext(ctx)
	// check if need to delete aws iam role
	if irsa.Spec.ARN != "" {
		l.V(5).Info("ARN is specified in spec, no need to delete")
		return nil
	}
	roleArn := irsa.Status.RoleArn
	if roleArn == "" {
		l.V(5).Info("ARN has not been generated, no need to delete")
		return nil
	}
	// clean aws iam role
	return r.IamRoleClient.Delete(ctx, roleArn)
}

func (r *IamRoleServiceAccountReconciler) updateConditionStatus(ctx context.Context, irsa *irsav1alpha1.IamRoleServiceAccount, condition irsav1alpha1.IrsaCondition) error {
	l := log.FromContext(ctx)
	from := irsa.Status.Condition
	irsa.Status.Condition = condition
	err := r.Status().Update(ctx, irsa)
	if err != nil {
		l.Error(err, "Update status failed", "from", from, "to", condition)
	}
	return err
}
