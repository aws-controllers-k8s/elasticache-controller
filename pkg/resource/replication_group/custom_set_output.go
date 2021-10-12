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
	"encoding/json"

	svcapitypes "github.com/aws-controllers-k8s/elasticache-controller/apis/v1alpha1"
	ackv1alpha1 "github.com/aws-controllers-k8s/runtime/apis/core/v1alpha1"
	"github.com/aws/aws-sdk-go/service/elasticache"
	svcsdk "github.com/aws/aws-sdk-go/service/elasticache"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	// The number of minutes worth of events to retrieve.
	// 14 days in minutes
	eventsDuration = 20160
)

func (rm *resourceManager) CustomDescribeReplicationGroupsSetOutput(
	ctx context.Context,
	r *resource,
	resp *elasticache.DescribeReplicationGroupsOutput,
	ko *svcapitypes.ReplicationGroup,
) (*svcapitypes.ReplicationGroup, error) {
	if len(resp.ReplicationGroups) == 0 {
		return ko, nil
	}
	elem := resp.ReplicationGroups[0]
	rm.customSetOutput(elem, ko)
	err := rm.customSetOutputSupplementAPIs(ctx, r, elem, ko)
	if err != nil {
		return nil, err
	}
	return ko, nil
}

func (rm *resourceManager) CustomCreateReplicationGroupSetOutput(
	ctx context.Context,
	r *resource,
	resp *elasticache.CreateReplicationGroupOutput,
	ko *svcapitypes.ReplicationGroup,
) (*svcapitypes.ReplicationGroup, error) {
	rm.customSetOutput(resp.ReplicationGroup, ko)
	rm.setAnnotationsFields(r, ko)
	return ko, nil
}

func (rm *resourceManager) CustomModifyReplicationGroupSetOutput(
	ctx context.Context,
	r *resource,
	resp *elasticache.ModifyReplicationGroupOutput,
	ko *svcapitypes.ReplicationGroup,
) (*svcapitypes.ReplicationGroup, error) {
	rm.customSetOutput(resp.ReplicationGroup, ko)

	// reset latest.spec.LDC to original value in desired to prevent stale data
	// from the modify API being merged back into desired upon spec patching
	var logDeliveryConfig []*svcapitypes.LogDeliveryConfigurationRequest
	for _, ldc := range r.ko.Spec.LogDeliveryConfigurations {
		logDeliveryConfig = append(logDeliveryConfig, ldc.DeepCopy())
	}
	ko.Spec.LogDeliveryConfigurations = logDeliveryConfig

	// Keep the value of desired for CacheNodeType.
	ko.Spec.CacheNodeType = r.ko.Spec.CacheNodeType

	rm.setAnnotationsFields(r, ko)
	return ko, nil
}

func (rm *resourceManager) customSetOutput(
	respRG *elasticache.ReplicationGroup,
	ko *svcapitypes.ReplicationGroup,
) {
	if ko.Status.Conditions == nil {
		ko.Status.Conditions = []*ackv1alpha1.Condition{}
	}

	allNodeGroupsAvailable := true
	nodeGroupMembersCount := 0
	memberClustersCount := 0
	if respRG.NodeGroups != nil {
		for _, nodeGroup := range respRG.NodeGroups {
			if nodeGroup.Status == nil || *nodeGroup.Status != "available" {
				allNodeGroupsAvailable = false
				break
			}
		}
		for _, nodeGroup := range respRG.NodeGroups {
			if nodeGroup.NodeGroupMembers == nil {
				continue
			}
			nodeGroupMembersCount = nodeGroupMembersCount + len(nodeGroup.NodeGroupMembers)
		}
	}
	if respRG.MemberClusters != nil {
		memberClustersCount = len(respRG.MemberClusters)
	}

	rgStatus := respRG.Status
	syncConditionStatus := corev1.ConditionUnknown
	if rgStatus != nil {
		if (*rgStatus == "available" && allNodeGroupsAvailable && memberClustersCount == nodeGroupMembersCount) ||
			*rgStatus == "create-failed" {
			syncConditionStatus = corev1.ConditionTrue
		} else {
			// resource in "creating", "modifying" , "deleting", "snapshotting"
			// states is being modified at server end
			// thus current status is considered out of sync.
			syncConditionStatus = corev1.ConditionFalse
		}
	}
	var resourceSyncedCondition *ackv1alpha1.Condition = nil
	for _, condition := range ko.Status.Conditions {
		if condition.Type == ackv1alpha1.ConditionTypeResourceSynced {
			resourceSyncedCondition = condition
			break
		}
	}
	if resourceSyncedCondition == nil {
		resourceSyncedCondition = &ackv1alpha1.Condition{
			Type:   ackv1alpha1.ConditionTypeResourceSynced,
			Status: syncConditionStatus,
		}
		ko.Status.Conditions = append(ko.Status.Conditions, resourceSyncedCondition)
	} else {
		resourceSyncedCondition.Status = syncConditionStatus
	}

	if rgStatus != nil && (*rgStatus == "available" || *rgStatus == "snapshotting") {
		input, err := rm.newListAllowedNodeTypeModificationsPayLoad(respRG)

		if err == nil {
			resp, apiErr := rm.sdkapi.ListAllowedNodeTypeModifications(input)
			rm.metrics.RecordAPICall("READ_MANY", "ListAllowedNodeTypeModifications", apiErr)
			// Overwrite the values for ScaleUp and ScaleDown
			if apiErr == nil {
				ko.Status.AllowedScaleDownModifications = resp.ScaleDownModifications
				ko.Status.AllowedScaleUpModifications = resp.ScaleUpModifications
			}
		}
	} else {
		ko.Status.AllowedScaleDownModifications = nil
		ko.Status.AllowedScaleUpModifications = nil
	}

	// populate status logDeliveryConfigurations struct
	if respRG.LogDeliveryConfigurations != nil {
		var f11 []*svcapitypes.LogDeliveryConfiguration
		for _, f11iter := range respRG.LogDeliveryConfigurations {
			f11elem := &svcapitypes.LogDeliveryConfiguration{}
			if f11iter.DestinationDetails != nil {
				f11elemf0 := &svcapitypes.DestinationDetails{}
				if f11iter.DestinationDetails.CloudWatchLogsDetails != nil {
					f11elemf0f0 := &svcapitypes.CloudWatchLogsDestinationDetails{}
					if f11iter.DestinationDetails.CloudWatchLogsDetails.LogGroup != nil {
						f11elemf0f0.LogGroup = f11iter.DestinationDetails.CloudWatchLogsDetails.LogGroup
					}
					f11elemf0.CloudWatchLogsDetails = f11elemf0f0
				}
				if f11iter.DestinationDetails.KinesisFirehoseDetails != nil {
					f11elemf0f1 := &svcapitypes.KinesisFirehoseDestinationDetails{}
					if f11iter.DestinationDetails.KinesisFirehoseDetails.DeliveryStream != nil {
						f11elemf0f1.DeliveryStream = f11iter.DestinationDetails.KinesisFirehoseDetails.DeliveryStream
					}
					f11elemf0.KinesisFirehoseDetails = f11elemf0f1
				}
				f11elem.DestinationDetails = f11elemf0
			}
			if f11iter.DestinationType != nil {
				f11elem.DestinationType = f11iter.DestinationType
			}
			if f11iter.LogFormat != nil {
				f11elem.LogFormat = f11iter.LogFormat
			}
			if f11iter.LogType != nil {
				f11elem.LogType = f11iter.LogType
			}
			if f11iter.Status != nil {
				f11elem.Status = f11iter.Status
			}
			if f11iter.Message != nil {
				f11elem.Message = f11iter.Message
			}
			f11 = append(f11, f11elem)
		}
		ko.Status.LogDeliveryConfigurations = f11
	} else {
		ko.Status.LogDeliveryConfigurations = nil
	}
}

// newListAllowedNodeTypeModificationsPayLoad returns an SDK-specific struct for the HTTP request
// payload of the ListAllowedNodeTypeModifications API call.
func (rm *resourceManager) newListAllowedNodeTypeModificationsPayLoad(respRG *elasticache.ReplicationGroup) (
	*svcsdk.ListAllowedNodeTypeModificationsInput, error) {
	res := &svcsdk.ListAllowedNodeTypeModificationsInput{}

	if respRG.ReplicationGroupId != nil {
		res.SetReplicationGroupId(*respRG.ReplicationGroupId)
	}

	return res, nil
}

func (rm *resourceManager) customSetOutputSupplementAPIs(
	ctx context.Context,
	r *resource,
	respRG *elasticache.ReplicationGroup,
	ko *svcapitypes.ReplicationGroup,
) error {
	events, err := rm.provideEvents(ctx, r.ko.Spec.ReplicationGroupID, 20)
	if err != nil {
		return err
	}
	ko.Status.Events = events
	return nil
}

func (rm *resourceManager) provideEvents(
	ctx context.Context,
	replicationGroupId *string,
	maxRecords int64,
) ([]*svcapitypes.Event, error) {
	input := &elasticache.DescribeEventsInput{}
	input.SetSourceType("replication-group")
	input.SetSourceIdentifier(*replicationGroupId)
	input.SetMaxRecords(maxRecords)
	input.SetDuration(eventsDuration)
	resp, err := rm.sdkapi.DescribeEventsWithContext(ctx, input)
	rm.metrics.RecordAPICall("READ_MANY", "DescribeEvents-ReplicationGroup", err)
	if err != nil {
		rm.log.V(1).Info("Error during DescribeEvents-ReplicationGroup", "error", err)
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

// setAnnotationsFields copies the desired object's annotations, populates any
// relevant fields, and sets the latest object's annotations to this newly populated map.
// This should only be called upon a successful create or modify call.
func (rm *resourceManager) setAnnotationsFields(
	r *resource,
	ko *svcapitypes.ReplicationGroup,
) {
	desiredAnnotations := r.ko.ObjectMeta.GetAnnotations()
	annotations := make(map[string]string)
	for k, v := range desiredAnnotations {
		annotations[k] = v
	}

	rm.setLastRequestedLogDeliveryConfigurations(r, annotations)

	ko.ObjectMeta.Annotations = annotations
}

// setLastRequestedLogDeliveryConfigurations copies desired.Spec.LogDeliveryConfigurations
// into the annotations of the object.
// r is the desired resource, and annotations is the annotations map modified by this method
func (rm *resourceManager) setLastRequestedLogDeliveryConfigurations(
	r *resource,
	annotations map[string]string,
) {
	lastRequestedConfigs, err := json.Marshal(r.ko.Spec.LogDeliveryConfigurations)
	if err != nil {
		annotations[AnnotationLastRequestedLDCs] = "null"
	} else {
		annotations[AnnotationLastRequestedLDCs] = string(lastRequestedConfigs)
	}
}
