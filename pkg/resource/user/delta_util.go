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

package user

import ackcompare "github.com/aws-controllers-k8s/runtime/pkg/compare"

// remove differences which are not meaningful (i.e. ones that don't warrant a call to rm.Update)
func filterDelta(
	delta *ackcompare.Delta,
	desired *resource,
	latest *resource,
) {
	// the returned AccessString can be different than the specified one; as long as the last requested AccessString
	// matches the currently desired one, remove this difference from the delta
	//TODO: revert this call to Spec.AccessString once we have a new implementation of it
	if delta.DifferentAt("AccessString") {
		if *desired.ko.Spec.AccessString == *desired.ko.Status.LastRequestedAccessString {

			//TODO: revert the call to Spec.AccessString once removeFromDelta implementation changes
			removeFromDelta(delta, "AccessString")
		}
	}
}

// remove the Difference corresponding to the given subject from the delta struct
//TODO: ideally this would have a common implementation in compare/delta.go
func removeFromDelta(
	delta *ackcompare.Delta,
	subject string,
) {
	// copy slice
	differences := delta.Differences

	// identify index of Difference to remove
	//TODO: change once we get a Path.Equals or similar method
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
