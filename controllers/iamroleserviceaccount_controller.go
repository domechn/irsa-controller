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
	"fmt"
	"reflect"
	"time"

	gerrors "github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	irsav1alpha1 "domc.me/irsa-controller/api/v1alpha1"
	"domc.me/irsa-controller/pkg/aws"
	"domc.me/irsa-controller/pkg/utils/slices"
)

var (
	ErrIamRoleNotCreated = gerrors.New("Iam role has not been created")
	requeuePeriod        = time.Minute * 3
)

// IamRoleServiceAccountReconciler reconciles a IamRoleServiceAccount object
type IamRoleServiceAccountReconciler struct {
	client.Client
	Scheme *runtime.Scheme

	OIDC string

	IamRolePrefix string
	ClusterName   string
	IamRoleClient *aws.IamClient
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
	l.Info("Syncing the status of irsa")

	irsa := new(irsav1alpha1.IamRoleServiceAccount)

	err := r.Client.Get(ctx, req.NamespacedName, irsa)
	if err != nil {
		res := ctrl.Result{
			Requeue: true,
		}
		if errors.IsNotFound(err) {
			// means irsa is deleted, ignore this event cause we using finalizers to handle deleting events
			l.Info("IRSA is deleted, ignore events")
			res.Requeue = false
		} else {
			l.Error(err, "Get irsa failed")
		}
		return res, nil
	}

	// check whether irsa has been deleted
	if irsa.ObjectMeta.DeletionTimestamp.IsZero() {
		// irsa has not been deleted, add finalizer to irsa
		hit, requeue, err := r.finalize(ctx, irsa, false)
		// only do one thing in once Reconcile
		if hit {
			if err != nil {
				l.Error(err, "Update finalizer failed")
			}
			return ctrl.Result{
				Requeue: requeue,
			}, err
		}

		// begin to sync status
		if requeue, err := r.reconcile(ctx, irsa); err != nil {
			l.Error(err, "Reconcile irsa failed")
			res := ctrl.Result{}
			if requeue {
				res.RequeueAfter = requeuePeriod
			}
			return res, nil
		}
	} else {
		// irsa is being deleted
		if _, _, err := r.finalize(ctx, irsa, true); err != nil {
			l.Error(err, "Delete aws iam role failed", irsa.Status.RoleArn)
			return ctrl.Result{
				Requeue: true,
			}, nil
		}
		l.Info("Delete IRSA successfully")

		return ctrl.Result{}, nil
	}
	l.Info("The status of irsa has been synced")

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *IamRoleServiceAccountReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&irsav1alpha1.IamRoleServiceAccount{}).
		Complete(r)
}

// reconcile returns requeue and errors
func (r *IamRoleServiceAccountReconciler) reconcile(ctx context.Context, irsa *irsav1alpha1.IamRoleServiceAccount) (bool, error) {
	l := log.FromContext(ctx)

	// irsa is just created
	if irsa.Status.Condition == irsav1alpha1.IrsaSubmitted {
		l.Info("Irsa is submitted, begin to reconcile")
		updated := r.updateIrsaStatus(ctx, irsa, irsav1alpha1.IrsaPending, nil)
		return !updated, nil
	}

	// check before createing
	if irsa.Status.Condition == irsav1alpha1.IrsaPending || irsa.Status.Condition == irsav1alpha1.IrsaForbidden || irsa.Status.Condition == irsav1alpha1.IrsaRoleConflict {
		l.Info("Checking before creating iam role in aws account")
		status, err := r.checkExternalResources(ctx, irsa)
		if err != nil {
			irsa.Status.Reason = err.Error()
		}
		updated := r.updateIrsaStatus(ctx, irsa, status, err)
		return !updated, gerrors.Wrap(err, "Check iam role failed when irsa is pending")
	}

	// init irsa and iam role
	if irsa.Status.Condition == irsav1alpha1.IrsaProgressing {
		l.Info("Creating iam role in aws account")
		err := r.createExternalResources(ctx, irsa)
		if err != nil {
			// set to failed, and update detail status in next reconcile
			updated := r.updateIrsaStatus(ctx, irsa, irsav1alpha1.IrsaFailed, err)
			return !updated, gerrors.Wrap(err, "Init iam role failed when irsa is in progress")
		}
		updated := r.updateIrsaStatus(ctx, irsa, irsav1alpha1.IrsaOK, nil)
		return !updated, nil
	}

	// role has been reconciled
	roleArn := irsa.Status.RoleArn
	// role has not been created
	if roleArn == "" {
		l.Info("Creating iam role in aws account again")
		err := r.createExternalResources(ctx, irsa)
		if err != nil {
			updated := r.updateIrsaStatus(ctx, irsa, irsav1alpha1.IrsaFailed, err)
			return !updated, gerrors.Wrap(err, "Create external resources failed")
		}
	}

	err := r.updateExternalResourcesIfNeed(ctx, irsa)
	if err != nil {
		updated := r.updateIrsaStatus(ctx, irsa, irsav1alpha1.IrsaFailed, err)
		return !updated, gerrors.Wrap(err, "Update external resources failed")
	}

	return true, nil
}

// finalize returns hit rules, need requeue, errors
func (r *IamRoleServiceAccountReconciler) finalize(ctx context.Context, irsa *irsav1alpha1.IamRoleServiceAccount, deleted bool) (bool, bool, error) {
	myFinalizerName := "iamRole.finalizer.irsa.domc.me"
	l := log.FromContext(ctx)
	hit := true
	needRequeue := func(e error) bool {
		return hit && e != nil
	}
	// irsa is being deleted and finalizer has not been handled
	if deleted && slices.ContainsString(irsa.ObjectMeta.Finalizers, myFinalizerName) {
		l.Info("IRSA is being deleted, cleaning aws iam role")
		if err := r.deleteExternalResources(ctx, irsa); err != nil {
			return hit, needRequeue(err), err
		}

		// clean finalizers when aws iam role has been deleted successfully
		irsa.ObjectMeta.Finalizers = slices.RemoveString(irsa.ObjectMeta.Finalizers, myFinalizerName)
		err := r.Update(ctx, irsa)
		return hit, needRequeue(err), err
	}
	// irsa is not being deleted and finalizer has not been handled
	if !deleted && !slices.ContainsString(irsa.ObjectMeta.Finalizers, myFinalizerName) {
		irsa.ObjectMeta.Finalizers = append(irsa.ObjectMeta.Finalizers, myFinalizerName)
		err := r.Update(ctx, irsa)
		return hit, needRequeue(err), err
	}
	return false, needRequeue(nil), nil
}

func (r *IamRoleServiceAccountReconciler) checkExternalResources(ctx context.Context, irsa *irsav1alpha1.IamRoleServiceAccount) (irsav1alpha1.IrsaCondition, error) {
	role, err := r.IamRoleClient.Get(ctx, r.IamRoleClient.RoleName(irsa))
	if err != nil {
		// TODO: Distinguish between roles because they don't exist and because they don't have permissions
		return irsav1alpha1.IrsaForbidden, err
	}
	// check whether role is managed by irsa
	if role != nil && !role.IsManagedByIrsaController() {
		return irsav1alpha1.IrsaRoleConflict, fmt.Errorf("Iam role is not managed by irsa controller")
	}
	return irsav1alpha1.IrsaProgressing, nil
}

func (r *IamRoleServiceAccountReconciler) createExternalResources(ctx context.Context, irsa *irsav1alpha1.IamRoleServiceAccount) error {
	// determine the role
	roleName := irsa.Spec.RoleName
	var roleArn, inlinePolicyArn string
	var err error

	// update role arn and inline policy arn in irsa
	defer func() {
		irsa.Status.RoleArn = roleArn
		if inlinePolicyArn != "" {
			irsa.Status.InlinePolicyArn = &inlinePolicyArn
		}
	}()

	if roleName == "" {
		roleArn, inlinePolicyArn, err = r.IamRoleClient.Create(ctx, irsa)
		// TODO: if role already exists, check its tags, if its tag contains `irsa-controller: y` , update it. Else return error
		if err != nil {
			// if role has been created, set it into status

			return gerrors.Wrap(err, "Create iam role failed")
		}
	} else {
		// update its trust entities
		role, err := r.IamRoleClient.Get(ctx, roleName)
		roleArn = role.RoleArn
		if err != nil {
			return gerrors.Wrap(err, "Get iam role failed")
		}

		if !role.AssumeRolePolicy.IsAllowOIDC(r.OIDC, irsa.GetNamespace(), irsa.GetName()) {
			if err := r.IamRoleClient.AllowServiceAccountAccess(ctx, role, r.OIDC, irsa.GetNamespace(), irsa.GetName()); err != nil {
				return gerrors.Wrap(err, "Allow sa access iam role failed in create")
			}
		}

	}
	return nil
}

func (r *IamRoleServiceAccountReconciler) updateExternalResourcesIfNeed(ctx context.Context, irsa *irsav1alpha1.IamRoleServiceAccount) error {
	// the role is created externally
	if irsa.Spec.RoleName != "" {
		return r.updateExternalIamRoleIfNeed(ctx, irsa)
	}
	roleArn := irsa.Status.RoleArn
	inlinePolicyArn := irsa.Status.InlinePolicyArn
	if roleArn == "" {
		return ErrIamRoleNotCreated
	}
	roleName := aws.RoleNameByArn(roleArn)
	gotRole, err := r.IamRoleClient.Get(ctx, roleName)

	if err != nil {
		return gerrors.Wrap(err, "Get iam role by arn failed")
	}

	wantRole := aws.NewIamRole(irsa)

	// compare spec and iam role detail

	// equal, not need to update
	if reflect.DeepEqual(gotRole, wantRole) {
		return nil
	}

	// compare managedPolicies
	if !slices.Equal(gotRole.ManagedPolicies, wantRole.ManagedPolicies) {
		// r.IamRoleClient.
		// todo update managed polices
		var attaches, deAttaches []string
		for _, want := range wantRole.ManagedPolicies {
			found := false
			for _, got := range gotRole.ManagedPolicies {
				if want == got {
					found = true
				}
			}

			if !found {
				attaches = append(attaches, want)
			}
		}
		for _, got := range gotRole.ManagedPolicies {
			found := false
			for _, want := range wantRole.ManagedPolicies {
				if got == want {
					found = true
				}
			}

			if !found {
				deAttaches = append(deAttaches, got)
			}
		}

		if err := r.IamRoleClient.AttachRolePolicy(ctx, roleName, attaches); err != nil {
			return gerrors.Wrap(err, "Sync missing managed roles failed")
		}
		if err := r.IamRoleClient.DeAttachRolePolicy(ctx, roleName, deAttaches); err != nil {
			return gerrors.Wrap(err, "Sync overflow managed roles failed")
		}
	}

	if inlinePolicyArn != nil && !reflect.DeepEqual(gotRole.InlinePolicy, wantRole.InlinePolicy) {
		err = r.IamRoleClient.UpdatePolicy(ctx, *inlinePolicyArn, wantRole.InlinePolicy)
		if err != nil {
			return gerrors.Wrap(err, "Sync inline policy failed")
		}
	}

	if !reflect.DeepEqual(gotRole.AssumeRolePolicy, wantRole.AssumeRolePolicy) {
		assumeRolePolicy := aws.NewAssumeRolePolicy(r.OIDC, irsa.GetNamespace(), irsa.GetName())
		err = r.IamRoleClient.UpdateAssumePolicy(ctx, roleName, &assumeRolePolicy)
		if err != nil {
			return gerrors.Wrap(err, "Sync assume role policy failed")
		}
	}

	if !reflect.DeepEqual(gotRole.Tags, wantRole.Tags) {
		err = r.IamRoleClient.UpdateTags(ctx, roleName, wantRole.Tags)
		if err != nil {
			return gerrors.Wrap(err, "Sync iam role tag failed")
		}
	}

	return nil
}

// updateExternalIamRoleIfNeed checks the role can be assumed by oidc
// if not let it can be accessed, or do nothing
func (r *IamRoleServiceAccountReconciler) updateExternalIamRoleIfNeed(ctx context.Context, irsa *irsav1alpha1.IamRoleServiceAccount) error {
	roleName := irsa.Spec.RoleName
	// role is not created externally
	if roleName == "" {
		return nil
	}

	role, err := r.IamRoleClient.Get(ctx, roleName)
	if err != nil {
		return gerrors.Wrap(err, "Get role failed")
	}
	if !role.AssumeRolePolicy.IsAllowOIDC(r.OIDC, irsa.GetNamespace(), irsa.GetName()) {
		if err := r.IamRoleClient.AllowServiceAccountAccess(ctx, role, r.OIDC, irsa.GetNamespace(), irsa.GetName()); err != nil {
			return gerrors.Wrap(err, "Allow sa access iam role failed in update")
		}
	}
	return nil
}

func (r *IamRoleServiceAccountReconciler) deleteExternalResources(ctx context.Context, irsa *irsav1alpha1.IamRoleServiceAccount) error {
	l := log.FromContext(ctx)
	// check if need to delete aws iam role
	if irsa.Spec.RoleName != "" {
		l.V(5).Info("ARN is specified in spec, no need to delete")
		return nil
	}
	roleArn := irsa.Status.RoleArn
	if roleArn == "" {
		l.V(5).Info("ARN has not been generated, no need to delete")
		return nil
	}
	// clean aws iam role
	if err := r.IamRoleClient.Delete(ctx, roleArn); err != nil {
		return gerrors.Wrap(err, "Delete iam role failed")
	}
	inlinePolicyArn := irsa.Status.InlinePolicyArn
	if inlinePolicyArn == nil {
		l.V(5).Info("Inline policy arn has not been generated, no need to delete")
		return nil
	}
	if err := r.IamRoleClient.Delete(ctx, *inlinePolicyArn); err != nil {
		return gerrors.Wrap(err, "Delete inline policy failed")
	}
	return nil

}

func (r *IamRoleServiceAccountReconciler) updateIrsaStatus(ctx context.Context, irsa *irsav1alpha1.IamRoleServiceAccount, condition irsav1alpha1.IrsaCondition, reconcileErr error) bool {
	l := log.FromContext(ctx)
	from := irsa.Status.Condition
	newReason := ""
	if reconcileErr != nil {
		newReason = reconcileErr.Error()
	}
	if from == condition && irsa.Status.Reason == newReason {
		return false
	}
	irsa.Status.Reason = newReason
	irsa.Status.Condition = condition
	err := r.Status().Update(ctx, irsa)
	if err != nil {
		l.Error(err, "Update status failed", "from", from, "to", condition)
	}
	return err == nil
}

func (r *IamRoleServiceAccountReconciler) compareIrsaDetailAndRoleDetail(irsa *irsav1alpha1.IamRoleServiceAccount, role *aws.IamRole) (bool, error) {
	// check managed policies
	if !slices.Equal(role.ManagedPolicies, irsa.Spec.ManagedPolicies) {
		return false, nil
	}

	return false, nil
}
