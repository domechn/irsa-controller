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

	"github.com/aws/aws-sdk-go/service/iam"
	gerrors "github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"domc.me/irsa-controller/api/v1alpha1"
	irsav1alpha1 "domc.me/irsa-controller/api/v1alpha1"
	"domc.me/irsa-controller/pkg/aws"
	"domc.me/irsa-controller/pkg/utils/slices"
)

var (
	ErrIamRoleNotCreated      = gerrors.New("Iam role has not been created")
	ErrServiceAccountConflict = gerrors.New("ServiceAccount is already exists and not manged by irsa-controller")
	ErrIamRoleConflict        = gerrors.New("Iam role is already exists and not manged by irsa-controller")
	requeuePeriod             = time.Minute * 3
	irsaAnnotationKey         = "eks.amazonaws.com/role-arn"
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
		Owns(&corev1.ServiceAccount{}).
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
	if irsa.Status.Condition == irsav1alpha1.IrsaPending || irsa.Status.Condition == irsav1alpha1.IrsaForbidden || irsa.Status.Condition == irsav1alpha1.IrsaConflict {
		l.Info("Checking before creating iam role in aws account")
		if err := r.reconcileServiceAccount(ctx, irsa, true); err != nil {
			updated := false
			if gerrors.Is(err, ErrServiceAccountConflict) {
				updated = r.updateIrsaStatus(ctx, irsa, irsav1alpha1.IrsaConflict, err)
			} else {
				updated = r.updateIrsaStatus(ctx, irsa, irsav1alpha1.IrsaForbidden, err)
			}
			return !updated, gerrors.Wrap(err, "Check service account failed")
		}
		status, err := r.checkExternalResources(ctx, irsa)
		updated := r.updateIrsaStatus(ctx, irsa, status, err)
		if err != nil {
			return !updated, gerrors.Wrap(err, "Check iam role failed when irsa is pending")
		}
		return !updated, nil
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

	if err := r.updateExternalResourcesIfNeed(ctx, irsa); err != nil {
		updated := r.updateIrsaStatus(ctx, irsa, irsav1alpha1.IrsaFailed, err)
		return !updated, gerrors.Wrap(err, "Update external resources failed")
	}

	if err := r.reconcileServiceAccount(ctx, irsa, false); err != nil {
		updated := false
		if gerrors.Is(err, ErrServiceAccountConflict) {
			updated = r.updateIrsaStatus(ctx, irsa, irsav1alpha1.IrsaConflict, err)
		}
		return !updated, gerrors.Wrap(err, "Reconcile service account failed")
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
		l.Info("IRSA is being deleted, cleaning service account and aws iam role")

		if err := r.deleteServiceAccount(ctx, irsa); err != nil {
			return hit, needRequeue(err), err
		}

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
		// if err is not not found
		if err != nil {
			return irsav1alpha1.IrsaForbidden, err
		}
	}
	// check whether role is managed by irsa
	if role != nil && !role.IsManagedByIrsaController() {
		return irsav1alpha1.IrsaConflict, fmt.Errorf("Iam role is not managed by irsa controller")
	}
	return irsav1alpha1.IrsaProgressing, nil
}

func (r *IamRoleServiceAccountReconciler) createExternalResources(ctx context.Context, irsa *irsav1alpha1.IamRoleServiceAccount) error {
	// determine the role
	roleName := irsa.Spec.RoleName
	var roleArn string
	var err error

	// update role arn and inline policy arn in irsa
	defer func() {
		// if role has been created, set it into status
		irsa.Status.RoleArn = roleArn
	}()

	if roleName == "" {
		roleArn, err = r.IamRoleClient.Create(ctx, r.OIDC, irsa)
		if err != nil {
			// if role already exists, check its tags, if its tag contains `irsa-controller: y` , update it. Else return error
			// TODO fix type of ErrCodeEntityAlreadyExistsException
			if gerrors.As(err, iam.ErrCodeEntityAlreadyExistsException) {
				role, err := r.IamRoleClient.Get(ctx, r.IamRoleClient.RoleName(irsa))
				if err != nil {
					return gerrors.Wrap(err, "Iam has already exists and cannot be getten")
				}
				if !role.IsManagedByIrsaController() {
					return ErrIamRoleConflict
				}
			}

			return gerrors.Wrap(err, "Create iam role failed")
		}
	}

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

	return nil
}

func (r *IamRoleServiceAccountReconciler) reconcileServiceAccount(ctx context.Context, irsa *v1alpha1.IamRoleServiceAccount, dryRun bool) error {
	roleArn := irsa.Status.RoleArn
	var sa corev1.ServiceAccount
	namespace := irsa.GetNamespace()
	saName := irsa.GetName()
	dryRunCreateOption := func(dry bool) []client.CreateOption {
		if dry {
			return []client.CreateOption{client.DryRunAll}
		}
		return []client.CreateOption{}
	}
	dryRunUpdateOption := func(dry bool) []client.UpdateOption {
		if dry {
			return []client.UpdateOption{client.DryRunAll}
		}
		return []client.UpdateOption{}
	}

	err := r.Client.Get(ctx, types.NamespacedName{
		Namespace: namespace,
		Name:      saName,
	}, &sa)
	// update sa to latest
	sa.Namespace = namespace
	sa.Name = saName
	if sa.Annotations == nil {
		sa.Annotations = make(map[string]string)
	}
	sa.Annotations[irsaAnnotationKey] = roleArn
	if err != nil {
		if errors.IsNotFound(err) {
			if err := ctrl.SetControllerReference(irsa, &sa, r.Scheme); err != nil {
				return gerrors.Wrap(err, "Set controller reference failed")
			}

			return r.Client.Create(ctx, &sa, dryRunCreateOption(dryRun)...)
		}
		return gerrors.Wrap(err, "Get service account failed")
	}

	if owned := r.serviceAccountNameIsOwnedByIrsa(&sa, irsa); !owned {
		return ErrServiceAccountConflict
	}

	// role is not created, no need to reconcile sa
	if roleArn == "" || irsa.Status.Condition != irsav1alpha1.IrsaOK {
		return nil
	}

	return r.Client.Update(ctx, &sa, dryRunUpdateOption(dryRun)...)
}

func (r *IamRoleServiceAccountReconciler) serviceAccountNameIsOwnedByIrsa(sa *corev1.ServiceAccount, irsa *v1alpha1.IamRoleServiceAccount) bool {
	for _, or := range sa.GetOwnerReferences() {
		if or.UID == irsa.UID {
			return true
		}
	}
	return false
}

func (r *IamRoleServiceAccountReconciler) deleteServiceAccount(ctx context.Context, irsa *v1alpha1.IamRoleServiceAccount) error {
	var sa corev1.ServiceAccount
	err := r.Client.Get(ctx, types.NamespacedName{
		Namespace: irsa.GetNamespace(),
		Name:      irsa.GetName(),
	}, &sa)
	if err != nil {
		if errors.IsNotFound(err) {
			return nil
		}
		return gerrors.Wrap(err, "Get service account by irsa failed")
	}
	owned := r.serviceAccountNameIsOwnedByIrsa(&sa, irsa)
	if !owned {
		return nil
	}

	err = r.Delete(ctx, &sa)
	if err != nil {
		return gerrors.Wrap(err, "Delete service account failed")
	}
	return nil
}

func (r *IamRoleServiceAccountReconciler) updateExternalResourcesIfNeed(ctx context.Context, irsa *irsav1alpha1.IamRoleServiceAccount) error {
	// the role is created externally
	if irsa.Spec.RoleName != "" {
		return r.updateExternalIamRoleIfNeed(ctx, irsa)
	}
	roleArn := irsa.Status.RoleArn
	if roleArn == "" {
		return ErrIamRoleNotCreated
	}
	roleName := aws.RoleNameByArn(roleArn)
	gotRole, err := r.IamRoleClient.Get(ctx, roleName)

	if err != nil {
		return gerrors.Wrap(err, "Get iam role by arn failed")
	}

	wantRole := aws.NewIamRole(r.OIDC, irsa)

	// compare spec and iam role detail

	// equal, not need to update
	if reflect.DeepEqual(gotRole, wantRole) {
		return nil
	}

	// compare managedPolicies
	if !slices.Equal(gotRole.ManagedPolicies, wantRole.ManagedPolicies) {
		// update managed polices
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
		if err := r.IamRoleClient.DetachRolePolicy(ctx, roleName, deAttaches); err != nil {
			return gerrors.Wrap(err, "Sync overflow managed roles failed")
		}
	}

	if !reflect.DeepEqual(gotRole.InlinePolicy, wantRole.InlinePolicy) {
		err = r.IamRoleClient.UpdateInlinePolicy(ctx, roleName, wantRole.InlinePolicy)
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
