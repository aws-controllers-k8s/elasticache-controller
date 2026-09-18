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
	"fmt"
	"testing"

	ackcompare "github.com/aws-controllers-k8s/runtime/pkg/compare"
	svcsdktypes "github.com/aws/aws-sdk-go-v2/service/elasticache/types"
	smithy "github.com/aws/smithy-go"

	svcapitypes "github.com/aws-controllers-k8s/elasticache-controller/apis/v1alpha1"
)

func strp(s string) *string { return &s }
func boolp(b bool) *bool    { return &b }
func i64p(i int64) *int64   { return &i }

func fmtBoolp(b *bool) string {
	if b == nil {
		return "nil"
	}
	return fmt.Sprintf("%t", *b)
}

func fmtI64p(i *int64) string {
	if i == nil {
		return "nil"
	}
	return fmt.Sprintf("%d", *i)
}

// observedNodeGroupCount must never report a count for a field the user does not
// manage: doing so makes the delta fire and the runtime patch nodeGroupCount into
// a spec that never declared it.
func TestObservedNodeGroupCount(t *testing.T) {
	cases := []struct {
		name       string
		desired    *int64
		nodeGroups int
		want       *int64
	}{
		{"unmanaged, no shards reported -> nil", nil, 0, nil},
		{"unmanaged, shards reported -> nil", nil, 3, nil},
		{"managed, three shards -> 3", i64p(2), 3, i64p(3)},
		{"managed, matching count -> same", i64p(2), 2, i64p(2)},
		{"managed, cluster-mode disabled reports none -> 1", i64p(1), 0, i64p(1)},
		{"managed, cluster-mode disabled but user wants 2 -> 1 so a delta fires", i64p(2), 0, i64p(1)},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := observedNodeGroupCount(tc.desired, tc.nodeGroups)
			if fmtI64p(got) != fmtI64p(tc.want) {
				t.Fatalf("want %s, got %s", fmtI64p(tc.want), fmtI64p(got))
			}
		})
	}
}

func member(af *string) *svcapitypes.GlobalReplicationGroupMember {
	return &svcapitypes.GlobalReplicationGroupMember{AutomaticFailover: af}
}

func TestDeriveAutomaticFailoverEnabled(t *testing.T) {
	enabled := string(svcsdktypes.AutomaticFailoverStatusEnabled)
	enabling := string(svcsdktypes.AutomaticFailoverStatusEnabling)
	disabled := string(svcsdktypes.AutomaticFailoverStatusDisabled)
	disabling := string(svcsdktypes.AutomaticFailoverStatusDisabling)

	cases := []struct {
		name    string
		members []*svcapitypes.GlobalReplicationGroupMember
		want    *bool
	}{
		{"no members -> undeterminable", nil, nil},
		{"empty members -> undeterminable", []*svcapitypes.GlobalReplicationGroupMember{}, nil},
		{"all enabled -> true", []*svcapitypes.GlobalReplicationGroupMember{member(&enabled), member(&enabled)}, boolp(true)},
		{"enabling counts as enabled -> true", []*svcapitypes.GlobalReplicationGroupMember{member(&enabled), member(&enabling)}, boolp(true)},
		{"one disabled -> false", []*svcapitypes.GlobalReplicationGroupMember{member(&enabled), member(&disabled)}, boolp(false)},
		{"disabling -> false", []*svcapitypes.GlobalReplicationGroupMember{member(&disabling)}, boolp(false)},
		{"a member with no value -> undeterminable", []*svcapitypes.GlobalReplicationGroupMember{member(&enabled), member(nil)}, nil},
		{"a nil member -> undeterminable", []*svcapitypes.GlobalReplicationGroupMember{nil}, nil},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := deriveAutomaticFailoverEnabled(tc.members)
			if fmtBoolp(got) != fmtBoolp(tc.want) {
				t.Fatalf("want %s, got %s", fmtBoolp(tc.want), fmtBoolp(got))
			}
		})
	}
}

func TestObservedAutomaticFailover(t *testing.T) {
	enabled := string(svcsdktypes.AutomaticFailoverStatusEnabled)
	disabled := string(svcsdktypes.AutomaticFailoverStatusDisabled)

	cases := []struct {
		name    string
		desired *bool
		members []*svcapitypes.GlobalReplicationGroupMember
		want    *bool
	}{
		{"unmanaged -> nil regardless of members", nil, []*svcapitypes.GlobalReplicationGroupMember{member(&enabled)}, nil},
		{"managed and derivable -> derived value", boolp(true), []*svcapitypes.GlobalReplicationGroupMember{member(&disabled)}, boolp(false)},
		{"managed, derivable and agreeing -> no delta", boolp(true), []*svcapitypes.GlobalReplicationGroupMember{member(&enabled)}, boolp(true)},
		{"managed but undeterminable -> falls back to desired so no Modify is issued", boolp(false), nil, boolp(false)},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := observedAutomaticFailover(tc.desired, tc.members)
			if fmtBoolp(got) != fmtBoolp(tc.want) {
				t.Fatalf("want %s, got %s", fmtBoolp(tc.want), fmtBoolp(got))
			}
		})
	}
}

func nodeGroup(id string) *svcapitypes.GlobalNodeGroup {
	return &svcapitypes.GlobalNodeGroup{GlobalNodeGroupID: strp(id)}
}

func TestRetainedNodeGroupIDs(t *testing.T) {
	cases := []struct {
		name       string
		nodeGroups []*svcapitypes.GlobalNodeGroup
		keep       int64
		want       []string
	}{
		{"keep two of three, lowest ordered", []*svcapitypes.GlobalNodeGroup{nodeGroup("ng-03"), nodeGroup("ng-01"), nodeGroup("ng-02")}, 2, []string{"ng-01", "ng-02"}},
		{"order is independent of the API's ordering", []*svcapitypes.GlobalNodeGroup{nodeGroup("ng-02"), nodeGroup("ng-03"), nodeGroup("ng-01")}, 2, []string{"ng-01", "ng-02"}},
		{"keep more than exist -> all", []*svcapitypes.GlobalNodeGroup{nodeGroup("ng-01")}, 3, []string{"ng-01"}},
		{"keep none -> nil", []*svcapitypes.GlobalNodeGroup{nodeGroup("ng-01")}, 0, nil},
		{"no node groups -> nil", nil, 2, nil},
		{"entries without an id are skipped", []*svcapitypes.GlobalNodeGroup{nodeGroup("ng-01"), {}, nil}, 2, []string{"ng-01"}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := retainedNodeGroupIDs(tc.nodeGroups, tc.keep)
			if len(got) != len(tc.want) {
				t.Fatalf("want %v, got %v", tc.want, got)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Fatalf("want %v, got %v", tc.want, got)
				}
			}
		})
	}
}

// pendingModify is the guard against sending AWS more than one logical change per
// ModifyGlobalReplicationGroup request.
func TestPendingModify(t *testing.T) {
	cases := []struct {
		name        string
		paths       []string
		wantChange  modifyChange
		wantPending int
	}{
		{"nothing pending", nil, modifyNone, 0},
		{"description only", []string{"Spec.Description"}, modifyDescription, 1},
		{"failover only", []string{"Spec.AutomaticFailoverEnabled"}, modifyFailover, 1},
		{"node type only", []string{"Spec.CacheNodeType"}, modifyCacheNodeType, 1},
		{"engine version only", []string{"Spec.EngineVersion"}, modifyEngineUpgrade, 1},
		{"engine only", []string{"Spec.Engine"}, modifyEngineUpgrade, 1},
		{"engine and version are one logical change", []string{"Spec.Engine", "Spec.EngineVersion"}, modifyEngineUpgrade, 1},
		{"description wins and node type stays pending", []string{"Spec.CacheNodeType", "Spec.Description"}, modifyDescription, 2},
		{"three pending, description first", []string{"Spec.EngineVersion", "Spec.CacheNodeType", "Spec.Description"}, modifyDescription, 3},
		{"failover before node type", []string{"Spec.CacheNodeType", "Spec.AutomaticFailoverEnabled"}, modifyFailover, 2},
		{"a parameter group change alone is not a modifiable change", []string{"Spec.CacheParameterGroupName"}, modifyNone, 0},
		{"an unmodifiable field alone yields nothing", []string{"Spec.GlobalReplicationGroupIDSuffix"}, modifyNone, 0},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			delta := ackcompare.NewDelta()
			for _, p := range tc.paths {
				delta.Add(p, nil, nil)
			}
			gotChange, gotPending := pendingModify(delta)
			if gotChange != tc.wantChange {
				t.Fatalf("change: want %q, got %q", tc.wantChange, gotChange)
			}
			if gotPending != tc.wantPending {
				t.Fatalf("pending: want %d, got %d", tc.wantPending, gotPending)
			}
		})
	}
}

func TestRequiresCacheParameterGroup(t *testing.T) {
	cases := []struct {
		name               string
		dEngine, lEngine   *string
		dVersion, lVersion *string
		want               bool
	}{
		{"engine family change -> required", strp("valkey"), strp("redis"), strp("7.2"), strp("7.0"), true},
		{"engine casing only -> not a family change, minor bump", strp("Redis"), strp("redis"), strp("7.1"), strp("7.0"), false},
		{"major bump -> required", strp("redis"), strp("redis"), strp("8.0"), strp("7.1"), true},
		{"minor bump -> not required", strp("redis"), strp("redis"), strp("7.1"), strp("7.0"), false},
		{"same version -> not required", strp("redis"), strp("redis"), strp("7.0"), strp("7.0"), false},
		{"unknown current version -> required, erring towards including it", strp("redis"), strp("redis"), strp("7.0"), nil, true},
		{"no desired version -> not required", strp("redis"), strp("redis"), nil, strp("7.0"), false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := requiresCacheParameterGroup(tc.dEngine, tc.lEngine, tc.dVersion, tc.lVersion)
			if got != tc.want {
				t.Fatalf("want %t, got %t", tc.want, got)
			}
		})
	}
}

func TestIsMajorVersionUpgrade(t *testing.T) {
	cases := []struct {
		name            string
		desired, latest *string
		want            bool
	}{
		{"7.1 -> 8.0 is major", strp("8.0"), strp("7.1"), true},
		{"7.0 -> 7.1 is not", strp("7.1"), strp("7.0"), false},
		{"7.1.0 -> 7.1 is not", strp("7.1"), strp("7.1.0"), false},
		{"downgrade is not an upgrade", strp("6.2"), strp("7.0"), false},
		{"8.x parses as 8", strp("8.x"), strp("7.1"), true},
		{"nil desired -> false", nil, strp("7.0"), false},
		{"nil latest -> true", strp("7.0"), nil, true},
		{"unparseable desired -> false", strp("latest"), strp("7.0"), false},
		{"unparseable latest -> false", strp("8.0"), strp("unknown"), false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := isMajorVersionUpgrade(tc.desired, tc.latest)
			if got != tc.want {
				t.Fatalf("want %t, got %t", tc.want, got)
			}
		})
	}
}

func TestMajorVersion(t *testing.T) {
	cases := map[string]int{
		"7":      7,
		"7.1":    7,
		"7.1.0":  7,
		"8.x":    8,
		"":       -1,
		"x":      -1,
		"latest": -1,
	}
	for in, want := range cases {
		t.Run(fmt.Sprintf("%q", in), func(t *testing.T) {
			if got := majorVersion(in); got != want {
				t.Fatalf("want %d, got %d", want, got)
			}
		})
	}
}

func sdkMember(role, status *string) svcsdktypes.GlobalReplicationGroupMember {
	return svcsdktypes.GlobalReplicationGroupMember{Role: role, Status: status}
}

// findSecondaryMembers must report a secondary in ANY state: while one is listed at
// all, AWS refuses to delete the datastore, so concluding there is nothing left to
// wait for would drive a premature Delete on every reconcile.
func TestFindSecondaryMembers(t *testing.T) {
	associated := "associated"
	leaving := "deleting"

	cases := []struct {
		name    string
		members []svcsdktypes.GlobalReplicationGroupMember
		want    int
	}{
		{"no members", nil, 0},
		{"primary only", []svcsdktypes.GlobalReplicationGroupMember{sdkMember(strp("PRIMARY"), &associated)}, 0},
		{"one secondary", []svcsdktypes.GlobalReplicationGroupMember{sdkMember(strp("PRIMARY"), &associated), sdkMember(strp("SECONDARY"), &associated)}, 1},
		{"two secondaries", []svcsdktypes.GlobalReplicationGroupMember{sdkMember(strp("SECONDARY"), &associated), sdkMember(strp("SECONDARY"), &associated)}, 2},
		{"a secondary already leaving still counts", []svcsdktypes.GlobalReplicationGroupMember{sdkMember(strp("SECONDARY"), &leaving)}, 1},
		{"a secondary with no status counts", []svcsdktypes.GlobalReplicationGroupMember{sdkMember(strp("SECONDARY"), nil)}, 1},
		{"a member with no role is skipped", []svcsdktypes.GlobalReplicationGroupMember{sdkMember(nil, &associated)}, 0},
		{"primary matching is case-insensitive", []svcsdktypes.GlobalReplicationGroupMember{sdkMember(strp("primary"), &associated)}, 0},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := len(findSecondaryMembers(tc.members)); got != tc.want {
				t.Fatalf("want %d secondaries, got %d", tc.want, got)
			}
		})
	}
}

func fullMember(role, status, id, region *string) svcsdktypes.GlobalReplicationGroupMember {
	return svcsdktypes.GlobalReplicationGroupMember{
		Role: role, Status: status, ReplicationGroupId: id, ReplicationGroupRegion: region,
	}
}

func TestCanDisassociate(t *testing.T) {
	associated := "associated"
	leaving := "deleting"

	cases := []struct {
		name   string
		member svcsdktypes.GlobalReplicationGroupMember
		want   bool
	}{
		{"associated with both identifiers", fullMember(strp("SECONDARY"), &associated, strp("rg"), strp("us-west-2")), true},
		{"no status is treated as associated", fullMember(strp("SECONDARY"), nil, strp("rg"), strp("us-west-2")), true},
		{"already leaving -> wait it out", fullMember(strp("SECONDARY"), &leaving, strp("rg"), strp("us-west-2")), false},
		{"missing replication group id", fullMember(strp("SECONDARY"), &associated, nil, strp("us-west-2")), false},
		{"missing region", fullMember(strp("SECONDARY"), &associated, strp("rg"), nil), false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := canDisassociate(tc.member); got != tc.want {
				t.Fatalf("want %t, got %t", tc.want, got)
			}
		})
	}
}

// The shared util.EngineVersionsMatch indexes the last byte of its argument and
// panics on an empty string, which nothing in the CRD schema prevents a user from
// submitting.
func TestEngineVersionsEquivalent(t *testing.T) {
	cases := []struct {
		name            string
		desired, latest *string
		want            bool
	}{
		{"7.1 against a normalized 7.1.0", strp("7.1"), strp("7.1.0"), true},
		{"identical", strp("7.0"), strp("7.0"), true},
		{"a real upgrade", strp("8.0"), strp("7.1.0"), false},
		{"empty desired must not panic", strp(""), strp("7.0"), false},
		{"empty latest must not panic", strp("7.0"), strp(""), false},
		{"both empty must not panic", strp(""), strp(""), false},
		{"nil desired", nil, strp("7.0"), false},
		{"nil latest", strp("7.0"), nil, false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := engineVersionsEquivalent(tc.desired, tc.latest); got != tc.want {
				t.Fatalf("want %t, got %t", tc.want, got)
			}
		})
	}
}

// The delete path's retry decision must not depend on an AWS error message, which
// can change without notice.
func TestIsInvalidGlobalReplicationGroupState(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{"nil", nil, false},
		{"typed fault", &svcsdktypes.InvalidGlobalReplicationGroupStateFault{}, true},
		{"wrapped typed fault", fmt.Errorf("operation error: %w", &svcsdktypes.InvalidGlobalReplicationGroupStateFault{}), true},
		{"generic api error with the code", &smithy.GenericAPIError{Code: "InvalidGlobalReplicationGroupState"}, true},
		{"a different api error", &smithy.GenericAPIError{Code: "InvalidParameterValue"}, false},
		{"a plain error that merely mentions the code", errors.New("InvalidGlobalReplicationGroupState"), false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := isInvalidGlobalReplicationGroupState(tc.err); got != tc.want {
				t.Fatalf("want %t, got %t", tc.want, got)
			}
		})
	}
}

func grgResource(spec svcapitypes.GlobalReplicationGroupSpec, status *string) *resource {
	ko := &svcapitypes.GlobalReplicationGroup{Spec: spec}
	ko.Status.Status = status
	return &resource{ko: ko}
}

func TestModifyDeltaSuppressesNormalizedEngineVersion(t *testing.T) {
	cases := []struct {
		name            string
		desired, latest string
		stillDifferent  bool
	}{
		{"7.1 against a normalized 7.1.0 is not a difference", "7.1", "7.1.0", false},
		{"a real upgrade stays a difference", "8.0", "7.1.0", true},
		{"an empty version stays a difference and does not panic", "", "7.1.0", true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			desired := grgResource(svcapitypes.GlobalReplicationGroupSpec{EngineVersion: strp(tc.desired)}, nil)
			latest := grgResource(svcapitypes.GlobalReplicationGroupSpec{EngineVersion: strp(tc.latest)}, nil)
			delta := ackcompare.NewDelta()
			delta.Add("Spec.EngineVersion", desired.ko.Spec.EngineVersion, latest.ko.Spec.EngineVersion)

			modifyDelta(delta, desired, latest)

			if got := delta.DifferentAt("Spec.EngineVersion"); got != tc.stillDifferent {
				t.Fatalf("DifferentAt: want %t, got %t", tc.stillDifferent, got)
			}
		})
	}
}

func TestModifyDeltaSuppressesEngineCasing(t *testing.T) {
	desired := grgResource(svcapitypes.GlobalReplicationGroupSpec{Engine: strp("Redis")}, nil)
	latest := grgResource(svcapitypes.GlobalReplicationGroupSpec{Engine: strp("redis")}, nil)
	delta := ackcompare.NewDelta()
	delta.Add("Spec.Engine", desired.ko.Spec.Engine, latest.ko.Spec.Engine)

	modifyDelta(delta, desired, latest)

	if delta.DifferentAt("Spec.Engine") {
		t.Fatal("engine casing should not be a difference")
	}
}

// The scaling decision must never silently report "nothing to do" when a count is
// missing, which is how the prior attempt discarded a user's request while
// reporting Synced=True.
func TestNodeGroupCountDecision(t *testing.T) {
	cases := []struct {
		name            string
		desired, latest *int64
		wantAction      nodeGroupCountAction
		wantErr         bool
	}{
		{"scale up", i64p(3), i64p(2), nodeGroupCountIncrease, false},
		{"scale down", i64p(2), i64p(3), nodeGroupCountDecrease, false},
		{"already equal", i64p(2), i64p(2), nodeGroupCountNone, false},
		{"missing observed count is an error, not a no-op", i64p(2), nil, nodeGroupCountNone, true},
		{"missing desired count is an error, not a no-op", nil, i64p(2), nodeGroupCountNone, true},
		{"both missing", nil, nil, nodeGroupCountNone, true},
		{"cluster-mode disabled reports 1, user wants 2 -> increase and let AWS reject", i64p(2), i64p(1), nodeGroupCountIncrease, false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			action, err := nodeGroupCountDecision(tc.desired, tc.latest)
			if action != tc.wantAction {
				t.Fatalf("action: want %q, got %q", tc.wantAction, action)
			}
			if tc.wantErr && err == nil {
				t.Fatal("want an error, got nil")
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("want no error, got %v", err)
			}
		})
	}
}

func TestSteadyState(t *testing.T) {
	cases := []struct {
		name   string
		status *string
		want   bool
	}{
		{"primary-only is steady", strp("primary-only"), true},
		{"available is steady", strp("available"), true},
		{"modifying is not", strp("modifying"), false},
		{"creating is not", strp("creating"), false},
		{"deleting is not", strp("deleting"), false},
		{"unset is not", nil, false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := grgResource(svcapitypes.GlobalReplicationGroupSpec{}, tc.status)
			if got := isSteadyState(r); got != tc.want {
				t.Fatalf("want %t, got %t", tc.want, got)
			}
		})
	}
}
