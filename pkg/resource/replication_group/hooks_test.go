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

	"github.com/aws/aws-sdk-go-v2/aws"

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
