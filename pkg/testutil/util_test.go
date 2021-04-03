// Copyright Amazon.com Inc. or its affiliates. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License"). You may
// not use this file except in compliance with the License. A copy of the
// License is located at
//
//     http://aws.amazon.com/apache2.0/
//
// or in the "license" file accompanying this file. This file is distributed
// on an "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either
// express or implied. See the License for the specific language governing
// permissions and limitations under the License.

package testutil

import (
	ackerr "github.com/aws-controllers-k8s/runtime/pkg/errors"
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestCreateAWSError(t *testing.T) {
	assert := assert.New(t)

	// Basic case to test type conversion and extraction of error code/message
	t.Run("CreateAWSError", func(t *testing.T) {
		errorSpec := ServiceAPIError{Code: "ReplicationGroupNotFoundFault", Message: "ReplicationGroup rg-cmd not found"}
		respErr := CreateAWSError(errorSpec)

		awsErr, ok := ackerr.AWSError(respErr)

		assert.True(ok)
		assert.Equal("ReplicationGroupNotFoundFault", awsErr.Code())
		assert.Equal("ReplicationGroup rg-cmd not found", awsErr.Message())
	})


}