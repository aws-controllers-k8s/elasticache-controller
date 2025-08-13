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

	"github.com/aws/aws-sdk-go/aws"
	svcsdk "github.com/aws/aws-sdk-go/service/elasticache"
)

// TagType represents the type definition for an AWS Tag
type TagType struct {
	Key   *string
	Value *string
}

// TagsToSDKTags converts a map[string]*string to a slice of elasticache.Tag
func TagsToSDKTags(tags map[string]*string) []*svcsdk.Tag {
	sdkTags := make([]*svcsdk.Tag, 0, len(tags))
	for k, v := range tags {
		sdkTags = append(sdkTags, &svcsdk.Tag{
			Key:   aws.String(k),
			Value: v,
		})
	}
	return sdkTags
}

// SDKTagsToTags converts a slice of elasticache.Tag to a map[string]*string
func SDKTagsToTags(sdkTags []*svcsdk.Tag) map[string]*string {
	tags := make(map[string]*string, len(sdkTags))
	for _, tag := range sdkTags {
		tags[aws.StringValue(tag.Key)] = tag.Value
	}
	return tags
}

// GetTags returns the tags for a given resource ARN
func GetTags(
	ctx context.Context,
	resourceARN string,
	sdkapi SDKAPI,
) (map[string]*string, error) {
	input := &svcsdk.ListTagsForResourceInput{
		ResourceName: aws.String(resourceARN),
	}
	resp, err := sdkapi.ListTagsForResourceWithContext(ctx, input)
	if err != nil {
		return nil, err
	}
	return SDKTagsToTags(resp.TagList), nil
}

// SyncTags synchronizes the tags between the resource spec and the AWS resource
func SyncTags(
	ctx context.Context,
	desired map[string]*string,
	latest map[string]*string,
	resourceARN string,
	sdkapi SDKAPI,
) error {
	// Find tags to add or update
	tagsToAdd := make(map[string]*string)
	for k, v := range desired {
		if latestVal, exists := latest[k]; !exists || aws.StringValue(latestVal) != aws.StringValue(v) {
			tagsToAdd[k] = v
		}
	}

	// Find tags to remove
	tagsToRemove := make([]string, 0)
	for k := range latest {
		if _, exists := desired[k]; !exists {
			tagsToRemove = append(tagsToRemove, k)
		}
	}

	// Handle additions and updates
	if len(tagsToAdd) > 0 {
		addInput := &svcsdk.AddTagsToResourceInput{
			ResourceName: aws.String(resourceARN),
			Tags:         TagsToSDKTags(tagsToAdd),
		}
		_, err := sdkapi.AddTagsToResourceWithContext(ctx, addInput)
		if err != nil {
			return err
		}
	}

	// Handle removals
	if len(tagsToRemove) > 0 {
		tagKeys := make([]*string, 0, len(tagsToRemove))
		for _, k := range tagsToRemove {
			tagKeys = append(tagKeys, aws.String(k))
		}
		removeInput := &svcsdk.RemoveTagsFromResourceInput{
			ResourceName: aws.String(resourceARN),
			TagKeys:      tagKeys,
		}
		_, err := sdkapi.RemoveTagsFromResourceWithContext(ctx, removeInput)
		if err != nil {
			return err
		}
	}

	return nil
}

// SDKAPI represents the subset of elasticache API used for tag operations
type SDKAPI interface {
	ListTagsForResourceWithContext(context.Context, *svcsdk.ListTagsForResourceInput, ...interface{}) (*svcsdk.ListTagsForResourceOutput, error)
	AddTagsToResourceWithContext(context.Context, *svcsdk.AddTagsToResourceInput, ...interface{}) (*svcsdk.AddTagsToResourceOutput, error)
	RemoveTagsFromResourceWithContext(context.Context, *svcsdk.RemoveTagsFromResourceInput, ...interface{}) (*svcsdk.RemoveTagsFromResourceOutput, error)
}
