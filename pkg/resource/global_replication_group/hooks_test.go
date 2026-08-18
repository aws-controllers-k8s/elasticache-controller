// Copyright Amazon.com Inc. or its affiliates. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License"). You may
// not use this file except in compliance with the License. A copy of the
// License is located at
//
//	http://aws.amazon.com/apache2.0/
//
// or in the "license" file accompanying this file. This file is distributed
// on an "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either
// express or implied. See the License for the specific language governing
// permissions and limitations under the License.

package global_replication_group

import (
	"testing"

	svcapitypes "github.com/aws-controllers-k8s/elasticache-controller/apis/v1alpha1"
)

func strp(s string) *string { return &s }

func member(af string) *svcapitypes.GlobalReplicationGroupMember {
	return &svcapitypes.GlobalReplicationGroupMember{AutomaticFailover: strp(af)}
}

func TestDeriveAutomaticFailoverEnabled(t *testing.T) {
	cases := []struct {
		name    string
		members []*svcapitypes.GlobalReplicationGroupMember
		want    *bool // nil means "undeterminable"
	}{
		{"no members -> nil", nil, nil},
		{"empty members -> nil", []*svcapitypes.GlobalReplicationGroupMember{}, nil},
		{"all enabled -> true", []*svcapitypes.GlobalReplicationGroupMember{member("enabled"), member("enabled")}, boolp(true)},
		{"enabled + enabling -> true", []*svcapitypes.GlobalReplicationGroupMember{member("enabled"), member("enabling")}, boolp(true)},
		{"one disabled -> false", []*svcapitypes.GlobalReplicationGroupMember{member("enabled"), member("disabled")}, boolp(false)},
		{"all disabled -> false", []*svcapitypes.GlobalReplicationGroupMember{member("disabled")}, boolp(false)},
		{"disabling -> false", []*svcapitypes.GlobalReplicationGroupMember{member("disabling")}, boolp(false)},
		{"nil status on a member -> nil", []*svcapitypes.GlobalReplicationGroupMember{member("enabled"), {AutomaticFailover: nil}}, nil},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ko := &svcapitypes.GlobalReplicationGroup{}
			ko.Status.Members = tc.members
			got := deriveAutomaticFailoverEnabled(ko)
			switch {
			case tc.want == nil && got != nil:
				t.Fatalf("want nil, got %v", *got)
			case tc.want != nil && got == nil:
				t.Fatalf("want %v, got nil", *tc.want)
			case tc.want != nil && got != nil && *tc.want != *got:
				t.Fatalf("want %v, got %v", *tc.want, *got)
			}
		})
	}
}

func boolp(b bool) *bool { return &b }
