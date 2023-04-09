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
	"errors"
	"fmt"
	"path/filepath"
	"testing"

	acktypes "github.com/aws-controllers-k8s/runtime/pkg/types"
	svcsdk "github.com/aws/aws-sdk-go/service/elasticache"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"

	mocksvcsdkapi "github.com/aws-controllers-k8s/elasticache-controller/mocks/aws-sdk-go/elasticache"
	"github.com/aws-controllers-k8s/elasticache-controller/pkg/testutil"
)

// TestDeclarativeTestSuite runs the test suite for replication group
func TestDeclarativeTestSuite(t *testing.T) {
	var ts = testutil.TestSuite{}
	testutil.LoadFromFixture(filepath.Join("testdata", "test_suite.yaml"), &ts)
	var delegate = testRunnerDelegate{t: t}
	var runner = testutil.TestSuiteRunner{TestSuite: &ts, Delegate: &delegate}
	runner.RunTests()
}

// testRunnerDelegate implements testutil.TestRunnerDelegate
type testRunnerDelegate struct {
	t *testing.T
}

func (d *testRunnerDelegate) ResourceDescriptor() acktypes.AWSResourceDescriptor {
	return &resourceDescriptor{}
}

func (d *testRunnerDelegate) ResourceManager(mocksdkapi *mocksvcsdkapi.ElastiCacheAPI) acktypes.AWSResourceManager {
	return provideResourceManagerWithMockSDKAPI(mocksdkapi)
}

func (d *testRunnerDelegate) GoTestRunner() *testing.T {
	return d.t
}

func (d *testRunnerDelegate) EmptyServiceAPIOutput(apiName string) (interface{}, error) {
	if apiName == "" {
		return nil, errors.New("no API name specified")
	}
	//TODO: use reflection, template to auto generate this block/method.
	switch apiName {
	case "DescribeReplicationGroupsWithContext":
		var output svcsdk.DescribeReplicationGroupsOutput
		return &output, nil
	case "ListAllowedNodeTypeModifications":
		var output svcsdk.ListAllowedNodeTypeModificationsOutput
		return &output, nil
	case "DescribeEventsWithContext":
		var output svcsdk.DescribeEventsOutput
		return &output, nil
	case "CreateReplicationGroupWithContext":
		var output svcsdk.CreateReplicationGroupOutput
		return &output, nil
	case "DecreaseReplicaCountWithContext":
		var output svcsdk.DecreaseReplicaCountOutput
		return &output, nil
	case "DeleteReplicationGroupWithContext":
		var output svcsdk.DeleteReplicationGroupOutput
		return &output, nil
	case "DescribeCacheClustersWithContext":
		var output svcsdk.DescribeCacheClustersOutput
		return &output, nil
	case "IncreaseReplicaCountWithContext":
		var output svcsdk.IncreaseReplicaCountOutput
		return &output, nil
	case "ModifyReplicationGroupShardConfigurationWithContext":
		var output svcsdk.ModifyReplicationGroupShardConfigurationOutput
		return &output, nil
	case "ModifyReplicationGroupWithContext":
		var output svcsdk.ModifyReplicationGroupOutput
		return &output, nil
	case "ListTagsForResourceWithContext":
		var output svcsdk.TagListMessage
		return &output, nil
	}
	return nil, errors.New(fmt.Sprintf("no matching API name found for: %s", apiName))
}

func (d *testRunnerDelegate) Equal(a acktypes.AWSResource, b acktypes.AWSResource) bool {
	ac := a.(*resource)
	bc := b.(*resource)
	opts := []cmp.Option{cmpopts.EquateEmpty()}

	var specMatch = false
	if cmp.Equal(ac.ko.Spec, bc.ko.Spec, opts...) {
		specMatch = true
	} else {
		fmt.Printf("Difference ko.Spec (-expected +actual):\n\n")
		fmt.Println(cmp.Diff(ac.ko.Spec, bc.ko.Spec, opts...))
		specMatch = false
	}

	var statusMatch = false
	if cmp.Equal(ac.ko.Status, bc.ko.Status, opts...) {
		statusMatch = true
	} else {
		fmt.Printf("Difference ko.Status (-expected +actual):\n\n")
		fmt.Println(cmp.Diff(ac.ko.Status, bc.ko.Status, opts...))
		statusMatch = false
	}

	return statusMatch && specMatch
}
