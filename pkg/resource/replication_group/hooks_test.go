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

	"github.com/aws/aws-sdk-go-v2/aws"

	svcapitypes "github.com/aws-controllers-k8s/elasticache-controller/apis/v1alpha1"
)

func newDurabilityResources(desired *string, lastRequested *string) (*resource, *resource) {
	d := &resource{
		ko: &svcapitypes.ReplicationGroup{
			Spec: svcapitypes.ReplicationGroupSpec{
				Durability: desired,
			},
		},
	}
	l := &resource{
		ko: &svcapitypes.ReplicationGroup{
			Status: svcapitypes.ReplicationGroupStatus{
				ObservedDurability: lastRequested,
			},
		},
	}
	return d, l
}

func Test_durabilityRequiresUpdate(t *testing.T) {
	tests := []struct {
		name string
		// desired is the durability in the resource's spec
		desired *string
		// lastRequested is the durability the API reports for the replication group
		lastRequested *string
		want          bool
	}{
		{
			name:          "durability unmanaged, none reported",
			desired:       nil,
			lastRequested: nil,
			want:          false,
		},
		{
			// AWS offers no way to unset durability, so a nil desired value means the field
			// is unmanaged and no request should be made.
			name:          "durability unmanaged, one reported",
			desired:       nil,
			lastRequested: aws.String("async"),
			want:          false,
		},
		{
			name:          "reported durability matches desired",
			desired:       aws.String("sync"),
			lastRequested: aws.String("sync"),
			want:          false,
		},
		{
			name:          "reported durability differs from desired",
			desired:       aws.String("sync"),
			lastRequested: aws.String("async"),
			want:          true,
		},
		{
			// "default" is reported unresolved, so it compares equal to itself. Only
			// Status.EffectiveDurability holds the value the service resolved it to.
			name:          "default reported for a group created with default",
			desired:       aws.String("default"),
			lastRequested: aws.String("default"),
			want:          false,
		},
		{
			name:          "desired moves away from default",
			desired:       aws.String("sync"),
			lastRequested: aws.String("default"),
			want:          true,
		},
		{
			name:          "nothing reported by the API",
			desired:       aws.String("sync"),
			lastRequested: nil,
			want:          true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			desired, latest := newDurabilityResources(tt.desired, tt.lastRequested)
			if got := durabilityRequiresUpdate(desired, latest); got != tt.want {
				t.Errorf("durabilityRequiresUpdate() = %v, want %v", got, tt.want)
			}
		})
	}
}
