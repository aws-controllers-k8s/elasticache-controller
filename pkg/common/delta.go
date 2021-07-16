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

package common

import ackcompare "github.com/aws-controllers-k8s/runtime/pkg/compare"

// remove the Difference corresponding to the given subject from the delta struct
//TODO: ideally this would have a common implementation in compare/delta.go
func RemoveFromDelta(
	delta *ackcompare.Delta,
	subject string,
) {
	// copy slice
	differences := delta.Differences

	// identify index of Difference to remove
	//TODO: this could require a stricter Path.Equals down the road
	var i *int = nil
	for j, diff := range differences {
		if diff.Path.Contains(subject) {
			i = &j
			break
		}
	}

	// if found, create a new slice and replace the original
	if i != nil {
		differences = append(differences[:*i], differences[*i+1:]...)
		delta.Differences = differences
	}
}
