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

package cache_parameter_group

import (
	"context"

	svcapitypes "github.com/aws-controllers-k8s/elasticache-controller/apis/v1alpha1"
	ackv1alpha1 "github.com/aws-controllers-k8s/runtime/apis/core/v1alpha1"
	ackcompare "github.com/aws-controllers-k8s/runtime/pkg/compare"
	ackerr "github.com/aws-controllers-k8s/runtime/pkg/errors"
	"github.com/aws/aws-sdk-go-v2/aws"
	svcsdk "github.com/aws/aws-sdk-go-v2/service/elasticache"
	svcsdktypes "github.com/aws/aws-sdk-go-v2/service/elasticache/types"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	// The number of minutes worth of events to retrieve.
	// 14 days in minutes
	eventsDuration = 20160
)

// customSetOutputDescribeCacheParameters queries cache parameters for given cache parameter group
// and sets parameter name, value for 'user' source type parameters in supplied ko.Spec
// and sets detailed parameters for both 'user', 'system' source types parameters in supplied ko.Status
func (rm *resourceManager) customSetOutputDescribeCacheParameters(
	ctx context.Context,
	cacheParameterGroupName *string,
	ko *svcapitypes.CacheParameterGroup,
) error {
	// Populate latest.ko.Spec.ParameterNameValues with latest 'user' parameter values
	source := "user"
	parameters, err := rm.describeCacheParameters(ctx, cacheParameterGroupName, &source)
	if err != nil {
		return err
	}
	parameterNameValues := []*svcapitypes.ParameterNameValue{}
	for _, p := range parameters {
		sp := svcapitypes.ParameterNameValue{
			ParameterName:  p.ParameterName,
			ParameterValue: p.ParameterValue,
		}
		parameterNameValues = append(parameterNameValues, &sp)
	}
	ko.Spec.ParameterNameValues = parameterNameValues

	// Populate latest.ko.Status.Parameters with latest all (user, system) detailed parameters
	parameters, err = rm.describeCacheParameters(ctx, cacheParameterGroupName, nil)
	if err != nil {
		return err
	}
	ko.Status.Parameters = parameters
	err = rm.customSetOutputSupplementAPIs(ctx, cacheParameterGroupName, ko)
	if err != nil {
		return err
	}
	return nil
}

func (rm *resourceManager) customSetOutputSupplementAPIs(
	ctx context.Context,
	cacheParameterGroupName *string,
	ko *svcapitypes.CacheParameterGroup,
) error {
	events, err := rm.provideEvents(ctx, cacheParameterGroupName, 20)
	if err != nil {
		return err
	}
	ko.Status.Events = events
	return nil
}

func (rm *resourceManager) provideEvents(
	ctx context.Context,
	cacheParameterGroupName *string,
	maxRecords int64,
) ([]*svcapitypes.Event, error) {
	input := &svcsdk.DescribeEventsInput{}
	input.SourceType = svcsdktypes.SourceTypeCacheParameterGroup
	input.SourceIdentifier = cacheParameterGroupName
	input.MaxRecords = aws.Int32(int32(maxRecords))
	input.Duration = aws.Int32(eventsDuration)
	resp, err := rm.sdkapi.DescribeEvents(ctx, input)
	rm.metrics.RecordAPICall("READ_MANY", "DescribeEvents-CacheParameterGroup", err)
	if err != nil {
		rm.log.V(1).Info("Error during DescribeEvents-CacheParameterGroup", "error", err)
		return nil, err
	}
	events := []*svcapitypes.Event{}
	if resp.Events != nil {
		for _, respEvent := range resp.Events {
			event := &svcapitypes.Event{}
			if respEvent.Message != nil {
				event.Message = respEvent.Message
			}
			if respEvent.Date != nil {
				eventDate := metav1.NewTime(*respEvent.Date)
				event.Date = &eventDate
			}
			// Not copying redundant source id (replication id)
			// and source type (replication group)
			// into each event object
			events = append(events, event)
		}
	}
	return events, nil
}

// describeCacheParameters returns Cache Parameters for given Cache Parameter Group name and source
func (rm *resourceManager) describeCacheParameters(
	ctx context.Context,
	cacheParameterGroupName *string,
	source *string,
) ([]*svcapitypes.Parameter, error) {
	parameters := []*svcapitypes.Parameter{}
	var paginationMarker *string = nil
	for {
		input, err := rm.newDescribeCacheParametersRequestPayload(cacheParameterGroupName, source, paginationMarker)
		if err != nil {
			return nil, err
		}
		response, respErr := rm.sdkapi.DescribeCacheParameters(ctx, input)
		rm.metrics.RecordAPICall("READ_MANY", "DescribeCacheParameters", respErr)
		if respErr != nil {
			if awsErr, ok := ackerr.AWSError(respErr); ok && awsErr.ErrorCode() == "CacheParameterGroupNotFound" {
				return nil, ackerr.NotFound
			}
			rm.log.V(1).Info("Error during DescribeCacheParameters", "error", respErr)
			return nil, respErr
		}

		if response.Parameters == nil || len(response.Parameters) == 0 {
			break
		}
		for _, p := range response.Parameters {
			sp := svcapitypes.Parameter{
				ParameterName:        p.ParameterName,
				ParameterValue:       p.ParameterValue,
				Source:               p.Source,
				Description:          p.Description,
				IsModifiable:         p.IsModifiable,
				DataType:             p.DataType,
				AllowedValues:        p.AllowedValues,
				MinimumEngineVersion: p.MinimumEngineVersion,
			}
			parameters = append(parameters, &sp)
		}
		paginationMarker = response.Marker
		if paginationMarker == nil || *paginationMarker == "" ||
			response.Parameters == nil || len(response.Parameters) == 0 {
			break
		}
	}

	return parameters, nil
}

// newDescribeCacheParametersRequestPayload returns SDK-specific struct for the HTTP request
// payload of the DescribeCacheParameters API to get properties that have
// given cacheParameterGroupName and given source
func (rm *resourceManager) newDescribeCacheParametersRequestPayload(
	cacheParameterGroupName *string,
	source *string,
	paginationMarker *string,
) (*svcsdk.DescribeCacheParametersInput, error) {
	res := &svcsdk.DescribeCacheParametersInput{}

	if cacheParameterGroupName != nil {
		res.CacheParameterGroupName = cacheParameterGroupName
	}
	if source != nil {
		res.Source = source
	}
	if paginationMarker != nil {
		res.Marker = paginationMarker
	}
	return res, nil
}

// resetAllParameters resets cache parameters for given CacheParameterGroup in desired custom resource.
func (rm *resourceManager) resetAllParameters(
	ctx context.Context,
	desired *resource,
) (bool, error) {
	input := &svcsdk.ResetCacheParameterGroupInput{}
	if desired.ko.Spec.CacheParameterGroupName != nil {
		input.CacheParameterGroupName = desired.ko.Spec.CacheParameterGroupName
	}
	input.ResetAllParameters = aws.Bool(true)

	_, err := rm.sdkapi.ResetCacheParameterGroup(ctx, input)
	rm.metrics.RecordAPICall("UPDATE", "ResetCacheParameterGroup-ResetAllParameters", err)
	if err != nil {
		rm.log.V(1).Info("Error during ResetCacheParameterGroup-ResetAllParameters", "error", err)
		return false, err
	}
	return true, nil
}

// resetParameters resets given cache parameters for given CacheParameterGroup in desired custom resource.
func (rm *resourceManager) resetParameters(
	ctx context.Context,
	desired *resource,
	parameters []*svcapitypes.ParameterNameValue,
) (bool, error) {
	input := &svcsdk.ResetCacheParameterGroupInput{}
	if desired.ko.Spec.CacheParameterGroupName != nil {
		input.CacheParameterGroupName = desired.ko.Spec.CacheParameterGroupName
	}
	if parameters != nil && len(parameters) > 0 {
		parametersToReset := []*svcsdktypes.ParameterNameValue{}
		for _, parameter := range parameters {
			parameterToReset := &svcsdktypes.ParameterNameValue{}
			if parameter.ParameterName != nil {
				parameterToReset.ParameterName = parameter.ParameterName
			}
			if parameter.ParameterValue != nil {
				parameterToReset.ParameterValue = parameter.ParameterValue
			}
			parametersToReset = append(parametersToReset, parameterToReset)
		}
		parameterNameValues := make([]svcsdktypes.ParameterNameValue, len(parametersToReset))
		for i, parameter := range parametersToReset {
			if parameter != nil {
				parameterNameValues[i] = *parameter
			}
		}
		input.ParameterNameValues = parameterNameValues
	}

	_, err := rm.sdkapi.ResetCacheParameterGroup(ctx, input)
	rm.metrics.RecordAPICall("UPDATE", "ResetCacheParameterGroup", err)
	if err != nil {
		rm.log.V(1).Info("Error during ResetCacheParameterGroup", "error", err)
		return false, err
	}
	return true, nil
}

// saveParameters saves given cache parameters for given CacheParameterGroup in desired custom resource.
// This invokes the modify API in the batches of 20 parameters.
func (rm *resourceManager) saveParameters(
	ctx context.Context,
	desired *resource,
	parameters []*svcapitypes.ParameterNameValue,
) (bool, error) {
	modifyApiBatchSize := 20
	// Paginated save: 20 parameters in single api call
	parametersToSave := []svcsdktypes.ParameterNameValue{}
	for _, parameter := range parameters {
		parameterToSave := svcsdktypes.ParameterNameValue{}
		if parameter.ParameterName != nil {
			parameterToSave.ParameterName = parameter.ParameterName
		}
		if parameter.ParameterValue != nil {
			parameterToSave.ParameterValue = parameter.ParameterValue
		}
		parametersToSave = append(parametersToSave, parameterToSave)

		if len(parametersToSave) == modifyApiBatchSize {
			done, err := rm.modifyCacheParameterGroup(ctx, desired, parametersToSave)
			if !done || err != nil {
				return false, err
			}
			// re-init to save next set of parameters
			parametersToSave = []svcsdktypes.ParameterNameValue{}
		}
	}
	if len(parametersToSave) > 0 { // when len(parameters) % modifyApiBatchSize != 0
		done, err := rm.modifyCacheParameterGroup(ctx, desired, parametersToSave)
		if !done || err != nil {
			return false, err
		}
	}
	return true, nil
}

// modifyCacheParameterGroup saves given cache parameters for given CacheParameterGroup in desired custom resource.
// see 'saveParameters' method for paginated API call
func (rm *resourceManager) modifyCacheParameterGroup(
	ctx context.Context,
	desired *resource,
	parameters []svcsdktypes.ParameterNameValue,
) (bool, error) {
	input := &svcsdk.ModifyCacheParameterGroupInput{}
	if desired.ko.Spec.CacheParameterGroupName != nil {
		input.CacheParameterGroupName = desired.ko.Spec.CacheParameterGroupName
	}
	input.ParameterNameValues = parameters
	_, err := rm.sdkapi.ModifyCacheParameterGroup(ctx, input)
	rm.metrics.RecordAPICall("UPDATE", "ModifyCacheParameterGroup", err)
	if err != nil {
		rm.log.V(1).Info("Error during ModifyCacheParameterGroup", "error", err)
		return false, err
	}
	return true, nil
}

// Helper method to set Condition on custom resource.
func (rm *resourceManager) setCondition(
	ko *svcapitypes.CacheParameterGroup,
	cType ackv1alpha1.ConditionType,
	cStatus corev1.ConditionStatus,
) {
	if ko.Status.Conditions == nil {
		ko.Status.Conditions = []*ackv1alpha1.Condition{}
	}
	var condition *ackv1alpha1.Condition = nil
	for _, c := range ko.Status.Conditions {
		if c.Type == cType {
			condition = c
			break
		}
	}
	if condition == nil {
		condition = &ackv1alpha1.Condition{
			Type:   cType,
			Status: cStatus,
		}
		ko.Status.Conditions = append(ko.Status.Conditions, condition)
	} else {
		condition.Status = cStatus
	}
}

func (rm *resourceManager) CustomDescribeCacheParameterGroupsSetOutput(
	ctx context.Context,
	r *resource,
	resp *svcsdk.DescribeCacheParameterGroupsOutput,
	ko *svcapitypes.CacheParameterGroup,
) (*svcapitypes.CacheParameterGroup, error) {
	// Retrieve parameters using DescribeCacheParameters API and populate ko.Status.ParameterNameValues
	if len(resp.CacheParameterGroups) == 0 {
		return ko, nil
	}
	cpg := resp.CacheParameterGroups[0]
	// Populate latest.ko.Spec.ParameterNameValues with latest parameter values
	// Populate latest.ko.Status.Parameters with latest detailed parameters
	error := rm.customSetOutputDescribeCacheParameters(ctx, cpg.CacheParameterGroupName, ko)
	if error != nil {
		return nil, error
	}
	return ko, nil
}

func (rm *resourceManager) CustomCreateCacheParameterGroupSetOutput(
	ctx context.Context,
	r *resource,
	resp *svcsdk.CreateCacheParameterGroupOutput,
	ko *svcapitypes.CacheParameterGroup,
) (*svcapitypes.CacheParameterGroup, error) {
	if r.ko.Spec.ParameterNameValues != nil && len(r.ko.Spec.ParameterNameValues) != 0 {
		// Spec has parameters name and values. Create API does not save these, but Modify API does.
		// Thus, Create needs to be followed by Modify call to save parameters from Spec.
		// Setting synched condition to false, so that reconciler gets invoked again
		// and modify logic gets executed.
		rm.setCondition(ko, ackv1alpha1.ConditionTypeResourceSynced, corev1.ConditionFalse)
	}
	return ko, nil
}

// Implements specialized logic for update CacheParameterGroup.
func (rm *resourceManager) customUpdateCacheParameterGroup(
	ctx context.Context,
	desired *resource,
	latest *resource,
	delta *ackcompare.Delta,
) (*resource, error) {
	desiredParameters := desired.ko.Spec.ParameterNameValues
	latestParameters := latest.ko.Spec.ParameterNameValues

	updated := false
	var err error
	// Update
	if (desiredParameters == nil || len(desiredParameters) == 0) &&
		(latestParameters != nil && len(latestParameters) > 0) {
		updated, err = rm.resetAllParameters(ctx, desired)
		if !updated || err != nil {
			return nil, err
		}
	} else {
		removedParameters, modifiedParameters, addedParameters := rm.provideDelta(desiredParameters, latestParameters)
		if removedParameters != nil && len(removedParameters) > 0 {
			updated, err = rm.resetParameters(ctx, desired, removedParameters)
			if !updated || err != nil {
				return nil, err
			}
		}
		if modifiedParameters != nil && len(modifiedParameters) > 0 {
			updated, err = rm.saveParameters(ctx, desired, modifiedParameters)
			if !updated || err != nil {
				return nil, err
			}
		}
		if addedParameters != nil && len(addedParameters) > 0 {
			updated, err = rm.saveParameters(ctx, desired, addedParameters)
			if !updated || err != nil {
				return nil, err
			}
		}
	}
	if updated {
		rm.setStatusDefaults(latest.ko)
		// Populate latest.ko.Spec.ParameterNameValues with latest parameter values
		// Populate latest.ko.Status.Parameters with latest detailed parameters
		error := rm.customSetOutputDescribeCacheParameters(ctx, desired.ko.Spec.CacheParameterGroupName, latest.ko)
		if error != nil {
			return nil, error
		}
	}
	return latest, nil
}

// provideDelta compares given desired and latest Parameters and returns
// removedParameters, modifiedParameters, addedParameters
func (rm *resourceManager) provideDelta(
	desiredParameters []*svcapitypes.ParameterNameValue,
	latestParameters []*svcapitypes.ParameterNameValue,
) ([]*svcapitypes.ParameterNameValue, []*svcapitypes.ParameterNameValue, []*svcapitypes.ParameterNameValue) {

	desiredPametersMap := map[string]*svcapitypes.ParameterNameValue{}
	for _, parameter := range desiredParameters {
		p := *parameter
		desiredPametersMap[*p.ParameterName] = &p
	}
	latestPametersMap := map[string]*svcapitypes.ParameterNameValue{}
	for _, parameter := range latestParameters {
		p := *parameter
		latestPametersMap[*p.ParameterName] = &p
	}

	removedParameters := []*svcapitypes.ParameterNameValue{}  // available in latest but not found in desired
	modifiedParameters := []*svcapitypes.ParameterNameValue{} // available in both desired, latest but values differ
	addedParameters := []*svcapitypes.ParameterNameValue{}    // available in desired but not found in latest
	for latestParameterName, latestParameterNameValue := range latestPametersMap {
		desiredParameterNameValue, found := desiredPametersMap[latestParameterName]
		if found && desiredParameterNameValue != nil &&
			desiredParameterNameValue.ParameterValue != nil && *desiredParameterNameValue.ParameterValue != "" {
			if *desiredParameterNameValue.ParameterValue != *latestParameterNameValue.ParameterValue {
				// available in both desired, latest but values differ
				modified := *desiredParameterNameValue
				modifiedParameters = append(modifiedParameters, &modified)
			}
		} else {
			// available in latest but not found in desired
			removed := *latestParameterNameValue
			removedParameters = append(removedParameters, &removed)
		}
	}
	for desiredParameterName, desiredParameterNameValue := range desiredPametersMap {
		_, found := latestPametersMap[desiredParameterName]
		if !found && desiredParameterNameValue != nil {
			// available in desired but not found in latest
			added := *desiredParameterNameValue
			if added.ParameterValue != nil && *added.ParameterValue != "" {
				addedParameters = append(addedParameters, &added)
			}
		}
	}
	return removedParameters, modifiedParameters, addedParameters
}

func Int32OrNil(i *int64) *int32 {
	if i == nil {
		return nil
	}
	return aws.Int32(int32(*i))
}
