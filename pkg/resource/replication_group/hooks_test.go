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
	"testing"

	ackcompare "github.com/aws-controllers-k8s/runtime/pkg/compare"

	svcapitypes "github.com/aws-controllers-k8s/elasticache-controller/apis/v1alpha1"
)

func Test_modifyDelta_Durability(t *testing.T) {
	strp := func(s string) *string { return &s }

	tests := []struct {
		name         string
		desired      *string
		latest       *string
		wantDeltaAdd bool
	}{
		{"nil to async (pre-feature cluster)", strp("async"), nil, true},
		{"async to sync", strp("sync"), strp("async"), true},
		{"sync to async", strp("async"), strp("sync"), true},
		{"async to disabled", strp("disabled"), strp("async"), true},
		{"disabled to async", strp("async"), strp("disabled"), true},
		{"default to sync", strp("sync"), strp("default"), true},
		{"default to default", strp("default"), strp("default"), false},
		{"async to async", strp("async"), strp("async"), false},
		{"disabled to default", strp("default"), strp("disabled"), true},
		{"async to nil", nil, strp("async"), true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			delta := ackcompare.NewDelta()
			desired := &resource{ko: &svcapitypes.ReplicationGroup{
				Spec: svcapitypes.ReplicationGroupSpec{Durability: tt.desired},
			}}
			latest := &resource{ko: &svcapitypes.ReplicationGroup{
				Spec: svcapitypes.ReplicationGroupSpec{Durability: tt.latest},
			}}

			modifyDelta(delta, desired, latest)

			got := delta.DifferentAt("Spec.Durability")
			if got != tt.wantDeltaAdd {
				t.Errorf("modifyDelta() Spec.Durability delta = %v, want %v", got, tt.wantDeltaAdd)
			}
		})
	}
}
