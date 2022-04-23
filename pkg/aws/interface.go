package aws

import "context"

type IamRoleClient interface {
	// Create creates aws iam role in aws account by role policies
	// returns roleArn of created role
	Create(ctx context.Context) (string, error)

	// UpdateTrustRelationship updates aws iam role's trust entities
	UpdateTrustRelationship(ctx context.Context, roleArn string) error

	// Delete the aws iam role
	Delete(ctx context.Context, roleArn string) error
}
