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

package testutil

import (
	"encoding/json"
	"io/ioutil"
)

// LoadFromFixture fills an empty pointer variable with the
// data from a fixture JSON file.
func LoadFromFixture(
	fixturePath string,
	output interface{}, // output should be an addressable type (i.e. a pointer)
) {
	contents, err := ioutil.ReadFile(fixturePath)
	if err != nil {
		panic(err)
	}
	err = json.Unmarshal(contents, output)
	if err != nil {
		panic(err)
	}
}