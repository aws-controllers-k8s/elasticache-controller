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

package cache_cluster

import (
	"testing"

	ackcompare "github.com/aws-controllers-k8s/runtime/pkg/compare"
	ackmetrics "github.com/aws-controllers-k8s/runtime/pkg/metrics"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/elasticache"
	"github.com/stretchr/testify/assert"
	"go.uber.org/zap/zapcore"
	ctrlrtzap "sigs.k8s.io/controller-runtime/pkg/log/zap"

	svcapitypes "github.com/aws-controllers-k8s/elasticache-controller/apis/v1alpha1"
)

func resourceWithSpec(spec svcapitypes.CacheClusterSpec) *resource {
	return newResource(spec, svcapitypes.CacheClusterStatus{})
}

func newResource(spec svcapitypes.CacheClusterSpec, status svcapitypes.CacheClusterStatus) *resource {
	return &resource{
		ko: &svcapitypes.CacheCluster{
			Spec:   spec,
			Status: status,
		},
	}
}

func provideResourceManager() *resourceManager {
	zapOptions := ctrlrtzap.Options{
		Development: true,
		Level:       zapcore.InfoLevel,
	}
	fakeLogger := ctrlrtzap.New(ctrlrtzap.UseFlagOptions(&zapOptions))
	return &resourceManager{
		log:     fakeLogger,
		metrics: ackmetrics.NewMetrics("elasticache"),
	}
}

func TestCustomUpdateInput(t *testing.T) {
	tests := []struct {
		description string
		desired     *resource
		latest      *resource
		makeDelta   func() *ackcompare.Delta

		expectedPayload *elasticache.ModifyCacheClusterInput
		expectedErr     string
	}{
		{
			description: "no changes",
			desired: resourceWithSpec(svcapitypes.CacheClusterSpec{
				NumCacheNodes: aws.Int64(1),
			}),
			latest: resourceWithSpec(svcapitypes.CacheClusterSpec{
				NumCacheNodes: aws.Int64(1),
			}),
			makeDelta: ackcompare.NewDelta,

			expectedPayload: &elasticache.ModifyCacheClusterInput{},
		},
		{
			description: "increase NumCacheNodes with new PreferredAvailabilityZones",
			desired: resourceWithSpec(svcapitypes.CacheClusterSpec{
				NumCacheNodes:              aws.Int64(3),
				PreferredAvailabilityZones: aws.StringSlice([]string{"us-west-2a", "us-west-2b"}),
			}),
			latest: resourceWithSpec(svcapitypes.CacheClusterSpec{
				NumCacheNodes: aws.Int64(1),
			}),
			makeDelta: func() *ackcompare.Delta {
				var delta ackcompare.Delta
				delta.Add("Spec.NumCacheNodes", aws.Int64(3), aws.Int64(1))
				delta.Add("Spec.PreferredAvailabilityZones", aws.StringSlice([]string{"us-west-2a", "us-west-2b"}), nil)
				return &delta
			},

			expectedPayload: &elasticache.ModifyCacheClusterInput{
				NewAvailabilityZones: aws.StringSlice([]string{"us-west-2a", "us-west-2b"}),
			},
		},
		{
			description: "increase NumCacheNodes again with new PreferredAvailabilityZones",
			desired: resourceWithSpec(svcapitypes.CacheClusterSpec{
				NumCacheNodes:              aws.Int64(5),
				PreferredAvailabilityZones: aws.StringSlice([]string{"us-west-2a", "us-west-2b", "us-west-2c", "us-west-2b"}),
			}),
			latest: resourceWithSpec(svcapitypes.CacheClusterSpec{
				NumCacheNodes:              aws.Int64(3),
				PreferredAvailabilityZones: aws.StringSlice([]string{"us-west-2a", "us-west-2b"}),
			}),
			makeDelta: func() *ackcompare.Delta {
				var delta ackcompare.Delta
				delta.Add("Spec.NumCacheNodes", aws.Int64(5), aws.Int64(3))
				delta.Add("Spec.PreferredAvailabilityZones", aws.StringSlice([]string{"us-west-2a", "us-west-2b", "us-west-2c", "us-west-2b"}),
					aws.StringSlice([]string{"us-west-2a", "us-west-2b"}))
				return &delta
			},

			expectedPayload: &elasticache.ModifyCacheClusterInput{
				NewAvailabilityZones: aws.StringSlice([]string{"us-west-2c", "us-west-2b"}),
			},
		},
		{
			description: "decrease NumCacheNodes",
			desired: resourceWithSpec(svcapitypes.CacheClusterSpec{
				NumCacheNodes: aws.Int64(3),
			}),
			latest: resourceWithSpec(svcapitypes.CacheClusterSpec{
				NumCacheNodes: aws.Int64(5),
			}),
			makeDelta: func() *ackcompare.Delta {
				var delta ackcompare.Delta
				delta.Add("Spec.NumCacheNodes", aws.Int64(3), aws.Int64(5))
				return &delta
			},
			expectedPayload: &elasticache.ModifyCacheClusterInput{
				CacheNodeIdsToRemove: aws.StringSlice([]string{"0005", "0004"}),
			},
		},
		{
			description: "PreferredAvailabilityZones changed with no change in NumCacheNodes",
			desired: resourceWithSpec(svcapitypes.CacheClusterSpec{
				PreferredAvailabilityZones: aws.StringSlice([]string{"us-west-2c"}),
				NumCacheNodes:              aws.Int64(3),
			}),
			latest: resourceWithSpec(svcapitypes.CacheClusterSpec{
				NumCacheNodes: aws.Int64(3),
			}),
			makeDelta: func() *ackcompare.Delta {
				var delta ackcompare.Delta
				delta.Add("Spec.PreferredAvailabilityZones", aws.StringSlice([]string{"us-west-2a"}), nil)
				return &delta
			},
			expectedErr: "spec.preferredAvailabilityZones can only be changed when new nodes are being added via spec.numCacheNodes",
		},
		{
			description: "decrease NumCacheNodes when a modification is pending",
			desired: resourceWithSpec(svcapitypes.CacheClusterSpec{
				NumCacheNodes: aws.Int64(3),
			}),
			latest: newResource(svcapitypes.CacheClusterSpec{
				NumCacheNodes:              aws.Int64(5),
				PreferredAvailabilityZones: aws.StringSlice([]string{"us-west-2a", "us-west-2b"}),
			}, svcapitypes.CacheClusterStatus{
				PendingModifiedValues: &svcapitypes.PendingModifiedValues{
					NumCacheNodes: aws.Int64(7),
				},
			}),
			makeDelta: func() *ackcompare.Delta {
				var delta ackcompare.Delta
				delta.Add("Spec.NumCacheNodes", aws.Int64(3), aws.Int64(5))
				return &delta
			},

			expectedPayload: &elasticache.ModifyCacheClusterInput{
				CacheNodeIdsToRemove: aws.StringSlice([]string{"0007", "0006", "0005", "0004"}),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.description, func(t *testing.T) {
			assert := assert.New(t)
			rm := provideResourceManager()
			var input elasticache.ModifyCacheClusterInput
			err := rm.updateCacheClusterPayload(&input, tt.desired, tt.latest, tt.makeDelta())
			if tt.expectedErr != "" {
				assert.NotNil(err)
				assert.Contains(err.Error(), tt.expectedErr)
				return
			}
			assert.Nil(err)
			assert.Equal(tt.expectedPayload, &input)
		})
	}
}
