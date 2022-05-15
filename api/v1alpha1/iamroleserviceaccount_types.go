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
	// +kubebuilder:validation:OneOf
	RoleName string `json:"roleName,omitempty"`

	// +kubebuilder:validation:OneOf
	// +optional
	// Policy defines the policy list of iam role in aws account
	Policy *PolicySpec `json:"policy,omitempty"`

	// +optional
	// Tags is a list of tags to apply to the IAM role ( only if the iam role is created by irsa-controller )
	Tags map[string]string `json:"tags,omitempty"`
}

type PolicySpec struct {
	// +optional
	// ManagedPolicies will make the iam role be attached with a list of managed policies
	ManagedPolicies []string `json:"managedPolicies"`
	// +optional
	// InlinePolicy defines the details of inline policy of iam role in aws account
	InlinePolicy *InlinePolicySpec `json:"inlinePolicy"`
}

// InlinePolicySpec defines the policy create within iam role
type InlinePolicySpec struct {
	// Version defines policy version, default is "2012-10-17"
	Version string `json:"version"`
	// Statement defines the policy statement
	Statement []StatementSpec `json:"statement"`
}

type StatementConditionSpec map[string]map[string]string

// StatementSpec defines the policy statement
type StatementSpec struct {
	Resource []string `json:"resource"`
	Action   []string `json:"action"`
	// +kubebuilder:validation:Enum=Allow;Deny
	Effect string `json:"effect"`
	// +optional
	Condition StatementConditionSpec `json:"condition"`
}

// IamRoleServiceAccountStatus defines the observed state of IamRoleServiceAccount
type IamRoleServiceAccountStatus struct {
	// +optional
	// RoleArn is the arn of iam role in aws account if the iam role is created or is external role
	RoleArn string `json:"roleArn,omitempty"`
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file
	// +optional
	// Conditions is a list of conditions and their status. Pending, Conflict, and Forbidden are in the status before resources creation, and Failed, Progressing and Synced are the status after resources creation
	Condition IrsaCondition `json:"condition,omitempty"`
	// +optional
	// Reason is a brief string that describes any failure.
	Reason string `json:"reason,omitempty"`
}

// +kubebuilder:validation:Enum=Pending;Conflict;Forbidden;Failed;Progressing;Synced
type IrsaCondition string

var (
	IrsaSubmitted   IrsaCondition = ""
	IrsaPending     IrsaCondition = "Pending"
	IrsaConflict    IrsaCondition = "Conflict"
	IrsaForbidden   IrsaCondition = "Forbidden"
	IrsaFailed      IrsaCondition = "Failed"
	IrsaProgressing IrsaCondition = "Progressing"
	IrsaOK          IrsaCondition = "Synced"
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
	prefixClusterName := fmt.Sprintf("%s-%s", prefix, cluster)
	if prefix == "" {
		prefixClusterName = cluster
	}
	return fmt.Sprintf("%s-%s-%s", prefixClusterName, i.GetNamespace(), i.GetName())
}
