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
	svcsdk "github.com/aws/aws-sdk-go-v2/service/elasticache"
	corev1 "k8s.io/api/core/v1"

	svcapitypes "github.com/aws-controllers-k8s/elasticache-controller/apis/v1alpha1"
	"github.com/aws-controllers-k8s/elasticache-controller/pkg/common"
)

// datastoreOwnedSpecPaths are the spec fields a Global Datastore owns once a
// replication group is one of its members. AWS rejects changing them through
// ModifyReplicationGroup on a member, even to the current value, so they can
// only change through the GlobalReplicationGroup. While the group is a member,
// differences on them are ignored and the declared values are left in spec.
// The shard count in NodeGroupConfiguration is owned too; per-shard replica
// settings are not, so that field is handled separately.
var datastoreOwnedSpecPaths = []string{
	"Spec.CacheNodeType",
	"Spec.CacheParameterGroupName",
	"Spec.Engine",
	"Spec.EngineVersion",
	"Spec.AutomaticFailoverEnabled",
	"Spec.AutoMinorVersionUpgrade",
	"Spec.TransitEncryptionEnabled",
	"Spec.Durability",
	"Spec.NumNodeGroups",
	"Spec.PrimaryClusterID",
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

// dropDatastoreOwnedDifferences removes the datastore-owned differences from a
// member's delta, so they never trigger an update. It runs last in modifyDelta,
// after the differences modifyDelta itself adds.
func dropDatastoreOwnedDifferences(
	delta *ackcompare.Delta,
	desired *resource,
	latest *resource,
) {
	if !isGlobalDatastoreMember(latest) {
		return
	}
	for _, path := range datastoreOwnedSpecPaths {
		if delta.DifferentAt(path) {
			common.RemoveFromDelta(delta, path)
		}
	}
	// Status is the reliable shard count: the read path does not rebuild
	// Spec.NodeGroupConfiguration.
	ngc := desired.ko.Spec.NodeGroupConfiguration
	if delta.DifferentAt("Spec.NodeGroupConfiguration") && ngc != nil &&
		len(ngc) != len(latest.ko.Status.NodeGroups) {
		common.RemoveFromDelta(delta, "Spec.NodeGroupConfiguration")
	}
}

// restoreDatastoreOwnedSpec copies the declared datastore-owned values from
// desired onto an update result. The update responses report the values AWS
// holds, and the runtime patches the result into the CR, so without this an
// unrelated update on a member would write the datastore's values into spec.
func restoreDatastoreOwnedSpec(
	desired *svcapitypes.ReplicationGroup,
	updated *svcapitypes.ReplicationGroup,
) {
	d := desired.Spec.DeepCopy()
	updated.Spec.CacheNodeType = d.CacheNodeType
	updated.Spec.CacheParameterGroupName = d.CacheParameterGroupName
	updated.Spec.Engine = d.Engine
	updated.Spec.EngineVersion = d.EngineVersion
	updated.Spec.AutomaticFailoverEnabled = d.AutomaticFailoverEnabled
	updated.Spec.AutoMinorVersionUpgrade = d.AutoMinorVersionUpgrade
	updated.Spec.TransitEncryptionEnabled = d.TransitEncryptionEnabled
	updated.Spec.Durability = d.Durability
	updated.Spec.NumNodeGroups = d.NumNodeGroups
	updated.Spec.NodeGroupConfiguration = d.NodeGroupConfiguration
	updated.Spec.PrimaryClusterID = d.PrimaryClusterID
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
	input.TransitEncryptionEnabled = nil
	input.TransitEncryptionMode = ""
	input.Durability = ""
	input.PrimaryClusterId = nil
}

// datastoreMemberAdvisoryReason keys the ACK.Advisory condition that tells
// users a member's datastore-owned fields are not applied.
const datastoreMemberAdvisoryReason = "GlobalDatastoreMember"

// syncDatastoreMemberAdvisory sets the advisory while the group is a Global
// Datastore member and removes it once the group leaves.
func syncDatastoreMemberAdvisory(ko *svcapitypes.ReplicationGroup) {
	r := &resource{ko: ko}
	if isGlobalDatastoreMember(r) {
		msg := fmt.Sprintf("Member of Global Datastore %s: cacheNodeType, "+
			"cacheParameterGroupName, engine, engineVersion, automaticFailoverEnabled, "+
			"autoMinorVersionUpgrade, transitEncryptionEnabled, durability, primaryClusterID and the shard count "+
			"are managed by the GlobalReplicationGroup, and their declared values are not "+
			"applied while the group is a member. After it leaves the datastore, update "+
			"these fields to the values AWS reports: a different cacheNodeType or shard "+
			"count fails with InvalidParameterCombination, and a lower engineVersion is "+
			"rejected.",
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
