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
	"github.com/aws/aws-sdk-go/aws/request"
	"github.com/aws/aws-sdk-go/service/iam"
	"github.com/aws/aws-sdk-go/service/iam/iamiface"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

type mockedIamClient struct {
	iamiface.IAMAPI
	mockRoles            map[string]*iam.Role
	mockAttachedPolicies map[string][]*iam.AttachedPolicy
}

func NewMockedIamClient() *mockedIamClient {
	return &mockedIamClient{
		mockRoles:            make(map[string]*iam.Role),
		mockAttachedPolicies: make(map[string][]*iam.AttachedPolicy),
	}
}

func (m *mockedIamClient) CreateRole(input *iam.CreateRoleInput) (*iam.CreateRoleOutput, error) {
	m.mockRoles[*input.RoleName] = &iam.Role{
		RoleName:                 input.RoleName,
		Arn:                      goAws.String(fmt.Sprintf("arn:aws:iam::000000000000:role/%s", *input.RoleName)),
		AssumeRolePolicyDocument: input.AssumeRolePolicyDocument,
	}
	return &iam.CreateRoleOutput{
		Role: m.mockRoles[*input.RoleName],
	}, nil
}

func (m *mockedIamClient) GetRoleWithContext(ctx context.Context, input *iam.GetRoleInput, opts ...request.Option) (*iam.GetRoleOutput, error) {
	return &iam.GetRoleOutput{
		Role: m.mockRoles[*input.RoleName],
	}, nil
}

func (m *mockedIamClient) ListAttachedRolePoliciesWithContext(ctx context.Context, input *iam.ListAttachedRolePoliciesInput, opts ...request.Option) (*iam.ListAttachedRolePoliciesOutput, error) {
	return &iam.ListAttachedRolePoliciesOutput{
		AttachedPolicies: m.mockAttachedPolicies[*input.RoleName],
	}, nil
}

func (m *mockedIamClient) GetRolePolicyWithContext(ctx context.Context, input *iam.GetRolePolicyInput, opts ...request.Option) (*iam.GetRolePolicyOutput, error) {
	return &iam.GetRolePolicyOutput{
		RoleName:       input.RoleName,
		PolicyName:     input.PolicyName,
		PolicyDocument: goAws.String(`{"Version":"2012-10-17","Statement":[]}`),
	}, nil
}

func (m *mockedIamClient) UpdateAssumeRolePolicyWithContext(ctx context.Context, input *iam.UpdateAssumeRolePolicyInput, opts ...request.Option) (*iam.UpdateAssumeRolePolicyOutput, error) {
	m.mockRoles[*input.RoleName].AssumeRolePolicyDocument = input.PolicyDocument
	return &iam.UpdateAssumeRolePolicyOutput{}, nil
}

func getReconciler(mic *mockedIamClient, mockIrsa *irsav1alpha1.IamRoleServiceAccount) *IamRoleServiceAccountReconciler {
	scheme := runtime.NewScheme()
	if err := irsav1alpha1.AddToScheme(scheme); err != nil {
		log.Fatalf("Unable to add irsa scheme: (%v)", err)
	}

	objs := []runtime.Object{mockIrsa}
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
	r := getReconciler(NewMockedIamClient(), irsa)
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
	mic := NewMockedIamClient()
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
