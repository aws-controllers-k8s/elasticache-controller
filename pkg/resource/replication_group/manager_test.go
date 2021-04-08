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

package replication_group

import (
	"context"
	mocksvcsdkapi "github.com/aws-controllers-k8s/elasticache-controller/mocks/aws-sdk-go/elasticache"
	"github.com/aws-controllers-k8s/elasticache-controller/pkg/testutil"
	svcsdk "github.com/aws/aws-sdk-go/service/elasticache"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"path/filepath"
	"testing"
)

// TestReadOne_Exists runs resource manager ReadOne test scenario
// TODO: Declarative Tests make this method redundant;remove this test when Declarative Test runner is checked in.
func TestReadOne_Exists(t *testing.T) {
	assert := assert.New(t)

	mocksdkapi := &mocksvcsdkapi.ElastiCacheAPI{}
	rm := provideResourceManagerWithMockSDKAPI(mocksdkapi)

	// Setup mock current ko state.
	var rd = resourceDescriptor{}
	ro := rd.EmptyRuntimeObject()
	testutil.LoadFromFixture(filepath.Join("testdata", "replication_group", "cr", "rg_cmd_create_completed.yaml"), ro)
	desired := rd.ResourceFromRuntimeObject(ro)

	// Setup mock API response
	// Describe RG
	var mockDescribeOutput svcsdk.DescribeReplicationGroupsOutput
	testutil.LoadFromFixture(filepath.Join("testdata", "replication_group", "read_one", "rg_cmd_create_completed.json"), &mockDescribeOutput)
	mocksdkapi.On("DescribeReplicationGroupsWithContext", mock.Anything, mock.Anything).Return(&mockDescribeOutput, nil)
	// ListAllowedNodeTypeModifications
	var mockAllowedNodeTypeOutput svcsdk.ListAllowedNodeTypeModificationsOutput
	testutil.LoadFromFixture(filepath.Join("testdata", "allowed_node_types", "read_many", "rg_cmd_allowed_node_types.json"), &mockAllowedNodeTypeOutput)
	mocksdkapi.On("ListAllowedNodeTypeModifications", mock.Anything, mock.Anything).Return(&mockAllowedNodeTypeOutput, nil)
	// DescribeEvents
	var mockDescribeEventsOutput svcsdk.DescribeEventsOutput
	testutil.LoadFromFixture(filepath.Join("testdata", "events", "read_many", "rg_cmd_events.json"), &mockDescribeEventsOutput)
	mocksdkapi.On("DescribeEventsWithContext", mock.Anything, mock.Anything).Return(&mockDescribeEventsOutput, nil)

	var delegate = testRunnerDelegate{t: t}

	// Tests
	t.Run("ReadOne=NoDiff", func(t *testing.T) {
		// Given: describe RG response has no diff compared to latest ko state.
		// Expect: no change in ko.status
		latest, err := rm.ReadOne(context.Background(), desired)
		assert.Nil(err)
		assert.True(delegate.Equal(rm.concreteResource(desired), rm.concreteResource(latest)))
	})
}
