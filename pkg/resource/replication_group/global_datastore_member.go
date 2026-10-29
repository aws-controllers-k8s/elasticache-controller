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

package replication_group

import (
	"fmt"

	ackv1alpha1 "github.com/aws-controllers-k8s/runtime/apis/core/v1alpha1"
	ackcompare "github.com/aws-controllers-k8s/runtime/pkg/compare"
	ackcondition "github.com/aws-controllers-k8s/runtime/pkg/condition"
	"github.com/aws/aws-sdk-go-v2/aws"
	svcsdk "github.com/aws/aws-sdk-go-v2/service/elasticache"
	corev1 "k8s.io/api/core/v1"

	svcapitypes "github.com/aws-controllers-k8s/elasticache-controller/apis/v1alpha1"
	"github.com/aws-controllers-k8s/elasticache-controller/pkg/common"
)

// datastoreOwnedSpecPaths are the spec fields a Global Datastore owns once a
// replication group is one of its members. ModifyReplicationGroup rejects them
// on a member with InvalidParameterValue, even when the value is unchanged, so
// they can only be changed through the GlobalReplicationGroup.
var datastoreOwnedSpecPaths = []string{
	"Spec.CacheNodeType",
	"Spec.CacheParameterGroupName",
	"Spec.Engine",
	"Spec.EngineVersion",
	"Spec.AutomaticFailoverEnabled",
}

// isGlobalDatastoreMember reports whether AWS lists the replication group as a
// member of a Global Datastore. It keys off observed status rather than the
// member's role or spec: roles swap on failover, and a primary joins through
// CreateGlobalReplicationGroup without anything in its own spec changing.
func isGlobalDatastoreMember(r *resource) bool {
	info := r.ko.Status.GlobalReplicationGroupInfo
	return info != nil && info.GlobalReplicationGroupID != nil &&
		*info.GlobalReplicationGroupID != ""
}

// reconcileDatastoreOwnedFields stops a member from trying to revert values
// the Global Datastore set. It removes the datastore-owned differences from
// delta and copies the observed values into desired, so the returned resource
// persists them to the CR instead of leaving stale values that would be
// re-applied once the group leaves the datastore.
func reconcileDatastoreOwnedFields(
	desired *resource,
	latest *resource,
	delta *ackcompare.Delta,
) {
	// modifyDelta has already dropped versions that match, such as "7.0"
	// against "7.0.7", so a remaining difference means the datastore changed it.
	if delta.DifferentAt("Spec.Engine") &&
		desired.ko.Spec.Engine != nil && latest.ko.Spec.Engine != nil {
		v := *latest.ko.Spec.Engine
		desired.ko.Spec.Engine = &v
	}
	if delta.DifferentAt("Spec.EngineVersion") &&
		desired.ko.Spec.EngineVersion != nil &&
		latest.ko.Spec.EngineVersion != nil {
		v := *latest.ko.Spec.EngineVersion
		desired.ko.Spec.EngineVersion = &v
	}
	// AutomaticFailoverEnabled is only observable through status. Adopt it
	// once settled; a transitional status is left for a later reconcile.
	if desired.ko.Spec.AutomaticFailoverEnabled != nil &&
		latest.ko.Status.AutomaticFailover != nil {
		switch *latest.ko.Status.AutomaticFailover {
		case string(svcapitypes.AutomaticFailoverStatus_enabled):
			desired.ko.Spec.AutomaticFailoverEnabled = aws.Bool(true)
		case string(svcapitypes.AutomaticFailoverStatus_disabled):
			desired.ko.Spec.AutomaticFailoverEnabled = aws.Bool(false)
		}
	}
	for _, path := range datastoreOwnedSpecPaths {
		if delta.DifferentAt(path) {
			common.RemoveFromDelta(delta, path)
		}
	}
	if latest.ko.Spec.CacheNodeType != nil {
		v := *latest.ko.Spec.CacheNodeType
		desired.ko.Spec.CacheNodeType = &v
	}
	if latest.ko.Spec.CacheParameterGroupName != nil {
		v := *latest.ko.Spec.CacheParameterGroupName
		desired.ko.Spec.CacheParameterGroupName = &v
	}
}

// stripDatastoreOwnedInput removes the datastore-owned fields from a
// ModifyReplicationGroup request for a member. The generated payload sends
// every spec field regardless of the delta, plus AutoMinorVersionUpgrade copied
// from status, so without this any member update is rejected.
func stripDatastoreOwnedInput(
	input *svcsdk.ModifyReplicationGroupInput,
	latest *resource,
) {
	if !isGlobalDatastoreMember(latest) {
		return
	}
	input.CacheNodeType = nil
	input.CacheParameterGroupName = nil
	input.Engine = nil
	input.EngineVersion = nil
	input.AutomaticFailoverEnabled = nil
	input.AutoMinorVersionUpgrade = nil
}

// datastoreMemberAdvisoryReason keys the ACK.Advisory condition that tells
// users a member's datastore-owned fields are adopted from AWS.
const datastoreMemberAdvisoryReason = "GlobalDatastoreMember"

// syncDatastoreMemberAdvisory sets the advisory while the group is a Global
// Datastore member and removes it once the group leaves.
func syncDatastoreMemberAdvisory(ko *svcapitypes.ReplicationGroup) {
	r := &resource{ko: ko}
	if isGlobalDatastoreMember(r) {
		msg := fmt.Sprintf("Member of Global Datastore %s: cacheNodeType, "+
			"cacheParameterGroupName, engine, engineVersion and "+
			"automaticFailoverEnabled are managed by the GlobalReplicationGroup, "+
			"and their spec values are replaced with the values AWS reports.",
			*ko.Status.GlobalReplicationGroupInfo.GlobalReplicationGroupID)
		// SetAdvisory stamps a new transition time, so only call it on a change.
		if c := ackcondition.AdvisoryWithReason(r, datastoreMemberAdvisoryReason); c != nil &&
			c.Status == corev1.ConditionTrue && c.Message != nil && *c.Message == msg {
			return
		}
		reason := datastoreMemberAdvisoryReason
		ackcondition.SetAdvisory(r, corev1.ConditionTrue, &msg, &reason)
		return
	}
	if ackcondition.AdvisoryWithReason(r, datastoreMemberAdvisoryReason) == nil {
		return
	}
	kept := []*ackv1alpha1.Condition{}
	for _, c := range ko.Status.Conditions {
		if c.Type == ackv1alpha1.ConditionTypeAdvisory && c.Reason != nil &&
			*c.Reason == datastoreMemberAdvisoryReason {
			continue
		}
		kept = append(kept, c)
	}
	ko.Status.Conditions = kept
}

// datastoreOwnedDifferences lists the datastore-owned fields that differ in delta.
func datastoreOwnedDifferences(delta *ackcompare.Delta) []string {
	owned := []string{}
	for _, path := range datastoreOwnedSpecPaths {
		if delta.DifferentAt(path) {
			owned = append(owned, path)
		}
	}
	return owned
}
