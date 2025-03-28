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

package cache_parameter_group

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

	if ackcompare.HasNilDifference(a.ko.Spec.CacheParameterGroupFamily, b.ko.Spec.CacheParameterGroupFamily) {
		delta.Add("Spec.CacheParameterGroupFamily", a.ko.Spec.CacheParameterGroupFamily, b.ko.Spec.CacheParameterGroupFamily)
	} else if a.ko.Spec.CacheParameterGroupFamily != nil && b.ko.Spec.CacheParameterGroupFamily != nil {
		if *a.ko.Spec.CacheParameterGroupFamily != *b.ko.Spec.CacheParameterGroupFamily {
			delta.Add("Spec.CacheParameterGroupFamily", a.ko.Spec.CacheParameterGroupFamily, b.ko.Spec.CacheParameterGroupFamily)
		}
	}
	if ackcompare.HasNilDifference(a.ko.Spec.CacheParameterGroupName, b.ko.Spec.CacheParameterGroupName) {
		delta.Add("Spec.CacheParameterGroupName", a.ko.Spec.CacheParameterGroupName, b.ko.Spec.CacheParameterGroupName)
	} else if a.ko.Spec.CacheParameterGroupName != nil && b.ko.Spec.CacheParameterGroupName != nil {
		if *a.ko.Spec.CacheParameterGroupName != *b.ko.Spec.CacheParameterGroupName {
			delta.Add("Spec.CacheParameterGroupName", a.ko.Spec.CacheParameterGroupName, b.ko.Spec.CacheParameterGroupName)
		}
	}
	if ackcompare.HasNilDifference(a.ko.Spec.Description, b.ko.Spec.Description) {
		delta.Add("Spec.Description", a.ko.Spec.Description, b.ko.Spec.Description)
	} else if a.ko.Spec.Description != nil && b.ko.Spec.Description != nil {
		if *a.ko.Spec.Description != *b.ko.Spec.Description {
			delta.Add("Spec.Description", a.ko.Spec.Description, b.ko.Spec.Description)
		}
	}
	if len(a.ko.Spec.ParameterNameValues) != len(b.ko.Spec.ParameterNameValues) {
		delta.Add("Spec.ParameterNameValues", a.ko.Spec.ParameterNameValues, b.ko.Spec.ParameterNameValues)
	} else if len(a.ko.Spec.ParameterNameValues) > 0 {
		if !reflect.DeepEqual(a.ko.Spec.ParameterNameValues, b.ko.Spec.ParameterNameValues) {
			delta.Add("Spec.ParameterNameValues", a.ko.Spec.ParameterNameValues, b.ko.Spec.ParameterNameValues)
		}
	}
	desiredACKTags, _ := convertToOrderedACKTags(a.ko.Spec.Tags)
	latestACKTags, _ := convertToOrderedACKTags(b.ko.Spec.Tags)
	if !ackcompare.MapStringStringEqual(desiredACKTags, latestACKTags) {
		delta.Add("Spec.Tags", a.ko.Spec.Tags, b.ko.Spec.Tags)
	}

	return delta
}
