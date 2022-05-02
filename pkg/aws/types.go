package aws

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	irsav1alpha1 "domc.me/irsa-controller/api/v1alpha1"
)

type StatementEffect string

const (
	StatementAllow StatementEffect = "Allow"
	StatementDeny  StatementEffect = "Deny"

	AssumeRoleWithWebIdentityAction = "sts:AssumeRoleWithWebIdentity"
	// IrsaContollerManagedTag is a fixed tag key, if tag value is "y"
	// means this iam role is manged by irsa-controller
	IrsaContollerManagedTag = "irsa-controller"
)

type IamRole struct {
	// RoleArn is "" when role is not created
	RoleArn string
	// RoleName is "" if RoleArn is ""
	RoleName        string
	InlinePolicy    *RoleDocument
	ManagedPolicies []string
	// AssumeRolePolicy defines the trust relationship of iam role
	AssumeRolePolicy *AssumeRoleDocument
	Tags             map[string]string
}

func (i *IamRole) IsManagedByIrsaController() bool {
	if val, ok := i.Tags[IrsaContollerManagedTag]; ok {
		return val == "y"
	}

	return false
}

func NewIamRole(irsa *irsav1alpha1.IamRoleServiceAccount) *IamRole {
	iamRole := new(IamRole)
	iamRole.fromIRSA(irsa)
	return iamRole
}

func (i *IamRole) fromIRSA(irsa *irsav1alpha1.IamRoleServiceAccount) {
	i.RoleArn = irsa.Status.RoleArn
	i.RoleName = RoleNameByArn(i.RoleArn)

	i.ManagedPolicies = irsa.Spec.ManagedPolicies
	if irsa.Spec.InlinePolicy != nil {
		ip := irsa.Spec.InlinePolicy
		stses := irsa.Spec.InlinePolicy.Statements
		i.InlinePolicy = &RoleDocument{
			Version:   ip.Version,
			Statement: make([]RoleStatement, len(stses)),
		}
		for idx, sts := range stses {
			i.InlinePolicy.Statement[idx] = roleStatementFromIRSAStatementSpec(&sts)
		}
	}

	// todo: update oidc arn
	arp := newAssumeRolePolicy(irsa.GetNamespace(), irsa.GetName(), "")
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
	Condition map[string]map[string]string
}

func roleStatementFromIRSAStatementSpec(sts *irsav1alpha1.StatementSpec) RoleStatement {
	return RoleStatement{
		Effect:    StatementEffect(sts.Effect),
		Action:    sts.Action,
		Resource:  sts.Resource,
		Condition: sts.Condition,
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

func NewAssumeRolePolicyDoc(namespace, serviceAccountName, oidcProviderArn string) (string, error) {
	// resource : https://aws.amazon.com/blogs/opensource/introducing-fine-grained-iam-roles-service-accounts

	// then create the json formatted Trust policy
	bytes, err := json.Marshal(newAssumeRolePolicy(namespace, serviceAccountName, oidcProviderArn))
	if err != nil {
		return "", err
	}

	return string(bytes), nil
}

func newAssumeRolePolicy(namespace, serviceAccountName, oidcProviderArn string) AssumeRoleDocument {
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
