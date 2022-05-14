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
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"domc.me/irsa-controller/api/v1alpha1"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/iam"
	"github.com/aws/aws-sdk-go/service/iam/iamiface"
	"github.com/pkg/errors"
)

type IamClient struct {
	prefix         string
	clusterName    string
	additionalTags map[string]string
	iamClient      iamiface.IAMAPI
}

func NewIamClient(clusterName, iamRolePrefix string, additionalTagsArgs []string, config *AWSConfig) *IamClient {
	awsconf := aws.NewConfig()
	if config != nil {
		awsconf = aws.NewConfig().WithEndpoint(config.Endpoint).WithRegion(config.Region).WithDisableSSL(config.DisableSSL).WithCredentials(credentials.NewStaticCredentials(config.AccessKeyID, config.SecretAccessKey, ""))
	}
	session := session.New()
	return NewIamClientWithIamAPI(clusterName, iamRolePrefix, additionalTagsArgs, iam.New(session, awsconf))
}

func NewIamClientWithIamAPI(clusterName, iamRolePrefix string, additionalTagsArgs []string, iamClient iamiface.IAMAPI) *IamClient {
	return &IamClient{
		prefix:         iamRolePrefix,
		clusterName:    clusterName,
		iamClient:      iamClient,
		additionalTags: parseAdditionalTagsArgs(additionalTagsArgs),
	}
}

func parseAdditionalTagsArgs(args []string) map[string]string {
	at := make(map[string]string)
	for _, arg := range args {
		if !strings.Contains(arg, "=") {
			continue
		}
		splits := strings.SplitN(arg, "=", 2)
		at[splits[0]] = splits[1]
	}
	return at
}

// Create creates aws iam role in aws account and attaches managed policies arn to role
// also create inline policy if defined in irsa
// returns arn of aws iam role and arn of inline policy if inline policy is created
func (c *IamClient) Create(ctx context.Context, oidcProvider string, irsa *v1alpha1.IamRoleServiceAccount) (string, error) {
	iamRole := NewIamRole(oidcProvider, irsa, c.additionalTags)

	assumeRoleDocument, err := iamRole.AssumeRolePolicy.AssumeRoleDocumentPolicyDocument()
	if err != nil {
		return "", errors.Wrap(err, "Marshal assume role policy doc failed")
	}

	roleName := c.RoleName(irsa)
	// create role
	output, err := c.iamClient.CreateRoleWithContext(ctx, &iam.CreateRoleInput{
		RoleName:                 aws.String(roleName),
		AssumeRolePolicyDocument: aws.String(assumeRoleDocument),
		Tags:                     getIamRoleTags(iamRole.Tags),
	})
	if err != nil {
		return "", errors.Wrap(err, "Create role in aws failed")
	}

	createdRoleArn := *output.Role.Arn

	// create inline policy if it is set
	pd, err := iamRole.InlinePolicy.RoleDocumentPolicyDocument()
	if err != nil {
		return "", errors.Wrap(err, "Create inline policy in aws failed")
	}
	if iamRole.InlinePolicy != nil {
		_, err = c.iamClient.PutRolePolicy(&iam.PutRolePolicyInput{
			PolicyName:     aws.String(c.getInlinePolicyName(roleName)),
			PolicyDocument: aws.String(pd),
			RoleName:       aws.String(roleName),
		})
		if err != nil {
			return createdRoleArn, errors.Wrap(err, "Create inline policy")
		}
	}

	// append managed policies and inline policy into role
	if err := c.AttachRolePolicy(ctx, roleName, iamRole.ManagedPolicies); err != nil {
		return createdRoleArn, errors.Wrap(err, "Attach managed policies failed")
	}

	return createdRoleArn, nil
}

func (c *IamClient) RoleName(irsa *v1alpha1.IamRoleServiceAccount) string {
	return irsa.AwsIamRoleName(c.prefix, c.clusterName)
}

func (c *IamClient) AttachRolePolicy(ctx context.Context, roleName string, polices []string) error {
	for _, policyArn := range polices {
		if policyArn == "" {
			continue
		}
		if _, err := c.iamClient.AttachRolePolicyWithContext(ctx, &iam.AttachRolePolicyInput{
			RoleName:  aws.String(roleName),
			PolicyArn: aws.String(policyArn),
		}); err != nil {
			return errors.Wrap(err, "Attach role policy failed")
		}
	}
	return nil
}

func (c *IamClient) DetachRolePolicy(ctx context.Context, roleName string, polices []string) error {
	for _, policyArn := range polices {
		if policyArn == "" {
			continue
		}
		fmt.Println(policyArn)
		if _, err := c.iamClient.DetachRolePolicyWithContext(ctx, &iam.DetachRolePolicyInput{
			RoleName:  aws.String(roleName),
			PolicyArn: aws.String(policyArn),
		}); err != nil {
			return errors.Wrap(err, "DeAttach role policy failed")
		}
	}
	return nil
}

func (c *IamClient) UpdateAssumePolicy(ctx context.Context, roleName string, assumePolicy *AssumeRoleDocument) error {
	doc, err := assumePolicy.AssumeRoleDocumentPolicyDocument()
	if err != nil {
		return errors.Wrap(err, "Marshal assume policy failed")
	}
	_, err = c.iamClient.UpdateAssumeRolePolicyWithContext(ctx, &iam.UpdateAssumeRolePolicyInput{
		RoleName:       aws.String(roleName),
		PolicyDocument: aws.String(doc),
	})
	if err != nil {
		return errors.Wrap(err, "Update assume role policy failed")
	}
	return nil
}

func (c *IamClient) UpdateTags(ctx context.Context, roleName string, tags map[string]string) error {
	// append fixed tags setten in controller started
	_, err := c.iamClient.TagRole(&iam.TagRoleInput{
		RoleName: aws.String(roleName),
		Tags:     getIamRoleTags(tags),
	})
	if err != nil {
		return errors.Wrap(err, "Tag iam role failed")
	}
	return nil
}

func (c *IamClient) UpdatePolicy(ctx context.Context, policyArn string, policy *RoleDocument) error {
	policyDocument, err := policy.RoleDocumentPolicyDocument()
	if err != nil {
		return errors.Wrap(err, "Update policy failed")
	}

	_, err = c.iamClient.CreatePolicyVersionWithContext(ctx, &iam.CreatePolicyVersionInput{
		PolicyArn:      aws.String(policyArn),
		PolicyDocument: aws.String(policyDocument),
		SetAsDefault:   aws.Bool(true),
	})
	return errors.Wrap(err, "Update policy failed")
}

func (c *IamClient) UpdateInlinePolicy(ctx context.Context, roleName string, policy *RoleDocument) error {
	policyDocument, err := policy.RoleDocumentPolicyDocument()
	if err != nil {
		return errors.Wrap(err, "Update policy failed")
	}

	_, err = c.iamClient.PutRolePolicy(&iam.PutRolePolicyInput{
		PolicyDocument: aws.String(policyDocument),
		PolicyName:     aws.String(c.getInlinePolicyName(roleName)),
		RoleName:       aws.String(roleName),
	})

	return errors.Wrap(err, "Update inline policy failed")
}

func (c *IamClient) transfer(role *iam.Role, managedPolicyArns []string, inlinePolicy *iam.PolicyDetail) (*IamRole, error) {
	res := new(IamRole)
	res.RoleArn = *role.Arn
	res.RoleName = *role.RoleName
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
	res.ManagedPolicies = managedPolicyArns
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
	output, err := c.iamClient.GetRoleWithContext(ctx, &iam.GetRoleInput{
		RoleName: aws.String(roleName),
	})
	if err != nil {
		return nil, errors.Wrap(err, "Get role with context failed")
	}
	// TODO: paging if the count of polices over than 100
	policiesOut, err := c.iamClient.ListAttachedRolePoliciesWithContext(ctx, &iam.ListAttachedRolePoliciesInput{
		RoleName: aws.String(roleName),
	})
	if err != nil {
		return nil, errors.Wrap(err, "List attached role policies failed")
	}
	var managedPolicyArns []string
	for _, p := range policiesOut.AttachedPolicies {
		managedPolicyArns = append(managedPolicyArns, *p.PolicyArn)
	}

	inlinePolicyName := c.getInlinePolicyName(roleName)
	ipo, err := c.iamClient.GetRolePolicyWithContext(ctx, &iam.GetRolePolicyInput{
		RoleName:   aws.String(roleName),
		PolicyName: aws.String(inlinePolicyName),
	})
	// err is not no such entities
	if err != nil && !ErrIsNotFound(err) {
		return nil, errors.Wrap(err, "Get inline policy failed")
	}
	var inlinePolicyDetail *iam.PolicyDetail
	// not found inline policy
	if ipo != nil {
		inlinePolicyDetail = new(iam.PolicyDetail)

		inlinePolicyDetail.PolicyDocument = ipo.PolicyDocument
		inlinePolicyDetail.PolicyName = ipo.PolicyName
	}
	iam, err := c.transfer(output.Role, managedPolicyArns, inlinePolicyDetail)
	return iam, err
}

func (c *IamClient) AllowServiceAccountAccess(ctx context.Context, role *IamRole, oidcProviderArn, namespace, serviceAccountName string) error {
	policy := NewAssumeRolePolicy(oidcProviderArn, namespace, serviceAccountName)
	role.AssumeRolePolicy.Statement = append(role.AssumeRolePolicy.Statement, policy.Statement...)
	doc, err := role.AssumeRolePolicy.AssumeRoleDocumentPolicyDocument()
	if err != nil {
		return errors.Wrap(err, "Allow serviceaccount access failed")
	}
	_, err = c.iamClient.UpdateAssumeRolePolicyWithContext(ctx, &iam.UpdateAssumeRolePolicyInput{
		RoleName:       aws.String(role.RoleName),
		PolicyDocument: aws.String(doc),
	})
	if err != nil {
		return errors.Wrap(err, "Update assume role policy failed")
	}
	return nil
}

func (c *IamClient) Delete(ctx context.Context, roleArn string) error {
	roleName := RoleNameByArn(roleArn)

	// TODO: fix (role|managed) policies pager
	rolePolicies, err := c.iamClient.ListRolePoliciesWithContext(ctx, &iam.ListRolePoliciesInput{
		RoleName: aws.String(roleName),
	})
	if err != nil {
		return errors.Wrap(err, "List role policies failed")
	}
	managedPolicies, err := c.iamClient.ListAttachedRolePoliciesWithContext(ctx, &iam.ListAttachedRolePoliciesInput{
		RoleName: aws.String(roleName),
	})
	if err != nil {
		return errors.Wrap(err, "List role policies failed")
	}

	// clean role policies
	for _, policyName := range rolePolicies.PolicyNames {
		if _, err := c.iamClient.DeleteRolePolicyWithContext(ctx, &iam.DeleteRolePolicyInput{
			RoleName:   aws.String(roleName),
			PolicyName: policyName,
		}); err != nil {
			return errors.Wrap(err, "Delete role policy failed")
		}
	}

	// detach managed role
	for _, policy := range managedPolicies.AttachedPolicies {
		if _, err := c.iamClient.DetachRolePolicyWithContext(ctx, &iam.DetachRolePolicyInput{
			RoleName:  aws.String(roleName),
			PolicyArn: policy.PolicyArn,
		}); err != nil {
			return errors.Wrap(err, "Detach role policy failed")
		}
	}

	_, err = c.iamClient.DeleteRoleWithContext(ctx, &iam.DeleteRoleInput{
		RoleName: aws.String(roleName),
	})
	if err != nil {
		return errors.Wrap(err, "Delete iam role failed")
	}
	return nil
}

func (c *IamClient) DeletePolicy(ctx context.Context, policyArn string) error {
	_, err := c.iamClient.DeletePolicyWithContext(ctx, &iam.DeletePolicyInput{
		PolicyArn: aws.String(policyArn),
	})
	if err != nil {
		return errors.Wrap(err, "Delete policy failed")
	}
	return nil
}

func (c *IamClient) getInlinePolicyName(roleName string) string {
	return fmt.Sprintf("%s-inline-policy", roleName)
}

func (c *IamClient) GetAdditionalTags() map[string]string {
	return c.additionalTags
}

func getIamRoleTags(tags map[string]string) []*iam.Tag {
	var res []*iam.Tag
	for k, v := range tags {
		tag := &iam.Tag{
			Key:   aws.String(k),
			Value: aws.String(v),
		}
		if k == IrsaContollerManagedTagKey {
			tag.Value = aws.String(IrsaContollerManagedTagVal)
		}
		res = append(res, tag)
	}
	return res
}
