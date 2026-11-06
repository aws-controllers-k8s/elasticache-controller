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
	"errors"
	"testing"

	ackcondition "github.com/aws-controllers-k8s/runtime/pkg/condition"
	corev1 "k8s.io/api/core/v1"

	svcapitypes "github.com/aws-controllers-k8s/elasticache-controller/apis/v1alpha1"
)

// updateConditions on success must report Synced only in a steady state. With
// requeue_on_success the generated code alone reports Synced=True for any status,
// so a datastore still "creating" looked ready.
func TestUpdateConditionsSyncedFollowsStatus(t *testing.T) {
	cases := []struct {
		status *string
		want   corev1.ConditionStatus
	}{
		{nil, corev1.ConditionFalse},
		{strp("creating"), corev1.ConditionFalse},
		{strp("modifying"), corev1.ConditionFalse},
		{strp(statusPrimaryOnly), corev1.ConditionTrue},
		{strp(statusAvailable), corev1.ConditionTrue},
	}
	rm := &resourceManager{}
	for _, c := range cases {
		ko := &svcapitypes.GlobalReplicationGroup{}
		ko.Status.Status = c.status
		updated, ok := rm.updateConditions(&resource{ko}, true, nil)
		if !ok {
			t.Fatalf("status %v: updateConditions reported no update", fmtStrp(c.status))
		}
		synced := ackcondition.Synced(updated)
		if synced == nil {
			t.Fatalf("status %v: no Synced condition", fmtStrp(c.status))
		}
		if synced.Status != c.want {
			t.Errorf("status %v: Synced = %s, want %s", fmtStrp(c.status), synced.Status, c.want)
		}
		if synced.LastTransitionTime == nil {
			t.Errorf("status %v: Synced has no lastTransitionTime", fmtStrp(c.status))
		}
	}
}

// On error the runtime's ensureConditions derives Synced, so the custom method
// must leave it unset.
func TestUpdateConditionsLeavesSyncedOnError(t *testing.T) {
	ko := &svcapitypes.GlobalReplicationGroup{}
	ko.Status.Status = strp(statusAvailable)
	rm := &resourceManager{}
	updated, _ := rm.updateConditions(&resource{ko}, false, errors.New("boom"))
	if updated != nil && ackcondition.Synced(updated) != nil {
		t.Errorf("Synced set on error: %+v", ackcondition.Synced(updated))
	}
}
