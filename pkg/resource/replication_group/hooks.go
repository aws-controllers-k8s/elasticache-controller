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
	"errors"
	"fmt"
	"reflect"
	"sort"
	"strconv"

	"github.com/aws/aws-sdk-go-v2/aws"
	svcsdk "github.com/aws/aws-sdk-go-v2/service/elasticache"
	svcsdktypes "github.com/aws/aws-sdk-go-v2/service/elasticache/types"
	"github.com/aws/aws-sdk-go/aws/awserr"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	svcapitypes "github.com/aws-controllers-k8s/elasticache-controller/apis/v1alpha1"
	"github.com/aws-controllers-k8s/elasticache-controller/pkg/common"
	"github.com/aws-controllers-k8s/elasticache-controller/pkg/util"
	ackv1alpha1 "github.com/aws-controllers-k8s/runtime/apis/core/v1alpha1"
	ackcompare "github.com/aws-controllers-k8s/runtime/pkg/compare"
	"github.com/aws-controllers-k8s/runtime/pkg/requeue"
	ackrequeue "github.com/aws-controllers-k8s/runtime/pkg/requeue"
)

const (
	// AnnotationLastRequestedLDCs is an annotation whose value is the marshaled list of pointers to
	// LogDeliveryConfigurationRequest structs passed in as input to either the create or modify API called most
	// recently
	AnnotationLastRequestedLDCs = svcapitypes.AnnotationPrefix + "last-requested-log-delivery-configurations"
	// AnnotationLastRequestedCNT is an annotation whose value is passed in as input to either the create or modify API
	// called most recently
	AnnotationLastRequestedCNT = svcapitypes.AnnotationPrefix + "last-requested-cache-node-type"
	// AnnotationLastRequestedNNG is an annotation whose value is passed in as input to either the create or modify API
	// called most recently
	AnnotationLastRequestedNNG = svcapitypes.AnnotationPrefix + "last-requested-num-node-groups"
	// AnnotationLastRequestedNGC is an annotation whose value is the marshaled list of pointers to
	// NodeGroupConfiguration structs passed in as input to either the create or modify API called most
	// recently
	AnnotationLastRequestedNGC = svcapitypes.AnnotationPrefix + "last-requested-node-group-configuration"
)

var (
	condMsgCurrentlyCreating      string = "replication group currently being created."
	condMsgCurrentlyDeleting      string = "replication group currently being deleted."
	condMsgNoDeleteWhileModifying string = "replication group currently being modified. cannot delete."
	condMsgTerminalCreateFailed   string = "replication group in create-failed status."
)

const (
	statusDeleting     string = "deleting"
	statusModifying    string = "modifying"
	statusCreating     string = "creating"
	statusCreateFailed string = "create-failed"
)

var (
	requeueWaitWhileDeleting = ackrequeue.NeededAfter(
		errors.New("Delete is in progress."),
		ackrequeue.DefaultRequeueAfterDuration,
	)
	requeueWaitWhileModifying = ackrequeue.NeededAfter(
		errors.New("Modify is in progress."),
		ackrequeue.DefaultRequeueAfterDuration,
	)
)

// isDeleting returns true if supplied replication group resource state is 'deleting'
func isDeleting(r *resource) bool {
	if r == nil || r.ko.Status.Status == nil {
		return false
	}
	status := *r.ko.Status.Status
	return status == statusDeleting
}

// isModifying returns true if supplied replication group resource state is 'modifying'
func isModifying(r *resource) bool {
	if r == nil || r.ko.Status.Status == nil {
		return false
	}
	status := *r.ko.Status.Status
	return status == statusModifying
}

// isCreating returns true if supplied replication group resource state is 'modifying'
func isCreating(r *resource) bool {
	if r == nil || r.ko.Status.Status == nil {
		return false
	}
	status := *r.ko.Status.Status
	return status == statusCreating
}

// isCreateFailed returns true if supplied replication group resource state is
// 'create-failed'
func isCreateFailed(r *resource) bool {
	if r == nil || r.ko.Status.Status == nil {
		return false
	}
	status := *r.ko.Status.Status
	return status == statusCreateFailed
}

// getTags retrieves the resource's associated tags.
func (rm *resourceManager) getTags(
	ctx context.Context,
	resourceARN string,
) ([]*svcapitypes.Tag, error) {
	return util.GetTags(ctx, rm.sdkapi, rm.metrics, resourceARN)
}

// syncTags keeps the resource's tags in sync.
func (rm *resourceManager) syncTags(
	ctx context.Context,
	desired *resource,
	latest *resource,
) (err error) {
	return util.SyncTags(ctx, desired.ko.Spec.Tags, latest.ko.Spec.Tags, latest.ko.Status.ACKResourceMetadata, convertToOrderedACKTags, rm.sdkapi, rm.metrics)
}

const (
	// The number of minutes worth of events to retrieve.
	// 14 days in minutes
	eventsDuration = 20160
)

func (rm *resourceManager) CustomDescribeReplicationGroupsSetOutput(
	ctx context.Context,
	r *resource,
	resp *svcsdk.DescribeReplicationGroupsOutput,
	ko *svcapitypes.ReplicationGroup,
) (*svcapitypes.ReplicationGroup, error) {
	if len(resp.ReplicationGroups) == 0 {
		return ko, nil
	}
	elem := resp.ReplicationGroups[0]
	rm.customSetOutput(ctx, elem, ko)
	err := rm.customSetOutputSupplementAPIs(ctx, r, &elem, ko)
	if err != nil {
		return nil, err
	}
	return ko, nil
}

func (rm *resourceManager) CustomCreateReplicationGroupSetOutput(
	ctx context.Context,
	r *resource,
	resp *svcsdk.CreateReplicationGroupOutput,
	ko *svcapitypes.ReplicationGroup,
) (*svcapitypes.ReplicationGroup, error) {
	rm.customSetOutput(ctx, *resp.ReplicationGroup, ko)
	rm.setAnnotationsFields(r, ko)
	rm.setLastRequestedNodeGroupConfiguration(r, ko)
	rm.setLastRequestedNumNodeGroups(r, ko)
	return ko, nil
}

func (rm *resourceManager) CustomModifyReplicationGroupSetOutput(
	ctx context.Context,
	r *resource,
	resp *svcsdk.ModifyReplicationGroupOutput,
	ko *svcapitypes.ReplicationGroup,
) (*svcapitypes.ReplicationGroup, error) {
	rm.customSetOutput(ctx, *resp.ReplicationGroup, ko)

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
	ctx context.Context,
	respRG svcsdktypes.ReplicationGroup,
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
		input, err := rm.newListAllowedNodeTypeModificationsPayLoad(&respRG)

		if err == nil {
			resp, apiErr := rm.sdkapi.ListAllowedNodeTypeModifications(ctx, input)
			rm.metrics.RecordAPICall("READ_MANY", "ListAllowedNodeTypeModifications", apiErr)
			// Overwrite the values for ScaleUp and ScaleDown
			if apiErr == nil {
				ko.Status.AllowedScaleDownModifications = aws.StringSlice(resp.ScaleDownModifications)
				ko.Status.AllowedScaleUpModifications = aws.StringSlice(resp.ScaleUpModifications)
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
			if f11iter.DestinationType != "" {
				f11elem.DestinationType = aws.String(string(f11iter.DestinationType))
			}
			if f11iter.LogFormat != "" {
				f11elem.LogFormat = aws.String(string(f11iter.LogFormat))
			}
			if f11iter.LogType != "" {
				f11elem.LogType = aws.String(string(f11iter.LogType))
			}
			if f11iter.Status != "" {
				f11elem.Status = aws.String(string(f11iter.Status))
			}
			if f11iter.Message != nil && *f11iter.Message != "" {
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
func (rm *resourceManager) newListAllowedNodeTypeModificationsPayLoad(respRG *svcsdktypes.ReplicationGroup) (
	*svcsdk.ListAllowedNodeTypeModificationsInput, error) {
	res := &svcsdk.ListAllowedNodeTypeModificationsInput{}

	if respRG.ReplicationGroupId != nil {
		res.ReplicationGroupId = respRG.ReplicationGroupId
	}

	return res, nil
}

func (rm *resourceManager) customSetOutputSupplementAPIs(
	ctx context.Context,
	r *resource,
	respRG *svcsdktypes.ReplicationGroup,
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
	input := &svcsdk.DescribeEventsInput{}
	input.SourceType = svcsdktypes.SourceTypeReplicationGroup
	input.SourceIdentifier = replicationGroupId
	input.MaxRecords = aws.Int32(int32(maxRecords))
	input.Duration = aws.Int32(eventsDuration)
	resp, err := rm.sdkapi.DescribeEvents(ctx, input)
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
// Fields that are handled by custom modify implementation are not set here.
// This should only be called upon a successful create or modify call.
func (rm *resourceManager) setAnnotationsFields(
	r *resource,
	ko *svcapitypes.ReplicationGroup,
) {
	annotations := getAnnotationsFields(r, ko)

	rm.setLastRequestedLogDeliveryConfigurations(r, annotations)
	rm.setLastRequestedCacheNodeType(r, annotations)
	ko.ObjectMeta.Annotations = annotations
}

// getAnnotationsFields return the annotations map that would be used to set the fields
func getAnnotationsFields(
	r *resource,
	ko *svcapitypes.ReplicationGroup) map[string]string {

	if ko.ObjectMeta.Annotations != nil {
		return ko.ObjectMeta.Annotations
	}

	desiredAnnotations := r.ko.ObjectMeta.GetAnnotations()
	annotations := make(map[string]string)
	for k, v := range desiredAnnotations {
		annotations[k] = v
	}

	ko.ObjectMeta.Annotations = annotations
	return annotations
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

// setLastRequestedCacheNodeType copies desired.Spec.CacheNodeType into the annotation
// of the object.
func (rm *resourceManager) setLastRequestedCacheNodeType(
	r *resource,
	annotations map[string]string,
) {
	if r.ko.Spec.CacheNodeType != nil {
		annotations[AnnotationLastRequestedCNT] = *r.ko.Spec.CacheNodeType
	}
}

// setLastRequestedNodeGroupConfiguration copies desired.spec.NodeGroupConfiguration into the
// annotation of the object
func (rm *resourceManager) setLastRequestedNodeGroupConfiguration(
	r *resource,
	ko *svcapitypes.ReplicationGroup,
) {
	annotations := getAnnotationsFields(r, ko)
	lastRequestedConfigs, err := json.Marshal(r.ko.Spec.NodeGroupConfiguration)
	if err != nil {
		annotations[AnnotationLastRequestedNGC] = "null"
	} else {
		annotations[AnnotationLastRequestedNGC] = string(lastRequestedConfigs)
	}
}

// setLastRequestedNumNodeGroups copies desired.spec.NumNodeGroups into the
// annotation of the object
func (rm *resourceManager) setLastRequestedNumNodeGroups(
	r *resource,
	ko *svcapitypes.ReplicationGroup,
) {
	annotations := getAnnotationsFields(r, ko)
	if r.ko.Spec.NumNodeGroups != nil {
		annotations[AnnotationLastRequestedNNG] = strconv.Itoa(int(*r.ko.Spec.NumNodeGroups))
	} else {
		annotations[AnnotationLastRequestedNNG] = "null"
	}
}

// Implements specialized logic for replication group updates.
func (rm *resourceManager) CustomModifyReplicationGroup(
	ctx context.Context,
	desired *resource,
	latest *resource,
	delta *ackcompare.Delta,
) (*resource, error) {

	latestRGStatus := latest.ko.Status.Status

	allNodeGroupsAvailable := true
	nodeGroupMembersCount := 0
	if latest.ko.Status.NodeGroups != nil {
		for _, nodeGroup := range latest.ko.Status.NodeGroups {
			if nodeGroup.Status == nil || *nodeGroup.Status != "available" {
				allNodeGroupsAvailable = false
				break
			}
		}
		for _, nodeGroup := range latest.ko.Status.NodeGroups {
			if nodeGroup.NodeGroupMembers == nil {
				continue
			}
			nodeGroupMembersCount = nodeGroupMembersCount + len(nodeGroup.NodeGroupMembers)
		}
	}

	if latestRGStatus == nil || *latestRGStatus != "available" || !allNodeGroupsAvailable {
		return nil, requeue.NeededAfter(
			errors.New("Replication Group can not be modified, it is not in 'available' state."),
			requeue.DefaultRequeueAfterDuration)
	}

	memberClustersCount := 0
	if latest.ko.Status.MemberClusters != nil {
		memberClustersCount = len(latest.ko.Status.MemberClusters)
	}
	if memberClustersCount != nodeGroupMembersCount {
		return nil, requeue.NeededAfter(
			errors.New("Replication Group can not be modified, "+
				"need to wait for member clusters and node group members."),
			requeue.DefaultRequeueAfterDuration)
	}

	// Handle the asynchronous rollback case for while Scaling down.
	// This means that we have already attempted to apply the CacheNodeType once and
	// were not successful hence we will set a terminal condition.
	if !cacheNodeTypeRequiresUpdate(desired) && delta.DifferentAt("Spec.CacheNodeType") {
		return nil, awserr.New("InvalidParameterCombination", "Cannot update CacheNodeType, "+
			"Please refer to Events for more details", nil)

	}

	// Handle the asynchronous rollback for Resharding.
	if !nodeGroupRequiresUpdate(desired) && rm.shardConfigurationsDiffer(desired, latest) {

		return nil, awserr.New("InvalidParameterCombination", "Cannot update NodeGroups, "+
			"Please refer to Events for more details", nil)
	}

	// Handle NodeGroupConfiguration asynchronous rollback situations other than Resharding.
	if !nodeGroupRequiresUpdate(desired) && (rm.replicaCountDifference(desired, latest) != 0 && !delta.DifferentAt("Spec.ReplicasPerNodeGroup")) {
		return nil, awserr.New("InvalidParameterCombination", "Cannot update NodeGroupConfiguration, "+
			"Please refer to Events for more details", nil)
	}

	// Order of operations when diffs map to multiple updates APIs:
	// 1. When automaticFailoverEnabled differs:
	//		if automaticFailoverEnabled == false; do nothing in this custom logic, let the modify execute first.
	// 		else if automaticFailoverEnabled == true then following logic should execute first.
	// 2. When multiAZ differs
	// 		if multiAZ = true  then below is fine.
	// 		else if multiAZ = false ; do nothing in custom logic, let the modify execute.
	// 3. updateReplicaCount() is invoked Before updateShardConfiguration()
	//		because both accept availability zones, however the number of
	//		values depend on replica count.
	if desired.ko.Spec.AutomaticFailoverEnabled != nil && *desired.ko.Spec.AutomaticFailoverEnabled == false {
		latestAutomaticFailoverEnabled := latest.ko.Status.AutomaticFailover != nil && *latest.ko.Status.AutomaticFailover == "enabled"
		if latestAutomaticFailoverEnabled != *desired.ko.Spec.AutomaticFailoverEnabled {
			return rm.modifyReplicationGroup(ctx, desired, latest, delta)
		}
	}
	if desired.ko.Spec.MultiAZEnabled != nil && *desired.ko.Spec.MultiAZEnabled == false {
		latestMultiAZEnabled := latest.ko.Status.MultiAZ != nil && *latest.ko.Status.MultiAZ == "enabled"
		if latestMultiAZEnabled != *desired.ko.Spec.MultiAZEnabled {
			return rm.modifyReplicationGroup(ctx, desired, latest, delta)
		}
	}

	// increase/decrease replica count
	if diff := rm.replicaCountDifference(desired, latest); diff != 0 {
		if diff > 0 {
			return rm.increaseReplicaCount(ctx, desired, latest)
		}
		return rm.decreaseReplicaCount(ctx, desired, latest)
	}

	// If there is a scale up modification, then we would prioritize it
	// over increase/decrease shards. This is important since performing
	// scale in without scale up might fail due to insufficient memory.
	if delta.DifferentAt("Spec.CacheNodeType") && desired.ko.Status.AllowedScaleUpModifications != nil {
		if desired.ko.Spec.CacheNodeType != nil {
			for _, scaleUpInstance := range desired.ko.Status.AllowedScaleUpModifications {
				if *scaleUpInstance == *desired.ko.Spec.CacheNodeType {
					return nil, nil
				}
			}
		}
	}

	// increase/decrease shards
	if rm.shardConfigurationsDiffer(desired, latest) {
		return rm.updateShardConfiguration(ctx, desired, latest)
	}

	return rm.modifyReplicationGroup(ctx, desired, latest, delta)
}

// modifyReplicationGroup updates replication group
// it handles properties that put replication group in
// modifying state if these are supplied to modify API
// irrespective of apply immediately.
func (rm *resourceManager) modifyReplicationGroup(
	ctx context.Context,
	desired *resource,
	latest *resource,
	delta *ackcompare.Delta,
) (*resource, error) {
	// Method currently handles SecurityGroupIDs, EngineVersion
	// Avoid making unnecessary DescribeCacheCluster API call if both fields are nil in spec.
	if desired.ko.Spec.SecurityGroupIDs == nil && desired.ko.Spec.EngineVersion == nil {
		// no updates done
		return nil, nil
	}

	// Get details using describe cache cluster to compute diff
	latestCacheCluster, err := rm.describeCacheCluster(ctx, latest)
	if err != nil {
		return nil, err
	}

	// SecurityGroupIds, EngineVersion
	if rm.securityGroupIdsDiffer(desired, latest, latestCacheCluster) ||
		delta.DifferentAt("Spec.EngineVersion") || delta.DifferentAt("Spec.Engine") || delta.DifferentAt("Spec.CacheParameterGroupName") {
		input := rm.newModifyReplicationGroupRequestPayload(desired, latest, latestCacheCluster, delta)
		resp, respErr := rm.sdkapi.ModifyReplicationGroup(ctx, input)
		rm.metrics.RecordAPICall("UPDATE", "ModifyReplicationGroup", respErr)
		if respErr != nil {
			rm.log.V(1).Info("Error during ModifyReplicationGroup", "error", respErr)
			return nil, respErr
		}

		// The ModifyReplicationGroup API returns stale field Engine that don't
		// immediately reflect the requested changes, causing the controller to detect false
		// differences and trigger terminal conditions. Override these fields with the user's
		// intended values before passing to the generated setReplicationGroupOutput function.
		normalizedRG := *resp.ReplicationGroup
		if desired.ko.Spec.Engine != nil {
			normalizedRG.Engine = desired.ko.Spec.Engine
		}

		return rm.setReplicationGroupOutput(ctx, desired, &normalizedRG)
	}

	// no updates done
	return nil, nil
}

// replicaConfigurationsDifference returns
// positive number if desired replica count is greater than latest replica count
// negative number if desired replica count is less than latest replica count
// 0 otherwise
func (rm *resourceManager) replicaCountDifference(
	desired *resource,
	latest *resource,
) int {
	desiredSpec := desired.ko.Spec

	// There are two ways of setting replica counts for NodeGroups in Elasticache ReplicationGroup.
	// - The first way is to have the same replica count for all node groups.
	//   In this case, the Spec.ReplicasPerNodeGroup field is set to a non-nil-value integer pointer.
	// - The second way is to set different replica counts per node group.
	//   In this case, the Spec.NodeGroupConfiguration field is set to a non-nil NodeGroupConfiguration slice
	//   of NodeGroupConfiguration structs that each have a ReplicaCount non-nil-value integer pointer field
	//   that contains the number of replicas for that particular node group.
	if desiredSpec.ReplicasPerNodeGroup != nil {
		return int(*desiredSpec.ReplicasPerNodeGroup - *latest.ko.Spec.ReplicasPerNodeGroup)
	} else if desiredSpec.NodeGroupConfiguration != nil {
		return rm.diffReplicasNodeGroupConfiguration(desired, latest)
	}
	return 0
}

// diffReplicasNodeGroupConfiguration takes desired Spec.NodeGroupConfiguration slice field into account to return
// positive number if desired replica count is greater than latest replica count
// negative number if desired replica count is less than latest replica count
// 0 otherwise
func (rm *resourceManager) diffReplicasNodeGroupConfiguration(
	desired *resource,
	latest *resource,
) int {
	desiredSpec := desired.ko.Spec
	latestStatus := latest.ko.Status
	// each shard could have different value for replica count
	latestReplicaCounts := map[string]int{}
	for _, latestShard := range latestStatus.NodeGroups {
		if latestShard.NodeGroupID == nil {
			continue
		}
		latestReplicaCount := 0
		if latestShard.NodeGroupMembers != nil {
			if len(latestShard.NodeGroupMembers) > 0 {
				latestReplicaCount = len(latestShard.NodeGroupMembers) - 1
			}
		}
		latestReplicaCounts[*latestShard.NodeGroupID] = latestReplicaCount
	}
	for _, desiredShard := range desiredSpec.NodeGroupConfiguration {
		if desiredShard.NodeGroupID == nil || desiredShard.ReplicaCount == nil {
			// no specs to compare for this shard
			continue
		}
		latestShardReplicaCount, found := latestReplicaCounts[*desiredShard.NodeGroupID]
		if !found {
			// shard not present in status
			continue
		}
		if desiredShardReplicaCount := int(*desiredShard.ReplicaCount); desiredShardReplicaCount != latestShardReplicaCount {
			rm.log.V(1).Info(
				"ReplicaCount differs",
				"NodeGroup", *desiredShard.NodeGroupID,
				"desired", int(*desiredShard.ReplicaCount),
				"latest", latestShardReplicaCount,
			)
			return desiredShardReplicaCount - latestShardReplicaCount
		}
	}
	return 0
}

// shardConfigurationsDiffer returns true if shard
// configuration differs between desired, latest resource.
func (rm *resourceManager) shardConfigurationsDiffer(
	desired *resource,
	latest *resource,
) bool {
	desiredSpec := desired.ko.Spec
	latestStatus := latest.ko.Status

	// desired shards
	var desiredShardsCount *int64 = desiredSpec.NumNodeGroups
	if desiredShardsCount == nil && desiredSpec.NodeGroupConfiguration != nil {
		numShards := int64(len(desiredSpec.NodeGroupConfiguration))
		desiredShardsCount = &numShards
	}
	if desiredShardsCount == nil {
		// no shards config in desired specs
		return false
	}

	// latest shards
	var latestShardsCount *int64 = nil
	if latestStatus.NodeGroups != nil {
		numShards := int64(len(latestStatus.NodeGroups))
		latestShardsCount = &numShards
	}

	return latestShardsCount == nil || *desiredShardsCount != *latestShardsCount
}

func (rm *resourceManager) increaseReplicaCount(
	ctx context.Context,
	desired *resource,
	latest *resource,
) (*resource, error) {
	input, err := rm.newIncreaseReplicaCountRequestPayload(desired, latest)
	if err != nil {
		return nil, err
	}
	resp, respErr := rm.sdkapi.IncreaseReplicaCount(ctx, input)
	rm.metrics.RecordAPICall("UPDATE", "IncreaseReplicaCount", respErr)
	if respErr != nil {
		rm.log.V(1).Info("Error during IncreaseReplicaCount", "error", respErr)
		return nil, respErr
	}
	return rm.setReplicationGroupOutput(ctx, desired, resp.ReplicationGroup)
}

func (rm *resourceManager) decreaseReplicaCount(
	ctx context.Context,
	desired *resource,
	latest *resource,
) (*resource, error) {
	input, err := rm.newDecreaseReplicaCountRequestPayload(desired, latest)
	if err != nil {
		return nil, err
	}
	resp, respErr := rm.sdkapi.DecreaseReplicaCount(ctx, input)
	rm.metrics.RecordAPICall("UPDATE", "DecreaseReplicaCount", respErr)
	if respErr != nil {
		rm.log.V(1).Info("Error during DecreaseReplicaCount", "error", respErr)
		return nil, respErr
	}
	return rm.setReplicationGroupOutput(ctx, desired, resp.ReplicationGroup)
}

func (rm *resourceManager) updateShardConfiguration(
	ctx context.Context,
	desired *resource,
	latest *resource,
) (*resource, error) {
	input, err := rm.newUpdateShardConfigurationRequestPayload(desired, latest)
	if err != nil {
		return nil, err
	}
	resp, respErr := rm.sdkapi.ModifyReplicationGroupShardConfiguration(ctx, input)
	rm.metrics.RecordAPICall("UPDATE", "ModifyReplicationGroupShardConfiguration", respErr)
	if respErr != nil {
		rm.log.V(1).Info("Error during ModifyReplicationGroupShardConfiguration", "error", respErr)
		return nil, respErr
	}

	r, err := rm.setReplicationGroupOutput(ctx, desired, resp.ReplicationGroup)

	if err != nil {
		return r, err
	}

	ko := r.ko.DeepCopy()
	// Update the annotations since API call was successful
	rm.setLastRequestedNodeGroupConfiguration(desired, ko)
	rm.setLastRequestedNumNodeGroups(desired, ko)
	return &resource{ko}, nil
}

// newIncreaseReplicaCountRequestPayload returns an SDK-specific struct for the HTTP request
// payload of the Create API call for the resource
func (rm *resourceManager) newIncreaseReplicaCountRequestPayload(
	desired *resource,
	latest *resource,
) (*svcsdk.IncreaseReplicaCountInput, error) {
	res := &svcsdk.IncreaseReplicaCountInput{}
	desiredSpec := desired.ko.Spec

	res.ApplyImmediately = aws.Bool(true)
	if desiredSpec.ReplicationGroupID != nil {
		res.ReplicationGroupId = desiredSpec.ReplicationGroupID
	}
	if desiredSpec.ReplicasPerNodeGroup != nil {
		res.NewReplicaCount = Int32OrNil(desiredSpec.ReplicasPerNodeGroup)
	}

	latestStatus := latest.ko.Status
	// each shard could have different value for replica count
	latestReplicaCounts := map[string]int{}
	for _, latestShard := range latestStatus.NodeGroups {
		if latestShard.NodeGroupID == nil {
			continue
		}
		latestReplicaCount := 0
		if latestShard.NodeGroupMembers != nil {
			if len(latestShard.NodeGroupMembers) > 0 {
				latestReplicaCount = len(latestShard.NodeGroupMembers) - 1
			}
		}
		latestReplicaCounts[*latestShard.NodeGroupID] = latestReplicaCount
	}

	if desiredSpec.NodeGroupConfiguration != nil {
		shardsConfig := []*svcsdktypes.ConfigureShard{}
		for _, desiredShard := range desiredSpec.NodeGroupConfiguration {
			if desiredShard.NodeGroupID == nil {
				continue
			}
			_, found := latestReplicaCounts[*desiredShard.NodeGroupID]
			if !found {
				continue
			}
			// shard has an Id and it is present on server.
			shardConfig := &svcsdktypes.ConfigureShard{}
			shardConfig.NodeGroupId = desiredShard.NodeGroupID
			if desiredShard.ReplicaCount != nil {
				shardConfig.NewReplicaCount = Int32OrNil(desiredShard.ReplicaCount)
			}
			shardAZs := []*string{}
			if desiredShard.PrimaryAvailabilityZone != nil {
				shardAZs = append(shardAZs, desiredShard.PrimaryAvailabilityZone)
			}
			if desiredShard.ReplicaAvailabilityZones != nil {
				for _, desiredAZ := range desiredShard.ReplicaAvailabilityZones {
					shardAZs = append(shardAZs, desiredAZ)
				}
			}
			if len(shardAZs) > 0 {
				stringAZs := make([]string, len(shardAZs))
				for i, az := range shardAZs {
					if az != nil {
						stringAZs[i] = *az
					}
				}
				shardConfig.PreferredAvailabilityZones = stringAZs
			}
			shardsConfig = append(shardsConfig, shardConfig)
		}

		// Convert []*ConfigureShard to []ConfigureShard
		configShards := make([]svcsdktypes.ConfigureShard, len(shardsConfig))
		for i, config := range shardsConfig {
			if config != nil {
				configShards[i] = *config
			}
		}
		res.ReplicaConfiguration = configShards
	}

	return res, nil
}

// newDecreaseReplicaCountRequestPayload returns an SDK-specific struct for the HTTP request
// payload of the Create API call for the resource
func (rm *resourceManager) newDecreaseReplicaCountRequestPayload(
	desired *resource,
	latest *resource,
) (*svcsdk.DecreaseReplicaCountInput, error) {
	res := &svcsdk.DecreaseReplicaCountInput{}
	desiredSpec := desired.ko.Spec

	res.ApplyImmediately = aws.Bool(true)
	if desiredSpec.ReplicationGroupID != nil {
		res.ReplicationGroupId = desiredSpec.ReplicationGroupID
	}
	if desiredSpec.ReplicasPerNodeGroup != nil {
		res.NewReplicaCount = Int32OrNil(desiredSpec.ReplicasPerNodeGroup)
	}

	latestStatus := latest.ko.Status
	// each shard could have different value for replica count
	latestReplicaCounts := map[string]int{}
	for _, latestShard := range latestStatus.NodeGroups {
		if latestShard.NodeGroupID == nil {
			continue
		}
		latestReplicaCount := 0
		if latestShard.NodeGroupMembers != nil {
			if len(latestShard.NodeGroupMembers) > 0 {
				latestReplicaCount = len(latestShard.NodeGroupMembers) - 1
			}
		}
		latestReplicaCounts[*latestShard.NodeGroupID] = latestReplicaCount
	}

	if desiredSpec.NodeGroupConfiguration != nil {
		configShards := make([]svcsdktypes.ConfigureShard, len(desiredSpec.NodeGroupConfiguration))
		for i, config := range desiredSpec.NodeGroupConfiguration {
			stringAZs := make([]string, len(config.ReplicaAvailabilityZones))
			for i, az := range config.ReplicaAvailabilityZones {
				stringAZs[i] = *az
			}
			configShards[i] = svcsdktypes.ConfigureShard{
				NodeGroupId:                config.NodeGroupID,
				NewReplicaCount:            Int32OrNil(config.ReplicaCount),
				PreferredAvailabilityZones: stringAZs,
			}
		}
		res.ReplicaConfiguration = configShards
	}

	return res, nil
}

// newUpdateShardConfigurationRequestPayload returns an SDK-specific struct for the HTTP request
// payload of the Update API call for the resource
func (rm *resourceManager) newUpdateShardConfigurationRequestPayload(
	desired *resource,
	latest *resource,
) (*svcsdk.ModifyReplicationGroupShardConfigurationInput, error) {
	res := &svcsdk.ModifyReplicationGroupShardConfigurationInput{}

	desiredSpec := desired.ko.Spec
	latestStatus := latest.ko.Status

	// Mandatory arguments
	//	- ApplyImmediately
	//	- ReplicationGroupId
	//  - NodeGroupCount
	res.ApplyImmediately = aws.Bool(true)
	if desiredSpec.ReplicationGroupID != nil {
		res.ReplicationGroupId = desiredSpec.ReplicationGroupID
	}
	desiredShardsCount := desiredSpec.NumNodeGroups
	if desiredShardsCount == nil && desiredSpec.NodeGroupConfiguration != nil {
		numShards := int64(len(desiredSpec.NodeGroupConfiguration))
		desiredShardsCount = &numShards
	}
	if desiredShardsCount != nil {
		res.NodeGroupCount = Int32OrNil(desiredShardsCount)
	}

	// If desired nodegroup count (number of shards):
	// - increases, then (optional) provide ReshardingConfiguration
	// - decreases, then (mandatory) provide
	//	 	either 	NodeGroupsToRemove
	//	 	or 		NodeGroupsToRetain
	var latestShardsCount *int64 = nil
	if latestStatus.NodeGroups != nil {
		numShards := int64(len(latestStatus.NodeGroups))
		latestShardsCount = &numShards
	}

	increase := (desiredShardsCount != nil && latestShardsCount != nil && *desiredShardsCount > *latestShardsCount) ||
		(desiredShardsCount != nil && latestShardsCount == nil)
	decrease := desiredShardsCount != nil && latestShardsCount != nil && *desiredShardsCount < *latestShardsCount
	// Additional arguments
	shardsConfig := []*svcsdktypes.ReshardingConfiguration{}
	shardsToRetain := []string{}
	if desiredSpec.NodeGroupConfiguration != nil {
		for _, desiredShard := range desiredSpec.NodeGroupConfiguration {
			shardConfig := &svcsdktypes.ReshardingConfiguration{}
			if desiredShard.NodeGroupID != nil {
				shardConfig.NodeGroupId = desiredShard.NodeGroupID
				shardsToRetain = append(shardsToRetain, *desiredShard.NodeGroupID)
			}
			shardAZs := []string{}
			if desiredShard.PrimaryAvailabilityZone != nil {
				shardAZs = append(shardAZs, *desiredShard.PrimaryAvailabilityZone)
			}
			if desiredShard.ReplicaAvailabilityZones != nil {
				for _, desiredAZ := range desiredShard.ReplicaAvailabilityZones {
					shardAZs = append(shardAZs, *desiredAZ)
				}
				shardConfig.PreferredAvailabilityZones = shardAZs
			}
			shardsConfig = append(shardsConfig, shardConfig)
		}
	} else if decrease {
		for i := 0; i < int(*desiredShardsCount); i++ {
			if desired.ko.Status.NodeGroups[i] != nil && desired.ko.Status.NodeGroups[i].NodeGroupID != nil {
				shardsToRetain = append(shardsToRetain, *desired.ko.Status.NodeGroups[i].NodeGroupID)
			}
		}
	}

	if increase {
		if len(shardsConfig) > 0 {
			reshardConfig := make([]svcsdktypes.ReshardingConfiguration, len(shardsConfig))
			for i, config := range shardsConfig {
				reshardConfig[i] = *config
			}
			res.ReshardingConfiguration = reshardConfig
		}
	} else if decrease {
		if len(shardsToRetain) == 0 {
			return nil, awserr.New("InvalidParameterValue", "At least one node group should be present.", nil)
		}
		res.NodeGroupsToRetain = shardsToRetain
	}

	return res, nil
}

// getAnyCacheClusterIDFromNodeGroups returns a cache cluster ID from supplied node groups.
// Any cache cluster Id which is not nil is returned.
func (rm *resourceManager) getAnyCacheClusterIDFromNodeGroups(
	nodeGroups []*svcapitypes.NodeGroup,
) *string {
	if nodeGroups == nil {
		return nil
	}

	var cacheClusterId *string = nil
	for _, nodeGroup := range nodeGroups {
		if nodeGroup.NodeGroupMembers == nil {
			continue
		}
		for _, nodeGroupMember := range nodeGroup.NodeGroupMembers {
			if nodeGroupMember.CacheClusterID == nil {
				continue
			}
			cacheClusterId = nodeGroupMember.CacheClusterID
			break
		}
		if cacheClusterId != nil {
			break
		}
	}
	return cacheClusterId
}

// describeCacheCluster provides CacheCluster object
// per the supplied latest Replication Group Id
// it invokes DescribeCacheClusters API to do so
func (rm *resourceManager) describeCacheCluster(
	ctx context.Context,
	resource *resource,
) (*svcsdktypes.CacheCluster, error) {
	input := &svcsdk.DescribeCacheClustersInput{}

	ko := resource.ko
	latestStatus := ko.Status
	if latestStatus.NodeGroups == nil {
		return nil, nil
	}
	cacheClusterId := rm.getAnyCacheClusterIDFromNodeGroups(latestStatus.NodeGroups)
	if cacheClusterId == nil {
		return nil, nil
	}

	input.CacheClusterId = cacheClusterId
	resp, respErr := rm.sdkapi.DescribeCacheClusters(ctx, input)
	rm.metrics.RecordAPICall("READ_MANY", "DescribeCacheClusters", respErr)
	if respErr != nil {
		rm.log.V(1).Info("Error during DescribeCacheClusters", "error", respErr)
		return nil, respErr
	}
	if resp.CacheClusters == nil {
		return nil, nil
	}

	for _, cc := range resp.CacheClusters {
		if cc.CacheClusterId == nil {
			continue
		}
		return &cc, nil
	}
	return nil, fmt.Errorf("could not find a non-nil cache cluster from API response")
}

// securityGroupIdsDiffer return true if
// Security Group Ids differ between desired spec and latest (from cache cluster) status
func (rm *resourceManager) securityGroupIdsDiffer(
	desired *resource,
	latest *resource,
	latestCacheCluster *svcsdktypes.CacheCluster,
) bool {
	if desired.ko.Spec.SecurityGroupIDs == nil {
		return false
	}

	desiredIds := []*string{}
	for _, id := range desired.ko.Spec.SecurityGroupIDs {
		if id == nil {
			continue
		}
		var value string
		value = *id
		desiredIds = append(desiredIds, &value)
	}
	sort.Slice(desiredIds, func(i, j int) bool {
		return *desiredIds[i] < *desiredIds[j]
	})

	latestIds := []*string{}
	if latestCacheCluster != nil && latestCacheCluster.SecurityGroups != nil {
		for _, latestSG := range latestCacheCluster.SecurityGroups {
			if latestSG.SecurityGroupId == nil {
				continue
			}
			var value string
			value = *latestSG.SecurityGroupId
			latestIds = append(latestIds, &value)
		}
	}
	sort.Slice(latestIds, func(i, j int) bool {
		return *latestIds[i] < *latestIds[j]
	})

	if len(desiredIds) != len(latestIds) {
		return true // differ
	}
	for index, desiredId := range desiredIds {
		if *desiredId != *latestIds[index] {
			return true // differ
		}
	}
	// no difference
	return false
}

// newModifyReplicationGroupRequestPayload provides request input object
func (rm *resourceManager) newModifyReplicationGroupRequestPayload(
	desired *resource,
	latest *resource,
	latestCacheCluster *svcsdktypes.CacheCluster,
	delta *ackcompare.Delta,
) *svcsdk.ModifyReplicationGroupInput {
	input := &svcsdk.ModifyReplicationGroupInput{}

	input.ApplyImmediately = aws.Bool(true)
	if desired.ko.Spec.ReplicationGroupID != nil {
		input.ReplicationGroupId = desired.ko.Spec.ReplicationGroupID
	}

	if rm.securityGroupIdsDiffer(desired, latest, latestCacheCluster) &&
		desired.ko.Spec.SecurityGroupIDs != nil {
		ids := []string{}
		for _, id := range desired.ko.Spec.SecurityGroupIDs {
			var value string
			value = *id
			ids = append(ids, value)
		}
		input.SecurityGroupIds = ids
	}

	if delta.DifferentAt("Spec.EngineVersion") &&
		desired.ko.Spec.EngineVersion != nil {
		input.EngineVersion = desired.ko.Spec.EngineVersion
	}

	if delta.DifferentAt("Spec.Engine") &&
		desired.ko.Spec.Engine != nil {
		input.Engine = desired.ko.Spec.Engine
	}

	if delta.DifferentAt("Spec.CacheParameterGroupName") &&
		desired.ko.Spec.CacheParameterGroupName != nil {
		input.CacheParameterGroupName = desired.ko.Spec.CacheParameterGroupName
	}

	return input
}

// cacheNodeTypeRequiresUpdate retrieves the last requested cacheNodeType saved in annotations and compares them
// to the current desired cacheNodeType
func cacheNodeTypeRequiresUpdate(desired *resource) bool {
	annotations := desired.ko.ObjectMeta.GetAnnotations()
	if val, ok := annotations[AnnotationLastRequestedCNT]; ok && desired.ko.Spec.CacheNodeType != nil {
		return val != *desired.ko.Spec.CacheNodeType
	}

	// This means there is delta and no value in annotation or in Spec
	return true
}

// nodeGroupRequiresUpdate retrieves the last applied NumNodeGroups and NodeGroupConfiguration and compares them
// to the current desired NumNodeGroups and NodeGroupConfiguration
func nodeGroupRequiresUpdate(desired *resource) bool {
	annotations := desired.ko.ObjectMeta.GetAnnotations()

	if val, ok := annotations[AnnotationLastRequestedNNG]; ok && val != "null" {
		numNodes, err := strconv.ParseInt(val, 10, 64)

		if err != nil {
			return false
		}

		if numNodes != *desired.ko.Spec.NumNodeGroups {
			return true
		}

		return false
	}

	desiredNodeGroupConfig := desired.ko.Spec.NodeGroupConfiguration
	if val, ok := annotations[AnnotationLastRequestedNGC]; ok && val != "null" {
		var lastRequestedNodeGroupConfig []*svcapitypes.NodeGroupConfiguration
		_ = json.Unmarshal([]byte(val), &lastRequestedNodeGroupConfig)
		return !reflect.DeepEqual(desiredNodeGroupConfig, lastRequestedNodeGroupConfig)
	}

	// This means there is delta and no value in annotation or in Spec
	return true
}

/*
To be called in sdkFind, this function updates the replication group's Spec fields with the latest observed state
This requires extra processing of the API response as well as additional API calls, and is necessary because
sdkFind does not update many of these Spec fields by default. "resource" is a wrapper around "ko", the object
which will eventually be returned as "latest".
*/
func (rm *resourceManager) updateSpecFields(
	ctx context.Context,
	respRG svcsdktypes.ReplicationGroup,
	resource *resource,
) {
	if isDeleting(resource) {
		return
	}
	// populate relevant ko.Spec fields with observed state of respRG.NodeGroups
	setReplicasPerNodeGroup(respRG, resource)
	setNodeGroupConfiguration(respRG, resource)

	// updating some Spec fields requires a DescribeCacheClusters call
	latestCacheCluster, err := rm.describeCacheCluster(ctx, resource)
	if err == nil && latestCacheCluster != nil {
		setEngineVersion(latestCacheCluster, resource)
		setMaintenanceWindow(latestCacheCluster, resource)
		setCacheParameterGroup(latestCacheCluster, resource)
	}
}

// if NodeGroupConfiguration was given in the desired.Spec, update ko.Spec with the latest observed value
func setNodeGroupConfiguration(
	respRG svcsdktypes.ReplicationGroup,
	resource *resource,
) {
	ko := resource.ko
	if respRG.NodeGroups != nil && ko.Spec.NodeGroupConfiguration != nil {
		nodeGroupConfigurations := []*svcapitypes.NodeGroupConfiguration{}
		for _, nodeGroup := range respRG.NodeGroups {
			nodeGroupConfiguration := &svcapitypes.NodeGroupConfiguration{}

			if nodeGroup.NodeGroupId != nil {
				nodeGroupConfiguration.NodeGroupID = nodeGroup.NodeGroupId
			}
			replicaAZs := []*string{}

			for _, nodeGroupMember := range nodeGroup.NodeGroupMembers {
				if nodeGroupMember.CurrentRole != nil && *nodeGroupMember.CurrentRole == "primary" {
					nodeGroupConfiguration.PrimaryAvailabilityZone = nodeGroupMember.PreferredAvailabilityZone
				}

				// In this case we cannot say what is primary AZ and replica AZ.
				if nodeGroupMember.CurrentRole == nil && nodeGroupConfiguration.PrimaryAvailabilityZone == nil {
					// We cannot determine the correct AZ so we would use the first node group member as primary
					nodeGroupConfiguration.PrimaryAvailabilityZone = nodeGroupMember.PreferredAvailabilityZone
				}

				if nodeGroupConfiguration.PrimaryAvailabilityZone != nil || *nodeGroupMember.CurrentRole == "replica" {
					replicaAZs = append(replicaAZs, nodeGroupMember.PreferredAvailabilityZone)
				}
			}

			if len(replicaAZs) > 0 {
				nodeGroupConfiguration.ReplicaAvailabilityZones = replicaAZs
			}

			replicaCount := int64(len(replicaAZs))
			nodeGroupConfiguration.ReplicaCount = &replicaCount
		}

		ko.Spec.NodeGroupConfiguration = nodeGroupConfigurations
	}

	if respRG.NodeGroups != nil && ko.Spec.NumNodeGroups != nil {
		*ko.Spec.NumNodeGroups = int64(len(respRG.NodeGroups))
	}
}

//TODO: for all the fields here, reevaluate if the latest observed state should always be populated,
// even if the corresponding field was not specified in desired

// if ReplicasPerNodeGroup was given in desired.Spec, update ko.Spec with the latest observed value
func setReplicasPerNodeGroup(
	respRG svcsdktypes.ReplicationGroup,
	resource *resource,
) {
	ko := resource.ko
	if respRG.NodeGroups != nil && ko.Spec.ReplicasPerNodeGroup != nil {
		// if ReplicasPerNodeGroup is specified, all node groups should have the same # replicas so use the first
		nodeGroup := respRG.NodeGroups[0]
		if nodeGroup.NodeGroupMembers != nil {
			if len(nodeGroup.NodeGroupMembers) > 0 {
				*ko.Spec.ReplicasPerNodeGroup = int64(len(nodeGroup.NodeGroupMembers) - 1)
			}
		}
	}
}

// if EngineVersion was specified in desired.Spec, update ko.Spec with the latest observed value (if non-nil)
func setEngineVersion(
	latestCacheCluster *svcsdktypes.CacheCluster,
	resource *resource,
) {
	ko := resource.ko
	if ko.Spec.EngineVersion != nil && latestCacheCluster.EngineVersion != nil {
		*ko.Spec.EngineVersion = *latestCacheCluster.EngineVersion
	}
}

// update maintenance window (if non-nil in API response) regardless of whether it was specified in desired
func setMaintenanceWindow(
	latestCacheCluster *svcsdktypes.CacheCluster,
	resource *resource,
) {
	ko := resource.ko
	if latestCacheCluster.PreferredMaintenanceWindow != nil {
		pmw := *latestCacheCluster.PreferredMaintenanceWindow
		ko.Spec.PreferredMaintenanceWindow = &pmw
	}
}

// setCacheParameterGroup updates the cache parameter group associated with the replication group
//
//	(if non-nil in API response) regardless of whether it was specified in desired
func setCacheParameterGroup(
	latestCacheCluster *svcsdktypes.CacheCluster,
	resource *resource,
) {
	ko := resource.ko
	if latestCacheCluster.CacheParameterGroup != nil && latestCacheCluster.CacheParameterGroup.CacheParameterGroupName != nil {
		cpgName := *latestCacheCluster.CacheParameterGroup.CacheParameterGroupName
		ko.Spec.CacheParameterGroupName = &cpgName
	}
}

// modifyDelta removes non-meaningful differences from the delta and adds additional differences if necessary
func modifyDelta(
	delta *ackcompare.Delta,
	desired *resource,
	latest *resource,
) {

	if delta.DifferentAt("Spec.EngineVersion") {
		if desired.ko.Spec.EngineVersion != nil && latest.ko.Spec.EngineVersion != nil {
			if util.EngineVersionsMatch(*desired.ko.Spec.EngineVersion, *latest.ko.Spec.EngineVersion) {
				common.RemoveFromDelta(delta, "Spec.EngineVersion")
			}
		}
		// TODO: handle the case of a nil difference (especially when desired EV is nil)
	}

	// if server has given PreferredMaintenanceWindow a default value, no action needs to be taken
	if delta.DifferentAt("Spec.PreferredMaintenanceWindow") {
		if desired.ko.Spec.PreferredMaintenanceWindow == nil && latest.ko.Spec.PreferredMaintenanceWindow != nil {
			common.RemoveFromDelta(delta, "Spec.PreferredMaintenanceWindow")
		}
	}

	// note that the comparison is actually done between desired.Spec.LogDeliveryConfigurations and
	// the last requested configurations saved in annotations (as opposed to latest.Spec.LogDeliveryConfigurations)
	if logDeliveryRequiresUpdate(desired) {
		delta.Add("Spec.LogDeliveryConfigurations", desired.ko.Spec.LogDeliveryConfigurations,
			unmarshalLastRequestedLDCs(desired))
	}

	if multiAZRequiresUpdate(desired, latest) {
		delta.Add("Spec.MultiAZEnabled", desired.ko.Spec.MultiAZEnabled, latest.ko.Status.MultiAZ)
	}

	if autoFailoverRequiresUpdate(desired, latest) {
		delta.Add("Spec.AutomaticFailoverEnabled", desired.ko.Spec.AutomaticFailoverEnabled,
			latest.ko.Status.AutomaticFailover)
	}

	if updateRequired, current := primaryClusterIDRequiresUpdate(desired, latest); updateRequired {
		delta.Add("Spec.PrimaryClusterID", desired.ko.Spec.PrimaryClusterID, *current)
	}
}

// logDeliveryRequiresUpdate retrieves the last requested configurations saved in annotations and compares them
// to the current desired configurations
func logDeliveryRequiresUpdate(desired *resource) bool {
	desiredConfigs := desired.ko.Spec.LogDeliveryConfigurations
	lastRequestedConfigs := unmarshalLastRequestedLDCs(desired)
	return !reflect.DeepEqual(desiredConfigs, lastRequestedConfigs)
}

// unmarshal the value found in annotations for the LogDeliveryConfigurations field requested in the last
// successful create or modify call
func unmarshalLastRequestedLDCs(desired *resource) []*svcapitypes.LogDeliveryConfigurationRequest {
	var lastRequestedConfigs []*svcapitypes.LogDeliveryConfigurationRequest

	annotations := desired.ko.ObjectMeta.GetAnnotations()
	if val, ok := annotations[AnnotationLastRequestedLDCs]; ok {
		_ = json.Unmarshal([]byte(val), &lastRequestedConfigs)
	}

	return lastRequestedConfigs
}

// multiAZRequiresUpdate returns true if the latest multi AZ status does not yet match the
// desired state, and false otherwise
func multiAZRequiresUpdate(desired *resource, latest *resource) bool {
	// no preference for multi AZ specified; no update required
	if desired.ko.Spec.MultiAZEnabled == nil {
		return false
	}

	// API should return a non-nil value, but if it doesn't then attempt to update
	if latest.ko.Status.MultiAZ == nil {
		return true
	}

	// true maps to "enabled"; false maps to "disabled"
	// this accounts for values such as "enabling" and "disabling"
	if *desired.ko.Spec.MultiAZEnabled {
		return *latest.ko.Status.MultiAZ != string(svcapitypes.MultiAZStatus_enabled)
	} else {
		return *latest.ko.Status.MultiAZ != string(svcapitypes.MultiAZStatus_disabled)
	}
}

// autoFailoverRequiresUpdate returns true if the latest auto failover status does not yet match the
// desired state, and false otherwise
func autoFailoverRequiresUpdate(desired *resource, latest *resource) bool {
	// the logic is exactly analogous to multiAZRequiresUpdate above
	if desired.ko.Spec.AutomaticFailoverEnabled == nil {
		return false
	}

	if latest.ko.Status.AutomaticFailover == nil {
		return true
	}

	if *desired.ko.Spec.AutomaticFailoverEnabled {
		return *latest.ko.Status.AutomaticFailover != string(svcapitypes.AutomaticFailoverStatus_enabled)
	} else {
		return *latest.ko.Status.AutomaticFailover != string(svcapitypes.AutomaticFailoverStatus_disabled)
	}
}

// primaryClusterIDRequiresUpdate retrieves the current primary cluster ID and determines whether
// an update is required. If no desired state is specified or there is an issue retrieving the
// latest state, return false, nil. Otherwise, return false or true depending on equality of
// the latest and desired states, and a non-nil pointer to the latest value
func primaryClusterIDRequiresUpdate(desired *resource, latest *resource) (bool, *string) {
	if desired.ko.Spec.PrimaryClusterID == nil {
		return false, nil
	}

	// primary cluster ID applies to cluster mode disabled only; if API returns multiple
	//   or no node groups, or the provided node group is nil, there is nothing that can be done
	if len(latest.ko.Status.NodeGroups) != 1 || latest.ko.Status.NodeGroups[0] == nil {
		return false, nil
	}

	// attempt to find primary cluster in node group. If for some reason it is not present, we
	//   don't have a reliable latest state, so do nothing
	ng := *latest.ko.Status.NodeGroups[0]
	for _, member := range ng.NodeGroupMembers {
		if member == nil {
			continue
		}

		if member.CurrentRole != nil && *member.CurrentRole == "primary" && member.CacheClusterID != nil {
			val := *member.CacheClusterID
			return val != *desired.ko.Spec.PrimaryClusterID, &val
		}
	}

	return false, nil
}

func Int32OrNil(i *int64) *int32 {
	if i == nil {
		return nil
	}
	return aws.Int32(int32(*i))
}
