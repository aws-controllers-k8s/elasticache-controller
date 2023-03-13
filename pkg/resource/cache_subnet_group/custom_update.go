package cache_subnet_group

import (
	"context"

	svcapitypes "github.com/aws-controllers-k8s/elasticache-controller/apis/v1alpha1"
	ackv1alpha1 "github.com/aws-controllers-k8s/runtime/apis/core/v1alpha1"
	ackcompare "github.com/aws-controllers-k8s/runtime/pkg/compare"
	ackrtlog "github.com/aws-controllers-k8s/runtime/pkg/runtime/log"
	svcsdk "github.com/aws/aws-sdk-go/service/elasticache"
)

func (rm *resourceManager) customUpdateCacheSubnetGroup(
	ctx context.Context,
	desired *resource,
	latest *resource,
	delta *ackcompare.Delta,
) (updated *resource, err error) {
	rlog := ackrtlog.FromContext(ctx)
	exit := rlog.Trace("rm.sdkUpdate")
	defer func() {
		exit(err)
	}()
	input, err := rm.newUpdateRequestPayload(ctx, desired, delta)
	if err != nil {
		return nil, err
	}

	var resp *svcsdk.ModifyCacheSubnetGroupOutput
	_ = resp
	resp, err = rm.sdkapi.ModifyCacheSubnetGroupWithContext(ctx, input)
	rm.metrics.RecordAPICall("UPDATE", "ModifyCacheSubnetGroup", err)
	if err != nil {
		return nil, err
	}
	// Merge in the information we read from the API call above to the copy of
	// the original Kubernetes object we passed to the function
	ko := desired.ko.DeepCopy()

	if ko.Status.ACKResourceMetadata == nil {
		ko.Status.ACKResourceMetadata = &ackv1alpha1.ResourceMetadata{}
	}
	if resp.CacheSubnetGroup.ARN != nil {
		arn := ackv1alpha1.AWSResourceName(*resp.CacheSubnetGroup.ARN)
		ko.Status.ACKResourceMetadata.ARN = &arn
	}
	if resp.CacheSubnetGroup.CacheSubnetGroupDescription != nil {
		ko.Spec.CacheSubnetGroupDescription = resp.CacheSubnetGroup.CacheSubnetGroupDescription
	} else {
		ko.Spec.CacheSubnetGroupDescription = nil
	}
	if resp.CacheSubnetGroup.CacheSubnetGroupName != nil {
		ko.Spec.CacheSubnetGroupName = resp.CacheSubnetGroup.CacheSubnetGroupName
	} else {
		ko.Spec.CacheSubnetGroupName = nil
	}
	if resp.CacheSubnetGroup.Subnets != nil {
		f3 := []*svcapitypes.Subnet{}
		for _, f3iter := range resp.CacheSubnetGroup.Subnets {
			f3elem := &svcapitypes.Subnet{}
			if f3iter.SubnetAvailabilityZone != nil {
				f3elemf0 := &svcapitypes.AvailabilityZone{}
				if f3iter.SubnetAvailabilityZone.Name != nil {
					f3elemf0.Name = f3iter.SubnetAvailabilityZone.Name
				}
				f3elem.SubnetAvailabilityZone = f3elemf0
			}
			if f3iter.SubnetIdentifier != nil {
				f3elem.SubnetIdentifier = f3iter.SubnetIdentifier
			}
			if f3iter.SubnetOutpost != nil {
				f3elemf2 := &svcapitypes.SubnetOutpost{}
				if f3iter.SubnetOutpost.SubnetOutpostArn != nil {
					f3elemf2.SubnetOutpostARN = f3iter.SubnetOutpost.SubnetOutpostArn
				}
				f3elem.SubnetOutpost = f3elemf2
			}
			f3 = append(f3, f3elem)
		}
		ko.Status.Subnets = f3
	} else {
		ko.Status.Subnets = nil
	}
	if resp.CacheSubnetGroup.VpcId != nil {
		ko.Status.VPCID = resp.CacheSubnetGroup.VpcId
	} else {
		ko.Status.VPCID = nil
	}

	rm.setStatusDefaults(ko)
	return &resource{ko}, nil
}

// newUpdateRequestPayload returns an SDK-specific struct for the HTTP request
// payload of the Update API call for the resource
func (rm *resourceManager) newUpdateRequestPayload(
	ctx context.Context,
	r *resource,
	delta *ackcompare.Delta,
) (*svcsdk.ModifyCacheSubnetGroupInput, error) {
	res := &svcsdk.ModifyCacheSubnetGroupInput{}

	if r.ko.Spec.CacheSubnetGroupDescription != nil {
		res.SetCacheSubnetGroupDescription(*r.ko.Spec.CacheSubnetGroupDescription)
	}
	if r.ko.Spec.CacheSubnetGroupName != nil {
		res.SetCacheSubnetGroupName(*r.ko.Spec.CacheSubnetGroupName)
	}
	if r.ko.Spec.SubnetIDs != nil {
		f2 := []*string{}
		for _, f2iter := range r.ko.Spec.SubnetIDs {
			var f2elem string
			f2elem = *f2iter
			f2 = append(f2, &f2elem)
		}
		res.SetSubnetIds(f2)
	}

	return res, nil
}
