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

	svcapitypes "github.com/aws-controllers-k8s/elasticache-controller/apis/v1alpha1"
	ackcompare "github.com/aws-controllers-k8s/runtime/pkg/compare"
	acktypes "github.com/aws-controllers-k8s/runtime/pkg/types"

	"github.com/aws-controllers-k8s/elasticache-controller/pkg/tags"
)

// Implement the TagsAccessor interface
func (r *resource) GetTags() []*svcapitypes.Tag {
	return r.ko.Spec.Tags
}

// getTags retrieves the tags for a given resource ARN
func (rm *resourceManager) getTags(ctx context.Context, resourceARN string) ([]*svcapitypes.Tag, error) {
	tagsManager := tags.NewTagsManager(rm.sdkapi)
	return tagsManager.GetTags(ctx, resourceARN)
}

// syncTags synchronizes tags between the spec and the resource
func (rm *resourceManager) syncTags(
	ctx context.Context,
	latest acktypes.AWSResource,
	desired acktypes.AWSResource,
) error {
	tagsManager := tags.NewTagsManager(rm.sdkapi)
	return tagsManager.SyncTags(ctx, desired, latest)
}

// tagsEqual compares two sets of tags for equality
func (rm *resourceManager) tagsEqual(a, b []*svcapitypes.Tag) bool {
	tagsManager := tags.NewTagsManager(rm.sdkapi)
	return tagsManager.TagsEqual(a, b)
}

// compareTags is a custom comparison function for tags
func (rm *resourceManager) compareTags(
	delta *ackcompare.Delta,
	a *resource,
	b *resource,
) {
	if len(a.ko.Spec.Tags) != len(b.ko.Spec.Tags) {
		delta.Add("Spec.Tags", a.ko.Spec.Tags, b.ko.Spec.Tags)
		return
	}

	if !rm.tagsEqual(a.ko.Spec.Tags, b.ko.Spec.Tags) {
		delta.Add("Spec.Tags", a.ko.Spec.Tags, b.ko.Spec.Tags)
	}
}
