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

func TestNewResourceDeltaEngineCase(t *testing.T) {
	tests := []struct {
		name          string
		desired       *string
		latest        *string
		wantDifferent bool
	}{
		{"same engine in another case", aws.String("valKey"), aws.String("valkey"), false},
		{"same engine", aws.String("redis"), aws.String("redis"), false},
		{"different engine", aws.String("redis"), aws.String("valkey"), true},
		{"engine set but not observed", aws.String("valkey"), nil, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			desired := &resource{ko: &svcapitypes.ReplicationGroup{}}
			desired.ko.Spec.Engine = tt.desired
			latest := &resource{ko: &svcapitypes.ReplicationGroup{}}
			latest.ko.Spec.Engine = tt.latest
			if got := newResourceDelta(desired, latest).DifferentAt("Spec.Engine"); got != tt.wantDifferent {
				t.Errorf("DifferentAt(Spec.Engine) = %v, want %v", got, tt.wantDifferent)
			}
		})
	}
}
