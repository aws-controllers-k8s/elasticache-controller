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
	ackcondition "github.com/aws-controllers-k8s/runtime/pkg/condition"
	ackrequeue "github.com/aws-controllers-k8s/runtime/pkg/requeue"
	"github.com/aws/aws-sdk-go-v2/aws"
	svcsdk "github.com/aws/aws-sdk-go-v2/service/elasticache"
	corev1 "k8s.io/api/core/v1"
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

func (rm *resourceManager) updateModifyUserGroupPayload(input *svcsdk.ModifyUserGroupInput, desired, latest *resource, delta *ackcompare.Delta) error {
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

	return nil
}

func (rm *resourceManager) CustomDescribeUserGroupsSetOutput(
	ctx context.Context,
	r *resource,
	resp *svcsdk.DescribeUserGroupsOutput,
	ko *svcapitypes.UserGroup,
) (*svcapitypes.UserGroup, error) {
	elem := resp.UserGroups[0]
	rm.customSetOutput(
		aws.StringSlice(elem.UserIds),
		elem.Engine,
		elem.Status,
		ko)
	rm.patchUserGroupPendingChanges(ko)
	return ko, nil
}

func (rm *resourceManager) CustomModifyUserGroupSetOutput(
	ctx context.Context,
	r *resource,
	resp *svcsdk.ModifyUserGroupOutput,
	ko *svcapitypes.UserGroup,
) (*svcapitypes.UserGroup, error) {
	rm.customSetOutput(
		aws.StringSlice(resp.UserIds),
		resp.Engine,
		resp.Status,
		ko)
	rm.patchUserGroupPendingChanges(ko)
	return ko, nil
}

func (rm *resourceManager) CustomCreateUserGroupSetOutput(
	ctx context.Context,
	r *resource,
	resp *svcsdk.CreateUserGroupOutput,
	ko *svcapitypes.UserGroup,
) (*svcapitypes.UserGroup, error) {
	rm.customSetOutput(
		aws.StringSlice(resp.UserIds),
		resp.Engine,
		resp.Status,
		ko)
	return ko, nil
}

func (rm *resourceManager) customSetOutput(
	userIds []*string,
	engine *string,
	status *string,
	ko *svcapitypes.UserGroup,
) {
	if userIds != nil {
		ko.Spec.UserIDs = userIds
	}

	if engine != nil {
		ko.Spec.Engine = engine
	}

	if isActive(ko) {
		ackcondition.SetSynced(&resource{ko}, corev1.ConditionTrue, nil, nil)
	} else {
		// Setting resource synced condition to false will trigger a requeue of
		// the resource. No need to return a requeue error here.
		ackcondition.SetSynced(&resource{ko}, corev1.ConditionFalse, nil, nil)
	}
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
