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

package user_group

import (
	"context"
	"errors"
	"fmt"
	"slices"

	svcapitypes "github.com/aws-controllers-k8s/elasticache-controller/apis/v1alpha1"
	ackcompare "github.com/aws-controllers-k8s/runtime/pkg/compare"
	ackrequeue "github.com/aws-controllers-k8s/runtime/pkg/requeue"
	"github.com/aws/aws-sdk-go-v2/aws"
	svcsdk "github.com/aws/aws-sdk-go-v2/service/elasticache"
)

const (
	statusActive = "active"
)

func hasStatus(ko *svcapitypes.UserGroup, status string) bool {
	return ko.Status.Status != nil && *ko.Status.Status == status
}

func isActive(ko *svcapitypes.UserGroup) bool {
	return hasStatus(ko, statusActive)
}

func (rm *resourceManager) updateModifyUserGroupPayload(input *svcsdk.ModifyUserGroupInput, desired, latest *resource, delta *ackcompare.Delta) {
	if delta.DifferentAt("Spec.UserIDs") {
		userIdsToAdd, userIdsToRemove := getUserIdsDifferences(
			desired.ko.Spec.UserIDs,
			latest.ko.Spec.UserIDs,
		)
		if len(userIdsToAdd) > 0 {
			input.UserIdsToAdd = userIdsToAdd
		}
		if len(userIdsToRemove) > 0 {
			input.UserIdsToRemove = userIdsToRemove
		}
	}
}

func (rm *resourceManager) CustomDescribeUserGroupsSetOutput(
	ctx context.Context,
	r *resource,
	resp *svcsdk.DescribeUserGroupsOutput,
	ko *svcapitypes.UserGroup,
) (*svcapitypes.UserGroup, error) {
	rm.patchUserGroupPendingChanges(ko)
	return ko, nil
}

func (rm *resourceManager) CustomModifyUserGroupSetOutput(
	ctx context.Context,
	r *resource,
	resp *svcsdk.ModifyUserGroupOutput,
	ko *svcapitypes.UserGroup,
) (*svcapitypes.UserGroup, error) {
	rm.patchUserGroupPendingChanges(ko)
	return ko, nil
}

func (rm *resourceManager) patchUserGroupPendingChanges(
	ko *svcapitypes.UserGroup,
) (*svcapitypes.UserGroup, error) {

	if pendingChanges := ko.Status.PendingChanges; pendingChanges != nil {
		if pendingChanges.UserIDsToAdd != nil {
			// append the IDs being added to the existing spec list
			ko.Spec.UserIDs = append(ko.Spec.UserIDs, pendingChanges.UserIDsToAdd...)
		}
		if pendingChanges.UserIDsToRemove != nil {
			// remove the IDs being removed from the existing spec list
			updatedUserIDs := []*string{}
			toRemove := map[string]struct{}{}
			for _, id := range pendingChanges.UserIDsToRemove {
				toRemove[aws.ToString(id)] = struct{}{}
			}
			for _, id := range ko.Spec.UserIDs {
				if _, found := toRemove[aws.ToString(id)]; !found {
					updatedUserIDs = append(updatedUserIDs, id)
				}
			}
			ko.Spec.UserIDs = updatedUserIDs
		}
	}
	return ko, nil
}

func requeueWaitUntilCanModify(r *resource) *ackrequeue.RequeueNeededAfter {
	if r.ko.Status.Status == nil {
		return nil
	}
	status := *r.ko.Status.Status
	msg := fmt.Sprintf(
		"User group in '%s' state, cannot be modified until '%s'.",
		status, statusActive,
	)
	return ackrequeue.NeededAfter(
		errors.New(msg),
		ackrequeue.DefaultRequeueAfterDuration,
	)
}

func getUserIdsDifferences(userIdsDesired []*string, userIdsLatest []*string) ([]string, []string) {
	userIdsToAdd := []string{}
	userIdsToRemove := []string{}
	desired := aws.ToStringSlice(userIdsDesired)
	latest := aws.ToStringSlice(userIdsLatest)

	for _, userId := range userIdsDesired {
		if !slices.Contains(latest, *userId) {
			userIdsToAdd = append(userIdsToAdd, *userId)
		}
	}
	for _, userId := range userIdsLatest {
		if !slices.Contains(desired, *userId) {
			userIdsToRemove = append(userIdsToRemove, *userId)
		}
	}
	return userIdsToAdd, userIdsToRemove

}
