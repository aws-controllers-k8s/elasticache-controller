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
	"fmt"
	"reflect"
	"sort"
	"strconv"

	"github.com/aws-controllers-k8s/runtime/pkg/requeue"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/pkg/errors"

	svcapitypes "github.com/aws-controllers-k8s/elasticache-controller/apis/v1alpha1"
	ackcompare "github.com/aws-controllers-k8s/runtime/pkg/compare"
	svcsdk "github.com/aws/aws-sdk-go/service/elasticache"
)

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
		delta.DifferentAt("Spec.EngineVersion") {
		input := rm.newModifyReplicationGroupRequestPayload(desired, latest, latestCacheCluster, delta)
		resp, respErr := rm.sdkapi.ModifyReplicationGroupWithContext(ctx, input)
		rm.metrics.RecordAPICall("UPDATE", "ModifyReplicationGroup", respErr)
		if respErr != nil {
			rm.log.V(1).Info("Error during ModifyReplicationGroup", "error", respErr)
			return nil, respErr
		}

		return rm.setReplicationGroupOutput(desired, resp.ReplicationGroup)
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
	resp, respErr := rm.sdkapi.IncreaseReplicaCountWithContext(ctx, input)
	rm.metrics.RecordAPICall("UPDATE", "IncreaseReplicaCount", respErr)
	if respErr != nil {
		rm.log.V(1).Info("Error during IncreaseReplicaCount", "error", respErr)
		return nil, respErr
	}
	return rm.setReplicationGroupOutput(desired, resp.ReplicationGroup)
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
	resp, respErr := rm.sdkapi.DecreaseReplicaCountWithContext(ctx, input)
	rm.metrics.RecordAPICall("UPDATE", "DecreaseReplicaCount", respErr)
	if respErr != nil {
		rm.log.V(1).Info("Error during DecreaseReplicaCount", "error", respErr)
		return nil, respErr
	}
	return rm.setReplicationGroupOutput(desired, resp.ReplicationGroup)
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
	resp, respErr := rm.sdkapi.ModifyReplicationGroupShardConfigurationWithContext(ctx, input)
	rm.metrics.RecordAPICall("UPDATE", "ModifyReplicationGroupShardConfiguration", respErr)
	if respErr != nil {
		rm.log.V(1).Info("Error during ModifyReplicationGroupShardConfiguration", "error", respErr)
		return nil, respErr
	}

	r, err := rm.setReplicationGroupOutput(desired, resp.ReplicationGroup)

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

	res.SetApplyImmediately(true)
	if desiredSpec.ReplicationGroupID != nil {
		res.SetReplicationGroupId(*desiredSpec.ReplicationGroupID)
	}
	if desiredSpec.ReplicasPerNodeGroup != nil {
		res.SetNewReplicaCount(*desiredSpec.ReplicasPerNodeGroup)
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
		shardsConfig := []*svcsdk.ConfigureShard{}
		for _, desiredShard := range desiredSpec.NodeGroupConfiguration {
			if desiredShard.NodeGroupID == nil {
				continue
			}
			_, found := latestReplicaCounts[*desiredShard.NodeGroupID]
			if !found {
				continue
			}
			// shard has an Id and it is present on server.
			shardConfig := &svcsdk.ConfigureShard{}
			shardConfig.SetNodeGroupId(*desiredShard.NodeGroupID)
			if desiredShard.ReplicaCount != nil {
				shardConfig.SetNewReplicaCount(*desiredShard.ReplicaCount)
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
				shardConfig.SetPreferredAvailabilityZones(shardAZs)
			}
			shardsConfig = append(shardsConfig, shardConfig)
		}
		res.SetReplicaConfiguration(shardsConfig)
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

	res.SetApplyImmediately(true)
	if desiredSpec.ReplicationGroupID != nil {
		res.SetReplicationGroupId(*desiredSpec.ReplicationGroupID)
	}
	if desiredSpec.ReplicasPerNodeGroup != nil {
		res.SetNewReplicaCount(*desiredSpec.ReplicasPerNodeGroup)
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
		shardsConfig := []*svcsdk.ConfigureShard{}
		for _, desiredShard := range desiredSpec.NodeGroupConfiguration {
			if desiredShard.NodeGroupID == nil {
				continue
			}
			_, found := latestReplicaCounts[*desiredShard.NodeGroupID]
			if !found {
				continue
			}
			// shard has an Id and it is present on server.
			shardConfig := &svcsdk.ConfigureShard{}
			shardConfig.SetNodeGroupId(*desiredShard.NodeGroupID)
			if desiredShard.ReplicaCount != nil {
				shardConfig.SetNewReplicaCount(*desiredShard.ReplicaCount)
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
				shardConfig.SetPreferredAvailabilityZones(shardAZs)
			}
			shardsConfig = append(shardsConfig, shardConfig)
		}
		res.SetReplicaConfiguration(shardsConfig)
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
	res.SetApplyImmediately(true)
	if desiredSpec.ReplicationGroupID != nil {
		res.SetReplicationGroupId(*desiredSpec.ReplicationGroupID)
	}
	var desiredShardsCount *int64 = desiredSpec.NumNodeGroups
	if desiredShardsCount == nil && desiredSpec.NodeGroupConfiguration != nil {
		numShards := int64(len(desiredSpec.NodeGroupConfiguration))
		desiredShardsCount = &numShards
	}
	if desiredShardsCount != nil {
		res.SetNodeGroupCount(*desiredShardsCount)
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
	shardsConfig := []*svcsdk.ReshardingConfiguration{}
	shardsToRetain := []*string{}
	if desiredSpec.NodeGroupConfiguration != nil {
		for _, desiredShard := range desiredSpec.NodeGroupConfiguration {
			shardConfig := &svcsdk.ReshardingConfiguration{}
			if desiredShard.NodeGroupID != nil {
				shardConfig.SetNodeGroupId(*desiredShard.NodeGroupID)
				shardsToRetain = append(shardsToRetain, desiredShard.NodeGroupID)
			}
			shardAZs := []*string{}
			if desiredShard.PrimaryAvailabilityZone != nil {
				shardAZs = append(shardAZs, desiredShard.PrimaryAvailabilityZone)
			}
			if desiredShard.ReplicaAvailabilityZones != nil {
				for _, desiredAZ := range desiredShard.ReplicaAvailabilityZones {
					shardAZs = append(shardAZs, desiredAZ)
				}
				shardConfig.SetPreferredAvailabilityZones(shardAZs)
			}
			shardsConfig = append(shardsConfig, shardConfig)
		}
	} else if decrease {
		for i := 0; i < int(*desiredShardsCount); i++ {
			shardsToRetain = append(shardsToRetain, desired.ko.Status.NodeGroups[i].NodeGroupID)
		}
	}

	if increase {
		if len(shardsConfig) > 0 {
			res.SetReshardingConfiguration(shardsConfig)
		}
	} else if decrease {
		if len(shardsToRetain) == 0 {
			return nil, awserr.New("InvalidParameterValue", "At least one node group should be present.", nil)
		}
		res.SetNodeGroupsToRetain(shardsToRetain)
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
) (*svcsdk.CacheCluster, error) {
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

	input.SetCacheClusterId(*cacheClusterId)
	resp, respErr := rm.sdkapi.DescribeCacheClustersWithContext(ctx, input)
	rm.metrics.RecordAPICall("READ_MANY", "DescribeCacheClusters", respErr)
	if respErr != nil {
		rm.log.V(1).Info("Error during DescribeCacheClusters", "error", respErr)
		return nil, respErr
	}
	if resp.CacheClusters == nil {
		return nil, nil
	}

	for _, cc := range resp.CacheClusters {
		if cc == nil {
			continue
		}
		return cc, nil
	}
	return nil, fmt.Errorf("could not find a non-nil cache cluster from API response")
}

// securityGroupIdsDiffer return true if
// Security Group Ids differ between desired spec and latest (from cache cluster) status
func (rm *resourceManager) securityGroupIdsDiffer(
	desired *resource,
	latest *resource,
	latestCacheCluster *svcsdk.CacheCluster,
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
			if latestSG == nil {
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
	latestCacheCluster *svcsdk.CacheCluster,
	delta *ackcompare.Delta,
) *svcsdk.ModifyReplicationGroupInput {
	input := &svcsdk.ModifyReplicationGroupInput{}

	input.SetApplyImmediately(true)
	if desired.ko.Spec.ReplicationGroupID != nil {
		input.SetReplicationGroupId(*desired.ko.Spec.ReplicationGroupID)
	}

	if rm.securityGroupIdsDiffer(desired, latest, latestCacheCluster) &&
		desired.ko.Spec.SecurityGroupIDs != nil {
		ids := []*string{}
		for _, id := range desired.ko.Spec.SecurityGroupIDs {
			var value string
			value = *id
			ids = append(ids, &value)
		}
		input.SetSecurityGroupIds(ids)
	}

	if delta.DifferentAt("Spec.EngineVersion") &&
		desired.ko.Spec.EngineVersion != nil {
		input.SetEngineVersion(*desired.ko.Spec.EngineVersion)
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
