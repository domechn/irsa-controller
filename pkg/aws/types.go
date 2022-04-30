package aws

import (
	"encoding/json"
	"fmt"
	"regexp"
)

type StatementEffect string

const (
	StatementAllow StatementEffect = "Allow"
	StatementDeny  StatementEffect = "Deny"

	AssumeRoleWithWebIdentityAction = "sts:AssumeRoleWithWebIdentity"
)

type IamRole struct {
	RoleArn         string
	InlinePolicy    *RoleDocument
	ManagedPolicies []string
	TrustEntity     *TrustEntity
	Tags            map[string]string
}

func (i *IamRole) IsManagedByIrsaController() bool {

	return false
}

type RoleDocument struct {
	Version   string
	Statement []RoleStatement
}

type RoleStatement struct {
	Effect    StatementEffect
	Principal struct {
		Federated string
	}
	Action    string
	Condition struct {
		StringEquals map[string]string
	}
}

type TrustEntity RoleDocument

func (t *TrustEntity) IsAllowOIDC(oidcProviderArn, namespace, serviceAccountName string) bool {
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
	bytes, err := json.Marshal(
		TrustEntity{
			Version: "2012-10-17",
			Statement: []RoleStatement{
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
		},
	)
	if err != nil {
		return "", err
	}

	return string(bytes), nil
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
