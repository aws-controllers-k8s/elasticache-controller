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
	"encoding/json"
	"reflect"
	"regexp"
	"strings"

	ackcompare "github.com/aws-controllers-k8s/runtime/pkg/compare"

	svcapitypes "github.com/aws-controllers-k8s/elasticache-controller/apis/v1alpha1"
	"github.com/aws-controllers-k8s/elasticache-controller/pkg/common"
)

// filterDelta removes non-meaningful differences from the delta and adds additional differences if necessary
func filterDelta(
	delta *ackcompare.Delta,
	desired *resource,
	latest *resource,
) {

	if delta.DifferentAt("Spec.EngineVersion") {
		if desired.ko.Spec.EngineVersion != nil && latest.ko.Spec.EngineVersion != nil {
			if engineVersionsMatch(*desired.ko.Spec.EngineVersion, *latest.ko.Spec.EngineVersion) {
				common.RemoveFromDelta(delta, "Spec.EngineVersion")
			}
		}
		// TODO: handle the case of a nil difference (especially when desired EV is nil)
	}

	// if server has given PreferredMaintenanceWindow a default value, no action needs to be taken
	if delta.DifferentAt("Spec.PreferredMaintenanceWindow") {
		if desired.ko.Spec.PreferredMaintenanceWindow == nil && latest.ko.Spec.PreferredMaintenanceWindow != nil {
			common.RemoveFromDelta(delta, "Spec.PreferredMaintenanceWindow")
		}
	}

	// note that the comparison is actually done between desired.Spec.LogDeliveryConfigurations and
	// the last requested configurations saved in annotations (as opposed to latest.Spec.LogDeliveryConfigurations)
	if logDeliveryRequiresUpdate(desired) {
		delta.Add("Spec.LogDeliveryConfigurations", desired.ko.Spec.LogDeliveryConfigurations,
			unmarshalLastRequestedLDCs(desired))
	}
}

// returns true if desired and latest engine versions match and false otherwise
// precondition: both desiredEV and latestEV are non-nil
// this handles the case where only the major EV is specified, e.g. "6.x" (or similar), but the latest
//   version shows the minor version, e.g. "6.0.5"
func engineVersionsMatch(
	desiredEV string,
	latestEV string,
) bool {
	if desiredEV == latestEV {
		return true
	}

	// if the last character of desiredEV is "x", only check for a major version match
	last := len(desiredEV) - 1
	if desiredEV[last:] == "x" {
		// cut off the "x" and replace all occurrences of '.' with '\.' (as '.' is a special regex character)
		desired := strings.Replace(desiredEV[:last], ".", "\\.", -1)
		r, _ := regexp.Compile(desired + ".*")
		return r.MatchString(latestEV)
	}

	return false
}

// logDeliveryRequiresUpdate retrieves the last requested configurations saved in annotations and compares them
// to the current desired configurations
func logDeliveryRequiresUpdate(desired *resource) bool {
	desiredConfigs := desired.ko.Spec.LogDeliveryConfigurations
	lastRequestedConfigs := unmarshalLastRequestedLDCs(desired)
	return !reflect.DeepEqual(desiredConfigs, lastRequestedConfigs)
}

// unmarshal the value found in annotations for the LogDeliveryConfigurations field requested in the last
// successful create or modify call
func unmarshalLastRequestedLDCs(desired *resource) []*svcapitypes.LogDeliveryConfigurationRequest {
	var lastRequestedConfigs []*svcapitypes.LogDeliveryConfigurationRequest

	annotations := desired.ko.ObjectMeta.GetAnnotations()
	if val, ok := annotations[AnnotationLastRequestedLDCs]; ok {
		_ = json.Unmarshal([]byte(val), &lastRequestedConfigs)
	}

	return lastRequestedConfigs
}
