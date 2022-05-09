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
