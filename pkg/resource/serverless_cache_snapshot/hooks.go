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

package serverless_cache_snapshot

import (
	"context"
	"errors"

	"github.com/aws-controllers-k8s/elasticache-controller/pkg/util"
	ackrequeue "github.com/aws-controllers-k8s/runtime/pkg/requeue"
	svcsdk "github.com/aws/aws-sdk-go/service/elasticache"
)

var requeueWaitWhileTagUpdated = ackrequeue.NeededAfter(
	errors.New("tags update is in progress"),
	ackrequeue.DefaultRequeueAfterDuration,
)

// getTags retrieves the tags for a given ServerlessCacheSnapshot
func (rm *resourceManager) getTags(
	ctx context.Context,
	resourceARN string,
) []*svcsdk.Tag {
	tags, err := util.GetTags(ctx, rm.sdkapi, rm.metrics, resourceARN)
	if err != nil {
		return nil
	}

	// Convert from svcapitypes.Tag to svcsdk.Tag
	sdkTags := make([]*svcsdk.Tag, len(tags))
	for i, tag := range tags {
		sdkTags[i] = &svcsdk.Tag{
			Key:   tag.Key,
			Value: tag.Value,
		}
	}
	return sdkTags
}

// syncTags synchronizes the tags between the resource spec and the AWS resource
func (rm *resourceManager) syncTags(
	ctx context.Context,
	latest *resource,
	desired *resource,
) error {
	// If the ARN is not set, we can't sync tags
	if latest.ko.Status.ACKResourceMetadata == nil || latest.ko.Status.ACKResourceMetadata.ARN == nil {
		return nil
	}

	return util.SyncTags(
		ctx,
		desired.ko.Spec.Tags,
		latest.ko.Spec.Tags,
		latest.ko.Status.ACKResourceMetadata,
		convertToOrderedACKTags,
		rm.sdkapi,
		rm.metrics,
	)
}
