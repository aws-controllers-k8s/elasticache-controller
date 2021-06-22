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

import (
	ackcompare "github.com/aws-controllers-k8s/runtime/pkg/compare"
	svcsdk "github.com/aws/aws-sdk-go/service/elasticache"
)

// TODO: this should be generated in the future. In general, it doesn't seem like a good idea to add every non-nil
// Spec field in desired.Spec to the payload (i.e. what we do when building most inputs), unless there is
// actually a difference in the Spec field between desired and latest
func (rm *resourceManager) populateUpdatePayload(
	input *svcsdk.ModifyUserInput,
	r *resource,
	delta *ackcompare.Delta,
) {
	if delta.DifferentAt("Spec.AccessString") && r.ko.Spec.AccessString != nil {
		input.AccessString = r.ko.Spec.AccessString
	}

	if delta.DifferentAt("Spec.NoPasswordRequired") && r.ko.Spec.NoPasswordRequired != nil {
		input.NoPasswordRequired = r.ko.Spec.NoPasswordRequired
	}

	//TODO: add the passwords field here once we have secrets support for it

}
