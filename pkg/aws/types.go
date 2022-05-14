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

package aws

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	irsav1alpha1 "domc.me/irsa-controller/api/v1alpha1"
)

type AWSConfig struct {
	Endpoint        string
	Region          string
	AccessKeyID     string
	SecretAccessKey string
	DisableSSL      bool
}

// NewAWSConfigFromSpec is used to create a AWSConfig from the irsa spec
// and set default value if it is not specified in spec
func NewAWSConfigFromSpec(cfg *irsav1alpha1.AWSConfigSpec) *AWSConfig {
	if cfg == nil {
		return nil
	}

	awsConfig := &AWSConfig{
		Endpoint:        cfg.Endpoint,
		Region:          cfg.Region,
		AccessKeyID:     cfg.AccessKeyID,
		SecretAccessKey: cfg.SecretAccessKey,
		DisableSSL:      cfg.DisableSSL,
	}
	// no need to set endpoint, cause aws sdk will handle ""
	// set region to us-east-1 if not set
	if awsConfig.Region == "" {
		awsConfig.Region = "us-east-1"
	}
	return awsConfig
}

type StatementEffect string

const (
	StatementAllow StatementEffect = "Allow"
	StatementDeny  StatementEffect = "Deny"

	AssumeRoleWithWebIdentityAction = "sts:AssumeRoleWithWebIdentity"
	// IrsaContollerManagedTagKey is a fixed tag key, if tag value is "y"
	// means this iam role is manged by irsa-controller
	IrsaContollerManagedTagKey = "irsa-controller"
	IrsaContollerManagedTagVal = "y"
)

type IamRole struct {
	// RoleArn is "" when role is not created
	RoleArn string
	// RoleName is "" if RoleArn is ""
	RoleName     string
	InlinePolicy *RoleDocument
	// ManagedPolicies defines the arns of ManagedPolicies
	ManagedPolicies []string
	// AssumeRolePolicy defines the trust relationship of iam role
	AssumeRolePolicy *AssumeRoleDocument
	Tags             map[string]string
}

func (i *IamRole) IsManagedByIrsaController() bool {
	if val, ok := i.Tags[IrsaContollerManagedTagKey]; ok {
		return val == IrsaContollerManagedTagVal
	}

	return false
}

// NewIamRole is used only if the iam role is created by irsa but not be specificed by irsa.roleName
func NewIamRole(oidcProviderArn string, irsa *irsav1alpha1.IamRoleServiceAccount) *IamRole {
	iamRole := new(IamRole)
	iamRole.fromIRSA(oidcProviderArn, irsa)
	return iamRole
}

func (i *IamRole) fromIRSA(oidcProviderArn string, irsa *irsav1alpha1.IamRoleServiceAccount) {
	i.RoleArn = irsa.Status.RoleArn
	i.RoleName = RoleNameByArn(i.RoleArn)

	policy := irsa.Spec.Policy

	if policy != nil {
		i.ManagedPolicies = policy.ManagedPolicies
		if policy.InlinePolicy != nil {
			ip := policy.InlinePolicy
			stses := policy.InlinePolicy.Statement
			i.InlinePolicy = &RoleDocument{
				Version:   ip.Version,
				Statement: make([]RoleStatement, len(stses)),
			}
			for idx, sts := range stses {
				i.InlinePolicy.Statement[idx] = roleStatementFromIRSAStatementSpec(&sts)
			}
		}
	}

	arp := NewAssumeRolePolicy(oidcProviderArn, irsa.GetNamespace(), irsa.GetName())
	i.AssumeRolePolicy = &arp

}

type RoleDocument struct {
	Version   string
	Statement []RoleStatement
}

func (r *RoleDocument) RoleDocumentPolicyDocument() (string, error) {
	bytes, err := json.Marshal(r)
	if err != nil {
		return "", err
	}
	return string(bytes), nil
}

type RoleStatement struct {
	Effect    StatementEffect
	Action    []string
	Resource  []string
	Condition StatementCondition `json:"Condition,omitempty"`
}

type StatementCondition map[string]map[string]string

func roleStatementFromIRSAStatementSpec(sts *irsav1alpha1.StatementSpec) RoleStatement {
	return RoleStatement{
		Effect:    StatementEffect(sts.Effect),
		Action:    sts.Action,
		Resource:  sts.Resource,
		Condition: StatementCondition(sts.Condition),
	}
}

// AssumeRoleStatement defines the structure of trust relationship policy in aws iam role
type AssumeRoleStatement struct {
	Effect    StatementEffect
	Principal struct {
		Federated string
	}
	Action    string
	Condition struct {
		StringEquals map[string]string
	}
}

// AssumeRoleDocument defines the trust relationship of aws iam role
type AssumeRoleDocument struct {
	Version   string
	Statement []AssumeRoleStatement
}

func (t *AssumeRoleDocument) AssumeRoleDocumentPolicyDocument() (string, error) {
	bytes, err := json.Marshal(t)
	if err != nil {
		return "", err
	}
	return string(bytes), nil
}

func (t *AssumeRoleDocument) IsAllowOIDC(oidcProviderArn, namespace, serviceAccountName string) bool {
	if t == nil {
		return false
	}
	for _, st := range t.Statement {
		if st.Action == AssumeRoleWithWebIdentityAction && st.Principal.Federated == oidcProviderArn {
			if val := st.Condition.StringEquals[fmt.Sprintf("%s:sub", getIssuerHostpath(oidcProviderArn))]; val == fmt.Sprintf("system:serviceaccount:%s:%s", namespace, serviceAccountName) {
				return true
			}
		}
	}
	return false
}

func NewAssumeRolePolicyDoc(oidcProviderArn, namespace, serviceAccountName string) (string, error) {
	// resource : https://aws.amazon.com/blogs/opensource/introducing-fine-grained-iam-roles-service-accounts

	// then create the json formatted Trust policy
	bytes, err := json.Marshal(NewAssumeRolePolicy(oidcProviderArn, namespace, serviceAccountName))
	if err != nil {
		return "", err
	}

	return string(bytes), nil
}

func NewAssumeRolePolicy(oidcProviderArn, namespace, serviceAccountName string) AssumeRoleDocument {
	// resource : https://aws.amazon.com/blogs/opensource/introducing-fine-grained-iam-roles-service-accounts

	// then create the json formatted Trust policy
	return AssumeRoleDocument{
		Version: "2012-10-17",
		Statement: []AssumeRoleStatement{
			{
				Effect: StatementAllow,
				Principal: struct{ Federated string }{
					Federated: string(oidcProviderArn),
				},
				Action: AssumeRoleWithWebIdentityAction,
				Condition: struct {
					StringEquals map[string]string
				}{
					StringEquals: map[string]string{
						fmt.Sprintf("%s:sub", getIssuerHostpath(oidcProviderArn)): fmt.Sprintf("system:serviceaccount:%s:%s", namespace, serviceAccountName),
					},
				},
			},
		},
	}
}

func getIssuerHostpath(oidcProviderArn string) string {
	// we extract the issuerHostpath from the oidcProviderARN (needed in the condition field)
	issuerHostpath := oidcProviderArn
	submatches := regexp.MustCompile(`(?s)/(.*)`).FindStringSubmatch(issuerHostpath)
	if len(submatches) == 2 {
		issuerHostpath = submatches[1]
	}
	return issuerHostpath
}

func RoleNameByArn(roleArn string) string {
	splits := strings.Split(roleArn, "/")
	return splits[len(splits)-1]
}
