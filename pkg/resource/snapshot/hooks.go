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

package snapshot

import (
	"context"

	svcapitypes "github.com/aws-controllers-k8s/elasticache-controller/apis/v1alpha1"
	ackv1alpha1 "github.com/aws-controllers-k8s/runtime/apis/core/v1alpha1"
	ackcompare "github.com/aws-controllers-k8s/runtime/pkg/compare"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/elasticache"
	svcsdk "github.com/aws/aws-sdk-go-v2/service/elasticache"
	svcsdktypes "github.com/aws/aws-sdk-go-v2/service/elasticache/types"
	"github.com/aws/aws-sdk-go/aws/awserr"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func (rm *resourceManager) CustomCreateSnapshot(
	ctx context.Context,
	r *resource,
) (*resource, error) {
	if r.ko.Spec.SourceSnapshotName != nil {
		if r.ko.Spec.CacheClusterID != nil || r.ko.Spec.ReplicationGroupID != nil {
			return nil, awserr.New("InvalidParameterCombination", "Cannot specify CacheClusteId or "+
				"ReplicationGroupId while SourceSnapshotName is specified", nil)
		}

		input, err := rm.newCopySnapshotPayload(r)
		if err != nil {
			return nil, err
		}

		resp, respErr := rm.sdkapi.CopySnapshot(ctx, input)

		rm.metrics.RecordAPICall("CREATE", "CopySnapshot", respErr)
		if respErr != nil {
			return nil, respErr
		}
		// Merge in the information we read from the API call above to the copy of
		// the original Kubernetes object we passed to the function
		ko := r.ko.DeepCopy()

		if ko.Status.ACKResourceMetadata == nil {
			ko.Status.ACKResourceMetadata = &ackv1alpha1.ResourceMetadata{}
		}
		if resp.Snapshot.ARN != nil {
			arn := ackv1alpha1.AWSResourceName(*resp.Snapshot.ARN)
			ko.Status.ACKResourceMetadata.ARN = &arn
		}
		if resp.Snapshot.AutoMinorVersionUpgrade != nil {
			ko.Status.AutoMinorVersionUpgrade = resp.Snapshot.AutoMinorVersionUpgrade
		}
		if resp.Snapshot.AutomaticFailover != "" {
			ko.Status.AutomaticFailover = aws.String(string(resp.Snapshot.AutomaticFailover))
		}
		if resp.Snapshot.CacheClusterCreateTime != nil {
			cacheClusterCreateTime := metav1.Time{Time: *resp.Snapshot.CacheClusterCreateTime}
			ko.Status.CacheClusterCreateTime = &cacheClusterCreateTime
		}
		if resp.Snapshot.CacheNodeType != nil {
			ko.Status.CacheNodeType = resp.Snapshot.CacheNodeType
		}
		if resp.Snapshot.CacheParameterGroupName != nil {
			ko.Status.CacheParameterGroupName = resp.Snapshot.CacheParameterGroupName
		}
		if resp.Snapshot.CacheSubnetGroupName != nil {
			ko.Status.CacheSubnetGroupName = resp.Snapshot.CacheSubnetGroupName
		}
		if resp.Snapshot.Engine != nil {
			ko.Status.Engine = resp.Snapshot.Engine
		}
		if resp.Snapshot.EngineVersion != nil {
			ko.Status.EngineVersion = resp.Snapshot.EngineVersion
		}
		if resp.Snapshot.NodeSnapshots != nil {
			f11 := []*svcapitypes.NodeSnapshot{}
			for _, f11iter := range resp.Snapshot.NodeSnapshots {
				f11elem := &svcapitypes.NodeSnapshot{}
				if f11iter.CacheClusterId != nil {
					f11elem.CacheClusterID = f11iter.CacheClusterId
				}
				if f11iter.CacheNodeCreateTime != nil {
					f11elem.CacheNodeCreateTime = &metav1.Time{Time: *f11iter.CacheNodeCreateTime}
				}
				if f11iter.CacheNodeId != nil {
					f11elem.CacheNodeID = f11iter.CacheNodeId
				}
				if f11iter.CacheSize != nil {
					f11elem.CacheSize = f11iter.CacheSize
				}
				if f11iter.NodeGroupConfiguration != nil {
					f11elemf4 := &svcapitypes.NodeGroupConfiguration{}
					if f11iter.NodeGroupConfiguration.NodeGroupId != nil {
						f11elemf4.NodeGroupID = f11iter.NodeGroupConfiguration.NodeGroupId
					}
					if f11iter.NodeGroupConfiguration.PrimaryAvailabilityZone != nil {
						f11elemf4.PrimaryAvailabilityZone = f11iter.NodeGroupConfiguration.PrimaryAvailabilityZone
					}
					if f11iter.NodeGroupConfiguration.ReplicaAvailabilityZones != nil {
						f11elemf4f2 := []*string{}
						for _, f11elemf4f2iter := range f11iter.NodeGroupConfiguration.ReplicaAvailabilityZones {
							f11elemf4f2iter := f11elemf4f2iter // Create new variable to avoid referencing loop variable
							f11elemf4f2 = append(f11elemf4f2, &f11elemf4f2iter)
						}
						f11elemf4.ReplicaAvailabilityZones = f11elemf4f2
					}
					if f11iter.NodeGroupConfiguration.ReplicaCount != nil {
						replicaCount := int64(*f11iter.NodeGroupConfiguration.ReplicaCount)
						f11elemf4.ReplicaCount = &replicaCount
					}
					if f11iter.NodeGroupConfiguration.Slots != nil {
						f11elemf4.Slots = f11iter.NodeGroupConfiguration.Slots
					}
					f11elem.NodeGroupConfiguration = f11elemf4
				}
				if f11iter.NodeGroupId != nil {
					f11elem.NodeGroupID = f11iter.NodeGroupId
				}
				if f11iter.SnapshotCreateTime != nil {
					f11elem.SnapshotCreateTime = &metav1.Time{Time: *f11iter.SnapshotCreateTime}
				}
				f11 = append(f11, f11elem)
			}
			ko.Status.NodeSnapshots = f11
		}
		if resp.Snapshot.NumCacheNodes != nil {
			numNodes := int64(*resp.Snapshot.NumCacheNodes)
			ko.Status.NumCacheNodes = &numNodes
		}
		if resp.Snapshot.NumNodeGroups != nil {
			numNodeGroups := int64(*resp.Snapshot.NumNodeGroups)
			ko.Status.NumNodeGroups = &numNodeGroups
		}
		if resp.Snapshot.Port != nil {
			port := int64(*resp.Snapshot.Port)
			ko.Status.Port = &port
		}
		if resp.Snapshot.PreferredAvailabilityZone != nil {
			ko.Status.PreferredAvailabilityZone = resp.Snapshot.PreferredAvailabilityZone
		}
		if resp.Snapshot.PreferredMaintenanceWindow != nil {
			ko.Status.PreferredMaintenanceWindow = resp.Snapshot.PreferredMaintenanceWindow
		}
		if resp.Snapshot.ReplicationGroupDescription != nil {
			ko.Status.ReplicationGroupDescription = resp.Snapshot.ReplicationGroupDescription
		}

		if resp.Snapshot.SnapshotRetentionLimit != nil {
			retentionLimit := int64(*resp.Snapshot.SnapshotRetentionLimit)
			ko.Status.SnapshotRetentionLimit = &retentionLimit
		}
		if resp.Snapshot.SnapshotSource != nil {
			ko.Status.SnapshotSource = resp.Snapshot.SnapshotSource
		}
		if resp.Snapshot.SnapshotStatus != nil {
			ko.Status.SnapshotStatus = resp.Snapshot.SnapshotStatus
		}
		if resp.Snapshot.SnapshotWindow != nil {
			ko.Status.SnapshotWindow = resp.Snapshot.SnapshotWindow
		}
		if resp.Snapshot.TopicArn != nil {
			ko.Status.TopicARN = resp.Snapshot.TopicArn
		}
		if resp.Snapshot.VpcId != nil {
			ko.Status.VPCID = resp.Snapshot.VpcId
		}

		rm.setStatusDefaults(ko)
		// custom set output from response
		rm.CustomCopySnapshotSetOutput(r, resp, ko)
		return &resource{ko}, nil
	}

	return nil, nil
}

// newCopySnapshotPayload returns an SDK-specific struct for the HTTP request
// payload of the CopySnapshot API call
func (rm *resourceManager) newCopySnapshotPayload(
	r *resource,
) (*svcsdk.CopySnapshotInput, error) {
	res := &svcsdk.CopySnapshotInput{}

	if r.ko.Spec.SourceSnapshotName != nil {
		res.SourceSnapshotName = r.ko.Spec.SourceSnapshotName
	}
	if r.ko.Spec.KMSKeyID != nil {
		res.KmsKeyId = r.ko.Spec.KMSKeyID
	}
	if r.ko.Spec.SnapshotName != nil {
		res.TargetSnapshotName = r.ko.Spec.SnapshotName
	}

	return res, nil
}

// CustomUpdateConditions sets conditions (terminal) on supplied snapshot
// it examines supplied resource to determine conditions.
// It returns true if conditions are updated
func (rm *resourceManager) CustomUpdateConditions(
	ko *svcapitypes.Snapshot,
	r *resource,
	err error,
) bool {
	snapshotStatus := r.ko.Status.SnapshotStatus
	if snapshotStatus == nil || *snapshotStatus != "failed" {
		return false
	}
	// Terminal condition
	var terminalCondition *ackv1alpha1.Condition = nil
	if ko.Status.Conditions == nil {
		ko.Status.Conditions = []*ackv1alpha1.Condition{}
	} else {
		for _, condition := range ko.Status.Conditions {
			if condition.Type == ackv1alpha1.ConditionTypeTerminal {
				terminalCondition = condition
				break
			}
		}
		if terminalCondition != nil && terminalCondition.Status == corev1.ConditionTrue {
			// some other exception already put the resource in terminal condition
			return false
		}
	}
	if terminalCondition == nil {
		terminalCondition = &ackv1alpha1.Condition{
			Type: ackv1alpha1.ConditionTypeTerminal,
		}
		ko.Status.Conditions = append(ko.Status.Conditions, terminalCondition)
	}
	terminalCondition.Status = corev1.ConditionTrue
	errorMessage := "Snapshot status: failed"
	terminalCondition.Message = &errorMessage
	return true
}

func (rm *resourceManager) CustomDescribeSnapshotSetOutput(
	ctx context.Context,
	r *resource,
	resp *elasticache.DescribeSnapshotsOutput,
	ko *svcapitypes.Snapshot,
) (*svcapitypes.Snapshot, error) {
	if len(resp.Snapshots) == 0 {
		return ko, nil
	}
	elem := resp.Snapshots[0]
	rm.customSetOutput(r, &elem, ko)
	return ko, nil
}

func (rm *resourceManager) CustomCreateSnapshotSetOutput(
	ctx context.Context,
	r *resource,
	resp *elasticache.CreateSnapshotOutput,
	ko *svcapitypes.Snapshot,
) (*svcapitypes.Snapshot, error) {
	rm.customSetOutput(r, resp.Snapshot, ko)
	return ko, nil
}

func (rm *resourceManager) CustomCopySnapshotSetOutput(
	r *resource,
	resp *elasticache.CopySnapshotOutput,
	ko *svcapitypes.Snapshot,
) *svcapitypes.Snapshot {
	rm.customSetOutput(r, resp.Snapshot, ko)
	return ko
}

func (rm *resourceManager) customSetOutput(
	r *resource,
	respSnapshot *svcsdktypes.Snapshot,
	ko *svcapitypes.Snapshot,
) {
	if respSnapshot.ReplicationGroupId != nil {
		ko.Spec.ReplicationGroupID = respSnapshot.ReplicationGroupId
	}

	if respSnapshot.KmsKeyId != nil {
		ko.Spec.KMSKeyID = respSnapshot.KmsKeyId
	}

	if respSnapshot.CacheClusterId != nil {
		ko.Spec.CacheClusterID = respSnapshot.CacheClusterId
	}

	if ko.Status.Conditions == nil {
		ko.Status.Conditions = []*ackv1alpha1.Condition{}
	}
	snapshotStatus := respSnapshot.SnapshotStatus
	syncConditionStatus := corev1.ConditionUnknown
	if snapshotStatus != nil {
		if *snapshotStatus == "available" ||
			*snapshotStatus == "failed" {
			syncConditionStatus = corev1.ConditionTrue
		} else {
			// resource in "creating", "restoring","exporting"
			syncConditionStatus = corev1.ConditionFalse
		}
	}
	var resourceSyncedCondition *ackv1alpha1.Condition = nil
	for _, condition := range ko.Status.Conditions {
		if condition.Type == ackv1alpha1.ConditionTypeResourceSynced {
			resourceSyncedCondition = condition
			break
		}
	}
	if resourceSyncedCondition == nil {
		resourceSyncedCondition = &ackv1alpha1.Condition{
			Type:   ackv1alpha1.ConditionTypeResourceSynced,
			Status: syncConditionStatus,
		}
		ko.Status.Conditions = append(ko.Status.Conditions, resourceSyncedCondition)
	} else {
		resourceSyncedCondition.Status = syncConditionStatus
	}
}

// Snapshot API has no update
func (rm *resourceManager) customUpdateSnapshot(
	ctx context.Context,
	desired *resource,
	latest *resource,
	delta *ackcompare.Delta,
) (*resource, error) {
	return latest, nil
}
