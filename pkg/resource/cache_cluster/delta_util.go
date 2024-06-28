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
	"encoding/json"
	"reflect"

	ackcompare "github.com/aws-controllers-k8s/runtime/pkg/compare"

	"github.com/aws-controllers-k8s/elasticache-controller/pkg/common"
	"github.com/aws-controllers-k8s/elasticache-controller/pkg/util"
)

// modifyDelta removes non-meaningful differences from the delta and adds additional differences if necessary.
func modifyDelta(
	delta *ackcompare.Delta,
	desired *resource,
	latest *resource,
) {
	if delta.DifferentAt("Spec.EngineVersion") && desired.ko.Spec.EngineVersion != nil && latest.ko.Spec.EngineVersion != nil &&
		util.EngineVersionsMatch(*desired.ko.Spec.EngineVersion, *latest.ko.Spec.EngineVersion) {
		common.RemoveFromDelta(delta, "Spec.EngineVersion")
		// TODO: handle the case of a nil difference (especially when desired EV is nil)
	}

	// if server has given PreferredMaintenanceWindow a default value, no action needs to be taken.
	if delta.DifferentAt("Spec.PreferredMaintenanceWindow") && desired.ko.Spec.PreferredMaintenanceWindow == nil &&
		latest.ko.Spec.PreferredMaintenanceWindow != nil {
		common.RemoveFromDelta(delta, "Spec.PreferredMaintenanceWindow")
	}

	if delta.DifferentAt("Spec.PreferredAvailabilityZone") && desired.ko.Spec.PreferredAvailabilityZone == nil &&
		latest.ko.Spec.PreferredAvailabilityZone != nil {
		common.RemoveFromDelta(delta, "Spec.PreferredAvailabilityZone")
	}

	updatePAZsDelta(desired, delta)
}

// updatePAZsDelta retrieves the last requested configurations saved in annotations and compares them
// to the current desired configurations. If a diff is found, it adds it to delta.
func updatePAZsDelta(desired *resource, delta *ackcompare.Delta) {
	var lastRequestedPAZs []*string
	unmarshalAnnotation(desired, AnnotationLastRequestedPAZs, &lastRequestedPAZs)
	if !reflect.DeepEqual(desired.ko.Spec.PreferredAvailabilityZones, lastRequestedPAZs) {
		delta.Add("Spec.PreferredAvailabilityZones", desired.ko.Spec.PreferredAvailabilityZones,
			lastRequestedPAZs)
	}
}

func unmarshalAnnotation(desired *resource, annotation string, val interface{}) {
	if data, ok := desired.ko.ObjectMeta.GetAnnotations()[annotation]; ok {
		_ = json.Unmarshal([]byte(data), val)
	}
}
