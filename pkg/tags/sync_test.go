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
	"github.com/aws/aws-sdk-go/service/elasticache"
	"github.com/aws/aws-sdk-go/service/elasticache/elasticacheiface"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

type mockElastiCacheClient struct {
	elasticacheiface.ElastiCacheAPI
	mock.Mock
}

func (m *mockElastiCacheClient) ListTagsForResourceWithContext(ctx context.Context, input *elasticache.ListTagsForResourceInput, opts ...interface{}) (*elasticache.TagListMessage, error) {
	args := m.Called(ctx, input)
	return args.Get(0).(*elasticache.TagListMessage), args.Error(1)
}

func (m *mockElastiCacheClient) AddTagsToResourceWithContext(ctx context.Context, input *elasticache.AddTagsToResourceInput, opts ...interface{}) (*elasticache.TagListMessage, error) {
	args := m.Called(ctx, input)
	return args.Get(0).(*elasticache.TagListMessage), args.Error(1)
}

func (m *mockElastiCacheClient) RemoveTagsFromResourceWithContext(ctx context.Context, input *elasticache.RemoveTagsFromResourceInput, opts ...interface{}) (*elasticache.TagListMessage, error) {
	args := m.Called(ctx, input)
	return args.Get(0).(*elasticache.TagListMessage), args.Error(1)
}

func TestGetTags(t *testing.T) {
	mockClient := &mockElastiCacheClient{}
	tm := NewTagManager(mockClient)
	ctx := context.Background()
	resourceARN := "arn:aws:elasticache:us-west-2:123456789012:securitygroup:my-cache-sec-group"

	expectedTags := []*elasticache.Tag{
		{
			Key:   aws.String("key1"),
			Value: aws.String("value1"),
		},
		{
			Key:   aws.String("key2"),
			Value: aws.String("value2"),
		},
	}

	mockClient.On("ListTagsForResourceWithContext", ctx, &elasticache.ListTagsForResourceInput{
		ResourceName: aws.String(resourceARN),
	}).Return(&elasticache.TagListMessage{
		TagList: expectedTags,
	}, nil)

	tags, err := tm.GetTags(ctx, resourceARN)

	assert.NoError(t, err)
	assert.Equal(t, expectedTags, tags)
	mockClient.AssertExpectations(t)
}

func TestSyncTags(t *testing.T) {
	mockClient := &mockElastiCacheClient{}
	tm := NewTagManager(mockClient)
	ctx := context.Background()
	resourceARN := "arn:aws:elasticache:us-west-2:123456789012:securitygroup:my-cache-sec-group"

	// Test adding tags
	desiredTags := []*elasticache.Tag{
		{
			Key:   aws.String("key1"),
			Value: aws.String("value1"),
		},
		{
			Key:   aws.String("key2"),
			Value: aws.String("value2"),
		},
	}
	latestTags := []*elasticache.Tag{}

	mockClient.On("AddTagsToResourceWithContext", ctx, &elasticache.AddTagsToResourceInput{
		ResourceName: aws.String(resourceARN),
		Tags:         desiredTags,
	}).Return(&elasticache.TagListMessage{}, nil)

	err := tm.SyncTags(ctx, resourceARN, desiredTags, latestTags)
	assert.NoError(t, err)

	// Test removing tags
	desiredTags = []*elasticache.Tag{}
	latestTags = []*elasticache.Tag{
		{
			Key:   aws.String("key1"),
			Value: aws.String("value1"),
		},
		{
			Key:   aws.String("key2"),
			Value: aws.String("value2"),
		},
	}

	mockClient.On("RemoveTagsFromResourceWithContext", ctx, &elasticache.RemoveTagsFromResourceInput{
		ResourceName: aws.String(resourceARN),
		TagKeys:      []*string{aws.String("key1"), aws.String("key2")},
	}).Return(&elasticache.TagListMessage{}, nil)

	err = tm.SyncTags(ctx, resourceARN, desiredTags, latestTags)
	assert.NoError(t, err)

	// Test updating tags
	desiredTags = []*elasticache.Tag{
		{
			Key:   aws.String("key1"),
			Value: aws.String("newvalue1"),
		},
		{
			Key:   aws.String("key3"),
			Value: aws.String("value3"),
		},
	}
	latestTags = []*elasticache.Tag{
		{
			Key:   aws.String("key1"),
			Value: aws.String("value1"),
		},
		{
			Key:   aws.String("key2"),
			Value: aws.String("value2"),
		},
	}

	mockClient.On("RemoveTagsFromResourceWithContext", ctx, &elasticache.RemoveTagsFromResourceInput{
		ResourceName: aws.String(resourceARN),
		TagKeys:      []*string{aws.String("key2")},
	}).Return(&elasticache.TagListMessage{}, nil)

	mockClient.On("AddTagsToResourceWithContext", ctx, &elasticache.AddTagsToResourceInput{
		ResourceName: aws.String(resourceARN),
		Tags: []*elasticache.Tag{
			{
				Key:   aws.String("key1"),
				Value: aws.String("newvalue1"),
			},
			{
				Key:   aws.String("key3"),
				Value: aws.String("value3"),
			},
		},
	}).Return(&elasticache.TagListMessage{}, nil)

	err = tm.SyncTags(ctx, resourceARN, desiredTags, latestTags)
	assert.NoError(t, err)
	mockClient.AssertExpectations(t)
}

func TestCompareElasticacheTags(t *testing.T) {
	tagsA := []*elasticache.Tag{
		{
			Key:   aws.String("key1"),
			Value: aws.String("value1"),
		},
		{
			Key:   aws.String("key2"),
			Value: aws.String("value2"),
		},
	}

	tagsB := []*elasticache.Tag{
		{
			Key:   aws.String("key1"),
			Value: aws.String("value1"),
		},
		{
			Key:   aws.String("key2"),
			Value: aws.String("value2"),
		},
	}

	tagsC := []*elasticache.Tag{
		{
			Key:   aws.String("key1"),
			Value: aws.String("different"),
		},
		{
			Key:   aws.String("key2"),
			Value: aws.String("value2"),
		},
	}

	assert.True(t, CompareElasticacheTags(tagsA, tagsB))
	assert.False(t, CompareElasticacheTags(tagsA, tagsC))
}
