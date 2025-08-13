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

	svcapitypes "github.com/aws-controllers-k8s/elasticache-controller/apis/v1alpha1"
	"github.com/aws/aws-sdk-go-v2/service/elasticache"
	elasticachetypes "github.com/aws/aws-sdk-go-v2/service/elasticache/types"
)

// TagManager handles the tagging operations for ElastiCache resources
type TagManager struct {
	client *elasticache.Client
}

// NewTagManager creates a new TagManager instance
func NewTagManager(client *elasticache.Client) *TagManager {
	return &TagManager{
		client: client,
	}
}

// GetTags returns the tags for a given resource ARN
func (tm *TagManager) GetTags(
	ctx context.Context,
	resourceARN string,
) ([]*svcapitypes.Tag, error) {
	input := &elasticache.ListTagsForResourceInput{
		ResourceName: &resourceARN,
	}
	resp, err := tm.client.ListTagsForResource(ctx, input)
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

// SyncTags synchronizes the tags between the desired resource and the latest resource
func (tm *TagManager) SyncTags(
	ctx context.Context,
	resourceARN string,
	desired []*svcapitypes.Tag,
	latest []*svcapitypes.Tag,
) error {
	// Calculate the tags to add and remove
	added, removed := diffTagSets(desired, latest)

	if len(removed) > 0 {
		tagKeys := make([]string, 0, len(removed))
		for _, tag := range removed {
			tagKeys = append(tagKeys, *tag.Key)
		}
		input := &elasticache.RemoveTagsFromResourceInput{
			ResourceName: &resourceARN,
			TagKeys:      tagKeys,
		}
		_, err := tm.client.RemoveTagsFromResource(ctx, input)
		if err != nil {
			return err
		}
	}

	if len(added) > 0 {
		// Convert to SDK tags
		sdkTags := make([]elasticachetypes.Tag, len(added))
		for i, tag := range added {
			sdkTags[i] = elasticachetypes.Tag{
				Key:   tag.Key,
				Value: tag.Value,
			}
		}

		input := &elasticache.AddTagsToResourceInput{
			ResourceName: &resourceARN,
			Tags:         sdkTags,
		}
		_, err := tm.client.AddTagsToResource(ctx, input)
		if err != nil {
			return err
		}
	}

	return nil
}

// diffTagSets returns the tags that need to be added and removed to transform the
// latest set of tags into the desired set of tags
func diffTagSets(
	desired []*svcapitypes.Tag,
	latest []*svcapitypes.Tag,
) ([]*svcapitypes.Tag, []*svcapitypes.Tag) {
	desiredTags := make(map[string]string)
	for _, tag := range desired {
		if tag.Key != nil && tag.Value != nil {
			desiredTags[*tag.Key] = *tag.Value
		}
	}

	latestTags := make(map[string]string)
	for _, tag := range latest {
		if tag.Key != nil && tag.Value != nil {
			latestTags[*tag.Key] = *tag.Value
		}
	}

	var added []*svcapitypes.Tag
	var removed []*svcapitypes.Tag

	// Find tags to add or update
	for key, value := range desiredTags {
		latestValue, exists := latestTags[key]
		if !exists || latestValue != value {
			k, v := key, value // Create local copies to avoid reference issues
			added = append(added, &svcapitypes.Tag{
				Key:   &k,
				Value: &v,
			})
		}
	}

	// Find tags to remove
	for key := range latestTags {
		_, exists := desiredTags[key]
		if !exists {
			k := key // Create local copy to avoid reference issues
			removed = append(removed, &svcapitypes.Tag{
				Key: &k,
			})
		}
	}

	return added, removed
}

// CompareElasticacheTags compares two sets of elasticache tags
func CompareElasticacheTags(
	a []*svcapitypes.Tag,
	b []*svcapitypes.Tag,
) bool {
	aMap := elasticacheTagsToMap(a)
	bMap := elasticacheTagsToMap(b)

	if len(aMap) != len(bMap) {
		return false
	}

	for k, v := range aMap {
		if bMap[k] != v {
			return false
		}
	}

	return true
}

// elasticacheTagsToMap converts a slice of elasticache.Tag to a map of key-value pairs
func elasticacheTagsToMap(
	tags []*svcapitypes.Tag,
) map[string]string {
	result := make(map[string]string, len(tags))
	for _, tag := range tags {
		if tag.Key != nil && tag.Value != nil {
			result[*tag.Key] = *tag.Value
		}
	}
	return result
}
