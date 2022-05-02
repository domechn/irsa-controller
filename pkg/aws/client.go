package aws

import (
	"context"
	"encoding/json"
	"fmt"

	"domc.me/irsa-controller/api/v1alpha1"
	"github.com/aws/aws-sdk-go/aws"
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

func NewIamClient(clusterName, iamRolePrefix string) *IamClient {
	awsconf := aws.NewConfig()
	session := session.New()
	// client.New(aws.NewConfig(), info metadata.ClientInfo, handlers request.Handlers, options ...func(*client.Client))
	return &IamClient{
		prefix:      iamRolePrefix,
		clusterName: clusterName,
		iamClient:   iam.New(session, awsconf),
		// TODO: update tags
		additionalTags: make(map[string]string),
	}
}

// Create creates aws iam role in aws account and attaches managed policies arn to role
// also create inline policy if defined in irsa
// returns arn of aws iam role and arn of inline policy if inline policy is created
func (c *IamClient) Create(ctx context.Context, irsa *v1alpha1.IamRoleServiceAccount) (string, string, error) {

	iamRole := NewIamRole(irsa)

	assumeRoleDocument, err := iamRole.AssumeRolePolicy.AssumeRoleDocumentPolicyDocument()
	if err != nil {
		return "", "", errors.Wrap(err, "Marshal assume role policy doc failed")
	}

	roleName := c.RoleName(irsa)
	// create role
	output, err := c.iamClient.CreateRoleWithContext(ctx, &iam.CreateRoleInput{
		RoleName:                 aws.String(roleName),
		AssumeRolePolicyDocument: aws.String(assumeRoleDocument),
		Tags:                     c.getIamRoleTags(),
	})
	if err != nil {
		return "", "", errors.Wrap(err, "Create role in aws failed")
	}

	createdRoleArn := *output.Role.Arn

	// create inline policy if it is set
	inlinePolicyArn := ""
	pd, err := iamRole.InlinePolicy.RoleDocumentPolicyDocument()
	if err != nil {
		return "", "", errors.Wrap(err, "Create inline policy in aws failed")
	}
	if iamRole.InlinePolicy != nil {
		policyOutput, err := c.iamClient.CreatePolicyWithContext(ctx, &iam.CreatePolicyInput{
			PolicyName:     aws.String(c.getInlinePolicyName(roleName)),
			PolicyDocument: aws.String(pd),
		})
		if err != nil {
			return createdRoleArn, inlinePolicyArn, errors.Wrap(err, "Create inline policy")
		}
		inlinePolicyArn = *policyOutput.Policy.Arn
	}

	// append managed policies and inline policy into role
	if err := c.AttachRolePolicy(ctx, roleName, append(iamRole.ManagedPolicies, inlinePolicyArn)); err != nil {
		return createdRoleArn, inlinePolicyArn, errors.Wrap(err, "Attach managed policies failed")
	}

	return createdRoleArn, inlinePolicyArn, nil
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

func (c *IamClient) DeAttachRolePolicy(ctx context.Context, roleName string, polices []string) error {
	for _, policyArn := range polices {
		if policyArn == "" {
			continue
		}
		if _, err := c.iamClient.DeleteRolePolicyWithContext(ctx, &iam.DeleteRolePolicyInput{
			RoleName: aws.String(roleName),
			// TODO: fix get policy name by arn
			PolicyName: aws.String(RoleNameByArn(policyArn)),
		}); err != nil {
			return errors.Wrap(err, "DeAttach role policy failed")
		}
	}
	return nil
}

func (c *IamClient) UpdateAssumePolicy(ctx context.Context, roleName string, assumePolicy *AssumeRoleDocument) error {
	doc, err := assumePolicy.AssumeRoleDocumentPolicyDocument()
	if err != nil {
		return errors.Wrap(err, "Marshal aasume policy failed")
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
	fixedTags := c.getIamRoleTags()
	for k, v := range tags {
		fixedTags = append(fixedTags, &iam.Tag{
			Key:   aws.String(k),
			Value: aws.String(v),
		})
	}
	// TODO: update tags to aws iam role
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
	output, err := c.iamClient.GetRoleWithContext(ctx, &iam.GetRoleInput{
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

func (c *IamClient) getIamRoleTags() []*iam.Tag {
	var res []*iam.Tag
	// fixed key value: managed by irsa-controller
	c.additionalTags[IrsaContollerManagedTagKey] = IrsaContollerManagedTagVal
	for k, v := range c.additionalTags {
		res = append(res, &iam.Tag{
			Key:   aws.String(k),
			Value: aws.String(v),
		})
	}
	return res
}
