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

package util_test

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/aws-controllers-k8s/elasticache-controller/pkg/util"
)

func TestEngineVersionsMatch(t *testing.T) {
	tests := []struct {
		desiredVersion string
		latestVersion  string
		expected       bool
	}{
		{
			desiredVersion: "6.3",
			latestVersion:  "6.2.6",
			expected:       false,
		},
		{
			desiredVersion: "6.2",
			latestVersion:  "6.2.6",
			expected:       true,
		},
		{
			desiredVersion: "6.x",
			latestVersion:  "6.0.5",
			expected:       true,
		},
		{
			desiredVersion: "13.x",
			latestVersion:  "6.0.6",
			expected:       false,
		},
		{
			desiredVersion: "5.0.3",
			latestVersion:  "5.0.3",
			expected:       true,
		},
		{
			desiredVersion: "5.0.3",
			latestVersion:  "5.0.4",
			expected:       false,
		},
	}

	for i, tt := range tests {
		t.Run(fmt.Sprintf("test-%d", i+1), func(t *testing.T) {
			require := require.New(t)
			require.Equal(util.EngineVersionsMatch(tt.desiredVersion, tt.latestVersion), tt.expected)
		})
	}
}
