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

package util

import (
	"strconv"
	"strings"
)

// EngineVersionsMatch returns true if desired and latest engine versions match and false otherwise
// precondition: both desiredEV and latestEV are non-nil
// this handles the case where only the major EV is specified, e.g. "6.x" (or similar),
// but the latest version shows the minor version, e.g. "6.0.5".
func EngineVersionsMatch(desiredEV, latestEV string) bool {
	if desiredEV == latestEV {
		return true
	}

	dMaj, dMin := versionNumbersFromString(desiredEV)
	lMaj, lMin := versionNumbersFromString(latestEV)
	last := len(desiredEV) - 1

	// if the last character of desiredEV is "x" or the major version is higher than 5, ignore patch version when comparing.
	// See https://github.com/aws-controllers-k8s/community/issues/1737
	if dMaj > 5 || desiredEV[last:] == "x" {
		return dMaj == lMaj && (dMin < 0 || dMin == lMin)
	}

	return false
}

// versionNumbersFromString takes a version string like "6.2", "6.x" or "7.0.4" and
// returns the major, minor and patch numbers. If no minor or patch numbers are present
// or contain the "x" placeholder, -1 is returned for that version number.
func versionNumbersFromString(version string) (int, int) {
	parts := strings.Split(version, ".")
	major := -1
	minor := -1
	if len(parts) == 0 {
		return major, minor
	}
	major, _ = strconv.Atoi(parts[0])
	if len(parts) > 1 {
		if !strings.EqualFold(parts[1], "x") {
			minor, _ = strconv.Atoi(parts[1])
		}
	}
	return major, minor
}
