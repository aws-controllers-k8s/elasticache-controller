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
	"context"
	"fmt"
	"testing"

	ackv1alpha1 "github.com/aws-controllers-k8s/runtime/apis/core/v1alpha1"
	ackcompare "github.com/aws-controllers-k8s/runtime/pkg/compare"
	ackcondition "github.com/aws-controllers-k8s/runtime/pkg/condition"
	"github.com/aws/aws-sdk-go-v2/aws"
	svcsdk "github.com/aws/aws-sdk-go-v2/service/elasticache"
	svcsdktypes "github.com/aws/aws-sdk-go-v2/service/elasticache/types"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"

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

func TestStripDatastoreOwnedInput(t *testing.T) {
	newInput := func() *svcsdk.ModifyReplicationGroupInput {
		return &svcsdk.ModifyReplicationGroupInput{
			CacheNodeType:               aws.String("cache.r6g.large"),
			CacheParameterGroupName:     aws.String("default.redis7"),
			Engine:                      aws.String("redis"),
			EngineVersion:               aws.String("7.0"),
			AutomaticFailoverEnabled:    aws.Bool(true),
			AutoMinorVersionUpgrade:     aws.Bool(true),
			TransitEncryptionEnabled:    aws.Bool(true),
			TransitEncryptionMode:       svcsdktypes.TransitEncryptionModePreferred,
			Durability:                  svcsdktypes.DurabilityAsync,
			PrimaryClusterId:            aws.String("node-0001"),
			ReplicationGroupDescription: aws.String("kept"),
		}
	}

	in := newInput()
	stripDatastoreOwnedInput(in, newMemberTestRG(aws.String("sgaui-grg")))
	if in.CacheNodeType != nil || in.CacheParameterGroupName != nil || in.Engine != nil || in.EngineVersion != nil ||
		in.AutomaticFailoverEnabled != nil || in.AutoMinorVersionUpgrade != nil ||
		in.TransitEncryptionEnabled != nil || in.TransitEncryptionMode != "" ||
		in.Durability != "" || in.PrimaryClusterId != nil {
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

// nodeGroups returns n observed node groups, as Describe reports after a reshard.
func nodeGroups(n int) []*svcapitypes.NodeGroup {
	ngs := make([]*svcapitypes.NodeGroup, 0, n)
	for i := 0; i < n; i++ {
		ngs = append(ngs, &svcapitypes.NodeGroup{})
	}
	return ngs
}

// memberDeltaFixtures returns a desired and latest pair in which every
// datastore-owned field differs, plus the description.
func memberDeltaFixtures(grgID *string) (*resource, *resource) {
	desired := newMemberTestRG(grgID)
	desired.ko.Spec.Description = aws.String("new")
	desired.ko.Spec.CacheNodeType = aws.String("cache.r6g.large")
	desired.ko.Spec.CacheParameterGroupName = aws.String("default.redis7")
	desired.ko.Spec.Engine = aws.String("redis")
	desired.ko.Spec.EngineVersion = aws.String("7.0")
	desired.ko.Spec.AutomaticFailoverEnabled = aws.Bool(false)
	desired.ko.Spec.AutoMinorVersionUpgrade = aws.Bool(false)
	desired.ko.Spec.TransitEncryptionEnabled = aws.Bool(false)
	desired.ko.Spec.Durability = aws.String("async")
	desired.ko.Spec.NumNodeGroups = aws.Int64(1)
	desired.ko.Spec.PrimaryClusterID = aws.String("node-0001")

	latest := newMemberTestRG(grgID)
	latest.ko.Spec.Description = aws.String("old")
	latest.ko.Spec.CacheNodeType = aws.String("cache.r6g.xlarge")
	latest.ko.Spec.CacheParameterGroupName = aws.String("global-datastore-redis7")
	latest.ko.Spec.Engine = aws.String("valkey")
	latest.ko.Spec.EngineVersion = aws.String("7.1")
	latest.ko.Spec.AutoMinorVersionUpgrade = aws.Bool(true)
	latest.ko.Spec.TransitEncryptionEnabled = aws.Bool(true)
	latest.ko.Spec.Durability = aws.String("disabled")
	latest.ko.Spec.NumNodeGroups = aws.Int64(2)
	latest.ko.Spec.PrimaryClusterID = aws.String("node-0001")
	latest.ko.Status.AutomaticFailover = aws.String("enabled")
	latest.ko.Status.NodeGroups = []*svcapitypes.NodeGroup{{
		NodeGroupMembers: []*svcapitypes.NodeGroupMember{
			{CacheClusterID: aws.String("node-0002"), CurrentRole: aws.String("primary")},
		},
	}}
	return desired, latest
}

func TestNewResourceDelta_MemberDropsDatastoreOwnedDifferences(t *testing.T) {
	desired, latest := memberDeltaFixtures(aws.String("sgaui-grg"))
	before := desired.ko.DeepCopy()

	delta := newResourceDelta(desired, latest)
	for _, p := range datastoreOwnedSpecPaths {
		if delta.DifferentAt(p) {
			t.Errorf("%s still in delta", p)
		}
	}
	if !delta.DifferentAt("Spec.Description") {
		t.Error("Spec.Description was dropped; only datastore-owned paths may be")
	}
	if !equality.Semantic.DeepEqual(before, desired.ko) {
		t.Error("desired was modified; declared values must stay in spec")
	}
}

func TestNewResourceDelta_NonMemberKeepsDifferences(t *testing.T) {
	desired, latest := memberDeltaFixtures(nil)
	delta := newResourceDelta(desired, latest)
	for _, p := range []string{"Spec.CacheNodeType", "Spec.AutomaticFailoverEnabled", "Spec.AutoMinorVersionUpgrade", "Spec.PrimaryClusterID"} {
		if !delta.DifferentAt(p) {
			t.Errorf("%s dropped for a non-member", p)
		}
	}
}

func TestDropDatastoreOwnedDifferences_NodeGroupConfiguration(t *testing.T) {
	cases := map[string]struct {
		declaredShards int
		wantDropped    bool
	}{
		"shard count differs": {declaredShards: 1, wantDropped: true},
		"replica-only change": {declaredShards: 2, wantDropped: false},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			desired := newMemberTestRG(aws.String("sgaui-grg"))
			for i := 0; i < tc.declaredShards; i++ {
				desired.ko.Spec.NodeGroupConfiguration = append(desired.ko.Spec.NodeGroupConfiguration,
					&svcapitypes.NodeGroupConfiguration{ReplicaCount: aws.Int64(2)})
			}
			latest := newMemberTestRG(aws.String("sgaui-grg"))
			latest.ko.Status.NodeGroups = nodeGroups(2)

			delta := ackcompare.NewDelta()
			delta.Add("Spec.NodeGroupConfiguration", desired.ko.Spec.NodeGroupConfiguration, nil)
			dropDatastoreOwnedDifferences(delta, desired, latest)
			if got := !delta.DifferentAt("Spec.NodeGroupConfiguration"); got != tc.wantDropped {
				t.Errorf("dropped = %v, want %v", got, tc.wantDropped)
			}
		})
	}
}

// availableRG returns a latest resource that passes CustomModifyReplicationGroup's
// availability checks, with n single-node shards.
func availableRG(grgID *string, n int) *resource {
	latest := newMemberTestRG(grgID)
	latest.ko.Status.Status = aws.String("available")
	for i := 0; i < n; i++ {
		id := fmt.Sprintf("node-%d", i)
		latest.ko.Status.NodeGroups = append(latest.ko.Status.NodeGroups, &svcapitypes.NodeGroup{
			Status:           aws.String("available"),
			NodeGroupMembers: []*svcapitypes.NodeGroupMember{{CacheClusterID: aws.String(id)}},
		})
		latest.ko.Status.MemberClusters = append(latest.ko.Status.MemberClusters, aws.String(id))
	}
	return latest
}

func TestCustomModifyReplicationGroup_MemberStaleShardCount(t *testing.T) {
	// The datastore resharded the member to 3 shards; spec still declares 2 and
	// the last-requested annotation matches spec, which for a standalone group
	// reads as a rolled-back reshard.
	desired := newMemberTestRG(aws.String("sgaui-grg"))
	desired.ko.Spec.NumNodeGroups = aws.Int64(2)
	desired.ko.Spec.Description = aws.String("new")
	desired.ko.SetAnnotations(map[string]string{AnnotationLastRequestedNNG: "2"})
	latest := availableRG(aws.String("sgaui-grg"), 3)

	delta := ackcompare.NewDelta()
	delta.Add("Spec.Description", desired.ko.Spec.Description, nil)

	// No AWS call is expected: rm has no client, so a reshard would panic.
	if _, err := (&resourceManager{}).CustomModifyReplicationGroup(context.TODO(), desired, latest, delta); err != nil {
		t.Fatalf("member with a datastore-set shard count went to error: %v", err)
	}
	if got := *desired.ko.Spec.NumNodeGroups; got != 2 {
		t.Errorf("NumNodeGroups = %d, want the declared 2 left in spec", got)
	}
}

func TestDisableFailoverFirst(t *testing.T) {
	for name, grgID := range map[string]*string{"member": aws.String("sgaui-grg"), "standalone": nil} {
		t.Run(name, func(t *testing.T) {
			desired := newMemberTestRG(grgID)
			desired.ko.Spec.AutomaticFailoverEnabled = aws.Bool(false)
			latest := newMemberTestRG(grgID)
			latest.ko.Status.AutomaticFailover = aws.String("enabled")
			if got, want := disableFailoverFirst(desired, latest), grgID == nil; got != want {
				t.Errorf("disableFailoverFirst = %v, want %v", got, want)
			}
		})
	}
}

func TestCustomModifyReplicationGroupSetOutput_MemberKeepsDeclaredValues(t *testing.T) {
	desired, _ := memberDeltaFixtures(aws.String("sgaui-grg"))
	resp := &svcsdk.ModifyReplicationGroupOutput{ReplicationGroup: &svcsdktypes.ReplicationGroup{}}
	// What the generated output code writes from the response.
	ko := desired.ko.DeepCopy()
	ko.Spec.CacheNodeType = aws.String("cache.r6g.xlarge")
	ko.Spec.Engine = aws.String("valkey")
	ko.Spec.TransitEncryptionEnabled = aws.Bool(true)

	got, err := (&resourceManager{}).CustomModifyReplicationGroupSetOutput(context.TODO(), desired, resp, ko)
	if err != nil {
		t.Fatal(err)
	}
	if *got.Spec.Engine != "redis" || *got.Spec.TransitEncryptionEnabled || *got.Spec.CacheNodeType != "cache.r6g.large" {
		t.Errorf("AWS values written into spec: engine=%s tls=%v nodeType=%s",
			*got.Spec.Engine, *got.Spec.TransitEncryptionEnabled, *got.Spec.CacheNodeType)
	}
}

func TestRestoreDatastoreOwnedSpec(t *testing.T) {
	desired, latest := memberDeltaFixtures(aws.String("sgaui-grg"))
	desired.ko.Spec.NodeGroupConfiguration = []*svcapitypes.NodeGroupConfiguration{{NodeGroupID: aws.String("0001")}}
	updated := latest.ko.DeepCopy()
	updated.Spec.ReplicasPerNodeGroup = aws.Int64(3)

	restoreDatastoreOwnedSpec(desired.ko, updated)
	want := desired.ko.Spec.DeepCopy()
	want.Description = updated.Spec.Description
	want.ReplicasPerNodeGroup = aws.Int64(3)
	if !equality.Semantic.DeepEqual(*want, updated.Spec) {
		t.Errorf("owned fields not restored, or other fields touched:\n%+v", updated.Spec)
	}
	*desired.ko.Spec.NodeGroupConfiguration[0].NodeGroupID = "changed"
	if *updated.Spec.NodeGroupConfiguration[0].NodeGroupID != "0001" {
		t.Error("restored NodeGroupConfiguration aliases desired")
	}
}
