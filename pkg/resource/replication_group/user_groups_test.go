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

package replication_group

import (
	"reflect"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"

	svcapitypes "github.com/aws-controllers-k8s/elasticache-controller/apis/v1alpha1"
)

func TestUserGroupIDsDifference(t *testing.T) {
	tests := []struct {
		name       string
		desired    []*string
		latest     []*string
		wantAdd    []string
		wantRemove []string
	}{
		{
			name:       "a group added",
			desired:    aws.StringSlice([]string{"ug-a", "ug-b"}),
			latest:     aws.StringSlice([]string{"ug-a"}),
			wantAdd:    []string{"ug-b"},
			wantRemove: []string{},
		},
		{
			name:       "a group removed",
			desired:    aws.StringSlice([]string{"ug-a"}),
			latest:     aws.StringSlice([]string{"ug-a", "ug-b"}),
			wantAdd:    []string{},
			wantRemove: []string{"ug-b"},
		},
		{
			name:       "a group replaced",
			desired:    aws.StringSlice([]string{"ug-b"}),
			latest:     aws.StringSlice([]string{"ug-a"}),
			wantAdd:    []string{"ug-b"},
			wantRemove: []string{"ug-a"},
		},
		{
			name:       "same groups in another order are not re-sent",
			desired:    aws.StringSlice([]string{"ug-b", "ug-a"}),
			latest:     aws.StringSlice([]string{"ug-a", "ug-b"}),
			wantAdd:    []string{},
			wantRemove: []string{},
		},
		{
			name:       "all groups removed",
			desired:    nil,
			latest:     aws.StringSlice([]string{"ug-a"}),
			wantAdd:    []string{},
			wantRemove: []string{"ug-a"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotAdd, gotRemove := userGroupIDsDifference(tt.desired, tt.latest)
			if !reflect.DeepEqual(gotAdd, tt.wantAdd) || !reflect.DeepEqual(gotRemove, tt.wantRemove) {
				t.Errorf("userGroupIDsDifference() = (%v, %v), want (%v, %v)", gotAdd, gotRemove, tt.wantAdd, tt.wantRemove)
			}
		})
	}
}

// The update hook used to gate the user group changes on
// DifferentAt("UserGroupIDs"), a path that never matches the Spec.UserGroupIDs
// difference the generated delta records.
func TestUserGroupIDsDeltaPath(t *testing.T) {
	desired := &resource{ko: &svcapitypes.ReplicationGroup{}}
	desired.ko.Spec.UserGroupIDs = aws.StringSlice([]string{"ug-a", "ug-b"})
	latest := &resource{ko: &svcapitypes.ReplicationGroup{}}
	latest.ko.Spec.UserGroupIDs = aws.StringSlice([]string{"ug-a"})

	delta := newResourceDelta(desired, latest)
	if !delta.DifferentAt("Spec.UserGroupIDs") {
		t.Fatalf("expected a Spec.UserGroupIDs difference, got %v", delta.Differences)
	}
	if delta.DifferentAt("UserGroupIDs") {
		t.Errorf("DifferentAt(\"UserGroupIDs\") matched; the hook must use the Spec prefixed path")
	}
}
