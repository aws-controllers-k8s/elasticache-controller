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
	"sort"
	"strconv"
	"strings"

	ackcompare "github.com/aws-controllers-k8s/runtime/pkg/compare"
	ackerr "github.com/aws-controllers-k8s/runtime/pkg/errors"
	ackrequeue "github.com/aws-controllers-k8s/runtime/pkg/requeue"
	"github.com/aws/aws-sdk-go-v2/aws"
	svcsdk "github.com/aws/aws-sdk-go-v2/service/elasticache"
	svcsdktypes "github.com/aws/aws-sdk-go-v2/service/elasticache/types"
	smithy "github.com/aws/smithy-go"

	svcapitypes "github.com/aws-controllers-k8s/elasticache-controller/apis/v1alpha1"
	"github.com/aws-controllers-k8s/elasticache-controller/pkg/common"
	"github.com/aws-controllers-k8s/elasticache-controller/pkg/util"
)

const (
	// A global datastore reports "primary-only" while no secondary member is
	// attached and "available" once one is; both are steady states.
	statusAvailable   = "available"
	statusPrimaryOnly = "primary-only"
	statusDeleting    = "deleting"

	// memberAssociated is the member status of a secondary that is still joined to
	// the datastore. A member being removed simply disappears from the list.
	memberAssociated = "associated"

	rolePrimary = "PRIMARY"

	// codeInvalidState is returned while a mutation is already in flight.
	codeInvalidState = "InvalidGlobalReplicationGroupState"
)

var (
	requeueWaitWhileDeleting = ackrequeue.NeededAfter(
		errors.New("global replication group is deleting"),
		ackrequeue.DefaultRequeueAfterDuration,
	)
	requeueWaitWhileTransitioning = ackrequeue.NeededAfter(
		errors.New("global replication group is not in a steady state"),
		ackrequeue.DefaultRequeueAfterDuration,
	)
	requeueWaitWhileDisassociating = ackrequeue.NeededAfter(
		errors.New("waiting for secondary members to be disassociated"),
		ackrequeue.DefaultRequeueAfterDuration,
	)
	requeueAfterMutation = ackrequeue.NeededAfter(
		errors.New("applied a change; re-reading to establish observed state"),
		ackrequeue.DefaultRequeueAfterDuration,
	)
)

// unmanagedWhenUnsetPaths are fields a datastore inherits from its primary or that
// AWS fills in. The read path reports them whether or not the manifest declares
// them, so an undeclared one would otherwise be a nil-versus-observed difference no
// Modify can clear.
var unmanagedWhenUnsetPaths = []string{
	"Spec.AutomaticFailoverEnabled",
	"Spec.CacheNodeType",
	"Spec.Description",
	"Spec.Engine",
	"Spec.EngineVersion",
	"Spec.NodeGroupCount",
}

// modifyDelta drops differences that are not real: a field the manifest leaves
// unset, and the normalization AWS applies to engine fields. AWS expands a
// requested engine version ("7.1") to a fully qualified one ("7.1.0") and may echo
// the engine name with different casing, either of which would otherwise re-issue
// the same upgrade on every reconcile.
func modifyDelta(
	delta *ackcompare.Delta,
	desired *resource,
	latest *resource,
) {
	for _, path := range unmanagedWhenUnsetPaths {
		if delta.DifferentAt(path) && specFieldUnset(desired, path) {
			common.RemoveFromDelta(delta, path)
		}
	}

	if delta.DifferentAt("Spec.EngineVersion") &&
		engineVersionsEquivalent(desired.ko.Spec.EngineVersion, latest.ko.Spec.EngineVersion) {
		common.RemoveFromDelta(delta, "Spec.EngineVersion")
	}

	if delta.DifferentAt("Spec.Engine") &&
		desired.ko.Spec.Engine != nil && latest.ko.Spec.Engine != nil &&
		strings.EqualFold(*desired.ko.Spec.Engine, *latest.ko.Spec.Engine) {
		common.RemoveFromDelta(delta, "Spec.Engine")
	}
}

// engineVersionsEquivalent reports whether two engine versions mean the same
// thing, treating an absent or empty value as "not equivalent" so the difference
// survives and AWS gets to reject it. The empty check is load-bearing:
// util.EngineVersionsMatch indexes the last byte of its argument and panics on an
// empty string, and nothing in the CRD schema stops a user submitting one.
func engineVersionsEquivalent(desired, latest *string) bool {
	if desired == nil || latest == nil || *desired == "" || *latest == "" {
		return false
	}
	return util.EngineVersionsMatch(*desired, *latest)
}

// CustomDescribeGlobalReplicationGroupsSetOutput fills in the two Spec fields the
// Describe response cannot map directly. Like the generated mapping, it reports
// the observed value whether or not the manifest declares the field, so adoption
// imports it; modifyDelta is what keeps an undeclared field from producing a
// difference.
func (rm *resourceManager) CustomDescribeGlobalReplicationGroupsSetOutput(
	ctx context.Context,
	r *resource,
	resp *svcsdk.DescribeGlobalReplicationGroupsOutput,
	ko *svcapitypes.GlobalReplicationGroup,
) (*svcapitypes.GlobalReplicationGroup, error) {
	if len(resp.GlobalReplicationGroups) == 0 {
		return ko, nil
	}

	ko.Spec.NodeGroupCount = observedNodeGroupCount(
		len(resp.GlobalReplicationGroups[0].GlobalNodeGroups),
	)
	ko.Spec.AutomaticFailoverEnabled = observedAutomaticFailover(
		r.ko.Spec.AutomaticFailoverEnabled,
		ko.Status.Members,
	)

	// spec.cacheParameterGroupName is deliberately untouched. It is an input to an
	// engine upgrade rather than observable state -- ElastiCache copies it onto the
	// members and never reports it back on the datastore -- so `latest` keeps the
	// desired value and the field never produces a delta of its own.

	// Describe does not report tags, so they are read separately, as
	// replication_group does.
	if ko.Status.ACKResourceMetadata != nil && ko.Status.ACKResourceMetadata.ARN != nil {
		tags, err := rm.getTags(ctx, string(*ko.Status.ACKResourceMetadata.ARN))
		if err != nil {
			return nil, err
		}
		ko.Spec.Tags = tags
	}
	return ko, nil
}

// getTags retrieves the datastore's tags.
func (rm *resourceManager) getTags(
	ctx context.Context,
	resourceARN string,
) ([]*svcapitypes.Tag, error) {
	return util.GetTags(ctx, rm.sdkapi, rm.metrics, resourceARN)
}

// syncTags adds or removes tags so the datastore matches the desired tags.
// ModifyGlobalReplicationGroup does not accept tags.
func (rm *resourceManager) syncTags(
	ctx context.Context,
	desired *resource,
	latest *resource,
) error {
	return util.SyncTags(ctx, desired.ko.Spec.Tags, latest.ko.Spec.Tags,
		latest.ko.Status.ACKResourceMetadata, convertToOrderedACKTags, rm.sdkapi, rm.metrics)
}

// specFieldUnset reports whether the manifest leaves one of
// unmanagedWhenUnsetPaths unset.
func specFieldUnset(r *resource, path string) bool {
	s := r.ko.Spec
	switch path {
	case "Spec.AutomaticFailoverEnabled":
		return s.AutomaticFailoverEnabled == nil
	case "Spec.CacheNodeType":
		return s.CacheNodeType == nil
	case "Spec.Description":
		return s.Description == nil
	case "Spec.Engine":
		return s.Engine == nil
	case "Spec.EngineVersion":
		return s.EngineVersion == nil
	case "Spec.NodeGroupCount":
		return s.NodeGroupCount == nil
	}
	return false
}

// observedNodeGroupCount reports the datastore's shard count. A cluster-mode
// disabled datastore reports no global node groups, which is one logical shard
// rather than zero.
func observedNodeGroupCount(globalNodeGroups int) *int64 {
	count := int64(globalNodeGroups)
	if count == 0 {
		count = 1
	}
	return &count
}

// observedAutomaticFailover reports the datastore's failover state. When the
// state cannot be determined from the members it falls back to the desired value,
// so an unknown state never produces a delta and never triggers a Modify that AWS
// would reject as a no-op.
func observedAutomaticFailover(
	desired *bool,
	members []*svcapitypes.GlobalReplicationGroupMember,
) *bool {
	if derived := deriveAutomaticFailoverEnabled(members); derived != nil {
		return derived
	}
	return desired
}

// deriveAutomaticFailoverEnabled infers the datastore's failover setting from its
// members, which is the only place Describe exposes it. It returns nil when any
// member omits the value, because a partial view cannot be compared safely.
func deriveAutomaticFailoverEnabled(
	members []*svcapitypes.GlobalReplicationGroupMember,
) *bool {
	if len(members) == 0 {
		return nil
	}
	allEnabled := true
	for _, m := range members {
		if m == nil || m.AutomaticFailover == nil {
			return nil
		}
		switch *m.AutomaticFailover {
		case string(svcsdktypes.AutomaticFailoverStatusEnabled),
			string(svcsdktypes.AutomaticFailoverStatusEnabling):
		default:
			allEnabled = false
		}
	}
	return &allEnabled
}

// customUpdateGlobalReplicationGroup applies at most one AWS mutation per
// reconcile. Shard count moves through the dedicated Increase/Decrease APIs and
// every other field through ModifyGlobalReplicationGroup, which accepts only one
// logical change at a time.
func (rm *resourceManager) customUpdateGlobalReplicationGroup(
	ctx context.Context,
	desired *resource,
	latest *resource,
	delta *ackcompare.Delta,
) (*resource, error) {
	if !isSteadyState(latest) {
		return nil, requeueWaitWhileTransitioning
	}

	globalID := latest.ko.Status.GlobalReplicationGroupID
	if globalID == nil {
		return nil, ackerr.NewTerminalError(errors.New(
			"status.globalReplicationGroupID is not set, so the datastore cannot be updated"))
	}

	// SyncTags requeues after a tag change, so any other change waits a pass.
	if delta.DifferentAt("Spec.Tags") {
		return nil, rm.syncTags(ctx, desired, latest)
	}
	if delta.DifferentAt("Spec.NodeGroupCount") {
		return rm.updateNodeGroupCount(ctx, desired, latest, globalID)
	}
	return rm.modifyGlobalReplicationGroup(ctx, desired, latest, globalID, delta)
}

// nodeGroupCountAction names the scaling call a shard-count difference requires.
type nodeGroupCountAction string

const (
	nodeGroupCountNone     nodeGroupCountAction = ""
	nodeGroupCountIncrease nodeGroupCountAction = "increase"
	nodeGroupCountDecrease nodeGroupCountAction = "decrease"
)

var errObservedShardCountUnavailable = errors.New(
	"spec.nodeGroupCount cannot be reconciled because the observed shard count is unavailable")

// nodeGroupCountDecision reports which scaling call a shard-count difference needs.
// A difference means the user manages the field, so the read path will have set both
// counts; a missing one is a defect rather than something to report as synced, so it
// is an error instead of a silent no-op.
func nodeGroupCountDecision(desired, latest *int64) (nodeGroupCountAction, error) {
	if desired == nil || latest == nil {
		return nodeGroupCountNone, errObservedShardCountUnavailable
	}
	switch {
	case *desired > *latest:
		return nodeGroupCountIncrease, nil
	case *desired < *latest:
		return nodeGroupCountDecrease, nil
	}
	return nodeGroupCountNone, nil
}

// updateNodeGroupCount scales the datastore's shards through the dedicated
// Increase/Decrease APIs.
func (rm *resourceManager) updateNodeGroupCount(
	ctx context.Context,
	desired *resource,
	latest *resource,
	globalID *string,
) (*resource, error) {
	action, err := nodeGroupCountDecision(
		desired.ko.Spec.NodeGroupCount, latest.ko.Spec.NodeGroupCount,
	)
	if err != nil {
		return nil, ackerr.NewTerminalError(err)
	}

	want := int32(0)
	if desired.ko.Spec.NodeGroupCount != nil {
		want = int32(*desired.ko.Spec.NodeGroupCount)
	}

	switch action {
	case nodeGroupCountIncrease:
		input := &svcsdk.IncreaseNodeGroupsInGlobalReplicationGroupInput{
			GlobalReplicationGroupId: globalID,
			ApplyImmediately:         aws.Bool(true),
			NodeGroupCount:           aws.Int32(want),
		}
		resp, err := rm.sdkapi.IncreaseNodeGroupsInGlobalReplicationGroup(ctx, input)
		rm.metrics.RecordAPICall("UPDATE", "IncreaseNodeGroupsInGlobalReplicationGroup", err)
		if err != nil {
			return nil, err
		}
		out, err := rm.setResourceFromGlobalReplicationGroup(desired, resp.GlobalReplicationGroup)
		if err != nil {
			return nil, err
		}
		return out, requeueAfterMutation
	case nodeGroupCountDecrease:
		input := &svcsdk.DecreaseNodeGroupsInGlobalReplicationGroupInput{
			GlobalReplicationGroupId: globalID,
			ApplyImmediately:         aws.Bool(true),
			NodeGroupCount:           aws.Int32(want),
		}
		// The API requires naming which shards survive a shrink. Retaining the
		// lowest-ordered IDs is deterministic and safe for data, because
		// ElastiCache migrates slots off a shard before removing it, but it does
		// mean the user cannot choose which shards go.
		if retain := retainedNodeGroupIDs(latest.ko.Status.GlobalNodeGroups, int64(want)); len(retain) > 0 {
			input.GlobalNodeGroupsToRetain = retain
		}
		resp, err := rm.sdkapi.DecreaseNodeGroupsInGlobalReplicationGroup(ctx, input)
		rm.metrics.RecordAPICall("UPDATE", "DecreaseNodeGroupsInGlobalReplicationGroup", err)
		if err != nil {
			return nil, err
		}
		out, err := rm.setResourceFromGlobalReplicationGroup(desired, resp.GlobalReplicationGroup)
		if err != nil {
			return nil, err
		}
		return out, requeueAfterMutation
	}
	return latest, nil
}

// retainedNodeGroupIDs picks the shards to keep when shrinking, sorted so the
// choice does not depend on the order AWS happens to return them in.
func retainedNodeGroupIDs(
	nodeGroups []*svcapitypes.GlobalNodeGroup,
	keep int64,
) []string {
	if keep <= 0 {
		return nil
	}
	ids := make([]string, 0, len(nodeGroups))
	for _, ng := range nodeGroups {
		if ng != nil && ng.GlobalNodeGroupID != nil {
			ids = append(ids, *ng.GlobalNodeGroupID)
		}
	}
	sort.Strings(ids)
	if int64(len(ids)) <= keep {
		return ids
	}
	return ids[:keep]
}

// modifyChange names a single logical change that ModifyGlobalReplicationGroup
// will accept in one request.
type modifyChange string

const (
	modifyNone          modifyChange = ""
	modifyDescription   modifyChange = "description"
	modifyFailover      modifyChange = "automaticFailoverEnabled"
	modifyCacheNodeType modifyChange = "cacheNodeType"
	modifyEngineUpgrade modifyChange = "engineUpgrade"
)

// nextModify reports which logical change to send now. AWS rejects a request
// carrying more than one logical change, and counts engine and engine version as
// one upgrade rather than two, so those two may share a request while any other
// change waits for a requeue. Grouping does not make either field mandatory when
// the other is absent: a version-only bump sends no engine, which is the request
// replication_group's modify path builds for the same change.
func nextModify(delta *ackcompare.Delta) modifyChange {
	ordered := []struct {
		change  modifyChange
		pending bool
	}{
		{modifyDescription, delta.DifferentAt("Spec.Description")},
		{modifyFailover, delta.DifferentAt("Spec.AutomaticFailoverEnabled")},
		{modifyCacheNodeType, delta.DifferentAt("Spec.CacheNodeType")},
		{modifyEngineUpgrade, delta.DifferentAt("Spec.Engine") || delta.DifferentAt("Spec.EngineVersion")},
	}

	for _, c := range ordered {
		if c.pending {
			return c.change
		}
	}
	return modifyNone
}

// requiresCacheParameterGroup reports whether an engine upgrade has to carry
// spec.cacheParameterGroupName. AWS requires it for an engine-family change or a
// major version bump and rejects it on a minor one.
func requiresCacheParameterGroup(desiredEngine, latestEngine, desiredVersion, latestVersion *string) bool {
	if desiredEngine != nil && latestEngine != nil &&
		!strings.EqualFold(*desiredEngine, *latestEngine) {
		return true
	}
	return isMajorVersionUpgrade(desiredVersion, latestVersion)
}

// isMajorVersionUpgrade reports whether the major version component increases. An
// unknown current version is treated as a major change so the parameter group is
// included rather than omitted from a request that would need it.
func isMajorVersionUpgrade(desired, latest *string) bool {
	if desired == nil {
		return false
	}
	if latest == nil {
		return true
	}
	d, l := majorVersion(*desired), majorVersion(*latest)
	return d >= 0 && l >= 0 && d > l
}

// majorVersion extracts the leading major version from strings like "7.1",
// "7.1.0" or "8.x", returning -1 when it cannot be parsed.
func majorVersion(v string) int {
	major, err := strconv.Atoi(strings.Split(v, ".")[0])
	if err != nil {
		return -1
	}
	return major
}

func (rm *resourceManager) modifyGlobalReplicationGroup(
	ctx context.Context,
	desired *resource,
	latest *resource,
	globalID *string,
	delta *ackcompare.Delta,
) (*resource, error) {
	next := nextModify(delta)
	if next == modifyNone {
		// The delta is entirely in fields this API cannot change. Return desired
		// rather than nil, which would nil-deref in the runtime's post-update
		// patch, and rather than latest, whose read-path Spec the runtime would
		// then patch into the manifest.
		return desired, nil
	}

	input := &svcsdk.ModifyGlobalReplicationGroupInput{
		GlobalReplicationGroupId: globalID,
		ApplyImmediately:         aws.Bool(true),
	}
	switch next {
	case modifyDescription:
		input.GlobalReplicationGroupDescription = desired.ko.Spec.Description
	case modifyFailover:
		input.AutomaticFailoverEnabled = desired.ko.Spec.AutomaticFailoverEnabled
	case modifyCacheNodeType:
		input.CacheNodeType = desired.ko.Spec.CacheNodeType
	case modifyEngineUpgrade:
		input.Engine = desired.ko.Spec.Engine
		input.EngineVersion = desired.ko.Spec.EngineVersion
		if requiresCacheParameterGroup(
			desired.ko.Spec.Engine, latest.ko.Spec.Engine,
			desired.ko.Spec.EngineVersion, latest.ko.Spec.EngineVersion,
		) {
			if desired.ko.Spec.CacheParameterGroupName == nil {
				return nil, ackerr.NewTerminalError(errors.New(
					"spec.cacheParameterGroupName is required for a major engine version upgrade"))
			}
			input.CacheParameterGroupName = desired.ko.Spec.CacheParameterGroupName
		}
	}

	resp, err := rm.sdkapi.ModifyGlobalReplicationGroup(ctx, input)
	rm.metrics.RecordAPICall("UPDATE", "ModifyGlobalReplicationGroup", err)
	if err != nil {
		return nil, err
	}

	out, err := rm.setResourceFromGlobalReplicationGroup(desired, resp.GlobalReplicationGroup)
	if err != nil {
		return nil, err
	}
	// Requeue after every successful mutation, not just when more of the delta is
	// pending. The response echoes the datastore's pre-mutation field values and
	// can report a status the subsequent Describe contradicts, so without a
	// requeue nothing schedules the read that establishes observed state and the
	// resource holds stale values until the next resync period.
	return out, requeueAfterMutation
}

// customDeleteGlobalReplicationGroup removes the datastore, which AWS only allows
// once every secondary member has been disassociated. Each phase requeues, so the
// work is spread over reconciles rather than blocking on AWS in one pass.
func (rm *resourceManager) customDeleteGlobalReplicationGroup(
	ctx context.Context,
	r *resource,
) (*resource, error) {
	if statusIs(r, statusDeleting) {
		return r, requeueWaitWhileDeleting
	}
	// Only a steady state accepts the call; "creating" or "modifying" means a
	// mutation is in flight and AWS would reject the delete.
	if !isSteadyState(r) {
		return r, requeueWaitWhileTransitioning
	}

	globalID := r.ko.Status.GlobalReplicationGroupID
	if globalID == nil {
		return nil, ackerr.NewTerminalError(errors.New(
			"status.globalReplicationGroupID is not set, so the datastore cannot be deleted"))
	}

	descResp, err := rm.sdkapi.DescribeGlobalReplicationGroups(
		ctx,
		&svcsdk.DescribeGlobalReplicationGroupsInput{
			GlobalReplicationGroupId: globalID,
			ShowMemberInfo:           aws.Bool(true),
		},
	)
	rm.metrics.RecordAPICall("READ_ONE", "DescribeGlobalReplicationGroups", err)
	if err != nil {
		return nil, err
	}
	if len(descResp.GlobalReplicationGroups) == 0 {
		return nil, nil
	}

	if secondaries := findSecondaryMembers(descResp.GlobalReplicationGroups[0].Members); len(secondaries) > 0 {
		// Delete is rejected while any secondary is still listed, so this returns a
		// requeue whether or not a disassociation was issued this pass: a member
		// already on its way out just needs to be waited for.
		for _, secondary := range secondaries {
			if !canDisassociate(secondary) {
				continue
			}
			_, err := rm.sdkapi.DisassociateGlobalReplicationGroup(
				ctx,
				&svcsdk.DisassociateGlobalReplicationGroupInput{
					GlobalReplicationGroupId: globalID,
					ReplicationGroupId:       secondary.ReplicationGroupId,
					ReplicationGroupRegion:   secondary.ReplicationGroupRegion,
				},
			)
			rm.metrics.RecordAPICall("UPDATE", "DisassociateGlobalReplicationGroup", err)
			if err != nil {
				// A disassociation already under way races with the check above and
				// is retryable; anything else is not.
				if isInvalidGlobalReplicationGroupState(err) {
					return r, requeueWaitWhileDisassociating
				}
				return nil, err
			}
		}
		return r, requeueWaitWhileDisassociating
	}

	// The primary is retained: deleting the datastore must not delete the
	// ReplicationGroup this resource only references.
	_, err = rm.sdkapi.DeleteGlobalReplicationGroup(
		ctx,
		&svcsdk.DeleteGlobalReplicationGroupInput{
			GlobalReplicationGroupId:      globalID,
			RetainPrimaryReplicationGroup: aws.Bool(true),
		},
	)
	rm.metrics.RecordAPICall("DELETE", "DeleteGlobalReplicationGroup", err)
	if err != nil {
		return nil, err
	}
	return r, requeueWaitWhileDeleting
}

// findSecondaryMembers returns every non-primary member still listed on the
// datastore. It deliberately does not filter on member status: while any secondary
// is listed at all, AWS refuses to delete the datastore, so the caller must wait
// rather than conclude there is nothing left to disassociate.
func findSecondaryMembers(
	members []svcsdktypes.GlobalReplicationGroupMember,
) []svcsdktypes.GlobalReplicationGroupMember {
	var secondaries []svcsdktypes.GlobalReplicationGroupMember
	for _, m := range members {
		if m.Role == nil || strings.EqualFold(*m.Role, rolePrimary) {
			continue
		}
		secondaries = append(secondaries, m)
	}
	return secondaries
}

// canDisassociate reports whether a disassociation is worth issuing for a member.
// A member that is not associated is already leaving, and Disassociate requires
// both identifiers, so a partially reported member is waited out instead of being
// sent as a request AWS would reject.
func canDisassociate(m svcsdktypes.GlobalReplicationGroupMember) bool {
	if m.ReplicationGroupId == nil || m.ReplicationGroupRegion == nil {
		return false
	}
	return m.Status == nil || strings.EqualFold(*m.Status, memberAssociated)
}

// isInvalidGlobalReplicationGroupState reports whether AWS refused the call
// because the datastore is mid-transition, which is retryable. Matched on the
// typed fault and its error code rather than on message text, which changes.
func isInvalidGlobalReplicationGroupState(err error) bool {
	if err == nil {
		return false
	}
	var stateFault *svcsdktypes.InvalidGlobalReplicationGroupStateFault
	if errors.As(err, &stateFault) {
		return true
	}
	var apiErr smithy.APIError
	return errors.As(err, &apiErr) && apiErr.ErrorCode() == codeInvalidState
}

// setResourceFromGlobalReplicationGroup folds an Increase/Decrease/Modify
// response back into the resource. Only the identifier and status move: the
// response echoes the datastore's pre-mutation field values, and can report a
// status the subsequent Describe contradicts, so callers requeue to schedule the
// read that establishes observed state.
func (rm *resourceManager) setResourceFromGlobalReplicationGroup(
	r *resource,
	grg *svcsdktypes.GlobalReplicationGroup,
) (*resource, error) {
	if r == nil || r.ko == nil {
		return nil, nil
	}
	// A successful call always returns the datastore; if it somehow did not,
	// return the input unchanged rather than a nil resource, which would
	// nil-deref in the runtime's post-update patch.
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
	rm.setStatusDefaults(ko)
	return &resource{ko}, nil
}

func statusIs(r *resource, status string) bool {
	if r == nil || r.ko == nil || r.ko.Status.Status == nil {
		return false
	}
	return *r.ko.Status.Status == status
}

func isSteadyState(r *resource) bool {
	return statusIs(r, statusAvailable) || statusIs(r, statusPrimaryOnly)
}
