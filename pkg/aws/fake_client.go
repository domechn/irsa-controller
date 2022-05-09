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
	"fmt"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/request"
	"github.com/aws/aws-sdk-go/service/iam"
	"github.com/aws/aws-sdk-go/service/iam/iamiface"
)

type MockedIamClient struct {
	iamiface.IAMAPI
	mockRoles            map[string]*iam.Role
	mockRolePolicies     map[string][]*iam.Policy
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

func (m *MockedIamClient) ListRolePoliciesWithContext(ctx context.Context, input *iam.ListRolePoliciesInput, opts ...request.Option) (*iam.ListRolePoliciesOutput, error) {
	res := &iam.ListRolePoliciesOutput{}
	for _, policy := range m.mockRolePolicies[*input.RoleName] {
		res.PolicyNames = append(res.PolicyNames, policy.PolicyName)
	}
	return res, nil
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

func (m *MockedIamClient) DeleteRoleWithContext(ctx context.Context, input *iam.DeleteRoleInput, opts ...request.Option) (*iam.DeleteRoleOutput, error) {
	delete(m.mockRoles, *input.RoleName)
	return &iam.DeleteRoleOutput{}, nil
}
