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
	svcapitypes "github.com/aws-controllers-k8s/elasticache-controller/apis/v1alpha1"
)

const (
	// AnnotationLastRequestedLDCs is an annotation whose value is the marshaled list of pointers to
	// LogDeliveryConfigurationRequest structs passed in as input to either the create or modify API called most
	// recently
	AnnotationLastRequestedLDCs = svcapitypes.AnnotationPrefix + "last-requested-log-delivery-configurations"
	// AnnotationLastRequestedCNT is an annotation whose value is passed in as input to either the create or modify API
	// called most recently
	AnnotationLastRequestedCNT = svcapitypes.AnnotationPrefix + "last-requested-cache-node-type"
	// AnnotationLastRequestedNNG is an annotation whose value is passed in as input to either the create or modify API
	// called most recently
	AnnotationLastRequestedNNG = svcapitypes.AnnotationPrefix + "last-requested-num-node-groups"
	// AnnotationLastRequestedNGC is an annotation whose value is the marshaled list of pointers to
	// NodeGroupConfiguration structs passed in as input to either the create or modify API called most
	// recently
	AnnotationLastRequestedNGC = svcapitypes.AnnotationPrefix + "last-requested-node-group-configuration"
)
