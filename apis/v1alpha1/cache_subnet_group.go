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

package v1alpha1

import (
	ackv1alpha1 "github.com/aws-controllers-k8s/runtime/apis/core/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// CacheSubnetGroupSpec defines the desired state of CacheSubnetGroup.
//
// Represents the output of one of the following operations:
//
//   - CreateCacheSubnetGroup
//
//   - ModifyCacheSubnetGroup
type CacheSubnetGroupSpec struct {

	// A description for the cache subnet group.
	// +kubebuilder:validation:Required
	CacheSubnetGroupDescription *string `json:"cacheSubnetGroupDescription"`
	// A name for the cache subnet group. This value is stored as a lowercase string.
	//
	// Constraints: Must contain no more than 255 alphanumeric characters or hyphens.
	//
	// Example: mysubnetgroup
	// +kubebuilder:validation:Required
	CacheSubnetGroupName *string `json:"cacheSubnetGroupName"`
	// A list of VPC subnet IDs for the cache subnet group.
	SubnetIDs  []*string                                  `json:"subnetIDs,omitempty"`
	SubnetRefs []*ackv1alpha1.AWSResourceReferenceWrapper `json:"subnetRefs,omitempty"`
	// A list of tags to be added to this resource. A tag is a key-value pair. A
	// tag key must be accompanied by a tag value, although null is accepted.
	Tags []*Tag `json:"tags,omitempty"`
}

// CacheSubnetGroupStatus defines the observed state of CacheSubnetGroup
type CacheSubnetGroupStatus struct {
	// All CRs managed by ACK have a common `Status.ACKResourceMetadata` member
	// that is used to contain resource sync state, account ownership,
	// constructed ARN for the resource
	// +kubebuilder:validation:Optional
	ACKResourceMetadata *ackv1alpha1.ResourceMetadata `json:"ackResourceMetadata"`
	// All CRs managed by ACK have a common `Status.Conditions` member that
	// contains a collection of `ackv1alpha1.Condition` objects that describe
	// the various terminal states of the CR and its backend AWS service API
	// resource
	// +kubebuilder:validation:Optional
	Conditions []*ackv1alpha1.Condition `json:"conditions"`
	// A list of events. Each element in the list contains detailed information
	// about one event.
	// +kubebuilder:validation:Optional
	Events []*Event `json:"events,omitempty"`
	// A list of subnets associated with the cache subnet group.
	// +kubebuilder:validation:Optional
	Subnets []*Subnet `json:"subnets,omitempty"`
	// The Amazon Virtual Private Cloud identifier (VPC ID) of the cache subnet
	// group.
	// +kubebuilder:validation:Optional
	VPCID *string `json:"vpcID,omitempty"`
}

// CacheSubnetGroup is the Schema for the CacheSubnetGroups API
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
type CacheSubnetGroup struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              CacheSubnetGroupSpec   `json:"spec,omitempty"`
	Status            CacheSubnetGroupStatus `json:"status,omitempty"`
}

// CacheSubnetGroupList contains a list of CacheSubnetGroup
// +kubebuilder:object:root=true
type CacheSubnetGroupList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []CacheSubnetGroup `json:"items"`
}

func init() {
	SchemeBuilder.Register(&CacheSubnetGroup{}, &CacheSubnetGroupList{})
}
