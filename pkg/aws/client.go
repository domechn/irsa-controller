package aws

import (
	"context"

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

func (c *IamClient) Create(ctx context.Context, irsa *v1alpha1.IamRoleServiceAccount) (string, error) {
	trustEntities, err := NewAssumeRolePolicyDoc(irsa.GetNamespace(), irsa.GetName(), c.oidcProvider)
	if err != nil {
		return "", errors.Wrap(err, "marshal assume role policy doc failed")
	}
	output, err := c.iamClient.CreateRole(&iam.CreateRoleInput{
		RoleName:                 aws.String(irsa.AwsIamRoleName(c.prefix, c.clusterName)),
		AssumeRolePolicyDocument: aws.String(trustEntities),
	})
	if err != nil {
		return "", errors.Wrap(err, "create role in aws failed")
	}
	return *output.Role.Arn, nil
}

func (c *IamClient) Get(ctx context.Context, roleName string) (*IamRole, error) {
	output, err := c.iamClient.GetRole(&iam.GetRoleInput{
		RoleName: aws.String(roleName),
	})
	if err != nil {
		return nil, err
	}
	return &IamRole{
		RoleArn: *output.Role.Arn,
	}, nil
}

func (c *IamClient) UpdateTrustRelationship(ctx context.Context, roleArn string) error {
	return nil
}

func (c *IamClient) Delete(ctx context.Context, roleArn string) error {
	return nil
}
