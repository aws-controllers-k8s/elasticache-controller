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
	"testing"

	svcapitypes "github.com/aws-controllers-k8s/elasticache-controller/apis/v1alpha1"
	"github.com/aws/aws-sdk-go-v2/aws"
)

func TestTagsEqual(t *testing.T) {
	tm := &tagsManager{}

	tests := []struct {
		name string
		a    []*svcapitypes.Tag
		b    []*svcapitypes.Tag
		want bool
	}{
		{
			name: "both nil",
			a:    nil,
			b:    nil,
			want: true,
		},
		{
			name: "a nil",
			a:    nil,
			b:    []*svcapitypes.Tag{{Key: aws.String("key"), Value: aws.String("value")}},
			want: false,
		},
		{
			name: "b nil",
			a:    []*svcapitypes.Tag{{Key: aws.String("key"), Value: aws.String("value")}},
			b:    nil,
			want: false,
		},
		{
			name: "empty slices",
			a:    []*svcapitypes.Tag{},
			b:    []*svcapitypes.Tag{},
			want: true,
		},
		{
			name: "same tags",
			a: []*svcapitypes.Tag{
				{Key: aws.String("key1"), Value: aws.String("value1")},
				{Key: aws.String("key2"), Value: aws.String("value2")},
			},
			b: []*svcapitypes.Tag{
				{Key: aws.String("key1"), Value: aws.String("value1")},
				{Key: aws.String("key2"), Value: aws.String("value2")},
			},
			want: true,
		},
		{
			name: "same tags different order",
			a: []*svcapitypes.Tag{
				{Key: aws.String("key1"), Value: aws.String("value1")},
				{Key: aws.String("key2"), Value: aws.String("value2")},
			},
			b: []*svcapitypes.Tag{
				{Key: aws.String("key2"), Value: aws.String("value2")},
				{Key: aws.String("key1"), Value: aws.String("value1")},
			},
			want: true,
		},
		{
			name: "different values",
			a: []*svcapitypes.Tag{
				{Key: aws.String("key1"), Value: aws.String("value1")},
				{Key: aws.String("key2"), Value: aws.String("value2")},
			},
			b: []*svcapitypes.Tag{
				{Key: aws.String("key1"), Value: aws.String("value1")},
				{Key: aws.String("key2"), Value: aws.String("different")},
			},
			want: false,
		},
		{
			name: "different keys",
			a: []*svcapitypes.Tag{
				{Key: aws.String("key1"), Value: aws.String("value1")},
				{Key: aws.String("key2"), Value: aws.String("value2")},
			},
			b: []*svcapitypes.Tag{
				{Key: aws.String("key1"), Value: aws.String("value1")},
				{Key: aws.String("different"), Value: aws.String("value2")},
			},
			want: false,
		},
		{
			name: "a has more tags",
			a: []*svcapitypes.Tag{
				{Key: aws.String("key1"), Value: aws.String("value1")},
				{Key: aws.String("key2"), Value: aws.String("value2")},
				{Key: aws.String("key3"), Value: aws.String("value3")},
			},
			b: []*svcapitypes.Tag{
				{Key: aws.String("key1"), Value: aws.String("value1")},
				{Key: aws.String("key2"), Value: aws.String("value2")},
			},
			want: false,
		},
		{
			name: "b has more tags",
			a: []*svcapitypes.Tag{
				{Key: aws.String("key1"), Value: aws.String("value1")},
				{Key: aws.String("key2"), Value: aws.String("value2")},
			},
			b: []*svcapitypes.Tag{
				{Key: aws.String("key1"), Value: aws.String("value1")},
				{Key: aws.String("key2"), Value: aws.String("value2")},
				{Key: aws.String("key3"), Value: aws.String("value3")},
			},
			want: false,
		},
		{
			name: "nil values",
			a: []*svcapitypes.Tag{
				{Key: aws.String("key1"), Value: nil},
				{Key: aws.String("key2"), Value: aws.String("value2")},
			},
			b: []*svcapitypes.Tag{
				{Key: aws.String("key1"), Value: nil},
				{Key: aws.String("key2"), Value: aws.String("value2")},
			},
			want: true,
		},
		{
			name: "one nil value vs empty string",
			a: []*svcapitypes.Tag{
				{Key: aws.String("key1"), Value: nil},
				{Key: aws.String("key2"), Value: aws.String("value2")},
			},
			b: []*svcapitypes.Tag{
				{Key: aws.String("key1"), Value: aws.String("")},
				{Key: aws.String("key2"), Value: aws.String("value2")},
			},
			want: false,
		},
		{
			name: "nil keys are ignored",
			a: []*svcapitypes.Tag{
				{Key: nil, Value: aws.String("value1")},
				{Key: aws.String("key2"), Value: aws.String("value2")},
			},
			b: []*svcapitypes.Tag{
				{Key: aws.String("key2"), Value: aws.String("value2")},
			},
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tm.TagsEqual(tt.a, tt.b); got != tt.want {
				t.Errorf("TagsEqual() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestDiffTags(t *testing.T) {
	tm := &tagsManager{}

	tests := []struct {
		name    string
		desired []*svcapitypes.Tag
		latest  []*svcapitypes.Tag
		wantAdd int
		wantDel int
	}{
		{
			name:    "both nil",
			desired: nil,
			latest:  nil,
			wantAdd: 0,
			wantDel: 0,
		},
		{
			name:    "add all tags",
			desired: []*svcapitypes.Tag{{Key: aws.String("key1"), Value: aws.String("value1")}},
			latest:  nil,
			wantAdd: 1,
			wantDel: 0,
		},
		{
			name:    "remove all tags",
			desired: nil,
			latest:  []*svcapitypes.Tag{{Key: aws.String("key1"), Value: aws.String("value1")}},
			wantAdd: 0,
			wantDel: 1,
		},
		{
			name: "update tag value",
			desired: []*svcapitypes.Tag{
				{Key: aws.String("key1"), Value: aws.String("newvalue")},
			},
			latest: []*svcapitypes.Tag{
				{Key: aws.String("key1"), Value: aws.String("oldvalue")},
			},
			wantAdd: 1,
			wantDel: 0,
		},
		{
			name: "add and remove tags",
			desired: []*svcapitypes.Tag{
				{Key: aws.String("key1"), Value: aws.String("value1")},
				{Key: aws.String("key3"), Value: aws.String("value3")},
			},
			latest: []*svcapitypes.Tag{
				{Key: aws.String("key1"), Value: aws.String("value1")},
				{Key: aws.String("key2"), Value: aws.String("value2")},
			},
			wantAdd: 1,
			wantDel: 1,
		},
		{
			name: "no changes",
			desired: []*svcapitypes.Tag{
				{Key: aws.String("key1"), Value: aws.String("value1")},
				{Key: aws.String("key2"), Value: aws.String("value2")},
			},
			latest: []*svcapitypes.Tag{
				{Key: aws.String("key1"), Value: aws.String("value1")},
				{Key: aws.String("key2"), Value: aws.String("value2")},
			},
			wantAdd: 0,
			wantDel: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			toAdd, toRemove := tm.diffTags(tt.desired, tt.latest)
			if len(toAdd) != tt.wantAdd {
				t.Errorf("diffTags() toAdd = %v, want %v", len(toAdd), tt.wantAdd)
			}
			if len(toRemove) != tt.wantDel {
				t.Errorf("diffTags() toRemove = %v, want %v", len(toRemove), tt.wantDel)
			}
		})
	}
}
