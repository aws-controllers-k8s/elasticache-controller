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

package user_group

import (
	"reflect"
	"testing"

	"github.com/aws/aws-sdk-go/aws"
)

func Test_getUserIdsDifferences(t *testing.T) {
	tests := []struct {
		name                string
		userIdsDesired      []*string
		userIdsLatest       []*string
		wantUserIdsToAdd    []string
		wantUserIdsToRemove []string
	}{
		{
			name:                "Additions and Removals",
			userIdsDesired:      aws.StringSlice([]string{"user1", "user2", "user3"}),
			userIdsLatest:       aws.StringSlice([]string{"user2", "user3", "user4"}),
			wantUserIdsToAdd:    []string{"user1"},
			wantUserIdsToRemove: []string{"user4"},
		},
		{
			name:                "Perfect Match",
			userIdsDesired:      aws.StringSlice([]string{"user1", "user2"}),
			userIdsLatest:       aws.StringSlice([]string{"user1", "user2"}),
			wantUserIdsToAdd:    []string{},
			wantUserIdsToRemove: []string{},
		},
		{
			name:                "All New Users",
			userIdsDesired:      aws.StringSlice([]string{"user1", "user2"}),
			userIdsLatest:       []*string{},
			wantUserIdsToAdd:    []string{"user1", "user2"},
			wantUserIdsToRemove: []string{},
		},
		{
			name:                "Remove All Users",
			userIdsDesired:      []*string{},
			userIdsLatest:       aws.StringSlice([]string{"user1", "user2"}),
			wantUserIdsToAdd:    []string{},
			wantUserIdsToRemove: []string{"user1", "user2"},
		},
		{
			name:                "Empty Inputs",
			userIdsDesired:      []*string{},
			userIdsLatest:       []*string{},
			wantUserIdsToAdd:    []string{},
			wantUserIdsToRemove: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotToAdd, gotToRemove := getUserIdsDifferences(tt.userIdsDesired, tt.userIdsLatest)

			if !reflect.DeepEqual(gotToAdd, tt.wantUserIdsToAdd) {
				t.Errorf("getUserIdsDifferences() gotToAdd = %v, want %v", gotToAdd, tt.wantUserIdsToAdd)
			}
			if !reflect.DeepEqual(gotToRemove, tt.wantUserIdsToRemove) {
				t.Errorf("getUserIdsDifferences() gotToRemove = %v, want %v", gotToRemove, tt.wantUserIdsToRemove)
			}
		})
	}
}
