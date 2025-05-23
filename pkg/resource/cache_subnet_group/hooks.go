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

package cache_subnet_group

import (
	"context"

	svcapitypes "github.com/aws-controllers-k8s/elasticache-controller/apis/v1alpha1"
	"github.com/aws/aws-sdk-go-v2/aws"
	svcsdk "github.com/aws/aws-sdk-go-v2/service/elasticache"
	svcsdktypes "github.com/aws/aws-sdk-go-v2/service/elasticache/types"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	// The number of minutes worth of events to retrieve.
	// 14 days in minutes
	eventsDuration = 20160
)

func (rm *resourceManager) CustomDescribeCacheSubnetGroupsSetOutput(
	ctx context.Context,
	r *resource,
	resp *svcsdk.DescribeCacheSubnetGroupsOutput,
	ko *svcapitypes.CacheSubnetGroup,
) (*svcapitypes.CacheSubnetGroup, error) {
	if len(resp.CacheSubnetGroups) == 0 {
		return ko, nil
	}
	elem := resp.CacheSubnetGroups[0]
	err := rm.customSetOutputSupplementAPIs(ctx, r, &elem, ko)
	if err != nil {
		return nil, err
	}
	return ko, nil
}

func (rm *resourceManager) customSetOutputSupplementAPIs(
	ctx context.Context,
	r *resource,
	subnetGroup *svcsdktypes.CacheSubnetGroup,
	ko *svcapitypes.CacheSubnetGroup,
) error {
	events, err := rm.provideEvents(ctx, r.ko.Spec.CacheSubnetGroupName, 20)
	if err != nil {
		return err
	}
	ko.Status.Events = events
	return nil
}

func (rm *resourceManager) provideEvents(
	ctx context.Context,
	subnetGroupName *string,
	maxRecords int64,
) ([]*svcapitypes.Event, error) {
	input := &svcsdk.DescribeEventsInput{}
	input.SourceType = svcsdktypes.SourceTypeCacheSubnetGroup
	input.SourceIdentifier = subnetGroupName
	input.MaxRecords = aws.Int32(int32(maxRecords))
	input.Duration = aws.Int32(eventsDuration)
	resp, err := rm.sdkapi.DescribeEvents(ctx, input)
	rm.metrics.RecordAPICall("READ_MANY", "DescribeEvents-CacheSubnetGroup", err)
	if err != nil {
		rm.log.V(1).Info("Error during DescribeEvents-CacheSubnetGroup", "error", err)
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

func Int32OrNil(i *int64) *int32 {
	if i == nil {
		return nil
	}
	return aws.Int32(int32(*i))
}
