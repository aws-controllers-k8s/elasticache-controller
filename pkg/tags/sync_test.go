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
	"testing"

	"github.com/aws/aws-sdk-go/aws"
	svcsdk "github.com/aws/aws-sdk-go/service/elasticache"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

type mockSDKAPI struct {
	mock.Mock
}

func (m *mockSDKAPI) ListTagsForResourceWithContext(ctx context.Context, input *svcsdk.ListTagsForResourceInput, opts ...interface{}) (*svcsdk.ListTagsForResourceOutput, error) {
	args := m.Called(ctx, input)
	return args.Get(0).(*svcsdk.ListTagsForResourceOutput), args.Error(1)
}

func (m *mockSDKAPI) AddTagsToResourceWithContext(ctx context.Context, input *svcsdk.AddTagsToResourceInput, opts ...interface{}) (*svcsdk.AddTagsToResourceOutput, error) {
	args := m.Called(ctx, input)
	return args.Get(0).(*svcsdk.AddTagsToResourceOutput), args.Error(1)
}

func (m *mockSDKAPI) RemoveTagsFromResourceWithContext(ctx context.Context, input *svcsdk.RemoveTagsFromResourceInput, opts ...interface{}) (*svcsdk.RemoveTagsFromResourceOutput, error) {
	args := m.Called(ctx, input)
	return args.Get(0).(*svcsdk.RemoveTagsFromResourceOutput), args.Error(1)
}

func TestTagsToSDKTags(t *testing.T) {
	tags := map[string]*string{
		"key1": aws.String("value1"),
		"key2": aws.String("value2"),
	}

	sdkTags := TagsToSDKTags(tags)

	assert.Len(t, sdkTags, 2)

	// Convert back to map for easier comparison
	tagMap := make(map[string]string)
	for _, tag := range sdkTags {
		tagMap[*tag.Key] = *tag.Value
	}

	assert.Equal(t, "value1", tagMap["key1"])
	assert.Equal(t, "value2", tagMap["key2"])
}

func TestSDKTagsToTags(t *testing.T) {
	sdkTags := []*svcsdk.Tag{
		{
			Key:   aws.String("key1"),
			Value: aws.String("value1"),
		},
		{
			Key:   aws.String("key2"),
			Value: aws.String("value2"),
		},
	}

	tags := SDKTagsToTags(sdkTags)

	assert.Len(t, tags, 2)
	assert.Equal(t, "value1", *tags["key1"])
	assert.Equal(t, "value2", *tags["key2"])
}

func TestGetTags(t *testing.T) {
	ctx := context.Background()
	resourceARN := "arn:aws:elasticache:us-west-2:123456789012:snapshot:my-snapshot"

	mockAPI := new(mockSDKAPI)
	mockAPI.On("ListTagsForResourceWithContext", ctx, &svcsdk.ListTagsForResourceInput{
		ResourceName: aws.String(resourceARN),
	}).Return(&svcsdk.ListTagsForResourceOutput{
		TagList: []*svcsdk.Tag{
			{
				Key:   aws.String("key1"),
				Value: aws.String("value1"),
			},
		},
	}, nil)

	tags, err := GetTags(ctx, resourceARN, mockAPI)

	assert.NoError(t, err)
	assert.Len(t, tags, 1)
	assert.Equal(t, "value1", *tags["key1"])
	mockAPI.AssertExpectations(t)
}

func TestSyncTags(t *testing.T) {
	ctx := context.Background()
	resourceARN := "arn:aws:elasticache:us-west-2:123456789012:snapshot:my-snapshot"

	desired := map[string]*string{
		"key1": aws.String("new-value"),
		"key3": aws.String("value3"),
	}

	latest := map[string]*string{
		"key1": aws.String("value1"),
		"key2": aws.String("value2"),
	}

	mockAPI := new(mockSDKAPI)

	// Expect AddTagsToResource to be called with key1 (updated) and key3 (added)
	mockAPI.On("AddTagsToResourceWithContext", ctx, mock.MatchedBy(func(input *svcsdk.AddTagsToResourceInput) bool {
		if *input.ResourceName != resourceARN {
			return false
		}

		tagMap := make(map[string]string)
		for _, tag := range input.Tags {
			tagMap[*tag.Key] = *tag.Value
		}

		return len(input.Tags) == 2 &&
			tagMap["key1"] == "new-value" &&
			tagMap["key3"] == "value3"
	})).Return(&svcsdk.AddTagsToResourceOutput{}, nil)

	// Expect RemoveTagsFromResource to be called with key2 (removed)
	mockAPI.On("RemoveTagsFromResourceWithContext", ctx, mock.MatchedBy(func(input *svcsdk.RemoveTagsFromResourceInput) bool {
		if *input.ResourceName != resourceARN {
			return false
		}

		tagKeys := make([]string, 0, len(input.TagKeys))
		for _, key := range input.TagKeys {
			tagKeys = append(tagKeys, *key)
		}

		return len(input.TagKeys) == 1 && tagKeys[0] == "key2"
	})).Return(&svcsdk.RemoveTagsFromResourceOutput{}, nil)

	err := SyncTags(ctx, desired, latest, resourceARN, mockAPI)

	assert.NoError(t, err)
	mockAPI.AssertExpectations(t)
}
