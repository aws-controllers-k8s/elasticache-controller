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
	"errors"

	ackcompare "github.com/aws-controllers-k8s/runtime/pkg/compare"
	ackrequeue "github.com/aws-controllers-k8s/runtime/pkg/requeue"
	ackrtlog "github.com/aws-controllers-k8s/runtime/pkg/runtime/log"
	svcsdk "github.com/aws/aws-sdk-go/service/elasticache"

	svcapitypes "github.com/aws-controllers-k8s/elasticache-controller/apis/v1alpha1"
)

var (
	condMsgCurrentlyDeleting      string = "replication group currently being deleted."
	condMsgNoDeleteWhileModifying string = "replication group currently being modified. cannot delete."
	condMsgTerminalCreateFailed   string = "replication group in create-failed status."
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
	return status == "deleting"
}

// isModifying returns true if supplied replication group resource state is 'modifying'
func isModifying(r *resource) bool {
	if r == nil || r.ko.Status.Status == nil {
		return false
	}
	status := *r.ko.Status.Status
	return status == "modifying"
}

// isCreateFailed returns true if supplied replication group resource state is
// 'create-failed'
func isCreateFailed(r *resource) bool {
	if r == nil || r.ko.Status.Status == nil {
		return false
	}
	status := *r.ko.Status.Status
	return status == "create-failed"
}

// getTags retrieves the resource's associated tags
func (rm *resourceManager) getTags(
	ctx context.Context,
	resourceARN string,
) ([]*svcapitypes.Tag, error) {
	resp, err := rm.sdkapi.ListTagsForResourceWithContext(
		ctx,
		&svcsdk.ListTagsForResourceInput{
			ResourceName: &resourceARN,
		},
	)
	rm.metrics.RecordAPICall("GET", "ListTagsForResource", err)
	if err != nil {
		return nil, err
	}
	tags := make([]*svcapitypes.Tag, 0, len(resp.TagList))
	for _, tag := range resp.TagList {
		tags = append(tags, &svcapitypes.Tag{
			Key:   tag.Key,
			Value: tag.Value,
		})
	}
	return tags, nil
}

// syncTags keeps the resource's tags in sync
//
// NOTE(jaypipes): Elasticache's Tagging APIs differ from other AWS APIs in the
// following ways:
//
//  1. The names of the tagging API operations are different. Other APIs use the
//     Tagris `ListTagsForResource`, `TagResource` and `UntagResource` API
//     calls. RDS uses `ListTagsForResource`, `AddTagsToResource` and
//     `RemoveTagsFromResource`.
//
//  2. Even though the name of the `ListTagsForResource` API call is the same,
//     the structure of the input and the output are different from other APIs.
//     For the input, instead of a `ResourceArn` field, Elasticache names the field
//     `ResourceName`, but actually expects an ARN, not the replication group
//     name.  This is the same for the `AddTagsToResource` and
//     `RemoveTagsFromResource` input shapes. For the output shape, the field is
//     called `TagList` instead of `Tags` but is otherwise the same struct with
//     a `Key` and `Value` member field.
func (rm *resourceManager) syncTags(
	ctx context.Context,
	desired *resource,
	latest *resource,
) (err error) {
	rlog := ackrtlog.FromContext(ctx)
	exit := rlog.Trace("rm.syncTags")
	defer func() { exit(err) }()

	arn := (*string)(latest.ko.Status.ACKResourceMetadata.ARN)

	from := ToACKTags(latest.ko.Spec.Tags)
	to := ToACKTags(desired.ko.Spec.Tags)

	added, _, removed := ackcompare.GetTagsDifference(from, to)

	// NOTE(jaypipes): According to the elasticache API documentation, adding a tag
	// with a new value overwrites any existing tag with the same key. So, we
	// don't need to do anything to "update" a Tag. Simply including it in the
	// AddTagsToResource call is enough.
	for key := range removed {
		if _, ok := added[key]; ok {
			delete(removed, key)
		}
	}

	if len(removed) > 0 {
		toRemove := make([]*string, 0, len(removed))
		for key := range removed {
			key := key
			toRemove = append(toRemove, &key)
		}
		rlog.Debug("removing tags from replication group", "tags", removed)
		_, err = rm.sdkapi.RemoveTagsFromResourceWithContext(
			ctx,
			&svcsdk.RemoveTagsFromResourceInput{
				ResourceName: arn,
				TagKeys:      toRemove,
			},
		)
		rm.metrics.RecordAPICall("UPDATE", "RemoveTagsFromResource", err)
		if err != nil {
			return err
		}
	}

	if len(added) > 0 {
		toAdd := make([]*svcsdk.Tag, 0, len(added))
		for key, val := range added {
			key, val := key, val
			toAdd = append(toAdd, &svcsdk.Tag{
				Key:   &key,
				Value: &val,
			})
		}

		rlog.Debug("adding tags to replication group", "tags", added)
		_, err = rm.sdkapi.AddTagsToResourceWithContext(
			ctx,
			&svcsdk.AddTagsToResourceInput{
				ResourceName: arn,
				Tags:         toAdd,
			},
		)
		rm.metrics.RecordAPICall("UPDATE", "AddTagsToResource", err)
		if err != nil {
			return err
		}
	}
	return nil
}
