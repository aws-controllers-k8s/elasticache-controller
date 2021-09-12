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
import "github.com/aws-controllers-k8s/elasticache-controller/pkg/common"

// remove differences which are not meaningful (i.e. ones that don't warrant a call to rm.Update)
func filterDelta(
	delta *ackcompare.Delta,
	desired *resource,
	latest *resource,
) {
	// the returned AccessString can be different than the specified one; as long as the last requested AccessString
	// matches the currently desired one, remove this difference from the delta
	if delta.DifferentAt("Spec.AccessString") {
		if desired.ko.Spec.AccessString != nil &&
			desired.ko.Status.LastRequestedAccessString != nil &&
			*desired.ko.Spec.AccessString == *desired.ko.Status.LastRequestedAccessString {

			common.RemoveFromDelta(delta, "Spec.AccessString")
		}
	}
}
