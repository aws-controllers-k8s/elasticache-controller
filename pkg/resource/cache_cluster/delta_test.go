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

package cache_cluster

import (
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"

	svcapitypes "github.com/aws-controllers-k8s/elasticache-controller/apis/v1alpha1"
)

func newTestResource(engineVersion string, autoMinorVersionUpgrade *bool) *resource {
	return &resource{ko: &svcapitypes.CacheCluster{
		Spec: svcapitypes.CacheClusterSpec{
			Engine:                  aws.String("valkey"),
			EngineVersion:           aws.String(engineVersion),
			AutoMinorVersionUpgrade: autoMinorVersionUpgrade,
		},
	}}
}

// AutoMinorVersionUpgrade does not influence whether a newer minor version is
// treated as an automatic upgrade — the skip applies regardless of that setting.
func TestModifyDeltaEngineVersionAutoUpgrade(t *testing.T) {
	tests := []struct {
		name           string
		desiredVersion string
		latestVersion  string
		wantDiffers    bool
	}{
		{"auto upgraded to newer minor", "7.2", "7.3", false},
		{"downgrade is not an auto upgrade", "7.3", "7.2", true},
		{"major version change is not an auto upgrade", "7.2", "8.0", true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			delta := newResourceDelta(
				newTestResource(tc.desiredVersion, aws.Bool(true)),
				newTestResource(tc.latestVersion, aws.Bool(true)),
			)
			if got := delta.DifferentAt("Spec.EngineVersion"); got != tc.wantDiffers {
				t.Errorf("DifferentAt(Spec.EngineVersion) = %v, want %v", got, tc.wantDiffers)
			}
		})
	}
}
