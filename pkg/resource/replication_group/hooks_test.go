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
	"reflect"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	svcsdk "github.com/aws/aws-sdk-go-v2/service/elasticache"
	svcsdktypes "github.com/aws/aws-sdk-go-v2/service/elasticache/types"
	"github.com/aws/smithy-go/middleware"

	ackmetrics "github.com/aws-controllers-k8s/runtime/pkg/metrics"

	svcapitypes "github.com/aws-controllers-k8s/elasticache-controller/apis/v1alpha1"
)

func cloudWatchDetails(logGroup string) *svcapitypes.DestinationDetails {
	return &svcapitypes.DestinationDetails{
		CloudWatchLogsDetails: &svcapitypes.CloudWatchLogsDestinationDetails{LogGroup: aws.String(logGroup)},
	}
}

func logDeliveryRequest(logType, logGroup string, enabled *bool) *svcapitypes.LogDeliveryConfigurationRequest {
	return &svcapitypes.LogDeliveryConfigurationRequest{
		DestinationDetails: cloudWatchDetails(logGroup),
		DestinationType:    aws.String("cloudwatch-logs"),
		Enabled:            enabled,
		LogFormat:          aws.String("text"),
		LogType:            aws.String(logType),
	}
}

func observedLogDelivery(logType, logGroup, status string) *svcapitypes.LogDeliveryConfiguration {
	return &svcapitypes.LogDeliveryConfiguration{
		DestinationDetails: cloudWatchDetails(logGroup),
		DestinationType:    aws.String("cloudwatch-logs"),
		LogFormat:          aws.String("text"),
		LogType:            aws.String(logType),
		Status:             aws.String(status),
	}
}

func pendingLogDelivery(logType, logGroup string) *svcapitypes.PendingLogDeliveryConfiguration {
	return &svcapitypes.PendingLogDeliveryConfiguration{
		DestinationDetails: cloudWatchDetails(logGroup),
		DestinationType:    aws.String("cloudwatch-logs"),
		LogFormat:          aws.String("text"),
		LogType:            aws.String(logType),
	}
}

func TestLogDeliveryRequiresUpdate(t *testing.T) {
	tests := []struct {
		name     string
		desired  []*svcapitypes.LogDeliveryConfigurationRequest
		observed []*svcapitypes.LogDeliveryConfiguration
		pending  []*svcapitypes.PendingLogDeliveryConfiguration
		want     bool
	}{
		{
			name: "adopted group already delivering both logs",
			desired: []*svcapitypes.LogDeliveryConfigurationRequest{
				logDeliveryRequest("slow-log", "/elasticache/rg", aws.Bool(true)),
				logDeliveryRequest("engine-log", "/elasticache/rg", aws.Bool(true)),
			},
			observed: []*svcapitypes.LogDeliveryConfiguration{
				observedLogDelivery("engine-log", "/elasticache/rg", "active"),
				observedLogDelivery("slow-log", "/elasticache/rg", "active"),
			},
			want: false,
		},
		{
			name:    "enabled by default and nothing observed",
			desired: []*svcapitypes.LogDeliveryConfigurationRequest{logDeliveryRequest("slow-log", "/elasticache/rg", nil)},
			want:    true,
		},
		{
			name:     "different log group",
			desired:  []*svcapitypes.LogDeliveryConfigurationRequest{logDeliveryRequest("slow-log", "/elasticache/new", nil)},
			observed: []*svcapitypes.LogDeliveryConfiguration{observedLogDelivery("slow-log", "/elasticache/old", "active")},
			want:     true,
		},
		{
			name: "different destination type",
			desired: []*svcapitypes.LogDeliveryConfigurationRequest{{
				DestinationDetails: &svcapitypes.DestinationDetails{
					KinesisFirehoseDetails: &svcapitypes.KinesisFirehoseDestinationDetails{DeliveryStream: aws.String("stream")},
				},
				DestinationType: aws.String("kinesis-firehose"),
				LogFormat:       aws.String("json"),
				LogType:         aws.String("slow-log"),
			}},
			observed: []*svcapitypes.LogDeliveryConfiguration{observedLogDelivery("slow-log", "/elasticache/rg", "active")},
			want:     true,
		},
		{
			name:     "requested change still pending",
			desired:  []*svcapitypes.LogDeliveryConfigurationRequest{logDeliveryRequest("slow-log", "/elasticache/new", nil)},
			observed: []*svcapitypes.LogDeliveryConfiguration{observedLogDelivery("slow-log", "/elasticache/old", "modifying")},
			pending:  []*svcapitypes.PendingLogDeliveryConfiguration{pendingLogDelivery("slow-log", "/elasticache/new")},
			want:     false,
		},
		{
			name: "type and format left unset accept what the API reports",
			desired: []*svcapitypes.LogDeliveryConfigurationRequest{{
				DestinationDetails: &svcapitypes.DestinationDetails{CloudWatchLogsDetails: &svcapitypes.CloudWatchLogsDestinationDetails{LogGroup: aws.String("/elasticache/rg")}},
				LogType:            aws.String("slow-log"),
			}},
			observed: []*svcapitypes.LogDeliveryConfiguration{observedLogDelivery("slow-log", "/elasticache/rg", "active")},
			want:     false,
		},
		{
			name:     "still enabling",
			desired:  []*svcapitypes.LogDeliveryConfigurationRequest{logDeliveryRequest("slow-log", "/elasticache/rg", nil)},
			observed: []*svcapitypes.LogDeliveryConfiguration{observedLogDelivery("slow-log", "/elasticache/rg", "enabling")},
			want:     false,
		},
		{
			name:     "delivery in error is not re-requested",
			desired:  []*svcapitypes.LogDeliveryConfigurationRequest{logDeliveryRequest("slow-log", "/elasticache/rg", nil)},
			observed: []*svcapitypes.LogDeliveryConfiguration{observedLogDelivery("slow-log", "/elasticache/rg", "error")},
			want:     false,
		},
		{
			name:     "being disabled while still wanted",
			desired:  []*svcapitypes.LogDeliveryConfigurationRequest{logDeliveryRequest("slow-log", "/elasticache/rg", nil)},
			observed: []*svcapitypes.LogDeliveryConfiguration{observedLogDelivery("slow-log", "/elasticache/rg", "disabling")},
			want:     true,
		},
		{
			name:     "disable requested while active",
			desired:  []*svcapitypes.LogDeliveryConfigurationRequest{logDeliveryRequest("slow-log", "/elasticache/rg", aws.Bool(false))},
			observed: []*svcapitypes.LogDeliveryConfiguration{observedLogDelivery("slow-log", "/elasticache/rg", "active")},
			want:     true,
		},
		{
			name:     "disable requested while disabling",
			desired:  []*svcapitypes.LogDeliveryConfigurationRequest{logDeliveryRequest("slow-log", "/elasticache/rg", aws.Bool(false))},
			observed: []*svcapitypes.LogDeliveryConfiguration{observedLogDelivery("slow-log", "/elasticache/rg", "disabling")},
			want:     false,
		},
		{
			name:    "disable requested and nothing observed",
			desired: []*svcapitypes.LogDeliveryConfigurationRequest{logDeliveryRequest("slow-log", "/elasticache/rg", aws.Bool(false))},
			want:    false,
		},
		{
			name:    "disable requested while an enable is pending",
			desired: []*svcapitypes.LogDeliveryConfigurationRequest{logDeliveryRequest("slow-log", "/elasticache/rg", aws.Bool(false))},
			pending: []*svcapitypes.PendingLogDeliveryConfiguration{pendingLogDelivery("slow-log", "/elasticache/rg")},
			want:    true,
		},
		{
			name:     "request without a log type is sent so the API can reject it",
			desired:  []*svcapitypes.LogDeliveryConfigurationRequest{{DestinationType: aws.String("cloudwatch-logs")}},
			observed: []*svcapitypes.LogDeliveryConfiguration{observedLogDelivery("slow-log", "/elasticache/rg", "active")},
			want:     true,
		},
		{
			name:     "nothing desired",
			observed: []*svcapitypes.LogDeliveryConfiguration{observedLogDelivery("slow-log", "/elasticache/rg", "active")},
			want:     false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			desired := &resource{ko: &svcapitypes.ReplicationGroup{}}
			desired.ko.Spec.LogDeliveryConfigurations = tt.desired
			latest := &resource{ko: &svcapitypes.ReplicationGroup{}}
			latest.ko.Status.LogDeliveryConfigurations = tt.observed
			if tt.pending != nil {
				latest.ko.Status.PendingModifiedValues = &svcapitypes.ReplicationGroupPendingModifiedValues{
					LogDeliveryConfigurations: tt.pending,
				}
			}
			if got := logDeliveryRequiresUpdate(desired, latest); got != tt.want {
				t.Errorf("logDeliveryRequiresUpdate() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestModifyDeltaLogDeliveryIgnoresAnnotation(t *testing.T) {
	desired := &resource{ko: &svcapitypes.ReplicationGroup{}}
	desired.ko.Spec.LogDeliveryConfigurations = []*svcapitypes.LogDeliveryConfigurationRequest{
		logDeliveryRequest("slow-log", "/elasticache/rg", aws.Bool(true)),
	}
	latest := &resource{ko: &svcapitypes.ReplicationGroup{}}
	latest.ko.Status.LogDeliveryConfigurations = []*svcapitypes.LogDeliveryConfiguration{
		observedLogDelivery("slow-log", "/elasticache/rg", "active"),
	}

	delta := newResourceDelta(desired, latest)
	if delta.DifferentAt("Spec.LogDeliveryConfigurations") {
		t.Errorf("an applied log delivery configuration must not differ without the last-requested annotation: %v", delta.Differences)
	}
}
func TestUserGroupIDsDifference(t *testing.T) {
	tests := []struct {
		name       string
		desired    []*string
		latest     []*string
		wantAdd    []string
		wantRemove []string
	}{
		{
			name:       "a group added",
			desired:    aws.StringSlice([]string{"ug-a", "ug-b"}),
			latest:     aws.StringSlice([]string{"ug-a"}),
			wantAdd:    []string{"ug-b"},
			wantRemove: nil,
		},
		{
			name:       "a group removed",
			desired:    aws.StringSlice([]string{"ug-a"}),
			latest:     aws.StringSlice([]string{"ug-a", "ug-b"}),
			wantAdd:    nil,
			wantRemove: []string{"ug-b"},
		},
		{
			name:       "a group replaced",
			desired:    aws.StringSlice([]string{"ug-b"}),
			latest:     aws.StringSlice([]string{"ug-a"}),
			wantAdd:    []string{"ug-b"},
			wantRemove: []string{"ug-a"},
		},
		{
			name:       "same groups in another order are not re-sent",
			desired:    aws.StringSlice([]string{"ug-b", "ug-a"}),
			latest:     aws.StringSlice([]string{"ug-a", "ug-b"}),
			wantAdd:    nil,
			wantRemove: nil,
		},
		{
			name:       "all groups removed",
			desired:    nil,
			latest:     aws.StringSlice([]string{"ug-a"}),
			wantAdd:    nil,
			wantRemove: []string{"ug-a"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotAdd, gotRemove := userGroupIDsDifference(tt.desired, tt.latest)
			if !reflect.DeepEqual(gotAdd, tt.wantAdd) || !reflect.DeepEqual(gotRemove, tt.wantRemove) {
				t.Errorf("userGroupIDsDifference() = (%v, %v), want (%v, %v)", gotAdd, gotRemove, tt.wantAdd, tt.wantRemove)
			}
		})
	}
}

// The update hook used to gate the user group changes on
// DifferentAt("UserGroupIDs"), a path that never matches the Spec.UserGroupIDs
// difference the generated delta records.
func TestUserGroupIDsDeltaPath(t *testing.T) {
	desired := &resource{ko: &svcapitypes.ReplicationGroup{}}
	desired.ko.Spec.UserGroupIDs = aws.StringSlice([]string{"ug-a", "ug-b"})
	latest := &resource{ko: &svcapitypes.ReplicationGroup{}}
	latest.ko.Spec.UserGroupIDs = aws.StringSlice([]string{"ug-a"})

	delta := newResourceDelta(desired, latest)
	if !delta.DifferentAt("Spec.UserGroupIDs") {
		t.Fatalf("expected a Spec.UserGroupIDs difference, got %v", delta.Differences)
	}
	if delta.DifferentAt("UserGroupIDs") {
		t.Errorf("DifferentAt(\"UserGroupIDs\") matched; the hook must use the Spec prefixed path")
	}
}

func userGroupResources(desired, latest []string) (*resource, *resource) {
	d := &resource{ko: &svcapitypes.ReplicationGroup{}}
	d.ko.Spec.UserGroupIDs = aws.StringSlice(desired)
	l := &resource{ko: &svcapitypes.ReplicationGroup{}}
	l.ko.Spec.UserGroupIDs = aws.StringSlice(latest)
	return d, l
}

// The query serializer writes an empty non-nil list as an explicit empty
// parameter, so a side with nothing to send has to stay nil on the input.
func TestUpdateUserGroupIDsPayload(t *testing.T) {
	tests := []struct {
		name       string
		desired    []string
		latest     []string
		wantAdd    []string
		wantRemove []string
	}{
		{"pure add leaves the remove side nil", []string{"ug-a", "ug-b"}, []string{"ug-a"}, []string{"ug-b"}, nil},
		{"pure remove leaves the add side nil", []string{"ug-a"}, []string{"ug-a", "ug-b"}, nil, []string{"ug-b"}},
		{"replace fills both sides", []string{"ug-b"}, []string{"ug-a"}, []string{"ug-b"}, []string{"ug-a"}},
		{"no difference touches neither side", []string{"ug-a"}, []string{"ug-a"}, nil, nil},
	}
	rm := &resourceManager{}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			desired, latest := userGroupResources(tt.desired, tt.latest)
			input := &svcsdk.ModifyReplicationGroupInput{}
			rm.updateUserGroupIDsPayload(input, desired, latest, newResourceDelta(desired, latest))
			if !reflect.DeepEqual(input.UserGroupIdsToAdd, tt.wantAdd) || !reflect.DeepEqual(input.UserGroupIdsToRemove, tt.wantRemove) {
				t.Errorf("input = (%#v, %#v), want (%#v, %#v)", input.UserGroupIdsToAdd, input.UserGroupIdsToRemove, tt.wantAdd, tt.wantRemove)
			}
		})
	}
}

// ModifyReplicationGroup applies the user group change asynchronously and its
// response still lists the previous groups. The stale list must not reach the
// spec, or the next reconcile swaps the groups back.
func TestCustomModifyReplicationGroupSetOutputKeepsDesiredUserGroupIDs(t *testing.T) {
	desired, _ := userGroupResources([]string{"ug-b"}, nil)
	ko := desired.ko.DeepCopy()
	ko.Spec.UserGroupIDs = aws.StringSlice([]string{"ug-a"})
	resp := &svcsdk.ModifyReplicationGroupOutput{ReplicationGroup: &svcsdktypes.ReplicationGroup{
		ReplicationGroupId: aws.String("rg"),
		Status:             aws.String("modifying"),
		UserGroupIds:       []string{"ug-a"},
	}}

	rm := &resourceManager{}
	out, err := rm.CustomModifyReplicationGroupSetOutput(context.Background(), desired, resp, ko)
	if err != nil {
		t.Fatal(err)
	}
	if got := aws.ToStringSlice(out.Spec.UserGroupIDs); !reflect.DeepEqual(got, []string{"ug-b"}) {
		t.Errorf("Spec.UserGroupIDs = %v, want the desired [ug-b]", got)
	}
}

// stubSDKClient returns an ElastiCache client whose operations never reach the
// network: each call is answered by respond, and its input is recorded in calls
// under the operation name.
func stubSDKClient(calls map[string]interface{}, respond func(op string) interface{}) *svcsdk.Client {
	return svcsdk.New(svcsdk.Options{
		Region:      "us-west-2",
		Credentials: aws.AnonymousCredentials{},
		APIOptions: []func(*middleware.Stack) error{
			func(s *middleware.Stack) error {
				return s.Initialize.Add(middleware.InitializeMiddlewareFunc("stub",
					func(ctx context.Context, in middleware.InitializeInput, _ middleware.InitializeHandler) (
						middleware.InitializeOutput, middleware.Metadata, error,
					) {
						op := middleware.GetOperationName(ctx)
						calls[op] = in.Parameters
						return middleware.InitializeOutput{Result: respond(op)}, middleware.Metadata{}, nil
					}), middleware.Before)
			},
		},
	})
}

// The custom update paths (ModifyReplicationGroup for engine changes, replica
// count and shard changes) return a replication group that still reports the
// previous AutoMinorVersionUpgrade. If that value reached the spec, the runtime
// would patch it onto the CR and a change made together with one of these
// fields would be lost.
func TestCustomUpdatePathsKeepDesiredAutoMinorVersionUpgrade(t *testing.T) {
	staleRG := &svcsdktypes.ReplicationGroup{
		ReplicationGroupId:      aws.String("rg"),
		Status:                  aws.String("modifying"),
		AutoMinorVersionUpgrade: aws.Bool(false),
	}
	newPair := func() (*resource, *resource) {
		latest := &resource{ko: &svcapitypes.ReplicationGroup{
			Spec: svcapitypes.ReplicationGroupSpec{
				ReplicationGroupID:      aws.String("rg"),
				Engine:                  aws.String("valkey"),
				EngineVersion:           aws.String("8.1"),
				NumNodeGroups:           aws.Int64(1),
				ReplicasPerNodeGroup:    aws.Int64(1),
				AutoMinorVersionUpgrade: aws.Bool(false),
			},
			Status: svcapitypes.ReplicationGroupStatus{
				Status: aws.String("available"),
				NodeGroups: []*svcapitypes.NodeGroup{{
					NodeGroupID: aws.String("0001"),
					Status:      aws.String("available"),
					NodeGroupMembers: []*svcapitypes.NodeGroupMember{
						{CacheClusterID: aws.String("rg-001")},
						{CacheClusterID: aws.String("rg-002")},
					},
				}},
			},
		}}
		desired := &resource{ko: latest.ko.DeepCopy()}
		desired.ko.Spec.AutoMinorVersionUpgrade = aws.Bool(true)
		return desired, latest
	}

	tests := []struct {
		name   string
		op     string
		change func(desired *resource)
		call   func(rm *resourceManager, desired, latest *resource) (*resource, error)
	}{
		{
			name:   "engine version change",
			op:     "ModifyReplicationGroup",
			change: func(d *resource) { d.ko.Spec.EngineVersion = aws.String("8.2") },
			call: func(rm *resourceManager, d, l *resource) (*resource, error) {
				return rm.modifyReplicationGroup(context.TODO(), d, l, newResourceDelta(d, l))
			},
		},
		{
			name:   "replica count increase",
			op:     "IncreaseReplicaCount",
			change: func(d *resource) { d.ko.Spec.ReplicasPerNodeGroup = aws.Int64(2) },
			call: func(rm *resourceManager, d, l *resource) (*resource, error) {
				return rm.increaseReplicaCount(context.TODO(), d, l)
			},
		},
		{
			name:   "replica count decrease",
			op:     "DecreaseReplicaCount",
			change: func(d *resource) { d.ko.Spec.ReplicasPerNodeGroup = aws.Int64(0) },
			call: func(rm *resourceManager, d, l *resource) (*resource, error) {
				return rm.decreaseReplicaCount(context.TODO(), d, l)
			},
		},
		{
			name:   "shard count change",
			op:     "ModifyReplicationGroupShardConfiguration",
			change: func(d *resource) { d.ko.Spec.NumNodeGroups = aws.Int64(2) },
			call: func(rm *resourceManager, d, l *resource) (*resource, error) {
				return rm.updateShardConfiguration(context.TODO(), d, l)
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			calls := map[string]interface{}{}
			rm := &resourceManager{
				metrics: ackmetrics.NewMetrics("elasticache"),
				sdkapi: stubSDKClient(calls, func(op string) interface{} {
					switch op {
					case "DescribeCacheClusters":
						return &svcsdk.DescribeCacheClustersOutput{CacheClusters: []svcsdktypes.CacheCluster{{
							CacheClusterId: aws.String("rg-001"),
						}}}
					case "ModifyReplicationGroup":
						return &svcsdk.ModifyReplicationGroupOutput{ReplicationGroup: staleRG}
					case "IncreaseReplicaCount":
						return &svcsdk.IncreaseReplicaCountOutput{ReplicationGroup: staleRG}
					case "DecreaseReplicaCount":
						return &svcsdk.DecreaseReplicaCountOutput{ReplicationGroup: staleRG}
					case "ModifyReplicationGroupShardConfiguration":
						return &svcsdk.ModifyReplicationGroupShardConfigurationOutput{ReplicationGroup: staleRG}
					}
					t.Fatalf("unexpected call to %s", op)
					return nil
				}),
			}
			desired, latest := newPair()
			tt.change(desired)

			out, err := tt.call(rm, desired, latest)
			if err != nil || out == nil {
				t.Fatalf("update returned (%v, %v)", out, err)
			}
			if _, ok := calls[tt.op]; !ok {
				t.Fatalf("%s was not called, calls: %v", tt.op, calls)
			}
			if in, ok := calls["ModifyReplicationGroup"].(*svcsdk.ModifyReplicationGroupInput); ok {
				if in.AutoMinorVersionUpgrade == nil || !*in.AutoMinorVersionUpgrade {
					t.Errorf("ModifyReplicationGroup input AutoMinorVersionUpgrade = %v, want true", in.AutoMinorVersionUpgrade)
				}
			}
			if newResourceDelta(desired, out).DifferentAt("Spec.AutoMinorVersionUpgrade") {
				t.Errorf("returned Spec.AutoMinorVersionUpgrade = %v, want the desired true", *out.ko.Spec.AutoMinorVersionUpgrade)
			}
		})
	}
}

func TestSetAnnotationsFieldsDropsLegacyLogDeliveryAnnotation(t *testing.T) {
	desired := &resource{ko: &svcapitypes.ReplicationGroup{}}
	desired.ko.Spec.CacheNodeType = aws.String("cache.t3.micro")
	desired.ko.ObjectMeta.Annotations = map[string]string{
		annotationLastRequestedLDCs: "[]",
		"keep.example.com/me":       "yes",
	}
	ko := &svcapitypes.ReplicationGroup{}

	rm := &resourceManager{}
	rm.setAnnotationsFields(desired, ko)

	if _, ok := ko.ObjectMeta.Annotations[annotationLastRequestedLDCs]; ok {
		t.Errorf("legacy log delivery annotation was kept")
	}
	if ko.ObjectMeta.Annotations["keep.example.com/me"] != "yes" {
		t.Errorf("unrelated annotation was dropped")
	}
	if ko.ObjectMeta.Annotations[AnnotationLastRequestedCNT] != "cache.t3.micro" {
		t.Errorf("cache node type annotation = %q", ko.ObjectMeta.Annotations[AnnotationLastRequestedCNT])
	}
}

func TestNewResourceDeltaEngineCase(t *testing.T) {
	tests := []struct {
		name          string
		desired       *string
		latest        *string
		wantDifferent bool
	}{
		{"same engine in another case", aws.String("valKey"), aws.String("valkey"), false},
		{"same engine", aws.String("redis"), aws.String("redis"), false},
		{"different engine", aws.String("redis"), aws.String("valkey"), true},
		{"engine set but not observed", aws.String("valkey"), nil, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			desired := &resource{ko: &svcapitypes.ReplicationGroup{}}
			desired.ko.Spec.Engine = tt.desired
			latest := &resource{ko: &svcapitypes.ReplicationGroup{}}
			latest.ko.Spec.Engine = tt.latest
			if got := newResourceDelta(desired, latest).DifferentAt("Spec.Engine"); got != tt.wantDifferent {
				t.Errorf("DifferentAt(Spec.Engine) = %v, want %v", got, tt.wantDifferent)
			}
		})
	}
}
