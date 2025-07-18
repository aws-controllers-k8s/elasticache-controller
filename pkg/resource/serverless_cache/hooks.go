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

package serverless_cache

import (
	"context"
	"fmt"
	"time"

	svcapitypes "github.com/aws-controllers-k8s/elasticache-controller/apis/v1alpha1"
	"github.com/aws-controllers-k8s/elasticache-controller/pkg/util"
	ackcompare "github.com/aws-controllers-k8s/runtime/pkg/compare"
	ackcondition "github.com/aws-controllers-k8s/runtime/pkg/condition"
	ackrequeue "github.com/aws-controllers-k8s/runtime/pkg/requeue"
	ackrtlog "github.com/aws-controllers-k8s/runtime/pkg/runtime/log"
	"github.com/aws/aws-sdk-go-v2/aws"
	svcsdk "github.com/aws/aws-sdk-go-v2/service/elasticache"
	svcsdktypes "github.com/aws/aws-sdk-go-v2/service/elasticache/types"
	corev1 "k8s.io/api/core/v1"
)

const (
	ServerlessCacheStatusCreating  = "creating"
	ServerlessCacheStatusDeleting  = "deleting"
	ServerlessCacheStatusModifying = "modifying"
)

var (
	ErrServerlessCacheDeleting = fmt.Errorf(
		"serverless cache in '%v' state, cannot be modified or deleted",
		ServerlessCacheStatusDeleting,
	)
	ErrServerlessCacheCreating = fmt.Errorf(
		"serverless cache in '%v' state, cannot be modified or deleted",
		ServerlessCacheStatusCreating,
	)
	ErrServerlessCacheModifying = fmt.Errorf(
		"serverless cache in '%v' state, cannot be further modified",
		ServerlessCacheStatusModifying,
	)
)

var (
	requeueWaitWhileDeleting = ackrequeue.NeededAfter(
		ErrServerlessCacheDeleting,
		5*time.Second,
	)
	requeueWaitWhileCreating = ackrequeue.NeededAfter(
		ErrServerlessCacheCreating,
		5*time.Second,
	)
	requeueWaitWhileModifying = ackrequeue.NeededAfter(
		ErrServerlessCacheModifying,
		10*time.Second,
	)
)

// modifyServerlessCache is a central function that creates the input object and makes the API call
// with consistent metrics recording
func (rm *resourceManager) modifyServerlessCache(
	ctx context.Context,
	serverlessCacheName *string,
	configFunc func(*svcsdk.ModifyServerlessCacheInput),
) error {
	input := &svcsdk.ModifyServerlessCacheInput{
		ServerlessCacheName: serverlessCacheName,
	}

	if configFunc != nil {
		configFunc(input)
	}

	_, err := rm.sdkapi.ModifyServerlessCache(ctx, input)
	rm.metrics.RecordAPICall("UPDATE", "ModifyServerlessCache", err)
	return err
}

func isServerlessCacheCreating(r *resource) bool {
	if r.ko.Status.Status == nil {
		return false
	}
	return *r.ko.Status.Status == ServerlessCacheStatusCreating
}

func isServerlessCacheDeleting(r *resource) bool {
	if r.ko.Status.Status == nil {
		return false
	}
	return *r.ko.Status.Status == ServerlessCacheStatusDeleting
}

func isServerlessCacheModifying(r *resource) bool {
	if r.ko.Status.Status == nil {
		return false
	}
	return *r.ko.Status.Status == ServerlessCacheStatusModifying
}

// customUpdateServerlessCache handles updates in a phased approach similar to DynamoDB table updates
func (rm *resourceManager) customUpdateServerlessCache(
	ctx context.Context,
	desired *resource,
	latest *resource,
	delta *ackcompare.Delta,
) (updated *resource, err error) {
	rlog := ackrtlog.FromContext(ctx)
	exit := rlog.Trace("rm.customUpdateServerlessCache")
	defer func() { exit(err) }()

	if isServerlessCacheDeleting(latest) {
		msg := "serverless cache is currently being deleted"
		ackcondition.SetSynced(desired, corev1.ConditionFalse, &msg, nil)
		return desired, requeueWaitWhileDeleting
	}
	if isServerlessCacheCreating(latest) {
		msg := "serverless cache is currently being created"
		ackcondition.SetSynced(desired, corev1.ConditionFalse, &msg, nil)
		return desired, requeueWaitWhileCreating
	}
	if isServerlessCacheModifying(latest) {
		msg := "serverless cache is currently being modified"
		ackcondition.SetSynced(desired, corev1.ConditionFalse, &msg, nil)
		return desired, requeueWaitWhileModifying
	}

	// Merge in the information we read from the API call above to the copy of
	// the original Kubernetes object we passed to the function
	ko := desired.ko.DeepCopy()
	rm.setStatusDefaults(ko)

	switch {
	case delta.DifferentAt("Spec.Description"):
		if err := rm.syncDescription(ctx, desired); err != nil {
			return nil, fmt.Errorf("cannot update serverless cache: %v", err)
		}

	case delta.DifferentAt("Spec.Tags"):
		if err := rm.syncTags(ctx, desired, latest); err != nil {
			return &resource{ko}, err
		}

	case delta.DifferentAt("Spec.DailySnapshotTime"):
		if err := rm.syncDailySnapshotTime(ctx, desired); err != nil {
			return nil, fmt.Errorf("cannot update serverless cache: %v", err)
		}

	case delta.DifferentAt("Spec.SnapshotRetentionLimit"):
		if err := rm.syncSnapshotRetentionLimit(ctx, desired); err != nil {
			return nil, fmt.Errorf("cannot update serverless cache: %v", err)
		}

	case delta.DifferentAt("Spec.SecurityGroupIDs"):
		if err := rm.syncSecurityGroupIDs(ctx, desired); err != nil {
			return nil, fmt.Errorf("cannot update serverless cache: %v", err)
		}

	case delta.DifferentAt("Spec.UserGroupID"):
		if err := rm.syncUserGroupID(ctx, desired); err != nil {
			return nil, fmt.Errorf("cannot update serverless cache: %v", err)
		}

	case delta.DifferentAt("Spec.CacheUsageLimits"):
		if err := rm.syncCacheUsageLimits(ctx, desired); err != nil {
			return nil, fmt.Errorf("cannot update serverless cache: %v", err)
		}

	case delta.DifferentAt("Spec.Engine") || delta.DifferentAt("Spec.MajorEngineVersion"):
		if err := rm.syncEngineAndVersion(ctx, desired); err != nil {
			return nil, fmt.Errorf("cannot update serverless cache: %v", err)
		}
	}

	return &resource{ko}, requeueWaitWhileModifying
}

// syncDescription handles updating only the description field
func (rm *resourceManager) syncDescription(
	ctx context.Context,
	desired *resource,
) error {
	return rm.modifyServerlessCache(ctx, desired.ko.Spec.ServerlessCacheName, func(input *svcsdk.ModifyServerlessCacheInput) {
		input.Description = desired.ko.Spec.Description
	})
}

// syncDailySnapshotTime handles updating only the daily snapshot time field
func (rm *resourceManager) syncDailySnapshotTime(
	ctx context.Context,
	desired *resource,
) error {
	return rm.modifyServerlessCache(ctx, desired.ko.Spec.ServerlessCacheName, func(input *svcsdk.ModifyServerlessCacheInput) {
		input.DailySnapshotTime = desired.ko.Spec.DailySnapshotTime
	})
}

// syncSnapshotRetentionLimit handles updating only the snapshot retention limit field
func (rm *resourceManager) syncSnapshotRetentionLimit(
	ctx context.Context,
	desired *resource,
) error {
	return rm.modifyServerlessCache(ctx, desired.ko.Spec.ServerlessCacheName, func(input *svcsdk.ModifyServerlessCacheInput) {
		if desired.ko.Spec.SnapshotRetentionLimit != nil {
			snapshotRetentionLimitCopy := int32(*desired.ko.Spec.SnapshotRetentionLimit)
			input.SnapshotRetentionLimit = &snapshotRetentionLimitCopy
		}
	})
}

// syncSecurityGroupIDs handles updating only the security group IDs field
func (rm *resourceManager) syncSecurityGroupIDs(
	ctx context.Context,
	desired *resource,
) error {
	securityGroupIDs := aws.ToStringSlice(desired.ko.Spec.SecurityGroupIDs)
	// AWS ElastiCache ModifyServerlessCache doesn't support unsetting SecurityGroupIds
	err := rm.modifyServerlessCache(ctx, desired.ko.Spec.ServerlessCacheName, func(input *svcsdk.ModifyServerlessCacheInput) {
		input.SecurityGroupIds = securityGroupIDs
	})
	return err
}

// syncUserGroupID handles updating only the user group ID field
func (rm *resourceManager) syncUserGroupID(
	ctx context.Context,
	desired *resource,
) error {
	return rm.modifyServerlessCache(ctx, desired.ko.Spec.ServerlessCacheName, func(input *svcsdk.ModifyServerlessCacheInput) {
		input.UserGroupId = desired.ko.Spec.UserGroupID
	})
}

// syncCacheUsageLimits handles updating the cache usage limits which may have restrictions
func (rm *resourceManager) syncCacheUsageLimits(
	ctx context.Context,
	desired *resource,
) error {
	return rm.modifyServerlessCache(ctx, desired.ko.Spec.ServerlessCacheName, func(input *svcsdk.ModifyServerlessCacheInput) {
		if desired.ko.Spec.CacheUsageLimits != nil {
			f0 := &svcsdktypes.CacheUsageLimits{}
			if desired.ko.Spec.CacheUsageLimits.DataStorage != nil {
				f0f0 := &svcsdktypes.DataStorage{}
				if desired.ko.Spec.CacheUsageLimits.DataStorage.Maximum != nil {
					maximumCopy0 := *desired.ko.Spec.CacheUsageLimits.DataStorage.Maximum
					maximumCopy := int32(maximumCopy0)
					f0f0.Maximum = &maximumCopy
				}
				if desired.ko.Spec.CacheUsageLimits.DataStorage.Minimum != nil {
					minimumCopy0 := *desired.ko.Spec.CacheUsageLimits.DataStorage.Minimum
					minimumCopy := int32(minimumCopy0)
					f0f0.Minimum = &minimumCopy
				}
				if desired.ko.Spec.CacheUsageLimits.DataStorage.Unit != nil {
					f0f0.Unit = svcsdktypes.DataStorageUnit(*desired.ko.Spec.CacheUsageLimits.DataStorage.Unit)
				}
				f0.DataStorage = f0f0
			}
			if desired.ko.Spec.CacheUsageLimits.ECPUPerSecond != nil {
				f0f1 := &svcsdktypes.ECPUPerSecond{}
				if desired.ko.Spec.CacheUsageLimits.ECPUPerSecond.Maximum != nil {
					maximumCopy0 := *desired.ko.Spec.CacheUsageLimits.ECPUPerSecond.Maximum
					maximumCopy := int32(maximumCopy0)
					f0f1.Maximum = &maximumCopy
				}
				if desired.ko.Spec.CacheUsageLimits.ECPUPerSecond.Minimum != nil {
					minimumCopy0 := *desired.ko.Spec.CacheUsageLimits.ECPUPerSecond.Minimum
					minimumCopy := int32(minimumCopy0)
					f0f1.Minimum = &minimumCopy
				}
				f0.ECPUPerSecond = f0f1
			}
			input.CacheUsageLimits = f0
		}
	})
}

// syncEngineAndVersion handles the special case of engine and version changes
func (rm *resourceManager) syncEngineAndVersion(
	ctx context.Context,
	desired *resource,
) error {
	return rm.modifyServerlessCache(ctx, desired.ko.Spec.ServerlessCacheName, func(input *svcsdk.ModifyServerlessCacheInput) {
		input.Engine = desired.ko.Spec.Engine
		input.MajorEngineVersion = desired.ko.Spec.MajorEngineVersion
	})
}

func (rm *resourceManager) syncTags(
	ctx context.Context,
	desired *resource,
	latest *resource,
) (err error) {
	return util.SyncTags(ctx, desired.ko.Spec.Tags, latest.ko.Spec.Tags, latest.ko.Status.ACKResourceMetadata, convertToOrderedACKTags, rm.sdkapi, rm.metrics)
}

func (rm *resourceManager) getTags(
	ctx context.Context,
	resourceARN string,
) ([]*svcapitypes.Tag, error) {
	return util.GetTags(ctx, rm.sdkapi, rm.metrics, resourceARN)
}
