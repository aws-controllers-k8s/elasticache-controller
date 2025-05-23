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

// Code generated by ack-generate. DO NOT EDIT.

package snapshot

import (
	"bytes"
	"reflect"

	ackcompare "github.com/aws-controllers-k8s/runtime/pkg/compare"
	acktags "github.com/aws-controllers-k8s/runtime/pkg/tags"
)

// Hack to avoid import errors during build...
var (
	_ = &bytes.Buffer{}
	_ = &reflect.Method{}
	_ = &acktags.Tags{}
)

// newResourceDelta returns a new `ackcompare.Delta` used to compare two
// resources
func newResourceDelta(
	a *resource,
	b *resource,
) *ackcompare.Delta {
	delta := ackcompare.NewDelta()
	if (a == nil && b != nil) ||
		(a != nil && b == nil) {
		delta.Add("", a, b)
		return delta
	}

	if ackcompare.HasNilDifference(a.ko.Spec.CacheClusterID, b.ko.Spec.CacheClusterID) {
		delta.Add("Spec.CacheClusterID", a.ko.Spec.CacheClusterID, b.ko.Spec.CacheClusterID)
	} else if a.ko.Spec.CacheClusterID != nil && b.ko.Spec.CacheClusterID != nil {
		if *a.ko.Spec.CacheClusterID != *b.ko.Spec.CacheClusterID {
			delta.Add("Spec.CacheClusterID", a.ko.Spec.CacheClusterID, b.ko.Spec.CacheClusterID)
		}
	}
	if ackcompare.HasNilDifference(a.ko.Spec.KMSKeyID, b.ko.Spec.KMSKeyID) {
		delta.Add("Spec.KMSKeyID", a.ko.Spec.KMSKeyID, b.ko.Spec.KMSKeyID)
	} else if a.ko.Spec.KMSKeyID != nil && b.ko.Spec.KMSKeyID != nil {
		if *a.ko.Spec.KMSKeyID != *b.ko.Spec.KMSKeyID {
			delta.Add("Spec.KMSKeyID", a.ko.Spec.KMSKeyID, b.ko.Spec.KMSKeyID)
		}
	}
	if ackcompare.HasNilDifference(a.ko.Spec.ReplicationGroupID, b.ko.Spec.ReplicationGroupID) {
		delta.Add("Spec.ReplicationGroupID", a.ko.Spec.ReplicationGroupID, b.ko.Spec.ReplicationGroupID)
	} else if a.ko.Spec.ReplicationGroupID != nil && b.ko.Spec.ReplicationGroupID != nil {
		if *a.ko.Spec.ReplicationGroupID != *b.ko.Spec.ReplicationGroupID {
			delta.Add("Spec.ReplicationGroupID", a.ko.Spec.ReplicationGroupID, b.ko.Spec.ReplicationGroupID)
		}
	}
	if ackcompare.HasNilDifference(a.ko.Spec.SnapshotName, b.ko.Spec.SnapshotName) {
		delta.Add("Spec.SnapshotName", a.ko.Spec.SnapshotName, b.ko.Spec.SnapshotName)
	} else if a.ko.Spec.SnapshotName != nil && b.ko.Spec.SnapshotName != nil {
		if *a.ko.Spec.SnapshotName != *b.ko.Spec.SnapshotName {
			delta.Add("Spec.SnapshotName", a.ko.Spec.SnapshotName, b.ko.Spec.SnapshotName)
		}
	}
	if ackcompare.HasNilDifference(a.ko.Spec.SourceSnapshotName, b.ko.Spec.SourceSnapshotName) {
		delta.Add("Spec.SourceSnapshotName", a.ko.Spec.SourceSnapshotName, b.ko.Spec.SourceSnapshotName)
	} else if a.ko.Spec.SourceSnapshotName != nil && b.ko.Spec.SourceSnapshotName != nil {
		if *a.ko.Spec.SourceSnapshotName != *b.ko.Spec.SourceSnapshotName {
			delta.Add("Spec.SourceSnapshotName", a.ko.Spec.SourceSnapshotName, b.ko.Spec.SourceSnapshotName)
		}
	}
	desiredACKTags, _ := convertToOrderedACKTags(a.ko.Spec.Tags)
	latestACKTags, _ := convertToOrderedACKTags(b.ko.Spec.Tags)
	if !ackcompare.MapStringStringEqual(desiredACKTags, latestACKTags) {
		delta.Add("Spec.Tags", a.ko.Spec.Tags, b.ko.Spec.Tags)
	}

	return delta
}
