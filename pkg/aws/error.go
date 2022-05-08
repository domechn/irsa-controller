package aws

import (
	"strings"

	"github.com/aws/aws-sdk-go/service/iam"
)

// ErrIsNotFound returns true if err is aws ErrCodeNoSuchEntityException
func ErrIsNotFound(err error) bool {
	return strings.Contains(err.Error(), iam.ErrCodeNoSuchEntityException)
}

// ErrIsNotFound returns true if err is aws ErrCodeEntityAlreadyExistsException
func ErrAlreadyExists(err error) bool {
	return strings.Contains(err.Error(), iam.ErrCodeEntityAlreadyExistsException)
}
