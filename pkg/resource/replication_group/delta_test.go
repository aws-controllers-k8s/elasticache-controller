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

func newTestResource(engineVersion *string, autoMinorVersionUpgrade *bool) *resource {
	return &resource{ko: &svcapitypes.ReplicationGroup{
		Spec: svcapitypes.ReplicationGroupSpec{
			Engine:                  aws.String("valkey"),
			EngineVersion:           engineVersion,
			AutoMinorVersionUpgrade: autoMinorVersionUpgrade,
		},
	}}
}

func TestModifyDeltaAutoMinorVersionUpgradeUnsetInSpec(t *testing.T) {
	tests := []struct {
		name        string
		desired     *bool
		latest      *bool
		wantDiffers bool
	}{
		{"unset in spec, service reports true", nil, aws.Bool(true), false},
		{"unset in spec, service reports false", nil, aws.Bool(false), false},
		{"set in spec, service reports different value", aws.Bool(true), aws.Bool(false), true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			desired := newTestResource(aws.String("7.2"), tc.desired)
			latest := newTestResource(aws.String("7.2"), tc.latest)
			delta := newResourceDelta(desired, latest)
			if got := delta.DifferentAt("Spec.AutoMinorVersionUpgrade"); got != tc.wantDiffers {
				t.Errorf("DifferentAt(Spec.AutoMinorVersionUpgrade) = %v, want %v", got, tc.wantDiffers)
			}
		})
	}
}

func TestModifyDeltaEngineVersionWithAutoMinorVersionUpgrade(t *testing.T) {
	tests := []struct {
		name           string
		desiredVersion string
		latestVersion  string
		latestAMVU     *bool
		wantDiffers    bool
	}{
		{"auto upgraded to newer minor", "7.2", "7.3", aws.Bool(true), false},
		{"auto upgraded redis 6 to newer minor", "6.0", "6.2.6", aws.Bool(true), false},
		{"newer minor and auto upgrade disabled", "7.2", "7.3", aws.Bool(false), false},
		{"newer minor and auto upgrade unreported", "7.2", "7.3", nil, false},
		{"downgrade is not an auto upgrade", "7.3", "7.2", aws.Bool(true), true},
		{"major version change is not an auto upgrade", "7.2", "8.0", aws.Bool(true), true},
		{"same minor", "7.2", "7.2", aws.Bool(true), false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			desired := newTestResource(aws.String(tc.desiredVersion), aws.Bool(true))
			latest := newTestResource(aws.String(tc.latestVersion), tc.latestAMVU)
			delta := newResourceDelta(desired, latest)
			if got := delta.DifferentAt("Spec.EngineVersion"); got != tc.wantDiffers {
				t.Errorf("DifferentAt(Spec.EngineVersion) = %v, want %v", got, tc.wantDiffers)
			}
		})
	}
}

func TestModifyDeltaEngineVersionAutoUpgradeIgnoredOnEngineChange(t *testing.T) {
	desired := newTestResource(aws.String("7.2"), aws.Bool(true))
	desired.ko.Spec.Engine = aws.String("valkey")
	latest := newTestResource(aws.String("7.3"), aws.Bool(true))
	latest.ko.Spec.Engine = aws.String("redis")

	delta := newResourceDelta(desired, latest)
	if !delta.DifferentAt("Spec.EngineVersion") {
		t.Errorf("engine version difference must be kept when the engine is also changing")
	}
}
