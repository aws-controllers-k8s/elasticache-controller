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

	svcsdk "github.com/aws/aws-sdk-go-v2/service/elasticache"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"

	svcapitypes "github.com/aws-controllers-k8s/elasticache-controller/apis/v1alpha1"
	"github.com/aws-controllers-k8s/elasticache-controller/pkg/common"
	ackv1alpha1 "github.com/aws-controllers-k8s/runtime/apis/core/v1alpha1"
	ackcompare "github.com/aws-controllers-k8s/runtime/pkg/compare"
	"github.com/aws-controllers-k8s/runtime/pkg/requeue"
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

// currently this function's only purpose is to requeue if the resource is currently unavailable
func (rm *resourceManager) CustomModifyUser(
	ctx context.Context,
	desired *resource,
	latest *resource,
	delta *ackcompare.Delta,
) (*resource, error) {

	// requeue if necessary
	latestStatus := latest.ko.Status.Status
	if latestStatus == nil || *latestStatus != "active" {
		return nil, requeue.NeededAfter(
			errors.New("User cannot be modified as its status is not 'active'."),
			requeue.DefaultRequeueAfterDuration)
	}

	return nil, nil
}

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

	//TODO: add update for passwords field once we have framework-level support

}

/*
	functions to update the state of the resource where the generated code or the set_output
	functions are insufficient
*/

// set the ResourceSynced condition based on the User's Status. r is a wrapper around the User resource which will
// eventually be returned as "latest"
func (rm *resourceManager) setSyncedCondition(
	status *string,
	r *resource,
) {
	// determine whether the resource can be considered synced
	syncedStatus := corev1.ConditionUnknown
	if status != nil {
		if *status == "active" {
			syncedStatus = corev1.ConditionTrue
		} else {
			syncedStatus = corev1.ConditionFalse
		}

	}

	// TODO: add utility function in a common repo to do the below as it's done at least once per resource

	// set existing condition to the above status (or create a new condition with this status)
	ko := r.ko
	var resourceSyncedCondition *ackv1alpha1.Condition = nil
	for _, condition := range ko.Status.Conditions {
		if condition.Type == ackv1alpha1.ConditionTypeResourceSynced {
			resourceSyncedCondition = condition
			break
		}
	}
	if resourceSyncedCondition == nil {
		resourceSyncedCondition = &ackv1alpha1.Condition{
			Type:   ackv1alpha1.ConditionTypeResourceSynced,
			Status: syncedStatus,
		}
		ko.Status.Conditions = append(ko.Status.Conditions, resourceSyncedCondition)
	} else {
		resourceSyncedCondition.Status = syncedStatus
	}
}

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
