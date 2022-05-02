package aws

import (
	"context"
	"encoding/json"
	"fmt"

	"domc.me/irsa-controller/api/v1alpha1"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/iam"
	"github.com/aws/aws-sdk-go/service/iam/iamiface"
	"github.com/pkg/errors"
)

type IamClient struct {
	prefix       string
	clusterName  string
	tags         map[string]string
	oidcProvider string
	iamClient    iamiface.IAMAPI
}

// Create returns arn of created role
func (c *IamClient) Create(ctx context.Context, irsa *v1alpha1.IamRoleServiceAccount) (string, error) {

	iamRole := NewIamRole(irsa)

	assumeRoleDocument, err := iamRole.AssumeRolePolicy.AssumeRoleDocumentPolicyDocument()
	if err != nil {
		return "", errors.Wrap(err, "marshal assume role policy doc failed")
	}

	roleName := c.RoleName(irsa)
	// create role
	output, err := c.iamClient.CreateRole(&iam.CreateRoleInput{
		RoleName:                 aws.String(roleName),
		AssumeRolePolicyDocument: aws.String(assumeRoleDocument),
	})
	if err != nil {
		return "", errors.Wrap(err, "create role in aws failed")
	}

	createdRoleArn := *output.Role.Arn

	// create inline policy if it is set
	inlinePolicyArn := ""
	pd, err := iamRole.InlinePolicy.RoleDocumentPolicyDocument()
	if err != nil {
		return "", errors.Wrap(err, "create inline policy in aws failed")
	}
	if iamRole.InlinePolicy != nil {
		policyOutput, err := c.iamClient.CreatePolicy(&iam.CreatePolicyInput{
			PolicyName:     aws.String(c.getInlinePolicyName(roleName)),
			PolicyDocument: aws.String(pd),
		})
		if err != nil {
			return createdRoleArn, errors.Wrap(err, "Create inline policy")
		}
		inlinePolicyArn = *policyOutput.Policy.Arn
	}

	// append managed policies and inline policy into role
	for _, pa := range append(iamRole.ManagedPolicies, inlinePolicyArn) {
		if pa == "" {
			continue
		}
		if _, err := c.iamClient.AttachRolePolicy(&iam.AttachRolePolicyInput{
			RoleName:  aws.String(roleName),
			PolicyArn: aws.String(pa),
		}); err != nil {
			return createdRoleArn, errors.Wrap(err, "Attach managedPolicy failed")
		}
	}

	return createdRoleArn, nil
}

func (c *IamClient) RoleName(irsa *v1alpha1.IamRoleServiceAccount) string {
	return irsa.AwsIamRoleName(c.prefix, c.clusterName)
}

func (c *IamClient) transfer(role *iam.Role, managedPolicies []string, inlinePolicy *iam.PolicyDetail) (*IamRole, error) {
	res := new(IamRole)
	res.RoleArn = *role.Arn
	if role.AssumeRolePolicyDocument != nil {
		var entity AssumeRoleDocument
		if err := json.Unmarshal([]byte(*role.AssumeRolePolicyDocument), &entity); err != nil {
			return nil, err
		}
		res.AssumeRolePolicy = &entity
	}
	res.Tags = make(map[string]string, len(role.Tags))
	// res.Tags
	for _, item := range role.Tags {
		if item.Key != nil {
			res.Tags[*item.Key] = ""
			if item.Value != nil {
				res.Tags[*item.Key] = *item.Value
			}
		}
	}
	res.ManagedPolicies = managedPolicies
	if inlinePolicy != nil && inlinePolicy.PolicyDocument != nil {
		var docJson RoleDocument
		err := json.Unmarshal([]byte(*inlinePolicy.PolicyDocument), &docJson)
		if err != nil {
			return nil, err
		}
		res.InlinePolicy = &docJson
	}
	return res, nil
}

func (c *IamClient) Get(ctx context.Context, roleName string) (*IamRole, error) {
	output, err := c.iamClient.GetRole(&iam.GetRoleInput{
		RoleName: aws.String(roleName),
	})
	if err != nil {
		return nil, err
	}
	// TODO: get policies from aws
	iam, err := c.transfer(output.Role, []string{}, &iam.PolicyDetail{})
	return iam, err
}

func (c *IamClient) AllowServiceAccountAccess(ctx context.Context, role *IamRole, oidcProviderArn, namespace, serviceAccountName string) error {
	// c.iamClient.Update(*iam.UpdateAssumeRolePolicyInput)
	return nil
}

func (c *IamClient) Delete(ctx context.Context, roleArn string) error {

	return nil
}

func (c *IamClient) getInlinePolicyName(roleName string) string {
	return fmt.Sprintf("%s-inline-policy", roleName)
}
