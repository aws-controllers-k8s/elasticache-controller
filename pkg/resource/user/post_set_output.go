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
	ackv1alpha1 "github.com/aws-controllers-k8s/runtime/apis/core/v1alpha1"
	corev1 "k8s.io/api/core/v1"
)

/*
	This file contains functions to update the state of the resource where the generated code or the set_output
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
