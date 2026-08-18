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
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	svcsdk "github.com/aws/aws-sdk-go-v2/service/elasticache"
	svcsdktypes "github.com/aws/aws-sdk-go-v2/service/elasticache/types"
	smithy "github.com/aws/smithy-go"

	svcapitypes "github.com/aws-controllers-k8s/elasticache-controller/apis/v1alpha1"
	"github.com/aws-controllers-k8s/elasticache-controller/pkg/util"
	ackcompare "github.com/aws-controllers-k8s/runtime/pkg/compare"
	ackerr "github.com/aws-controllers-k8s/runtime/pkg/errors"
	ackrequeue "github.com/aws-controllers-k8s/runtime/pkg/requeue"
)

const (
	statusAvailable   = "available"
	statusPrimaryOnly = "primary-only"
	statusDeleting    = "deleting"
	statusModifying   = "modifying"
	statusCreating    = "creating"
)

// The GlobalReplicationGroup Describe API does not return CacheParameterGroupName
// (ElastiCache copies it per member), so we cannot compare desired against
// observed state for it. We record the value passed to the most recent
// Create/Modify call in an annotation and use that as the "observed" value during
// delta comparison. AutomaticFailoverEnabled is also not returned as a top-level
// field, but it is derivable from live member state, so it does not need an
// annotation (see deriveAutomaticFailoverEnabled).
const (
	// AnnotationLastRequestedCPGN records the CacheParameterGroupName value
	// sent in the most recent Create or Modify call.
	AnnotationLastRequestedCPGN = svcapitypes.AnnotationPrefix + "last-requested-cache-parameter-group-name"
)

var (
	requeueWaitWhileDeleting = ackrequeue.NeededAfter(
		errors.New("GlobalReplicationGroup is being deleted."),
		ackrequeue.DefaultRequeueAfterDuration,
	)
	requeueWaitWhileDisassociating = ackrequeue.NeededAfter(
		errors.New("Waiting for secondary members to be disassociated."),
		ackrequeue.DefaultRequeueAfterDuration,
	)
	requeueWaitWhileModifying = ackrequeue.NeededAfter(
		errors.New("GlobalReplicationGroup is being modified."),
		ackrequeue.DefaultRequeueAfterDuration,
	)
)

// =============================================================================
// CUSTOM DELETE
// =============================================================================
// Multi-step delete:
//   Phase 1: If already deleting → requeue
//   Phase 2: If secondary members exist → disassociate each → requeue
//   Phase 3: If primary-only → call DeleteGlobalReplicationGroup → requeue
//   Phase 4: Eventually the Describe returns 404 → ACK runtime handles it
// =============================================================================

func (rm *resourceManager) customDeleteGlobalReplicationGroup(
	ctx context.Context,
	r *resource,
) (*resource, error) {
	// Phase 1: Already deleting — just wait
	if isGlobalDeleting(r) {
		return r, requeueWaitWhileDeleting
	}

	// Proceed only from a stable state. Anything else ("creating"/"modifying")
	// means an operation is in flight and DeleteGlobalReplicationGroup would be
	// rejected with InvalidGlobalReplicationGroupState, so requeue and wait.
	// Once stable, exactly one state applies: "available" (secondaries attached
	// -> disassociate first, Phase 2) or "primary-only" (none -> delete, Phase 3).
	if !isGlobalPrimaryOnly(r) && !isGlobalAvailable(r) {
		return r, requeueWaitWhileModifying
	}

	globalId := r.ko.Status.GlobalReplicationGroupID
	if globalId == nil {
		return nil, fmt.Errorf("GlobalReplicationGroupID not found in status, cannot delete")
	}

	// Describe with member info to see if secondaries still exist
	descInput := &svcsdk.DescribeGlobalReplicationGroupsInput{
		GlobalReplicationGroupId: globalId,
		ShowMemberInfo:           aws.Bool(true),
	}
	descResp, err := rm.sdkapi.DescribeGlobalReplicationGroups(ctx, descInput)
	rm.metrics.RecordAPICall("READ_ONE", "DescribeGlobalReplicationGroups", err)
	if err != nil {
		return nil, err
	}

	if len(descResp.GlobalReplicationGroups) == 0 {
		// Already gone
		return nil, nil
	}

	grg := descResp.GlobalReplicationGroups[0]

	// Find secondary members
	secondaries := findSecondaryMembers(grg.Members)

	// Phase 2: Disassociate any remaining secondaries.
	// Per integration testing, a member stays "associated" until it simply
	// vanishes from the list when done -- there is no "disassociating" state.
	// Disassociating one already in progress returns
	// InvalidGlobalReplicationGroupState, which we requeue on.
	if len(secondaries) > 0 {
		for _, secondary := range secondaries {
			if secondary.Status != nil && *secondary.Status != "associated" {
				continue
			}

			disInput := &svcsdk.DisassociateGlobalReplicationGroupInput{
				GlobalReplicationGroupId: globalId,
				ReplicationGroupId:       secondary.ReplicationGroupId,
				ReplicationGroupRegion:   secondary.ReplicationGroupRegion,
			}
			_, disErr := rm.sdkapi.DisassociateGlobalReplicationGroup(ctx, disInput)
			rm.metrics.RecordAPICall("UPDATE", "DisassociateGlobalReplicationGroup", disErr)
			if disErr != nil {
				// InvalidGlobalReplicationGroupState is retryable — race between
				// our guard and a state transition. Anything else is fatal.
				if isInvalidGlobalReplicationGroupStateError(disErr) {
					return r, requeueWaitWhileModifying
				}
				return nil, disErr
			}
		}

		return r, requeueWaitWhileDisassociating
	}

	// Phase 3: No secondaries remain — delete the global group
	delInput := &svcsdk.DeleteGlobalReplicationGroupInput{
		GlobalReplicationGroupId:      globalId,
		RetainPrimaryReplicationGroup: aws.Bool(true),
	}
	_, delErr := rm.sdkapi.DeleteGlobalReplicationGroup(ctx, delInput)
	rm.metrics.RecordAPICall("DELETE", "DeleteGlobalReplicationGroup", delErr)
	if delErr != nil {
		return nil, delErr
	}

	return r, requeueWaitWhileDeleting
}

// =============================================================================
// CUSTOM UPDATE
// =============================================================================
// Node group (shard) scaling uses the separate Increase/DecreaseNodeGroups
// APIs; every other field goes through modifyGlobalReplicationGroup. A fully
// custom Update handler is required because ModifyGlobalReplicationGroup
// rejects arbitrary field combinations (see modifyGlobalReplicationGroup).
// =============================================================================

func (rm *resourceManager) customUpdateGlobalReplicationGroup(
	ctx context.Context,
	desired *resource,
	latest *resource,
	delta *ackcompare.Delta,
) (*resource, error) {
	// Guard: must be in a modifiable state
	if !isGlobalAvailable(latest) && !isGlobalPrimaryOnly(latest) {
		return nil, requeueWaitWhileModifying
	}

	globalId := latest.ko.Status.GlobalReplicationGroupID
	if globalId == nil {
		return nil, fmt.Errorf("GlobalReplicationGroupID not found in status")
	}

	// --- Handle node group count changes ---
	if desired.ko.Spec.NodeGroupCount != nil {
		desiredCount := *desired.ko.Spec.NodeGroupCount
		currentCount := int64(0)
		if latest.ko.Status.GlobalNodeGroups != nil {
			currentCount = int64(len(latest.ko.Status.GlobalNodeGroups))
		} else {
			// GlobalNodeGroups not yet populated in status — fetch from AWS
			descInput := &svcsdk.DescribeGlobalReplicationGroupsInput{
				GlobalReplicationGroupId: globalId,
			}
			descResp, err := rm.sdkapi.DescribeGlobalReplicationGroups(ctx, descInput)
			rm.metrics.RecordAPICall("READ_ONE", "DescribeGlobalReplicationGroups", err)
			if err == nil && len(descResp.GlobalReplicationGroups) > 0 {
				currentCount = int64(len(descResp.GlobalReplicationGroups[0].GlobalNodeGroups))
			}
		}

		if currentCount > 0 {
			if desiredCount > currentCount {
				return rm.increaseNodeGroups(ctx, desired, globalId, int32(desiredCount))
			} else if desiredCount < currentCount {
				return rm.decreaseNodeGroups(ctx, desired, latest, globalId, int32(desiredCount))
			}
		}
	}

	// --- Standard Modify ---
	return rm.modifyGlobalReplicationGroup(ctx, desired, latest, globalId)
}

func (rm *resourceManager) increaseNodeGroups(
	ctx context.Context,
	desired *resource,
	globalId *string,
	nodeGroupCount int32,
) (*resource, error) {
	input := &svcsdk.IncreaseNodeGroupsInGlobalReplicationGroupInput{
		GlobalReplicationGroupId: globalId,
		ApplyImmediately:         aws.Bool(true),
		NodeGroupCount:           aws.Int32(nodeGroupCount),
	}

	resp, err := rm.sdkapi.IncreaseNodeGroupsInGlobalReplicationGroup(ctx, input)
	rm.metrics.RecordAPICall("UPDATE", "IncreaseNodeGroupsInGlobalReplicationGroup", err)
	if err != nil {
		return nil, err
	}

	return rm.setGlobalReplicationGroupOutput(desired, resp.GlobalReplicationGroup)
}

func (rm *resourceManager) decreaseNodeGroups(
	ctx context.Context,
	desired *resource,
	latest *resource,
	globalId *string,
	nodeGroupCount int32,
) (*resource, error) {
	input := &svcsdk.DecreaseNodeGroupsInGlobalReplicationGroupInput{
		GlobalReplicationGroupId: globalId,
		ApplyImmediately:         aws.Bool(true),
		NodeGroupCount:           aws.Int32(nodeGroupCount),
	}

	// Retain the first N node groups and drop the rest. Safe for data: ElastiCache
	// migrates slots off dropped shards before removing them. Trade-off: users
	// can't choose which specific shards are removed.
	if latest.ko.Status.GlobalNodeGroups != nil {
		retain := []string{}
		for i, ng := range latest.ko.Status.GlobalNodeGroups {
			if int32(i) >= nodeGroupCount {
				break
			}
			if ng.GlobalNodeGroupID != nil {
				retain = append(retain, *ng.GlobalNodeGroupID)
			}
		}
		input.GlobalNodeGroupsToRetain = retain
	}

	resp, err := rm.sdkapi.DecreaseNodeGroupsInGlobalReplicationGroup(ctx, input)
	rm.metrics.RecordAPICall("UPDATE", "DecreaseNodeGroupsInGlobalReplicationGroup", err)
	if err != nil {
		return nil, err
	}

	return rm.setGlobalReplicationGroupOutput(desired, resp.GlobalReplicationGroup)
}

// modifyGlobalReplicationGroup sends a single logical change per call. AWS's
// ModifyGlobalReplicationGroup does not accept arbitrary combinations of
// fields in one request, so changes are grouped and prioritized:
//
//   - Description, AutomaticFailoverEnabled, and CacheNodeType are each
//     independent single-field changes, sent one at a time.
//   - Engine / EngineVersion / CacheParameterGroupName form one logical
//     "engine upgrade" change and must be sent together: a MAJOR upgrade (or
//     any Engine change, e.g. redis -> valkey) requires all three
//     (Engine + EngineVersion + CacheParameterGroupName); a MINOR upgrade
//     requires Engine + EngineVersion (no parameter group).
//
// These groupings were verified empirically against the live API:
//   - Engine alone            -> "No modifications requested"
//   - Engine + EngineVersion  -> "Parameter group must be specified for major
//     engine version upgrade" (for a major bump)
//   - Engine + EngineVersion + CacheParameterGroupName -> accepted
//
// Only the first applicable change is sent per call. When more than one field
// changed, the function returns an explicit requeue so the remaining fields are
// applied on follow-up reconciles -- IsSynced() is status-based, so a fast
// change would otherwise return the group to a synced status and strand the
// rest of the delta. The delta is recomputed from the CR spec and live AWS
// state each reconcile, so this converges over multiple passes and is correct
// across controller restarts (no in-memory pending-change state).
//
// `latest` reflects observed AWS state: fields from Describe, AutomaticFailover
// derived from live member state, and CacheParameterGroupName restored from its
// last-requested annotation (Describe returns none of the latter two directly).
// Engine versions are compared with util.EngineVersionsMatch because AWS
// normalizes a requested "7.1" to "7.1.0" -- a plain string compare would treat
// them as different and re-issue the same upgrade on every reconcile.
func (rm *resourceManager) modifyGlobalReplicationGroup(
	ctx context.Context,
	desired *resource,
	latest *resource,
	globalId *string,
) (*resource, error) {
	input := &svcsdk.ModifyGlobalReplicationGroupInput{
		GlobalReplicationGroupId: globalId,
		ApplyImmediately:         aws.Bool(true),
	}

	// Detect each independent pending change once. Engine and EngineVersion
	// form a single logical "engine upgrade" change (sent together), so they
	// are combined into engineGroupChanged.
	descChanged := desired.ko.Spec.Description != nil &&
		!strPtrEqual(desired.ko.Spec.Description, latest.ko.Spec.Description)
	afeChanged := desired.ko.Spec.AutomaticFailoverEnabled != nil &&
		(latest.ko.Spec.AutomaticFailoverEnabled == nil ||
			*desired.ko.Spec.AutomaticFailoverEnabled != *latest.ko.Spec.AutomaticFailoverEnabled)
	nodeTypeChanged := desired.ko.Spec.CacheNodeType != nil &&
		!strPtrEqual(desired.ko.Spec.CacheNodeType, latest.ko.Spec.CacheNodeType)
	engineChanged := desired.ko.Spec.Engine != nil &&
		!strPtrEqual(desired.ko.Spec.Engine, latest.ko.Spec.Engine)
	versionChanged := desired.ko.Spec.EngineVersion != nil &&
		(latest.ko.Spec.EngineVersion == nil ||
			!util.EngineVersionsMatch(*desired.ko.Spec.EngineVersion, *latest.ko.Spec.EngineVersion))
	engineGroupChanged := engineChanged || versionChanged

	// Count independent pending changes. AWS accepts only one per call, so if
	// more than one is pending we apply the first and requeue explicitly for
	// the rest (see below). Note the engine-upgrade group is only ever reached
	// in the switch when it is the *sole* pending change (the fields ahead of
	// it are all false), so pending>1 never coincides with sending
	// CacheParameterGroupName -- the one field whose annotation must persist.
	pending := 0
	for _, c := range []bool{descChanged, afeChanged, nodeTypeChanged, engineGroupChanged} {
		if c {
			pending++
		}
	}

	switch {
	case descChanged:
		input.GlobalReplicationGroupDescription = desired.ko.Spec.Description
	case afeChanged:
		input.AutomaticFailoverEnabled = desired.ko.Spec.AutomaticFailoverEnabled
	case nodeTypeChanged:
		input.CacheNodeType = desired.ko.Spec.CacheNodeType
	case engineGroupChanged:
		// Engine upgrade: send Engine + EngineVersion together. A major upgrade
		// (or any Engine change) additionally requires CacheParameterGroupName.
		if desired.ko.Spec.Engine != nil {
			input.Engine = desired.ko.Spec.Engine
		}
		if desired.ko.Spec.EngineVersion != nil {
			input.EngineVersion = desired.ko.Spec.EngineVersion
		}
		if engineChanged || isMajorVersionUpgrade(desired.ko.Spec.EngineVersion, latest.ko.Spec.EngineVersion) {
			if desired.ko.Spec.CacheParameterGroupName == nil {
				return nil, ackerr.NewTerminalError(errors.New(
					"a major engine version upgrade requires spec.cacheParameterGroupName to be set"))
			}
			input.CacheParameterGroupName = desired.ko.Spec.CacheParameterGroupName
		}
	case desired.ko.Spec.CacheParameterGroupName != nil &&
		!strPtrEqual(desired.ko.Spec.CacheParameterGroupName, latest.ko.Spec.CacheParameterGroupName):
		// A parameter group change on its own (not accompanying an engine
		// upgrade) is only valid as part of a major upgrade; AWS rejects it
		// otherwise. This is a permanent user misconfiguration, so surface it
		// as terminal (recovers when the user corrects the spec) rather than
		// requeuing forever.
		return nil, ackerr.NewTerminalError(errors.New(
			"spec.cacheParameterGroupName can only be changed as part of a major engine version upgrade"))
	default:
		// A Spec delta triggered Update but none of the fields we can modify
		// changed (e.g. a change to an immutable field). Return the observed
		// state unchanged -- returning nil here would nil-deref in the runtime's
		// post-Update patch step.
		return latest, nil
	}

	resp, err := rm.sdkapi.ModifyGlobalReplicationGroup(ctx, input)
	rm.metrics.RecordAPICall("UPDATE", "ModifyGlobalReplicationGroup", err)
	if err != nil {
		return nil, err
	}

	// Only one change is applied per call. If more than one field changed, we
	// applied the first; requeue explicitly so the rest are applied on
	// follow-up reconciles. We cannot rely on the resource staying non-synced
	// to be requeued: a fast change (e.g. Description) returns the group to a
	// steady status almost immediately, so IsSynced() would report true and
	// the runtime would stop reconciling with the remaining delta unapplied.
	// The delta is recomputed from the CR spec and live AWS state each pass, so
	// this converges and is correct across controller restarts. Returning a
	// requeue skips the post-Update spec/annotation patch, but that is safe
	// here: the only annotated field is CacheParameterGroupName, which is only
	// sent when it is the sole pending change (pending==1, no requeue).
	out, err := rm.setGlobalReplicationGroupOutput(desired, resp.GlobalReplicationGroup)
	if err != nil {
		return nil, err
	}
	if pending > 1 {
		return out, ackrequeue.NeededAfter(
			errors.New("applied one change; additional field updates pending"),
			ackrequeue.DefaultRequeueAfterDuration,
		)
	}
	return out, nil
}

// isMajorVersionUpgrade reports whether moving from latest to desired engine
// version changes the major version component (e.g. 7.x -> 8.x). A nil desired
// version is not an upgrade; a nil latest version is treated conservatively as
// a major change so the parameter group is included.
func isMajorVersionUpgrade(desired, latest *string) bool {
	if desired == nil {
		return false
	}
	if latest == nil {
		return true
	}
	dMaj := majorVersion(*desired)
	lMaj := majorVersion(*latest)
	return dMaj >= 0 && lMaj >= 0 && dMaj > lMaj
}

// majorVersion extracts the leading major-version integer from strings like
// "7.1", "7.1.0", or "8.x". Returns -1 if it cannot be parsed.
func majorVersion(v string) int {
	parts := strings.Split(v, ".")
	if len(parts) == 0 {
		return -1
	}
	maj, err := strconv.Atoi(parts[0])
	if err != nil {
		return -1
	}
	return maj
}

func strPtrEqual(a, b *string) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}

// deriveAutomaticFailoverEnabled infers the global datastore's automatic-failover
// setting from live member state, which the Describe API does return (the
// top-level AutomaticFailoverEnabled field is not returned). Returns nil if it
// cannot be determined -- e.g. members are not yet populated or a member omits
// the status -- so callers can avoid diffing against an unknown value.
func deriveAutomaticFailoverEnabled(
	ko *svcapitypes.GlobalReplicationGroup,
) *bool {
	if len(ko.Status.Members) == 0 {
		return nil
	}
	allEnabled := true
	for _, m := range ko.Status.Members {
		if m == nil || m.AutomaticFailover == nil {
			return nil
		}
		switch *m.AutomaticFailover {
		case string(svcsdktypes.AutomaticFailoverStatusEnabled),
			string(svcsdktypes.AutomaticFailoverStatusEnabling):
			// failover on for this member
		default:
			allEnabled = false
		}
	}
	return &allEnabled
}

// =============================================================================
// CUSTOM DESCRIBE SET OUTPUT
// =============================================================================

func (rm *resourceManager) CustomDescribeGlobalReplicationGroupsSetOutput(
	ctx context.Context,
	r *resource,
	resp *svcsdk.DescribeGlobalReplicationGroupsOutput,
	ko *svcapitypes.GlobalReplicationGroup,
) (*svcapitypes.GlobalReplicationGroup, error) {
	if len(resp.GlobalReplicationGroups) == 0 {
		return ko, nil
	}
	elem := resp.GlobalReplicationGroups[0]
	rm.customSetGlobalReplicationGroupOutput(ctx, r, &elem, ko)

	// NodeGroupCount is a custom spec field for declarative scaling.
	// Populate latest from the actual AWS state (count of GlobalNodeGroups)
	// so the delta only fires when the user's desired count differs.
	if len(resp.GlobalReplicationGroups[0].GlobalNodeGroups) > 0 {
		count := int64(len(resp.GlobalReplicationGroups[0].GlobalNodeGroups))
		ko.Spec.NodeGroupCount = &count
	} else {
		ko.Spec.NodeGroupCount = nil
	}

	// AutomaticFailoverEnabled is derivable from live member state (which
	// Describe returns), so infer it rather than tracking an annotation. Only
	// populate it when the user manages the field; fall back to the desired
	// value when it can't be determined from members, so we never diff against
	// an unknown state and issue a no-op Modify (which AWS rejects).
	if r.ko.Spec.AutomaticFailoverEnabled != nil {
		if derived := deriveAutomaticFailoverEnabled(ko); derived != nil {
			ko.Spec.AutomaticFailoverEnabled = derived
		} else {
			ko.Spec.AutomaticFailoverEnabled = r.ko.Spec.AutomaticFailoverEnabled
		}
	} else {
		ko.Spec.AutomaticFailoverEnabled = nil
	}

	// CacheParameterGroupName is not returned by Describe and is not derivable
	// (ElastiCache copies it per member), so restore it from the last-requested
	// annotation.
	restoreUnreadableFieldsFromAnnotations(r, ko)

	return ko, nil
}

func (rm *resourceManager) customSetGlobalReplicationGroupOutput(
	_ context.Context,
	_ *resource,
	grg *svcsdktypes.GlobalReplicationGroup,
	ko *svcapitypes.GlobalReplicationGroup,
) {
	if grg == nil {
		return
	}

	// Ensure the full ID is always in status
	if grg.GlobalReplicationGroupId != nil {
		ko.Status.GlobalReplicationGroupID = grg.GlobalReplicationGroupId
	}

	// Map GlobalNodeGroups
	if grg.GlobalNodeGroups != nil {
		nodeGroups := make([]*svcapitypes.GlobalNodeGroup, len(grg.GlobalNodeGroups))
		for i, ng := range grg.GlobalNodeGroups {
			nodeGroups[i] = &svcapitypes.GlobalNodeGroup{
				GlobalNodeGroupID: ng.GlobalNodeGroupId,
				Slots:             ng.Slots,
			}
		}
		ko.Status.GlobalNodeGroups = nodeGroups
	}

	// Map Members
	if grg.Members != nil {
		members := make([]*svcapitypes.GlobalReplicationGroupMember, len(grg.Members))
		for i, m := range grg.Members {
			afStr := string(m.AutomaticFailover)
			members[i] = &svcapitypes.GlobalReplicationGroupMember{
				AutomaticFailover:      &afStr,
				ReplicationGroupID:     m.ReplicationGroupId,
				ReplicationGroupRegion: m.ReplicationGroupRegion,
				Role:                   m.Role,
				Status:                 m.Status,
			}
		}
		ko.Status.Members = members
	}
}

// =============================================================================
// HELPERS
// =============================================================================

func isGlobalDeleting(r *resource) bool {
	if r == nil || r.ko.Status.Status == nil {
		return false
	}
	return *r.ko.Status.Status == statusDeleting
}

func isGlobalAvailable(r *resource) bool {
	if r == nil || r.ko.Status.Status == nil {
		return false
	}
	return *r.ko.Status.Status == statusAvailable
}

func isGlobalPrimaryOnly(r *resource) bool {
	if r == nil || r.ko.Status.Status == nil {
		return false
	}
	return *r.ko.Status.Status == statusPrimaryOnly
}

// findSecondaryMembers returns all members whose role is NOT "PRIMARY".
func findSecondaryMembers(members []svcsdktypes.GlobalReplicationGroupMember) []svcsdktypes.GlobalReplicationGroupMember {
	var secondaries []svcsdktypes.GlobalReplicationGroupMember
	for _, m := range members {
		if m.Role != nil && *m.Role != "PRIMARY" {
			secondaries = append(secondaries, m)
		}
	}
	return secondaries
}

// setGlobalReplicationGroupOutput maps the SDK response back into the resource.
// It also records the values of spec fields that Describe does not return, so
// subsequent reconciles can detect real user-driven changes.
func (rm *resourceManager) setGlobalReplicationGroupOutput(
	r *resource,
	grg *svcsdktypes.GlobalReplicationGroup,
) (*resource, error) {
	if r == nil || r.ko == nil {
		return nil, nil
	}
	// A successful Modify/Increase/Decrease should always return the group;
	// if it somehow didn't, return the input resource unchanged rather than a
	// nil resource (which would nil-deref in the runtime's post-Update patch).
	if grg == nil {
		return r, nil
	}
	ko := r.ko.DeepCopy()

	if grg.GlobalReplicationGroupId != nil {
		ko.Status.GlobalReplicationGroupID = grg.GlobalReplicationGroupId
	}
	if grg.Status != nil {
		ko.Status.Status = grg.Status
	}

	// The API call succeeded, so record what we sent for the fields we can't
	// read back from Describe.
	rm.setLastRequestedUnreadableFields(r, ko)

	rm.setStatusDefaults(ko)
	return &resource{ko}, nil
}

// getAnnotationsFields returns the annotations map on ko, initializing it from
// the desired resource's annotations if not already present.
func getAnnotationsFields(
	r *resource,
	ko *svcapitypes.GlobalReplicationGroup,
) map[string]string {
	if ko.ObjectMeta.Annotations != nil {
		return ko.ObjectMeta.Annotations
	}

	desiredAnnotations := r.ko.ObjectMeta.GetAnnotations()
	annotations := make(map[string]string)
	for k, v := range desiredAnnotations {
		annotations[k] = v
	}

	ko.ObjectMeta.Annotations = annotations
	return annotations
}

// setLastRequestedUnreadableFields records the values of spec fields that the
// Describe API does not return, so subsequent reconciles can detect real changes.
func (rm *resourceManager) setLastRequestedUnreadableFields(
	r *resource,
	ko *svcapitypes.GlobalReplicationGroup,
) {
	annotations := getAnnotationsFields(r, ko)

	if r.ko.Spec.CacheParameterGroupName != nil {
		annotations[AnnotationLastRequestedCPGN] = *r.ko.Spec.CacheParameterGroupName
	} else {
		annotations[AnnotationLastRequestedCPGN] = "null"
	}
}

// restoreUnreadableFieldsFromAnnotations populates CacheParameterGroupName, which
// Describe does not return and is not derivable from other state, using the value
// recorded in the annotation. This makes `latest` reflect what was actually last
// sent to AWS, so the delta only fires when the user changes their desired state.
func restoreUnreadableFieldsFromAnnotations(
	r *resource,
	ko *svcapitypes.GlobalReplicationGroup,
) {
	annotations := r.ko.ObjectMeta.GetAnnotations()
	if annotations == nil {
		// No annotations yet (first reconcile). Clear the field so any
		// user-specified value is treated as a change.
		ko.Spec.CacheParameterGroupName = nil
		return
	}

	if val, ok := annotations[AnnotationLastRequestedCPGN]; ok {
		if val == "null" {
			ko.Spec.CacheParameterGroupName = nil
		} else {
			ko.Spec.CacheParameterGroupName = &val
		}
	} else {
		ko.Spec.CacheParameterGroupName = nil
	}
}

// isInvalidGlobalReplicationGroupStateError checks if the error is the
// retryable InvalidGlobalReplicationGroupState error (GRG is modifying).
func isInvalidGlobalReplicationGroupStateError(err error) bool {
	if err == nil {
		return false
	}
	var oe *smithy.OperationError
	if errors.As(err, &oe) {
		return strings.Contains(oe.Error(), "InvalidGlobalReplicationGroupState")
	}
	return strings.Contains(err.Error(), "InvalidGlobalReplicationGroupState")
}
