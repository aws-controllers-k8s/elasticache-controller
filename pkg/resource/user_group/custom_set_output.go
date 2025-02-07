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

	svcapitypes "github.com/aws-controllers-k8s/elasticache-controller/apis/v1alpha1"
	ackv1alpha1 "github.com/aws-controllers-k8s/runtime/apis/core/v1alpha1"
	"github.com/aws/aws-sdk-go/service/elasticache"
	corev1 "k8s.io/api/core/v1"
)

func (rm *resourceManager) CustomDescribeUserGroupsSetOutput(
	ctx context.Context,
	r *resource,
	resp *elasticache.DescribeUserGroupsOutput,
	ko *svcapitypes.UserGroup,
) (*svcapitypes.UserGroup, error) {
	elem := resp.UserGroups[0]
	rm.customSetOutput(elem.UserIds,
		elem.Engine,
		elem.Status,
		ko)
	return ko, nil
}

func (rm *resourceManager) CustomCreateUserGroupSetOutput(
	ctx context.Context,
	r *resource,
	resp *elasticache.CreateUserGroupOutput,
	ko *svcapitypes.UserGroup,
) (*svcapitypes.UserGroup, error) {
	rm.customSetOutput(resp.UserIds,
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
