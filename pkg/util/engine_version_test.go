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

package util

import "testing"

func TestEngineVersionAutoUpgraded(t *testing.T) {
	tests := []struct {
		name    string
		desired string
		latest  string
		want    bool
	}{
		{"newer minor, same major", "6.0", "6.2.6", true},
		{"newer minor valkey", "7.2", "7.3", true},
		{"same minor, patch differs", "7.2", "7.2.6", false},
		{"identical", "7.2", "7.2", false},
		{"latest is older minor", "7.2", "7.1", false},
		{"different major", "6.2", "7.0", false},
		{"major only", "7", "7.2", false},
		{"x placeholder", "6.x", "6.2", false},
		{"major 5 not covered", "5.0.4", "5.0.6", false},
		{"memcached not covered", "1.6.6", "1.6.22", false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := EngineVersionAutoUpgraded(tc.desired, tc.latest); got != tc.want {
				t.Errorf("EngineVersionAutoUpgraded(%q, %q) = %v, want %v", tc.desired, tc.latest, got, tc.want)
			}
		})
	}
}
