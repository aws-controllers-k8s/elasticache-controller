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

package cache_cluster

import (
	"context"
	"errors"
	"fmt"
	"slices"

	ackcompare "github.com/aws-controllers-k8s/runtime/pkg/compare"
	ackrequeue "github.com/aws-controllers-k8s/runtime/pkg/requeue"
	svcsdk "github.com/aws/aws-sdk-go/service/elasticache"

	svcapitypes "github.com/aws-controllers-k8s/elasticache-controller/apis/v1alpha1"
	"github.com/aws-controllers-k8s/elasticache-controller/pkg/util"
)

const (
	statusCreating  = "creating"
	statusAvailable = "available"
	statusModifying = "modifying"
	statusDeleting  = "deleting"
)

const (
	// AnnotationLastRequestedPAZs is an annotation whose value is a JSON representation of []*string,
	// passed in as input to either the create or modify API called most recently.
	AnnotationLastRequestedPAZs = svcapitypes.AnnotationPrefix + "last-requested-preferred-availability-zones"
)

var (
	condMsgCurrentlyDeleting      = "CacheCluster is currently being deleted"
	condMsgNoDeleteWhileModifying = "Cannot delete CacheCluster while it is being modified"
	condMsgCurrentlyUpdating      = "CacheCluster is currently being updated"
)

var (
	requeueWaitWhileDeleting = ackrequeue.NeededAfter(
		fmt.Errorf("CacheCluster is in %q state, it cannot be deleted", statusDeleting),
		ackrequeue.DefaultRequeueAfterDuration,
	)
	requeueWaitWhileModifying = ackrequeue.NeededAfter(
		fmt.Errorf("CacheCluster is in %q state, it cannot be modified", statusModifying),
		ackrequeue.DefaultRequeueAfterDuration,
	)
)

func hasStatus(r *resource, status string) bool {
	return r.ko.Status.CacheClusterStatus != nil && *r.ko.Status.CacheClusterStatus == status
}

func isCreating(r *resource) bool {
	return hasStatus(r, statusCreating)
}

func isAvailable(r *resource) bool {
	return hasStatus(r, statusAvailable)
}

func isDeleting(r *resource) bool {
	return hasStatus(r, statusDeleting)
}

func isModifying(r *resource) bool {
	return hasStatus(r, statusModifying)
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
	return util.SyncTags(ctx, desired.ko.Spec.Tags, latest.ko.Spec.Tags, latest.ko.Status.ACKResourceMetadata, ToACKTags, rm.sdkapi, rm.metrics)
}

func (rm *resourceManager) updateCacheClusterPayload(input *svcsdk.ModifyCacheClusterInput, desired, latest *resource, delta *ackcompare.Delta) error {
	desiredSpec := desired.ko.Spec
	var nodesDelta int64
	if delta.DifferentAt("Spec.NumCacheNodes") && desired.ko.Spec.NumCacheNodes != nil {
		numNodes := *latest.ko.Spec.NumCacheNodes
		if pendingModifications := latest.ko.Status.PendingModifiedValues; pendingModifications != nil &&
			pendingModifications.NumCacheNodes != nil && *pendingModifications.NumCacheNodes > numNodes {
			numNodes = *pendingModifications.NumCacheNodes
		}
		nodesDelta = numNodes - *desired.ko.Spec.NumCacheNodes
		if nodesDelta > 0 {
			for i := numNodes; i > numNodes-nodesDelta; i-- {
				nodeID := fmt.Sprintf("%04d", i)
				input.CacheNodeIdsToRemove = append(input.CacheNodeIdsToRemove, &nodeID)
			}
		}
	}

	if idx := slices.IndexFunc(delta.Differences, func(diff *ackcompare.Difference) bool {
		return diff.Path.Contains("Spec.PreferredAvailabilityZones")
	}); idx != -1 && desired.ko.Spec.PreferredAvailabilityZones != nil {
		if nodesDelta >= 0 {
			return errors.New("spec.preferredAvailabilityZones can only be changed when new nodes are being added via spec.numCacheNodes")
		}

		oldAZsLen := 0
		oldValues, ok := delta.Differences[idx].B.([]*string)
		if ok {
			oldAZsLen = len(oldValues)
		}
		if len(desiredSpec.PreferredAvailabilityZones) <= oldAZsLen {
			return errors.New("newly specified AZs in spec.preferredAvailabilityZones must match the number of cache nodes being added")
		}
		input.NewAvailabilityZones = desiredSpec.PreferredAvailabilityZones[oldAZsLen:]
	}
	return nil
}
