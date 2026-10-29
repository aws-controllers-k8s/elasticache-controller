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
	"testing"

	ackv1alpha1 "github.com/aws-controllers-k8s/runtime/apis/core/v1alpha1"
	ackcompare "github.com/aws-controllers-k8s/runtime/pkg/compare"
	ackcondition "github.com/aws-controllers-k8s/runtime/pkg/condition"
	"github.com/aws/aws-sdk-go-v2/aws"
	svcsdk "github.com/aws/aws-sdk-go-v2/service/elasticache"
	corev1 "k8s.io/api/core/v1"

	svcapitypes "github.com/aws-controllers-k8s/elasticache-controller/apis/v1alpha1"
)

func newMemberTestRG(grgID *string) *resource {
	ko := &svcapitypes.ReplicationGroup{}
	if grgID != nil {
		ko.Status.GlobalReplicationGroupInfo = &svcapitypes.GlobalReplicationGroupInfo{
			GlobalReplicationGroupID:         grgID,
			GlobalReplicationGroupMemberRole: aws.String("PRIMARY"),
		}
	}
	return &resource{ko: ko}
}

func TestIsGlobalDatastoreMember(t *testing.T) {
	cases := map[string]struct {
		id   *string
		want bool
	}{
		"no info":  {nil, false},
		"empty id": {aws.String(""), false},
		"member":   {aws.String("sgaui-grg"), true},
	}
	for name, tc := range cases {
		if got := isGlobalDatastoreMember(newMemberTestRG(tc.id)); got != tc.want {
			t.Errorf("%s: got %t, want %t", name, got, tc.want)
		}
	}
	// Status present with no ID at all.
	r := newMemberTestRG(nil)
	r.ko.Status.GlobalReplicationGroupInfo = &svcapitypes.GlobalReplicationGroupInfo{}
	if isGlobalDatastoreMember(r) {
		t.Error("info without an ID: got member")
	}
}

func TestReconcileDatastoreOwnedFields(t *testing.T) {
	desired := newMemberTestRG(aws.String("sgaui-grg"))
	desired.ko.Spec.CacheNodeType = aws.String("cache.r6g.large")
	desired.ko.Spec.CacheParameterGroupName = aws.String("default.redis7")
	desired.ko.Spec.Description = aws.String("new")

	latest := newMemberTestRG(aws.String("sgaui-grg"))
	latest.ko.Spec.CacheNodeType = aws.String("cache.r6g.xlarge")
	latest.ko.Spec.CacheParameterGroupName = aws.String("global-datastore-redis7")
	latest.ko.Spec.Description = aws.String("old")

	delta := ackcompare.NewDelta()
	delta.Add("Spec.CacheNodeType", desired.ko.Spec.CacheNodeType, latest.ko.Spec.CacheNodeType)
	delta.Add("Spec.CacheParameterGroupName", desired.ko.Spec.CacheParameterGroupName, latest.ko.Spec.CacheParameterGroupName)
	delta.Add("Spec.EngineVersion", aws.String("7.0"), aws.String("7.1"))
	delta.Add("Spec.AutomaticFailoverEnabled", aws.Bool(false), aws.String("enabled"))
	delta.Add("Spec.Description", desired.ko.Spec.Description, latest.ko.Spec.Description)

	reconcileDatastoreOwnedFields(desired, latest, delta)

	for _, p := range datastoreOwnedSpecPaths {
		if delta.DifferentAt(p) {
			t.Errorf("%s still in delta", p)
		}
	}
	if !delta.DifferentAt("Spec.Description") {
		t.Error("Spec.Description was removed; only datastore-owned paths may be")
	}
	if got := *desired.ko.Spec.CacheNodeType; got != "cache.r6g.xlarge" {
		t.Errorf("CacheNodeType = %q, want the observed value", got)
	}
	if got := *desired.ko.Spec.CacheParameterGroupName; got != "global-datastore-redis7" {
		t.Errorf("CacheParameterGroupName = %q, want the observed value", got)
	}
	// The copy must not alias latest.
	*latest.ko.Spec.CacheNodeType = "changed"
	if *desired.ko.Spec.CacheNodeType != "cache.r6g.xlarge" {
		t.Error("desired.CacheNodeType aliases latest")
	}
}

func TestReconcileDatastoreOwnedFields_OnlyOwnedDiffsEmptiesSpecDelta(t *testing.T) {
	desired := newMemberTestRG(aws.String("sgaui-grg"))
	latest := newMemberTestRG(aws.String("sgaui-grg"))
	latest.ko.Spec.CacheParameterGroupName = aws.String("global-datastore-redis7")

	delta := ackcompare.NewDelta()
	delta.Add("Spec.CacheParameterGroupName", aws.String("default.redis7"), latest.ko.Spec.CacheParameterGroupName)

	reconcileDatastoreOwnedFields(desired, latest, delta)
	if delta.DifferentAt("Spec") {
		t.Errorf("delta still differs: %v", delta.Differences)
	}
}

func TestReconcileDatastoreOwnedFields_AdoptsEngineVersion(t *testing.T) {
	desired := newMemberTestRG(aws.String("sgaui-grg"))
	desired.ko.Spec.EngineVersion = aws.String("7.0")
	latest := newMemberTestRG(aws.String("sgaui-grg"))
	latest.ko.Spec.EngineVersion = aws.String("7.1.0")

	delta := ackcompare.NewDelta()
	delta.Add("Spec.EngineVersion", desired.ko.Spec.EngineVersion, latest.ko.Spec.EngineVersion)

	reconcileDatastoreOwnedFields(desired, latest, delta)
	if got := *desired.ko.Spec.EngineVersion; got != "7.1.0" {
		t.Errorf("EngineVersion = %q, want the observed value", got)
	}
	*latest.ko.Spec.EngineVersion = "changed"
	if *desired.ko.Spec.EngineVersion != "7.1.0" {
		t.Error("desired.EngineVersion aliases latest")
	}
}

func TestReconcileDatastoreOwnedFields_KeepsMatchingEngineVersion(t *testing.T) {
	// modifyDelta removed the "7.0" vs "7.0.7" difference, so the user's
	// declared form stays.
	desired := newMemberTestRG(aws.String("sgaui-grg"))
	desired.ko.Spec.EngineVersion = aws.String("7.0")
	latest := newMemberTestRG(aws.String("sgaui-grg"))
	latest.ko.Spec.EngineVersion = aws.String("7.0.7")

	reconcileDatastoreOwnedFields(desired, latest, ackcompare.NewDelta())
	if got := *desired.ko.Spec.EngineVersion; got != "7.0" {
		t.Errorf("EngineVersion = %q, want the declared value", got)
	}
}

func TestReconcileDatastoreOwnedFields_AdoptsAutomaticFailover(t *testing.T) {
	cases := map[string]struct {
		declared *bool
		status   *string
		want     *bool
	}{
		"enabled by datastore":  {aws.Bool(false), aws.String("enabled"), aws.Bool(true)},
		"disabled by datastore": {aws.Bool(true), aws.String("disabled"), aws.Bool(false)},
		"transitional status":   {aws.Bool(false), aws.String("enabling"), aws.Bool(false)},
		"no status":             {aws.Bool(false), nil, aws.Bool(false)},
		"undeclared":            {nil, aws.String("enabled"), nil},
	}
	for name, tc := range cases {
		desired := newMemberTestRG(aws.String("sgaui-grg"))
		desired.ko.Spec.AutomaticFailoverEnabled = tc.declared
		latest := newMemberTestRG(aws.String("sgaui-grg"))
		latest.ko.Status.AutomaticFailover = tc.status

		reconcileDatastoreOwnedFields(desired, latest, ackcompare.NewDelta())
		got := desired.ko.Spec.AutomaticFailoverEnabled
		if (got == nil) != (tc.want == nil) || (got != nil && *got != *tc.want) {
			t.Errorf("%s: AutomaticFailoverEnabled = %v, want %v", name, aws.ToBool(got), aws.ToBool(tc.want))
		}
	}
}

func TestStripDatastoreOwnedInput(t *testing.T) {
	newInput := func() *svcsdk.ModifyReplicationGroupInput {
		return &svcsdk.ModifyReplicationGroupInput{
			CacheNodeType:               aws.String("cache.r6g.large"),
			CacheParameterGroupName:     aws.String("default.redis7"),
			Engine:                      aws.String("redis"),
			EngineVersion:               aws.String("7.0"),
			AutomaticFailoverEnabled:    aws.Bool(true),
			AutoMinorVersionUpgrade:     aws.Bool(true),
			ReplicationGroupDescription: aws.String("kept"),
		}
	}

	in := newInput()
	stripDatastoreOwnedInput(in, newMemberTestRG(aws.String("sgaui-grg")))
	if in.CacheNodeType != nil || in.CacheParameterGroupName != nil || in.Engine != nil || in.EngineVersion != nil ||
		in.AutomaticFailoverEnabled != nil || in.AutoMinorVersionUpgrade != nil {
		t.Errorf("member input still carries datastore-owned fields: %+v", in)
	}
	if in.ReplicationGroupDescription == nil {
		t.Error("description was stripped")
	}

	in = newInput()
	stripDatastoreOwnedInput(in, newMemberTestRG(nil))
	if in.CacheNodeType == nil || in.AutoMinorVersionUpgrade == nil {
		t.Error("non-member input was modified")
	}
}

func TestReconcileDatastoreOwnedFields_AdoptsEngine(t *testing.T) {
	desired := newMemberTestRG(aws.String("sgaui-grg"))
	desired.ko.Spec.Engine = aws.String("redis")
	latest := newMemberTestRG(aws.String("sgaui-grg"))
	latest.ko.Spec.Engine = aws.String("valkey")

	delta := ackcompare.NewDelta()
	delta.Add("Spec.Engine", desired.ko.Spec.Engine, latest.ko.Spec.Engine)

	reconcileDatastoreOwnedFields(desired, latest, delta)
	if got := *desired.ko.Spec.Engine; got != "valkey" {
		t.Errorf("Engine = %q, want the observed value", got)
	}
	if delta.DifferentAt("Spec.Engine") {
		t.Error("Engine difference left in delta")
	}
}

func TestSyncDatastoreMemberAdvisory(t *testing.T) {
	r := newMemberTestRG(aws.String("sgaui-grg"))
	other := "Other"
	ackcondition.SetAdvisory(r, corev1.ConditionTrue, nil, &other)

	syncDatastoreMemberAdvisory(r.ko)
	c := ackcondition.AdvisoryWithReason(r, datastoreMemberAdvisoryReason)
	if c == nil || c.Status != corev1.ConditionTrue || c.Message == nil {
		t.Fatalf("member advisory not set: %+v", c)
	}
	stamp := c.LastTransitionTime
	syncDatastoreMemberAdvisory(r.ko)
	if got := ackcondition.AdvisoryWithReason(r, datastoreMemberAdvisoryReason); got.LastTransitionTime != stamp {
		t.Error("unchanged advisory was re-stamped")
	}

	r.ko.Status.GlobalReplicationGroupInfo = nil
	syncDatastoreMemberAdvisory(r.ko)
	if ackcondition.AdvisoryWithReason(r, datastoreMemberAdvisoryReason) != nil {
		t.Error("advisory kept after leaving the datastore")
	}
	if len(ackcondition.AllOfType(r, ackv1alpha1.ConditionTypeAdvisory)) != 1 {
		t.Error("unrelated advisory was removed")
	}
}
