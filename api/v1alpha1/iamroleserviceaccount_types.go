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

package v1alpha1

import (
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// IamRoleServiceAccountSpec defines the desired state of IamRoleServiceAccount
type IamRoleServiceAccountSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// RoleName defines the name of iam role existing in aws account which irsa will use
	// if the fields is provided, ManagedPolicies and InlinePolicy will be useless
	// +optional
	RoleName string `json:"roleName,omitempty"`

	// +optional
	ManagedPolicies []ManagedPolicySpec `json:"managedPolicies"`
	// +optional
	InlinePolicy *InlinePolicySpec `json:"inlinePolicy"`
}

// ManagedPolicySpec defines the policies manged by aws
type ManagedPolicySpec []string

// InlinePolicySpec defines the policy create within iam role
type InlinePolicySpec struct {
	Statements []StatementSpec `json:"statements"`
}

type StatementSpec struct {
	Resource []string `json:"resource"`
	Action   []string `json:"action"`
	// +kubebuilder:validation:Enum=Allow;Deny
	Effect string `json:"effect"`
}

// IamRoleServiceAccountStatus defines the observed state of IamRoleServiceAccount
type IamRoleServiceAccountStatus struct {
	// +optional
	RoleArn string `json:"roleArn,omitempty"`
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file
	// +optional
	Condition IrsaCondition `json:"condition,omitempty"`
	// +optional
	Reason string `json:"reason,omitempty"`
}

// +kubebuilder:validation:Enum=Pending;SaNameConflict;Forbidden;Failed;Progressing;Created
type IrsaCondition string

var (
	IrsaSubmitted      IrsaCondition = ""
	IrsaPending        IrsaCondition = "Pending"
	IrsaSaNameConflict IrsaCondition = "SaNameConflict"
	IrsaForbidden      IrsaCondition = "Forbidden"
	IrsaFailed         IrsaCondition = "Failed"
	IrsaProgressing    IrsaCondition = "Progressing"
	IrsaOK             IrsaCondition = "Created"
)

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

// IamRoleServiceAccount is the Schema for the iamroleserviceaccounts API
// +kubebuilder:printcolumn:name="RoleArn",type=string,JSONPath=`.status.roleArn`
// +kubebuilder:printcolumn:name="Status",type=string,JSONPath=`.status.condition`
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"
// +kubebuilder:subresource:status
type IamRoleServiceAccount struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   IamRoleServiceAccountSpec   `json:"spec,omitempty"`
	Status IamRoleServiceAccountStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// IamRoleServiceAccountList contains a list of IamRoleServiceAccount
type IamRoleServiceAccountList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []IamRoleServiceAccount `json:"items"`
}

func init() {
	SchemeBuilder.Register(&IamRoleServiceAccount{}, &IamRoleServiceAccountList{})
}

// AwsIamRoleName returns the name of iam role in aws account
func (i *IamRoleServiceAccount) AwsIamRoleName(prefix, cluster string) string {
	return fmt.Sprintf("%s-%s-%s-%s", prefix, cluster, i.GetNamespace(), i.GetName())
}
