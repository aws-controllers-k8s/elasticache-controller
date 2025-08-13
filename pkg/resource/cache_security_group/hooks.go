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

package cache_security_group

import (
	"context"
	"fmt"

	svcapitypes "github.com/aws-controllers-k8s/elasticache-controller/apis/v1alpha1"

	"github.com/aws-controllers-k8s/elasticache-controller/pkg/tags"
)

// getTags returns the tags for a given CacheSecurityGroup
func (rm *resourceManager) getTags(
	ctx context.Context,
	cacheSecurityGroupName string,
) []*svcapitypes.Tag {
	// Construct the ARN for the CacheSecurityGroup
	// Format: arn:aws:elasticache:<region>:<account-id>:securitygroup:<cache-security-group-name>
	resourceARN := rm.buildCacheSecurityGroupARN(cacheSecurityGroupName)

	tagManager := tags.NewTagManager(rm.sdkapi)
	tags, err := tagManager.GetTags(ctx, resourceARN)
	if err != nil {
		return nil
	}
	return tags
}

// syncTags synchronizes tags between the desired and latest CacheSecurityGroup
func (rm *resourceManager) syncTags(
	ctx context.Context,
	latest *resource,
	desired *resource,
) error {
	if latest == nil || desired == nil {
		return nil
	}

	// Construct the ARN for the CacheSecurityGroup
	resourceARN := rm.buildCacheSecurityGroupARN(*desired.ko.Spec.CacheSecurityGroupName)

	// If the tags are the same, no need to sync
	if tags.CompareElasticacheTags(latest.ko.Spec.Tags, desired.ko.Spec.Tags) {
		return nil
	}

	tagManager := tags.NewTagManager(rm.sdkapi)
	return tagManager.SyncTags(ctx, resourceARN, desired.ko.Spec.Tags, latest.ko.Spec.Tags)
}

// buildCacheSecurityGroupARN constructs the ARN for a CacheSecurityGroup
func (rm *resourceManager) buildCacheSecurityGroupARN(
	cacheSecurityGroupName string,
) string {
	return fmt.Sprintf(
		"arn:aws:elasticache:%s:%s:securitygroup:%s",
		rm.awsRegion,
		rm.awsAccountID,
		cacheSecurityGroupName,
	)
}
