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

package global_replication_group

import (
	ackcondition "github.com/aws-controllers-k8s/runtime/pkg/condition"
	corev1 "k8s.io/api/core/v1"

	svcapitypes "github.com/aws-controllers-k8s/elasticache-controller/apis/v1alpha1"
)

// CustomUpdateConditions sets ACK.ResourceSynced from the datastore's status
// after every successful AWS call.
//
// With reconcile.requeue_on_success_seconds set, the generated updateConditions
// adds ResourceSynced=True after any successful call. The runtime then skips
// rm.IsSynced, so the synced.when rule never runs and a datastore that is still
// "creating" or "modifying" reports Synced. This applies the same rule here, as
// sagemaker-controller does for its requeue-on-success resources.
func (rm *resourceManager) CustomUpdateConditions(
	ko *svcapitypes.GlobalReplicationGroup,
	r *resource,
	err error,
) bool {
	if err != nil {
		// On error the runtime's ensureConditions derives Synced itself.
		return false
	}
	status := corev1.ConditionFalse
	message := ackcondition.NotSyncedMessage
	if isSteadyState(&resource{ko}) {
		status = corev1.ConditionTrue
		message = ackcondition.SyncedMessage
	}
	ackcondition.SetSynced(&resource{ko}, status, &message, nil)
	return true
}
