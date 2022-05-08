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
	"log"
	"testing"

	"domc.me/irsa-controller/api/v1alpha1"
	irsav1alpha1 "domc.me/irsa-controller/api/v1alpha1"
	"domc.me/irsa-controller/pkg/aws"
	goAws "github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/iam"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func getReconciler(mic *aws.MockedIamClient, objs ...runtime.Object) *IamRoleServiceAccountReconciler {
	scheme := runtime.NewScheme()
	if err := irsav1alpha1.AddToScheme(scheme); err != nil {
		log.Fatalf("Unable to add irsa scheme: (%v)", err)
	}
	if err := corev1.AddToScheme(scheme); err != nil {
		log.Fatalf("Unable to add core v1 scheme: (%v)", err)
	}

	fakeClient := fake.NewFakeClientWithScheme(scheme, objs...)

	oidc := "test"
	iamRoleClient := aws.NewIamClientWithIamAPI("test", "test", []string{}, mic)

	r := NewIamRoleServiceAccountReconciler(fakeClient, scheme, oidc, iamRoleClient)
	return r
}

func TestIamRoleServiceAccountReconciler_updateIrsaStatus(t *testing.T) {
	irsa := &irsav1alpha1.IamRoleServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "irsa",
			Namespace: "default",
		},
	}
	r := getReconciler(aws.NewMockedIamClient(), irsa)
	// 1. update status to failed should work
	updated := r.updateIrsaStatus(context.Background(), irsa, irsav1alpha1.IrsaFailed, fmt.Errorf("error"))
	if !updated {
		t.Fatal("Update irsa status to failed failed")
	}

	var gotIrsa v1alpha1.IamRoleServiceAccount
	if err := r.Get(context.Background(), types.NamespacedName{Namespace: irsa.GetNamespace(), Name: irsa.GetName()}, &gotIrsa); err != nil {
		t.Fatalf("Get irsa failed: %v", err)
	}
	if gotIrsa.Status.Reason != "error" || gotIrsa.Status.Condition != irsav1alpha1.IrsaFailed {
		t.Errorf("Update status failed")
	}

	// 2. update same status and err should not work
	updated = r.updateIrsaStatus(context.Background(), irsa, irsav1alpha1.IrsaFailed, fmt.Errorf("error"))
	if updated {
		t.Fatal("Update same status and err should not work")
	}

	// 3. update same status and differnet error should work
	updated = r.updateIrsaStatus(context.Background(), irsa, irsav1alpha1.IrsaFailed, fmt.Errorf("error2"))
	if !updated {
		t.Fatal("Update same status but differnet errs should work")
	}

}

func TestIamRoleServiceAccountReconciler_updateExternalResourcesIfNeed(t *testing.T) {
	irsa := &irsav1alpha1.IamRoleServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "irsa",
			Namespace: "default",
		},
	}
	oidc := "test"
	mic := aws.NewMockedIamClient()
	r := getReconciler(mic, irsa)

	externalRoleName := "external-role"
	// 1. external iam role allow irsa access
	externalRole, err := mic.CreateRole(&iam.CreateRoleInput{
		RoleName:                 goAws.String(externalRoleName),
		AssumeRolePolicyDocument: goAws.String(`{"Version":"2012-10-17","Statement":[]}`),
	})
	if err != nil {
		t.Fatalf("Create external role failed: %v", err)
	}
	irsa.Spec.RoleName = *externalRole.Role.RoleName
	err = r.updateExternalIamRoleIfNeed(context.Background(), irsa)
	if err != nil {
		t.Fatalf("updateExternalIamRoleIfNeed failed: %v", err)
	}
	gotExternalRole, err := r.iamRoleClient.Get(context.Background(), irsa.Spec.RoleName)
	if err != nil {
		t.Fatalf("Get external role failed: %v", err)
	}
	if !gotExternalRole.AssumeRolePolicy.IsAllowOIDC(oidc, irsa.GetNamespace(), irsa.GetName()) {
		t.Fatalf("External role should allow oidc, but not")
	}
	// r.iamRoleClient.Create(context.Background(), oidc, irsa*irsav1alpha1.IamRoleServiceAccount)

}

func TestIamRoleServiceAccountReconciler_deleteServiceAccount(t *testing.T) {
	irsa := &irsav1alpha1.IamRoleServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "irsa",
			Namespace: "default",
		},
	}
	sa := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "irsa",
			Namespace: "default",
		},
	}
	mic := aws.NewMockedIamClient()
	r := getReconciler(mic, irsa, sa)

	// 1. irsa is not managed by irsa-controller, so it cannot be deleted
	err := r.deleteServiceAccount(context.Background(), irsa)
	if err != nil {
		t.Fatalf("Delete service account failed: %v", err)
	}

	gotSA := &corev1.ServiceAccount{}
	// should be getten
	err = r.Client.Get(context.Background(), types.NamespacedName{Namespace: irsa.GetNamespace(), Name: irsa.GetName()}, gotSA)
	if err != nil {
		t.Fatalf("Get service account failed: %v", err)
	}

	// 2. irsa is manged by irsa-controller
	err = ctrl.SetControllerReference(irsa, sa, r.scheme)
	if err != nil {
		t.Fatalf("Set controller reference failed: %v", err)
	}
	r2 := getReconciler(mic, irsa, sa)
	err = r2.deleteServiceAccount(context.Background(), irsa)
	if err != nil {
		t.Fatalf("Delete service account 2 failed: %v", err)
	}
	err = r2.Client.Get(context.Background(), types.NamespacedName{Namespace: irsa.GetNamespace(), Name: irsa.GetName()}, gotSA)
	if !errors.IsNotFound(err) {
		t.Fatalf("Service account should be deleted")
	}
}

func TestIamRoleServiceAccountReconciler_checkExternalResources(t *testing.T) {
	irsa := &irsav1alpha1.IamRoleServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "irsa",
			Namespace: "default",
		},
	}
	sa := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "irsa",
			Namespace: "default",
		},
	}
	mic := aws.NewMockedIamClient()
	r := getReconciler(mic, irsa, sa)

	// 1. iam role not found, check pass
	status, err := r.checkExternalResources(context.Background(), irsa)
	if err != nil || status != irsav1alpha1.IrsaProgressing {
		t.Fatalf("Check external resource failed: %s,%v", status, err)
	}

	// 2. iam role exists and not manged by irsa-controller
	role := &iam.CreateRoleInput{
		RoleName:                 goAws.String(r.iamRoleClient.RoleName(irsa)),
		AssumeRolePolicyDocument: goAws.String(`{"Version":"2012-10-17","Statement":[]}`),
	}
	_, err = mic.CreateRole(role)
	if err != nil {
		t.Fatalf("Create irsa role failed: %v", err)
	}

	status, err = r.checkExternalResources(context.Background(), irsa)
	if err == nil || status != irsav1alpha1.IrsaConflict {
		t.Fatalf("Iam role exists and should return conflict, but got: %s", status)
	}

	// 3. iam role exists and manged by irsa-controller
	role.Tags = []*iam.Tag{{
		Key:   goAws.String(aws.IrsaContollerManagedTagKey),
		Value: goAws.String(aws.IrsaContollerManagedTagVal),
	}}

	_, err = mic.CreateRole(role)
	if err != nil {
		t.Fatalf("Update irsa role failed: %v", err)
	}

	status, err = r.checkExternalResources(context.Background(), irsa)
	if err != nil || status != irsav1alpha1.IrsaProgressing {
		t.Fatalf("Iam role exists and should return ok, but got: %s, %v", status, err)
	}
}
