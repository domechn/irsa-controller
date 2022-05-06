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
	"testing"

	"domc.me/irsa-controller/api/v1alpha1"
	irsav1alpha1 "domc.me/irsa-controller/api/v1alpha1"
	"domc.me/irsa-controller/pkg/aws"
	"github.com/aws/aws-sdk-go/service/iam/iamiface"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

type mockedIamClient struct {
	iamiface.IAMAPI
}

func TestIamRoleServiceAccountReconciler_updateIrsaStatus(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := irsav1alpha1.AddToScheme(scheme); err != nil {
		t.Fatalf("Unable to add irsa scheme: (%v)", err)
	}

	irsa := &irsav1alpha1.IamRoleServiceAccount{
		// TypeMeta: metav1.TypeMeta{
		// 	APIVersion: "v1alpha1",
		// 	Kind:       "IamRoleServiceAccount",
		// },
		ObjectMeta: metav1.ObjectMeta{
			Name:      "irsa",
			Namespace: "default",
		},
	}
	objs := []runtime.Object{irsa}
	fakeClient := fake.NewFakeClientWithScheme(scheme, objs...)

	oidc := "test"
	iamRoleClient := aws.NewIamClientWithIamAPI("test", "test", []string{}, &mockedIamClient{})

	r := NewIamRoleServiceAccountReconciler(fakeClient, scheme, oidc, iamRoleClient)

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
