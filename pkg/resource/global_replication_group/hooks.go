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

package global_replication_group

import (
	"context"

	svcapitypes "github.com/aws-controllers-k8s/elasticache-controller/apis/v1alpha1"
	"github.com/aws-controllers-k8s/elasticache-controller/pkg/util"
)

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
	return util.SyncTags(ctx, desired.ko.Spec.Tags, latest.ko.Spec.Tags, latest.ko.Status.ACKResourceMetadata, util.ConvertToOrderedACKTags, rm.sdkapi, rm.metrics)
}
