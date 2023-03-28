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
	"strconv"

	ackcompare "github.com/aws-controllers-k8s/runtime/pkg/compare"

	svcapitypes "github.com/aws-controllers-k8s/elasticache-controller/apis/v1alpha1"
	"github.com/aws-controllers-k8s/elasticache-controller/pkg/common"
)

// modifyDelta removes non-meaningful differences from the delta and adds additional differences if necessary
func modifyDelta(
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

	if multiAZRequiresUpdate(desired, latest) {
		delta.Add("Spec.MultiAZEnabled", desired.ko.Spec.MultiAZEnabled, latest.ko.Status.MultiAZ)
	}

	if autoFailoverRequiresUpdate(desired, latest) {
		delta.Add("Spec.AutomaticFailoverEnabled", desired.ko.Spec.AutomaticFailoverEnabled,
			latest.ko.Status.AutomaticFailover)
	}

	if updateRequired, current := primaryClusterIDRequiresUpdate(desired, latest); updateRequired {
		delta.Add("Spec.PrimaryClusterID", desired.ko.Spec.PrimaryClusterID, *current)
	}
}

// returns true if desired and latest engine versions match and false otherwise
// precondition: both desiredEV and latestEV are non-nil
// this handles the case where only the major EV is specified, e.g. "6.x" (or similar), but the latest
//
//	version shows the minor version, e.g. "6.0.5"
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

	// if the version is higher than 6, skip the upstream patch version when comparing.
	majorVersion, _ := strconv.Atoi(desiredEV[0:1])
	if majorVersion >= 6 {
		r, _ := regexp.Compile(desiredEV + ".*")
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

// multiAZRequiresUpdate returns true if the latest multi AZ status does not yet match the
// desired state, and false otherwise
func multiAZRequiresUpdate(desired *resource, latest *resource) bool {
	// no preference for multi AZ specified; no update required
	if desired.ko.Spec.MultiAZEnabled == nil {
		return false
	}

	// API should return a non-nil value, but if it doesn't then attempt to update
	if latest.ko.Status.MultiAZ == nil {
		return true
	}

	// true maps to "enabled"; false maps to "disabled"
	// this accounts for values such as "enabling" and "disabling"
	if *desired.ko.Spec.MultiAZEnabled {
		return *latest.ko.Status.MultiAZ != string(svcapitypes.MultiAZStatus_enabled)
	} else {
		return *latest.ko.Status.MultiAZ != string(svcapitypes.MultiAZStatus_disabled)
	}
}

// autoFailoverRequiresUpdate returns true if the latest auto failover status does not yet match the
// desired state, and false otherwise
func autoFailoverRequiresUpdate(desired *resource, latest *resource) bool {
	// the logic is exactly analogous to multiAZRequiresUpdate above
	if desired.ko.Spec.AutomaticFailoverEnabled == nil {
		return false
	}

	if latest.ko.Status.AutomaticFailover == nil {
		return true
	}

	if *desired.ko.Spec.AutomaticFailoverEnabled {
		return *latest.ko.Status.AutomaticFailover != string(svcapitypes.AutomaticFailoverStatus_enabled)
	} else {
		return *latest.ko.Status.AutomaticFailover != string(svcapitypes.AutomaticFailoverStatus_disabled)
	}
}

// primaryClusterIDRequiresUpdate retrieves the current primary cluster ID and determines whether
// an update is required. If no desired state is specified or there is an issue retrieving the
// latest state, return false, nil. Otherwise, return false or true depending on equality of
// the latest and desired states, and a non-nil pointer to the latest value
func primaryClusterIDRequiresUpdate(desired *resource, latest *resource) (bool, *string) {
	if desired.ko.Spec.PrimaryClusterID == nil {
		return false, nil
	}

	// primary cluster ID applies to cluster mode disabled only; if API returns multiple
	//   or no node groups, or the provided node group is nil, there is nothing that can be done
	if len(latest.ko.Status.NodeGroups) != 1 || latest.ko.Status.NodeGroups[0] == nil {
		return false, nil
	}

	// attempt to find primary cluster in node group. If for some reason it is not present, we
	//   don't have a reliable latest state, so do nothing
	ng := *latest.ko.Status.NodeGroups[0]
	for _, member := range ng.NodeGroupMembers {
		if member == nil {
			continue
		}

		if member.CurrentRole != nil && *member.CurrentRole == "primary" && member.CacheClusterID != nil {
			val := *member.CacheClusterID
			return val != *desired.ko.Spec.PrimaryClusterID, &val
		}
	}

	return false, nil
}
