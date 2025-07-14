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

package tags

import (
	"context"
	"fmt"

	acktypes "github.com/aws-controllers-k8s/runtime/pkg/types"
	"github.com/aws/aws-sdk-go-v2/aws"
	svcsdk "github.com/aws/aws-sdk-go-v2/service/elasticache"
	svcsdktypes "github.com/aws/aws-sdk-go-v2/service/elasticache/types"

	svcapitypes "github.com/aws-controllers-k8s/elasticache-controller/apis/v1alpha1"
)

// TagsAccessor is an interface for accessing tags on a resource
type TagsAccessor interface {
	// GetTags returns the tags on the resource
	GetTags() []*svcapitypes.Tag
}

// TagsManager is an interface for managing tags for ElastiCache resources
type TagsManager interface {
	// GetTags returns the tags for a resource ARN
	GetTags(ctx context.Context, resourceARN string) ([]*svcapitypes.Tag, error)

	// SyncTags synchronizes tags between the spec and the resource
	SyncTags(ctx context.Context, desired acktypes.AWSResource, latest acktypes.AWSResource) error

	// TagsEqual compares two sets of tags for equality
	TagsEqual(a, b []*svcapitypes.Tag) bool
}

// tagsManager implements the TagsManager interface
type tagsManager struct {
	sdkapi SDKAPI
}

// SDKAPI is the interface for ElastiCache SDK API operations related to tags
type SDKAPI interface {
	ListTagsForResource(context.Context, *svcsdk.ListTagsForResourceInput, ...func(*svcsdk.Options)) (*svcsdk.ListTagsForResourceOutput, error)
	AddTagsToResource(context.Context, *svcsdk.AddTagsToResourceInput, ...func(*svcsdk.Options)) (*svcsdk.AddTagsToResourceOutput, error)
	RemoveTagsFromResource(context.Context, *svcsdk.RemoveTagsFromResourceInput, ...func(*svcsdk.Options)) (*svcsdk.RemoveTagsFromResourceOutput, error)
}

// NewTagsManager returns a new TagsManager for ElastiCache resources
func NewTagsManager(sdkapi SDKAPI) TagsManager {
	return &tagsManager{sdkapi: sdkapi}
}

// GetTags returns the tags for a resource ARN
func (tm *tagsManager) GetTags(ctx context.Context, resourceARN string) ([]*svcapitypes.Tag, error) {
	input := &svcsdk.ListTagsForResourceInput{
		ResourceName: aws.String(resourceARN),
	}

	resp, err := tm.sdkapi.ListTagsForResource(ctx, input)
	if err != nil {
		return nil, err
	}

	tags := make([]*svcapitypes.Tag, len(resp.TagList))
	for i, tag := range resp.TagList {
		tags[i] = &svcapitypes.Tag{
			Key:   tag.Key,
			Value: tag.Value,
		}
	}

	return tags, nil
}

// SyncTags synchronizes tags between the spec and the resource
func (tm *tagsManager) SyncTags(ctx context.Context, desired acktypes.AWSResource, latest acktypes.AWSResource) error {
	resourceARN := latest.Identifiers().ARN()
	if resourceARN == nil || *resourceARN == "" {
		return fmt.Errorf("resource ARN is nil or empty")
	}

	var desiredTags []*svcapitypes.Tag
	if desired != nil {
		// Get tags from the desired resource
		desiredAccessor, ok := desired.(TagsAccessor)
		if !ok {
			return fmt.Errorf("desired resource does not implement TagsAccessor")
		}
		desiredTags = desiredAccessor.GetTags()
	}

	var latestTags []*svcapitypes.Tag
	if latest != nil {
		// Get tags from the latest resource
		latestAccessor, ok := latest.(TagsAccessor)
		if !ok {
			return fmt.Errorf("latest resource does not implement TagsAccessor")
		}
		latestTags = latestAccessor.GetTags()
	}

	// If tags are equal, no need to sync
	if tm.TagsEqual(desiredTags, latestTags) {
		return nil
	}

	// Determine which tags to add and which to remove
	toAdd, toRemove := tm.diffTags(desiredTags, latestTags)

	// Add tags
	if len(toAdd) > 0 {
		svcTags := make([]svcsdktypes.Tag, len(toAdd))
		for i, tag := range toAdd {
			svcTags[i] = svcsdktypes.Tag{
				Key:   tag.Key,
				Value: tag.Value,
			}
		}

		_, err := tm.sdkapi.AddTagsToResource(ctx, &svcsdk.AddTagsToResourceInput{
			ResourceName: aws.String(string(*resourceARN)),
			Tags:         svcTags,
		})
		if err != nil {
			return err
		}
	}

	// Remove tags
	if len(toRemove) > 0 {
		tagKeys := make([]string, len(toRemove))
		for i, tag := range toRemove {
			tagKeys[i] = *tag.Key
		}

		_, err := tm.sdkapi.RemoveTagsFromResource(ctx, &svcsdk.RemoveTagsFromResourceInput{
			ResourceName: aws.String(string(*resourceARN)),
			TagKeys:      tagKeys,
		})
		if err != nil {
			return err
		}
	}

	return nil
}

// TagsEqual compares two sets of tags for equality
func (tm *tagsManager) TagsEqual(a, b []*svcapitypes.Tag) bool {
	if len(a) != len(b) {
		return false
	}

	aMap := make(map[string]string, len(a))
	for _, tag := range a {
		if tag.Key != nil {
			key := *tag.Key
			var val string
			if tag.Value != nil {
				val = *tag.Value
			}
			aMap[key] = val
		}
	}

	for _, tag := range b {
		if tag.Key == nil {
			continue
		}
		key := *tag.Key
		var val string
		if tag.Value != nil {
			val = *tag.Value
		}
		aVal, ok := aMap[key]
		if !ok || aVal != val {
			return false
		}
		delete(aMap, key)
	}

	return len(aMap) == 0
}

// diffTags returns the tags that need to be added and removed to transform the
// source tags into the target tags.
func (tm *tagsManager) diffTags(desired, latest []*svcapitypes.Tag) (toAdd, toRemove []*svcapitypes.Tag) {
	desiredMap := make(map[string]*svcapitypes.Tag)
	for _, tag := range desired {
		if tag.Key != nil {
			desiredMap[*tag.Key] = tag
		}
	}

	latestMap := make(map[string]*svcapitypes.Tag)
	for _, tag := range latest {
		if tag.Key != nil {
			latestMap[*tag.Key] = tag
		}
	}

	// Find tags to add or update
	for key, desiredTag := range desiredMap {
		latestTag, exists := latestMap[key]
		if !exists {
			// Tag doesn't exist, add it
			toAdd = append(toAdd, desiredTag)
		} else {
			// Check if values are different
			desiredValue := ""
			if desiredTag.Value != nil {
				desiredValue = *desiredTag.Value
			}

			latestValue := ""
			if latestTag.Value != nil {
				latestValue = *latestTag.Value
			}

			if desiredValue != latestValue {
				// Tag exists but value is different, update it
				toAdd = append(toAdd, desiredTag)
			}
		}
	}

	// Find tags to remove
	for key, latestTag := range latestMap {
		if _, exists := desiredMap[key]; !exists {
			toRemove = append(toRemove, latestTag)
		}
	}

	return toAdd, toRemove
}
