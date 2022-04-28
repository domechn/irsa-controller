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
)

type IamRole struct {
	RoleArn         string
	InlinePolicy    RoleDocument
	ManagedPolicies []string
	TrustEntities   []TrustEntity
}

type RoleDocument struct {
	Version   string
	Statement []RoleStatement
}

type RoleStatement struct {
	Effect    StatementEffect
	Principal struct {
		Federated string
	} `json:"Principal"`
	Action    string
	Condition struct {
		StringEquals map[string]string
	}
}

type TrustEntity struct {
}

func NewAssumeRolePolicyDoc(namespace, serviceAccountName, oidcProviderArn string) (string, error) {
	// resource : https://aws.amazon.com/blogs/opensource/introducing-fine-grained-iam-roles-service-accounts

	// we extract the issuerHostpath from the oidcProviderARN (needed in the condition field)
	issuerHostpath := oidcProviderArn
	submatches := regexp.MustCompile(`(?s)/(.*)`).FindStringSubmatch(issuerHostpath)
	if len(submatches) == 2 {
		issuerHostpath = submatches[1]
	}

	// then create the json formatted Trust policy
	bytes, err := json.Marshal(
		RoleDocument{
			Version: "2012-10-17",
			Statement: []RoleStatement{
				{
					Effect: StatementAllow,
					Principal: struct{ Federated string }{
						Federated: string(oidcProviderArn),
					},
					Action: "sts:AssumeRoleWithWebIdentity",
					Condition: struct {
						StringEquals map[string]string
					}{
						StringEquals: map[string]string{
							fmt.Sprintf("%s:sub", issuerHostpath): fmt.Sprintf("system:serviceaccount:%s:%s", namespace, serviceAccountName)},
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
