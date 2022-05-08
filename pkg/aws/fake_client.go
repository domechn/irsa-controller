package aws

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/request"
	"github.com/aws/aws-sdk-go/service/iam"
	"github.com/aws/aws-sdk-go/service/iam/iamiface"
)

type MockedIamClient struct {
	iamiface.IAMAPI
	mockRoles            map[string]*iam.Role
	mockAttachedPolicies map[string][]*iam.AttachedPolicy
}

func NewMockedIamClient() *MockedIamClient {
	return &MockedIamClient{
		mockRoles:            make(map[string]*iam.Role),
		mockAttachedPolicies: make(map[string][]*iam.AttachedPolicy),
	}
}

func (m *MockedIamClient) CreateRole(input *iam.CreateRoleInput) (*iam.CreateRoleOutput, error) {
	m.mockRoles[*input.RoleName] = &iam.Role{
		RoleName:                 input.RoleName,
		Arn:                      aws.String(fmt.Sprintf("arn:aws:iam::000000000000:role/%s", *input.RoleName)),
		AssumeRolePolicyDocument: input.AssumeRolePolicyDocument,
		Tags:                     input.Tags,
	}
	return &iam.CreateRoleOutput{
		Role: m.mockRoles[*input.RoleName],
	}, nil
}

func (m *MockedIamClient) CreateRoleWithContext(ctx context.Context, input *iam.CreateRoleInput, opts ...request.Option) (*iam.CreateRoleOutput, error) {
	return m.CreateRole(input)
}

func (m *MockedIamClient) GetRoleWithContext(ctx context.Context, input *iam.GetRoleInput, opts ...request.Option) (*iam.GetRoleOutput, error) {
	role, ok := m.mockRoles[*input.RoleName]
	if !ok {
		return nil, fmt.Errorf("%s", iam.ErrCodeNoSuchEntityException)
	}
	return &iam.GetRoleOutput{
		Role: role,
	}, nil
}

func (m *MockedIamClient) ListAttachedRolePoliciesWithContext(ctx context.Context, input *iam.ListAttachedRolePoliciesInput, opts ...request.Option) (*iam.ListAttachedRolePoliciesOutput, error) {
	return &iam.ListAttachedRolePoliciesOutput{
		AttachedPolicies: m.mockAttachedPolicies[*input.RoleName],
	}, nil
}

func (m *MockedIamClient) GetRolePolicyWithContext(ctx context.Context, input *iam.GetRolePolicyInput, opts ...request.Option) (*iam.GetRolePolicyOutput, error) {
	return &iam.GetRolePolicyOutput{
		RoleName:       input.RoleName,
		PolicyName:     input.PolicyName,
		PolicyDocument: aws.String(`{"Version":"2012-10-17","Statement":[]}`),
	}, nil
}

func (m *MockedIamClient) UpdateAssumeRolePolicyWithContext(ctx context.Context, input *iam.UpdateAssumeRolePolicyInput, opts ...request.Option) (*iam.UpdateAssumeRolePolicyOutput, error) {
	m.mockRoles[*input.RoleName].AssumeRolePolicyDocument = input.PolicyDocument
	return &iam.UpdateAssumeRolePolicyOutput{}, nil
}
