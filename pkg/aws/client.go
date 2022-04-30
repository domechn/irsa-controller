package aws

import (
	"context"
	"encoding/json"

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
	trustEntitiesDoc, err := NewAssumeRolePolicyDoc(irsa.GetNamespace(), irsa.GetName(), c.oidcProvider)
	if err != nil {
		return "", errors.Wrap(err, "marshal assume role policy doc failed")
	}
	roleName := c.RoleName(irsa)
	// create role
	output, err := c.iamClient.CreateRole(&iam.CreateRoleInput{
		RoleName:                 aws.String(roleName),
		AssumeRolePolicyDocument: aws.String(trustEntitiesDoc),
	})
	if err != nil {
		return "", errors.Wrap(err, "create role in aws failed")
	}

	createdRoleArn := *output.Role.Arn

	// create inline policy if it is set
	inlinePolicyArn := ""
	if irsa.Spec.InlinePolicy != nil {
		policyOutput, err := c.iamClient.CreatePolicy(&iam.CreatePolicyInput{
			PolicyName: aws.String(roleName + "-inline"),
		})
		if err != nil {
			return createdRoleArn, errors.Wrap(err, "Create inline policy")
		}
		inlinePolicyArn = *policyOutput.Policy.Arn
	}

	// append managed policies and inline policy into role
	for _, policyArn := range append(irsa.Spec.ManagedPolicies, inlinePolicyArn) {
		if policyArn == "" {
			continue
		}
		if _, err := c.iamClient.AttachRolePolicy(&iam.AttachRolePolicyInput{
			RoleName:  aws.String(roleName),
			PolicyArn: aws.String(policyArn),
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
		var entity TrustEntity
		if err := json.Unmarshal([]byte(*role.AssumeRolePolicyDocument), &entity); err != nil {
			return nil, err
		}
		res.TrustEntity = &entity
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
	iam := &IamRole{
		RoleArn: *output.Role.Arn,
	}
	if output.Role.AssumeRolePolicyDocument != nil {
		var entity TrustEntity
		if err := json.Unmarshal([]byte(*output.Role.AssumeRolePolicyDocument), &entity); err != nil {
			return nil, err
		}
		iam.TrustEntity = &entity
	}
	return iam, nil
}

func (c *IamClient) AllowServiceAccountAccess(ctx context.Context, role *IamRole, oidcProviderArn, namespace, serviceAccountName string) error {
	// c.iamClient.Update(*iam.UpdateAssumeRolePolicyInput)
	return nil
}

func (c *IamClient) Delete(ctx context.Context, roleArn string) error {
	return nil
}
