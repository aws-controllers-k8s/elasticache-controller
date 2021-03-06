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
	"context"

	svcapitypes "github.com/aws-controllers-k8s/elasticache-controller/apis/v1alpha1"
	svcsdk "github.com/aws/aws-sdk-go/service/elasticache"
)

// set the custom Status fields upon creation
func (rm *resourceManager) CustomCreateUserSetOutput(
	ctx context.Context,
	r *resource,
	resp *svcsdk.CreateUserOutput,
	ko *svcapitypes.User,
) (*svcapitypes.User, error) {
	return rm.CustomSetOutput(r, resp.AccessString, ko)
}

// precondition: successful ModifyUserWithContext call
// By updating 'latest' Status fields, these changes should be applied to 'desired'
// upon patching
func (rm *resourceManager) CustomModifyUserSetOutput(
	ctx context.Context,
	r *resource,
	resp *svcsdk.ModifyUserOutput,
	ko *svcapitypes.User,
) (*svcapitypes.User, error) {
	return rm.CustomSetOutput(r, resp.AccessString, ko)
}

func (rm *resourceManager) CustomSetOutput(
	r *resource,
	responseAccessString *string,
	ko *svcapitypes.User,
) (*svcapitypes.User, error) {

	lastRequested := *r.ko.Spec.AccessString
	ko.Status.LastRequestedAccessString = &lastRequested

	expandedAccessStringValue := *responseAccessString
	ko.Status.ExpandedAccessString = &expandedAccessStringValue

	return ko, nil
}
