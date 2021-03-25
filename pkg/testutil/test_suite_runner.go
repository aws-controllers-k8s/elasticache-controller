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
	"context"
	"errors"
	"fmt"
	mocksvcsdkapi "github.com/aws-controllers-k8s/elasticache-controller/mocks/aws-sdk-go/elasticache"
	acktypes "github.com/aws-controllers-k8s/runtime/pkg/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"path/filepath"
	"strings"
	"testing"
)

// TestSuiteRunner runs given test suite config with the help of delegate supplied to it
type TestSuiteRunner struct {
	TestSuite *TestSuite
	Delegate  TestRunnerDelegate
}

// fixtureContext is runtime context for test scenario given fixture.
type fixtureContext struct {
	desired         acktypes.AWSResource
	latest          acktypes.AWSResource
	mocksdkapi      *mocksvcsdkapi.ElastiCacheAPI
	resourceManager acktypes.AWSResourceManager
}

// expectContext is runtime context for test scenario expectation fixture.
type expectContext struct {
	latest acktypes.AWSResource
	err    error
}

// TestRunnerDelegate provides interface for custom resource tests to implement.
// TestSuiteRunner depends on it to run tests for custom resource.
type TestRunnerDelegate interface {
	ResourceDescriptor() acktypes.AWSResourceDescriptor
	Equal(desired acktypes.AWSResource, latest acktypes.AWSResource) bool // remove it when ResourceDescriptor.Delta() is available
	ResourceManager(*mocksvcsdkapi.ElastiCacheAPI) acktypes.AWSResourceManager
	EmptyServiceAPIOutput(apiName string) (interface{}, error)
	GoTestRunner() *testing.T
}

// RunTests runs the tests from the test suite
func (runner *TestSuiteRunner) RunTests() {
	if runner.TestSuite == nil || runner.Delegate == nil {
		panic(errors.New("failed to run test suite"))
	}

	for _, test := range runner.TestSuite.Tests {
		fmt.Printf("Starting test: %s", test.Name)
		for _, scenario := range test.Scenarios {
			fmt.Printf("Running test scenario: %s", scenario.Name)
			fixtureCxt := runner.setupFixtureContext(&scenario.Fixture)
			expectationCxt := runner.setupExpectationContext(&scenario.Expect)
			runner.runTestScenario(scenario.Name, fixtureCxt, scenario.UnitUnderTest, expectationCxt)
		}
		fmt.Printf("Test: %s completed.", test.Name)
	}
}

// runTestScenario runs given test scenario which is expressed as: given fixture context, unit to test, expected fixture context.
func (runner *TestSuiteRunner) runTestScenario(scenarioName string, fixtureCxt *fixtureContext, unitUnderTest string, expectationCxt *expectContext) {
	t := runner.Delegate.GoTestRunner()
	t.Run(scenarioName, func(t *testing.T) {
		rm := fixtureCxt.resourceManager
		assert := assert.New(t)

		var actual acktypes.AWSResource = nil
		var err error = nil
		switch unitUnderTest {
		case "ReadOne":
			actual, err = rm.ReadOne(context.Background(), fixtureCxt.desired)
		case "Create":
			actual, err = rm.Create(context.Background(), fixtureCxt.desired)
		case "Update":
			delta := runner.Delegate.ResourceDescriptor().Delta(fixtureCxt.desired, fixtureCxt.latest)
			actual, err = rm.Update(context.Background(), fixtureCxt.desired, fixtureCxt.latest, delta)
		case "Delete":
			err = rm.Delete(context.Background(), fixtureCxt.desired)
		default:
			panic(errors.New(fmt.Sprintf("unit under test: %s not supported", unitUnderTest)))
		}
		runner.assertExpectations(assert, expectationCxt, actual, err)
	})
}

//assertExpectations validates the actual outcome against expected outcome
func (runner *TestSuiteRunner) assertExpectations(assert *assert.Assertions, expectationCxt *expectContext, actual acktypes.AWSResource, err error) {
	if expectationCxt.err != nil {
		assert.NotNil(err)
		assert.Nil(actual)
		assert.Equal(expectationCxt.err.Error(), err.Error())
	} else if expectationCxt.latest == nil { // successful delete scenario
		assert.Nil(err)
		assert.Nil(actual)
	} else {
		assert.Nil(err)
		delta := runner.Delegate.ResourceDescriptor().Delta(expectationCxt.latest, actual)
		assert.Equal(0, len(delta.Differences))
		if len(delta.Differences) > 0 {
			fmt.Println("Unexpected differences:")
			for _, difference := range delta.Differences {
				fmt.Printf("Path: %v, expected: %v, actual: %v", difference.Path, difference.A, difference.B)
			}
		}
		// Delta only contains `Spec` differences. Thus, need to have Delegate.Equal to compare `Status`.
		assert.True(runner.Delegate.Equal(expectationCxt.latest, actual))
	}
}

// setupFixtureContext provides runtime context for test scenario given fixture.
func (runner *TestSuiteRunner) setupFixtureContext(fixture *Fixture) *fixtureContext {
	if fixture == nil {
		return nil
	}
	var cxt = fixtureContext{}
	if fixture.DesiredState != "" {
		cxt.desired = runner.loadAWSResource(fixture.DesiredState)
	}
	if fixture.LatestState != "" {
		cxt.latest = runner.loadAWSResource(fixture.LatestState)
	}
	mocksdkapi := &mocksvcsdkapi.ElastiCacheAPI{}
	for _, serviceApi := range fixture.ServiceAPIs {
		if serviceApi.Operation != "" {

			if serviceApi.Error != "" {
				mocksdkapi.On(serviceApi.Operation, mock.Anything, mock.Anything).Return(nil, errors.New(serviceApi.Error))
			} else if serviceApi.Operation != "" && serviceApi.Output != "" {
				var outputObj, err = runner.Delegate.EmptyServiceAPIOutput(serviceApi.Operation)
				apiOutputFixturePath := append([]string{"testdata"}, strings.Split(serviceApi.Output, "/")...)
				LoadFromFixture(filepath.Join(apiOutputFixturePath...), outputObj)
				mocksdkapi.On(serviceApi.Operation, mock.Anything, mock.Anything).Return(outputObj, nil)
				if err != nil {
					panic(err)
				}
			}
		}
	}
	cxt.mocksdkapi = mocksdkapi
	cxt.resourceManager = runner.Delegate.ResourceManager(mocksdkapi)
	return &cxt
}

//setupExpectationContext provides runtime context for test scenario expectation fixture.
func (runner *TestSuiteRunner) setupExpectationContext(expect *Expect) *expectContext {
	if expect == nil {
		return nil
	}
	var cxt = expectContext{}
	if expect.LatestState != "" {
		cxt.latest = runner.loadAWSResource(expect.LatestState)
	}
	if expect.Error != "" {
		cxt.err = errors.New(expect.Error)
	}
	return &cxt
}

// loadAWSResource loads AWSResource from the supplied fixture file.
func (runner *TestSuiteRunner) loadAWSResource(resourceFixtureFilePath string) acktypes.AWSResource {
	if resourceFixtureFilePath == "" {
		panic(errors.New(fmt.Sprintf("resourceFixtureFilePath not specified")))
	}
	var rd = runner.Delegate.ResourceDescriptor()
	ro := rd.EmptyRuntimeObject()
	path := append([]string{"testdata"}, strings.Split(resourceFixtureFilePath, "/")...)
	LoadFromFixture(filepath.Join(path...), ro)
	return rd.ResourceFromRuntimeObject(ro)
}
