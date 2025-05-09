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

	svcapitypes "github.com/aws-controllers-k8s/elasticache-controller/apis/v1alpha1"
	ackv1alpha1 "github.com/aws-controllers-k8s/runtime/apis/core/v1alpha1"
	ackcompare "github.com/aws-controllers-k8s/runtime/pkg/compare"
	"github.com/aws-controllers-k8s/runtime/pkg/requeue"
	svcsdk "github.com/aws/aws-sdk-go-v2/service/elasticache"
	corev1 "k8s.io/api/core/v1"
)

// Implements custom logic for UpdateUserGroup
func (rm *resourceManager) customUpdateUserGroup(
	ctx context.Context,
	desired *resource,
	latest *resource,
	delta *ackcompare.Delta,
) (*resource, error) {

	// Check for the user group status
	if latest.ko.Status.Status == nil || *latest.ko.Status.Status != "active" {
		return nil, requeue.NeededAfter(
			errors.New("user group can not be modified, it is not in 'active' state"),
			requeue.DefaultRequeueAfterDuration)
	}

	for _, diff := range delta.Differences {
		if diff.Path.Contains("UserIDs") {
			existingUserIdsMap := createMapForUserIds(diff.B.([]*string))
			requiredUserIdsMap := createMapForUserIds(diff.A.([]*string))

			// If a user ID is not required to be deleted or added set its value as false
			for userId, _ := range existingUserIdsMap {
				if _, ok := requiredUserIdsMap[userId]; ok {
					requiredUserIdsMap[userId] = false
					existingUserIdsMap[userId] = false
				}
			}

			input, err := rm.newUpdateRequestPayload(ctx, desired)

			if err != nil {
				return nil, err
			}

			// User Ids to add
			{
				var userIdsToAdd []string

				for userId, include := range requiredUserIdsMap {
					if include {
						userIdsToAdd = append(userIdsToAdd, userId)
					}
				}

				input.UserIdsToAdd = userIdsToAdd
			}

			// User Ids to remove
			{
				var userIdsToRemove []string

				for userId, include := range existingUserIdsMap {
					if include {
						userIdsToRemove = append(userIdsToRemove, userId)
					}
				}

				input.UserIdsToRemove = userIdsToRemove
			}

			resp, respErr := rm.sdkapi.ModifyUserGroup(ctx, input)
			rm.metrics.RecordAPICall("UPDATE", "ModifyUserGroup", respErr)
			if respErr != nil {
				return nil, respErr
			}
			// Merge in the information we read from the API call above to the copy of
			// the original Kubernetes object we passed to the function
			ko := desired.ko.DeepCopy()

			if ko.Status.ACKResourceMetadata == nil {
				ko.Status.ACKResourceMetadata = &ackv1alpha1.ResourceMetadata{}
			}
			if resp.ARN != nil {
				arn := ackv1alpha1.AWSResourceName(*resp.ARN)
				ko.Status.ACKResourceMetadata.ARN = &arn
			}
			if resp.PendingChanges != nil {
				f2 := &svcapitypes.UserGroupPendingChanges{}
				if resp.PendingChanges.UserIdsToAdd != nil {
					f2f0 := []*string{}
					for _, f2f0iter := range resp.PendingChanges.UserIdsToAdd {
						f2f0 = append(f2f0, &f2f0iter)
					}
					f2.UserIDsToAdd = f2f0
				}
				if resp.PendingChanges.UserIdsToRemove != nil {
					f2f1 := []*string{}
					for _, f2f1iter := range resp.PendingChanges.UserIdsToRemove {
						f2f1 = append(f2f1, &f2f1iter)
					}
					f2.UserIDsToRemove = f2f1
				}
				ko.Status.PendingChanges = f2
			} else {
				ko.Status.PendingChanges = nil
			}
			if resp.ReplicationGroups != nil {
				f3 := []*string{}
				for _, f3iter := range resp.ReplicationGroups {
					f3 = append(f3, &f3iter)
				}
				ko.Status.ReplicationGroups = f3
			} else {
				ko.Status.ReplicationGroups = nil
			}
			if resp.Status != nil {
				ko.Status.Status = resp.Status
			} else {
				ko.Status.Status = nil
			}

			rm.setStatusDefaults(ko)
			rm.customSetOutput(
				stringSliceToPointers(resp.UserIds),
				resp.Engine,
				resp.Status,
				ko)
			return &resource{ko}, nil
		}
	}

	rm.customSetOutput(desired.ko.Spec.UserIDs, desired.ko.Spec.Engine,
		latest.ko.Status.Status, latest.ko)
	return &resource{latest.ko}, nil
}

// createMapForUserIds converts list of user ids to map of user ids and boolean value
// true as value
func createMapForUserIds(userIds []*string) map[string]bool {
	userIdsMap := make(map[string]bool)

	for _, userId := range userIds {
		userIdsMap[*userId] = true
	}

	return userIdsMap
}

// newUpdateRequestPayload returns an SDK-specific struct for the HTTP request
// payload of the Update API call for the resource
func (rm *resourceManager) newUpdateRequestPayload(
	ctx context.Context,
	r *resource,
) (*svcsdk.ModifyUserGroupInput, error) {
	res := &svcsdk.ModifyUserGroupInput{}

	if r.ko.Spec.UserGroupID != nil {
		res.UserGroupId = r.ko.Spec.UserGroupID
	}

	return res, nil
}

func (rm *resourceManager) CustomDescribeUserGroupsSetOutput(
	ctx context.Context,
	r *resource,
	resp *svcsdk.DescribeUserGroupsOutput,
	ko *svcapitypes.UserGroup,
) (*svcapitypes.UserGroup, error) {
	elem := resp.UserGroups[0]
	rm.customSetOutput(
		stringSliceToPointers(elem.UserIds),
		elem.Engine,
		elem.Status,
		ko)
	return ko, nil
}

func (rm *resourceManager) CustomCreateUserGroupSetOutput(
	ctx context.Context,
	r *resource,
	resp *svcsdk.CreateUserGroupOutput,
	ko *svcapitypes.UserGroup,
) (*svcapitypes.UserGroup, error) {
	rm.customSetOutput(
		stringSliceToPointers(resp.UserIds),
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

	syncConditionStatus := corev1.ConditionUnknown
	if status != nil {
		if *status == "active" {
			syncConditionStatus = corev1.ConditionTrue
		} else {
			syncConditionStatus = corev1.ConditionFalse
		}
	}
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
			Status: syncConditionStatus,
		}
		ko.Status.Conditions = append(ko.Status.Conditions, resourceSyncedCondition)
	} else {
		resourceSyncedCondition.Status = syncConditionStatus
	}
}

func stringSliceToPointers(slice []string) []*string {
	ptrs := make([]*string, len(slice))
	for i, s := range slice {
		s := s // Create new variable to avoid referencing loop variable
		ptrs[i] = &s
	}
	return ptrs
}
