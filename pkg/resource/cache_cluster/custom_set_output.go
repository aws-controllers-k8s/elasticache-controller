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
	"context"
	"encoding/json"

	"github.com/aws/aws-sdk-go/service/elasticache"

	svcapitypes "github.com/aws-controllers-k8s/elasticache-controller/apis/v1alpha1"
)

func (rm *resourceManager) customCreateCacheClusterSetOutput(
	_ context.Context,
	r *resource,
	_ *elasticache.CreateCacheClusterOutput,
	ko *svcapitypes.CacheCluster,
) (*svcapitypes.CacheCluster, error) {
	rm.setAnnotationsFields(r, ko)
	return ko, nil
}

func (rm *resourceManager) customModifyCacheClusterSetOutput(
	_ context.Context,
	r *resource,
	_ *elasticache.ModifyCacheClusterOutput,
	ko *svcapitypes.CacheCluster,
) (*svcapitypes.CacheCluster, error) {
	rm.setAnnotationsFields(r, ko)
	return ko, nil
}

// setAnnotationsFields copies the desired object's annotations, populates any
// relevant fields, and sets the latest object's annotations to this newly populated map.
// Fields that are handled by custom modify implementation are not set here.
// This should only be called upon a successful create or modify call.
func (rm *resourceManager) setAnnotationsFields(
	r *resource,
	ko *svcapitypes.CacheCluster,
) {
	annotations := getAnnotationsFields(r, ko)
	annotations[AnnotationLastRequestedPAZs] = marshalAsAnnotation(r.ko.Spec.PreferredAvailabilityZones)
	ko.ObjectMeta.Annotations = annotations
}

// getAnnotationsFields return the annotations map that would be used to set the fields.
func getAnnotationsFields(
	r *resource,
	ko *svcapitypes.CacheCluster,
) map[string]string {
	if ko.ObjectMeta.Annotations != nil {
		return ko.ObjectMeta.Annotations
	}
	desiredAnnotations := r.ko.ObjectMeta.GetAnnotations()
	annotations := make(map[string]string)
	for k, v := range desiredAnnotations {
		annotations[k] = v
	}
	ko.ObjectMeta.Annotations = annotations
	return annotations
}

func marshalAsAnnotation(val interface{}) string {
	data, err := json.Marshal(val)
	if err != nil {
		return "null"
	}
	return string(data)
}
