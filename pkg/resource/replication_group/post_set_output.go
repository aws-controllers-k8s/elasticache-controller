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

	svcsdk "github.com/aws/aws-sdk-go/service/elasticache"
)

/*
	To be called in sdkFind, this function updates the replication group's Spec fields with the latest observed state
	This requires extra processing of the API response as well as additional API calls, and is necessary because
	sdkFind does not update many of these Spec fields by default. "resource" is a wrapper around "ko", the object
	which will eventually be returned as "latest".
*/
func (rm *resourceManager) updateSpecFields(
	ctx context.Context,
	respRG *svcsdk.ReplicationGroup,
	resource *resource,
) {
	if isDeleting(resource) {
		return
	}
	// populate relevant ko.Spec fields with observed state of respRG.NodeGroups
	setReplicasPerNodeGroup(respRG, resource)

	//TODO: set Spec NodeGroupConfiguration

	// updating some Spec fields requires a DescribeCacheClusters call
	latestCacheCluster, err := rm.describeCacheCluster(ctx, resource)
	if err == nil && latestCacheCluster != nil {
		setEngineVersion(latestCacheCluster, resource)
		setMaintenanceWindow(latestCacheCluster, resource)
	}
}

//TODO: for all the fields here, reevaluate if the latest observed state should always be populated,
// even if the corresponding field was not specified in desired

// if ReplicasPerNodeGroup was given in desired.Spec, update ko.Spec with the latest observed value
func setReplicasPerNodeGroup(
	respRG *svcsdk.ReplicationGroup,
	resource *resource,
) {
	ko := resource.ko
	if respRG.NodeGroups != nil && ko.Spec.ReplicasPerNodeGroup != nil {
		// if ReplicasPerNodeGroup is specified, all node groups should have the same # replicas so use the first
		nodeGroup := respRG.NodeGroups[0]
		if nodeGroup != nil && nodeGroup.NodeGroupMembers != nil {
			if len(nodeGroup.NodeGroupMembers) > 0 {
				*ko.Spec.ReplicasPerNodeGroup = int64(len(nodeGroup.NodeGroupMembers) - 1)
			}
		}
	}
}

// if EngineVersion was specified in desired.Spec, update ko.Sepc with the latest observed value (if non-nil)
func setEngineVersion(
	latestCacheCluster *svcsdk.CacheCluster,
	resource *resource,
) {
	ko := resource.ko
	if ko.Spec.EngineVersion != nil && latestCacheCluster.EngineVersion != nil {
		*ko.Spec.EngineVersion = *latestCacheCluster.EngineVersion
	}
}

// update maintenance window (if non-nil in API response) regardless of whether it was specified in desired
func setMaintenanceWindow(
	latestCacheCluster *svcsdk.CacheCluster,
	resource *resource,
) {
	ko := resource.ko
	if latestCacheCluster.PreferredMaintenanceWindow != nil {
		pmw := *latestCacheCluster.PreferredMaintenanceWindow
		ko.Spec.PreferredMaintenanceWindow = &pmw
	}
}