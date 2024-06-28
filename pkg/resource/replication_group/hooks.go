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
	"errors"

	ackrequeue "github.com/aws-controllers-k8s/runtime/pkg/requeue"

	svcapitypes "github.com/aws-controllers-k8s/elasticache-controller/apis/v1alpha1"
	"github.com/aws-controllers-k8s/elasticache-controller/pkg/util"
)

var (
	condMsgCurrentlyCreating      string = "replication group currently being created."
	condMsgCurrentlyDeleting      string = "replication group currently being deleted."
	condMsgNoDeleteWhileModifying string = "replication group currently being modified. cannot delete."
	condMsgTerminalCreateFailed   string = "replication group in create-failed status."
)

const (
	statusDeleting     string = "deleting"
	statusModifying    string = "modifying"
	statusCreating     string = "creating"
	statusCreateFailed string = "create-failed"
)

var (
	requeueWaitWhileDeleting = ackrequeue.NeededAfter(
		errors.New("Delete is in progress."),
		ackrequeue.DefaultRequeueAfterDuration,
	)
	requeueWaitWhileModifying = ackrequeue.NeededAfter(
		errors.New("Modify is in progress."),
		ackrequeue.DefaultRequeueAfterDuration,
	)
)

// isDeleting returns true if supplied replication group resource state is 'deleting'
func isDeleting(r *resource) bool {
	if r == nil || r.ko.Status.Status == nil {
		return false
	}
	status := *r.ko.Status.Status
	return status == statusDeleting
}

// isModifying returns true if supplied replication group resource state is 'modifying'
func isModifying(r *resource) bool {
	if r == nil || r.ko.Status.Status == nil {
		return false
	}
	status := *r.ko.Status.Status
	return status == statusModifying
}

// isCreating returns true if supplied replication group resource state is 'modifying'
func isCreating(r *resource) bool {
	if r == nil || r.ko.Status.Status == nil {
		return false
	}
	status := *r.ko.Status.Status
	return status == statusCreating
}

// isCreateFailed returns true if supplied replication group resource state is
// 'create-failed'
func isCreateFailed(r *resource) bool {
	if r == nil || r.ko.Status.Status == nil {
		return false
	}
	status := *r.ko.Status.Status
	return status == statusCreateFailed
}

// getTags retrieves the resource's associated tags.
func (rm *resourceManager) getTags(
	ctx context.Context,
	resourceARN string,
) ([]*svcapitypes.Tag, error) {
	return util.GetTags(ctx, rm.sdkapi, rm.metrics, resourceARN)
}

// syncTags keeps the resource's tags in sync.
func (rm *resourceManager) syncTags(
	ctx context.Context,
	desired *resource,
	latest *resource,
) (err error) {
	return util.SyncTags(ctx, desired.ko.Spec.Tags, latest.ko.Spec.Tags, latest.ko.Status.ACKResourceMetadata, ToACKTags, rm.sdkapi, rm.metrics)
}
