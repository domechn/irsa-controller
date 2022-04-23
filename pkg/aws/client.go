package aws

import "context"

type Client struct {
}

func (c *Client) Create(ctx context.Context) (string, error) {
	return "", nil
}

func (c *Client) UpdateTrustRelationship(ctx context.Context, roleArn string) error {
	return nil
}

func (c *Client) Delete(ctx context.Context, roleArn string) error {
	return nil
}
