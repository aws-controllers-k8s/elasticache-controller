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

import "testing"
import "github.com/stretchr/testify/require"

func TestEngineVersionsMatch(t *testing.T) {
	require := require.New(t)

	require.True(engineVersionsMatch("6.2", "6.2.6"))
	require.True(engineVersionsMatch("6.x", "6.0.5"))
	require.False(engineVersionsMatch("13.x", "6.0.6"))
	require.True(engineVersionsMatch("5.0.3", "5.0.3"))
	require.False(engineVersionsMatch("5.0.3", "5.0.4"))
}
