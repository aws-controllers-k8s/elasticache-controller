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

package util

import (
	"context"
	"errors"

	ackv1alpha1 "github.com/aws-controllers-k8s/runtime/apis/core/v1alpha1"
	ackcompare "github.com/aws-controllers-k8s/runtime/pkg/compare"
	"github.com/aws-controllers-k8s/runtime/pkg/metrics"
	ackrequeue "github.com/aws-controllers-k8s/runtime/pkg/requeue"
	ackrtlog "github.com/aws-controllers-k8s/runtime/pkg/runtime/log"
	acktags "github.com/aws-controllers-k8s/runtime/pkg/tags"
	svcsdk "github.com/aws/aws-sdk-go-v2/service/elasticache"
	svcsdktypes "github.com/aws/aws-sdk-go-v2/service/elasticache/types"

	svcapitypes "github.com/aws-controllers-k8s/elasticache-controller/apis/v1alpha1"
)

var requeueWaitWhileTagUpdated = ackrequeue.NeededAfter(
	errors.New("tags Update is in progress"),
	ackrequeue.DefaultRequeueAfterDuration,
)

// GetTags retrieves the resource's associated tags.
func GetTags(
	ctx context.Context,
	sdkapi *svcsdk.Client,
	metrics *metrics.Metrics,
	resourceARN string,
) ([]*svcapitypes.Tag, error) {
	resp, err := sdkapi.ListTagsForResource(
		ctx,
		&svcsdk.ListTagsForResourceInput{
			ResourceName: &resourceARN,
		},
	)
	metrics.RecordAPICall("GET", "ListTagsForResource", err)
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

// ConvertToOrderedACKTags converts a list of Tag objects to a map of key/value pairs
// and a slice of keys in the order they appeared in the original list
func ConvertToOrderedACKTags(
	tags []*svcapitypes.Tag,
) (acktags.Tags, []string) {
	if len(tags) == 0 {
		return acktags.Tags{}, []string{}
	}
	res := acktags.Tags{}
	order := []string{}
	for _, t := range tags {
		if t.Key != nil && t.Value != nil {
			res[*t.Key] = *t.Value
			order = append(order, *t.Key)
		}
	}
	return res, order
}

// SyncTags keeps the resource's tags in sync
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
//     `ResourceName`, but actually expects an ARN, not the cache cluster
//     name.  This is the same for the `AddTagsToResource` and
//     `RemoveTagsFromResource` input shapes. For the output shape, the field is
//     called `TagList` instead of `Tags` but is otherwise the same struct with
//     a `Key` and `Value` member field.
func SyncTags(
	ctx context.Context,
	desiredTags []*svcapitypes.Tag,
	latestTags []*svcapitypes.Tag,
	latestACKResourceMetadata *ackv1alpha1.ResourceMetadata,
	toACKTags func(tags []*svcapitypes.Tag) (acktags.Tags, []string),
	sdkapi *svcsdk.Client,
	metrics *metrics.Metrics,
) (err error) {
	rlog := ackrtlog.FromContext(ctx)
	exit := rlog.Trace("rm.syncTags")
	defer func() { exit(err) }()

	arn := (*string)(latestACKResourceMetadata.ARN)

	from, _ := toACKTags(latestTags)
	to, _ := toACKTags(desiredTags)

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

	// Modify tags causing the cache cluster to be modified and become unavailable temporarily
	// so after adding or removing tags, we have to wait for the cache cluster to be available again
	// process: add tags -> requeue -> remove tags -> requeue -> other update
	if len(added) > 0 {
		toAdd := make([]svcsdktypes.Tag, 0, len(added))
		for key, val := range added {
			key, val := key, val
			toAdd = append(toAdd, svcsdktypes.Tag{
				Key:   &key,
				Value: &val,
			})
		}

		rlog.Debug("adding tags to cache cluster", "tags", added)
		_, err = sdkapi.AddTagsToResource(
			ctx,
			&svcsdk.AddTagsToResourceInput{
				ResourceName: arn,
				Tags:         toAdd,
			},
		)
		metrics.RecordAPICall("UPDATE", "AddTagsToResource", err)
		if err != nil {
			return err
		}
	} else if len(removed) > 0 {
		toRemove := make([]string, 0, len(removed))
		for key := range removed {
			key := key
			toRemove = append(toRemove, key)
		}
		rlog.Debug("removing tags from cache cluster", "tags", removed)
		_, err = sdkapi.RemoveTagsFromResource(
			ctx,
			&svcsdk.RemoveTagsFromResourceInput{
				ResourceName: arn,
				TagKeys:      toRemove,
			},
		)
		metrics.RecordAPICall("UPDATE", "RemoveTagsFromResource", err)
		if err != nil {
			return err
		}
	}

	return requeueWaitWhileTagUpdated
}
