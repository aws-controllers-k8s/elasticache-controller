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
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"

	ackcompare "github.com/aws-controllers-k8s/runtime/pkg/compare"
	ackmetrics "github.com/aws-controllers-k8s/runtime/pkg/metrics"
	"github.com/aws/aws-sdk-go-v2/aws"
	svcsdk "github.com/aws/aws-sdk-go-v2/service/elasticache"

	svcapitypes "github.com/aws-controllers-k8s/elasticache-controller/apis/v1alpha1"
)

// apiCall is one recorded ElastiCache Query-protocol request.
type apiCall struct {
	action string
	params url.Values
}

// fakeElastiCache records each request and answers with canned XML, so the
// update and delete paths can be tested against the real SDK client.
type fakeElastiCache struct {
	calls     []apiCall
	responses map[string][]string // action -> response bodies, consumed in order
}

func (f *fakeElastiCache) Do(req *http.Request) (*http.Response, error) {
	body, _ := io.ReadAll(req.Body)
	params, _ := url.ParseQuery(string(body))
	action := params.Get("Action")
	f.calls = append(f.calls, apiCall{action: action, params: params})

	inner := ""
	if queue := f.responses[action]; len(queue) > 0 {
		inner, f.responses[action] = queue[0], queue[1:]
	}
	xml := "<" + action + "Response><" + action + "Result>" + inner + "</" + action +
		"Result><ResponseMetadata><RequestId>test</RequestId></ResponseMetadata></" +
		action + "Response>"
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/xml"}},
		Body:       io.NopCloser(strings.NewReader(xml)),
		Request:    req,
	}, nil
}

func (f *fakeElastiCache) actions() []string {
	out := []string{}
	for _, c := range f.calls {
		out = append(out, c.action)
	}
	return out
}

func newFakeResourceManager(fake *fakeElastiCache) *resourceManager {
	client := svcsdk.NewFromConfig(aws.Config{
		Region:      "us-west-2",
		Credentials: aws.AnonymousCredentials{},
		HTTPClient:  fake,
		Retryer:     func() aws.Retryer { return aws.NopRetryer{} },
	})
	return &resourceManager{sdkapi: client, metrics: ackmetrics.NewMetrics("elasticache")}
}

const grgXML = "<GlobalReplicationGroup><GlobalReplicationGroupId>ldgnf-grg</GlobalReplicationGroupId>" +
	"<Status>modifying</Status></GlobalReplicationGroup>"

func steadyGRG(spec svcapitypes.GlobalReplicationGroupSpec) *resource {
	r := grgResource(spec, nil)
	r.ko.Status.GlobalReplicationGroupID = strp("ldgnf-grg")
	r.ko.Status.Status = strp(statusPrimaryOnly)
	return r
}

// ModifyGlobalReplicationGroup accepts one logical change per request, and the
// response does not reflect the change, so each successful Modify must carry
// one field and requeue so the next pass reads the result and sends the next.
func TestCustomUpdate_OneChangePerModifyAndRequeues(t *testing.T) {
	fake := &fakeElastiCache{responses: map[string][]string{"ModifyGlobalReplicationGroup": {grgXML}}}
	desired := steadyGRG(svcapitypes.GlobalReplicationGroupSpec{
		Description:              strp("new"),
		AutomaticFailoverEnabled: boolp(false),
		CacheNodeType:            strp("cache.r6g.xlarge"),
	})
	latest := steadyGRG(svcapitypes.GlobalReplicationGroupSpec{
		Description:              strp("old"),
		AutomaticFailoverEnabled: boolp(true),
		CacheNodeType:            strp("cache.r6g.large"),
	})
	delta := ackcompare.NewDelta()
	delta.Add("Spec.Description", desired.ko.Spec.Description, latest.ko.Spec.Description)
	delta.Add("Spec.AutomaticFailoverEnabled", desired.ko.Spec.AutomaticFailoverEnabled, latest.ko.Spec.AutomaticFailoverEnabled)
	delta.Add("Spec.CacheNodeType", desired.ko.Spec.CacheNodeType, latest.ko.Spec.CacheNodeType)

	_, err := newFakeResourceManager(fake).customUpdateGlobalReplicationGroup(context.TODO(), desired, latest, delta)
	if err != requeueAfterMutation {
		t.Errorf("err = %v, want requeueAfterMutation", err)
	}
	if len(fake.calls) != 1 || fake.calls[0].action != "ModifyGlobalReplicationGroup" {
		t.Fatalf("calls = %v, want one ModifyGlobalReplicationGroup", fake.actions())
	}
	p := fake.calls[0].params
	if p.Get("GlobalReplicationGroupDescription") != "new" {
		t.Errorf("first change sent = %v, want the description", p)
	}
	for _, other := range []string{"AutomaticFailoverEnabled", "CacheNodeType"} {
		if p.Has(other) {
			t.Errorf("%s sent in the same request as the description", other)
		}
	}
}

// AWS requires CacheParameterGroupName on a major upgrade and rejects it on a
// minor one.
func TestModify_ParameterGroupOnlyOnMajorUpgrade(t *testing.T) {
	cases := map[string]struct {
		from, to string
		wantPG   bool
	}{
		"minor": {from: "7.0", to: "7.1", wantPG: false},
		"major": {from: "6.2", to: "7.0", wantPG: true},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			fake := &fakeElastiCache{responses: map[string][]string{"ModifyGlobalReplicationGroup": {grgXML}}}
			desired := steadyGRG(svcapitypes.GlobalReplicationGroupSpec{
				Engine:                  strp("redis"),
				EngineVersion:           strp(tc.to),
				CacheParameterGroupName: strp("default.redis7"),
			})
			latest := steadyGRG(svcapitypes.GlobalReplicationGroupSpec{
				Engine:        strp("redis"),
				EngineVersion: strp(tc.from),
			})
			delta := ackcompare.NewDelta()
			delta.Add("Spec.EngineVersion", desired.ko.Spec.EngineVersion, latest.ko.Spec.EngineVersion)

			_, err := newFakeResourceManager(fake).customUpdateGlobalReplicationGroup(context.TODO(), desired, latest, delta)
			if err != requeueAfterMutation {
				t.Fatalf("err = %v, want requeueAfterMutation", err)
			}
			p := fake.calls[0].params
			if got := p.Has("CacheParameterGroupName"); got != tc.wantPG {
				t.Errorf("CacheParameterGroupName sent = %v, want %v", got, tc.wantPG)
			}
			if p.Get("EngineVersion") != tc.to {
				t.Errorf("EngineVersion = %q, want %q", p.Get("EngineVersion"), tc.to)
			}
		})
	}
}

// AWS rejects deleting a datastore that still lists a secondary, so delete
// disassociates secondaries first, then deletes while retaining the primary.
func TestCustomDelete_DisassociatesThenDeletesRetainingPrimary(t *testing.T) {
	primary := "<GlobalReplicationGroupMember><ReplicationGroupId>rg-primary</ReplicationGroupId>" +
		"<ReplicationGroupRegion>us-west-2</ReplicationGroupRegion><Role>PRIMARY</Role>" +
		"<Status>associated</Status></GlobalReplicationGroupMember>"
	secondary := "<GlobalReplicationGroupMember><ReplicationGroupId>rg-secondary</ReplicationGroupId>" +
		"<ReplicationGroupRegion>us-east-1</ReplicationGroupRegion><Role>SECONDARY</Role>" +
		"<Status>associated</Status></GlobalReplicationGroupMember>"
	describe := func(members string) string {
		return "<GlobalReplicationGroups><GlobalReplicationGroup>" +
			"<GlobalReplicationGroupId>ldgnf-grg</GlobalReplicationGroupId><Status>available</Status>" +
			"<Members>" + members + "</Members></GlobalReplicationGroup></GlobalReplicationGroups>"
	}
	fake := &fakeElastiCache{responses: map[string][]string{
		"DescribeGlobalReplicationGroups":    {describe(primary + secondary), describe(primary)},
		"DisassociateGlobalReplicationGroup": {grgXML},
		"DeleteGlobalReplicationGroup":       {grgXML},
	}}
	rm := newFakeResourceManager(fake)
	r := steadyGRG(svcapitypes.GlobalReplicationGroupSpec{})
	r.ko.Status.Status = strp(statusAvailable)

	if _, err := rm.customDeleteGlobalReplicationGroup(context.TODO(), r); err != requeueWaitWhileDisassociating {
		t.Fatalf("first pass err = %v, want requeueWaitWhileDisassociating", err)
	}
	if got := fake.actions(); len(got) != 2 || got[1] != "DisassociateGlobalReplicationGroup" {
		t.Fatalf("first pass calls = %v, want Describe then Disassociate", got)
	}
	d := fake.calls[1].params
	if d.Get("ReplicationGroupId") != "rg-secondary" || d.Get("ReplicationGroupRegion") != "us-east-1" {
		t.Errorf("disassociated %v, want the secondary in us-east-1", d)
	}

	if _, err := rm.customDeleteGlobalReplicationGroup(context.TODO(), r); err != requeueWaitWhileDeleting {
		t.Fatalf("second pass err = %v, want requeueWaitWhileDeleting", err)
	}
	if got := fake.actions(); len(got) != 4 || got[3] != "DeleteGlobalReplicationGroup" {
		t.Fatalf("calls = %v, want Describe then Delete on the second pass", got)
	}
	if fake.calls[3].params.Get("RetainPrimaryReplicationGroup") != "true" {
		t.Error("delete did not retain the primary")
	}
}
