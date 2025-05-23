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

// CacheClusterSpec defines the desired state of CacheCluster.
//
// Contains all of the attributes of a specific cluster.
type CacheClusterSpec struct {

	// Specifies whether the nodes in this Memcached cluster are created in a single
	// Availability Zone or created across multiple Availability Zones in the cluster's
	// region.
	//
	// This parameter is only supported for Memcached clusters.
	//
	// If the AZMode and PreferredAvailabilityZones are not specified, ElastiCache
	// assumes single-az mode.
	AZMode *string `json:"azMode,omitempty"`
	// Reserved parameter. The password used to access a password protected server.
	//
	// Password constraints:
	//
	//   - Must be only printable ASCII characters.
	//
	//   - Must be at least 16 characters and no more than 128 characters in length.
	AuthToken *ackv1alpha1.SecretKeyReference `json:"authToken,omitempty"`
	// If you are running Valkey 7.2 and above or Redis OSS engine version 6.0 and
	// above, set this parameter to yes to opt-in to the next auto minor version
	// upgrade campaign. This parameter is disabled for previous versions.
	AutoMinorVersionUpgrade *bool `json:"autoMinorVersionUpgrade,omitempty"`
	// The node group (shard) identifier. This parameter is stored as a lowercase
	// string.
	//
	// Constraints:
	//
	//   - A name must contain from 1 to 50 alphanumeric characters or hyphens.
	//
	//   - The first character must be a letter.
	//
	//   - A name cannot end with a hyphen or contain two consecutive hyphens.
	//
	// +kubebuilder:validation:Required
	CacheClusterID *string `json:"cacheClusterID"`
	// The compute and memory capacity of the nodes in the node group (shard).
	//
	// The following node types are supported by ElastiCache. Generally speaking,
	// the current generation types provide more memory and computational power
	// at lower cost when compared to their equivalent previous generation counterparts.
	//
	//   - General purpose: Current generation: M7g node types: cache.m7g.large,
	//     cache.m7g.xlarge, cache.m7g.2xlarge, cache.m7g.4xlarge, cache.m7g.8xlarge,
	//     cache.m7g.12xlarge, cache.m7g.16xlarge For region availability, see Supported
	//     Node Types (https://docs.aws.amazon.com/AmazonElastiCache/latest/dg/CacheNodes.SupportedTypes.html#CacheNodes.SupportedTypesByRegion)
	//     M6g node types (available only for Redis OSS engine version 5.0.6 onward
	//     and for Memcached engine version 1.5.16 onward): cache.m6g.large, cache.m6g.xlarge,
	//     cache.m6g.2xlarge, cache.m6g.4xlarge, cache.m6g.8xlarge, cache.m6g.12xlarge,
	//     cache.m6g.16xlarge M5 node types: cache.m5.large, cache.m5.xlarge, cache.m5.2xlarge,
	//     cache.m5.4xlarge, cache.m5.12xlarge, cache.m5.24xlarge M4 node types:
	//     cache.m4.large, cache.m4.xlarge, cache.m4.2xlarge, cache.m4.4xlarge, cache.m4.10xlarge
	//     T4g node types (available only for Redis OSS engine version 5.0.6 onward
	//     and Memcached engine version 1.5.16 onward): cache.t4g.micro, cache.t4g.small,
	//     cache.t4g.medium T3 node types: cache.t3.micro, cache.t3.small, cache.t3.medium
	//     T2 node types: cache.t2.micro, cache.t2.small, cache.t2.medium Previous
	//     generation: (not recommended. Existing clusters are still supported but
	//     creation of new clusters is not supported for these types.) T1 node types:
	//     cache.t1.micro M1 node types: cache.m1.small, cache.m1.medium, cache.m1.large,
	//     cache.m1.xlarge M3 node types: cache.m3.medium, cache.m3.large, cache.m3.xlarge,
	//     cache.m3.2xlarge
	//
	//   - Compute optimized: Previous generation: (not recommended. Existing clusters
	//     are still supported but creation of new clusters is not supported for
	//     these types.) C1 node types: cache.c1.xlarge
	//
	//   - Memory optimized: Current generation: R7g node types: cache.r7g.large,
	//     cache.r7g.xlarge, cache.r7g.2xlarge, cache.r7g.4xlarge, cache.r7g.8xlarge,
	//     cache.r7g.12xlarge, cache.r7g.16xlarge For region availability, see Supported
	//     Node Types (https://docs.aws.amazon.com/AmazonElastiCache/latest/dg/CacheNodes.SupportedTypes.html#CacheNodes.SupportedTypesByRegion)
	//     R6g node types (available only for Redis OSS engine version 5.0.6 onward
	//     and for Memcached engine version 1.5.16 onward): cache.r6g.large, cache.r6g.xlarge,
	//     cache.r6g.2xlarge, cache.r6g.4xlarge, cache.r6g.8xlarge, cache.r6g.12xlarge,
	//     cache.r6g.16xlarge R5 node types: cache.r5.large, cache.r5.xlarge, cache.r5.2xlarge,
	//     cache.r5.4xlarge, cache.r5.12xlarge, cache.r5.24xlarge R4 node types:
	//     cache.r4.large, cache.r4.xlarge, cache.r4.2xlarge, cache.r4.4xlarge, cache.r4.8xlarge,
	//     cache.r4.16xlarge Previous generation: (not recommended. Existing clusters
	//     are still supported but creation of new clusters is not supported for
	//     these types.) M2 node types: cache.m2.xlarge, cache.m2.2xlarge, cache.m2.4xlarge
	//     R3 node types: cache.r3.large, cache.r3.xlarge, cache.r3.2xlarge, cache.r3.4xlarge,
	//     cache.r3.8xlarge
	//
	// Additional node type info
	//
	//   - All current generation instance types are created in Amazon VPC by default.
	//
	//   - Valkey or Redis OSS append-only files (AOF) are not supported for T1
	//     or T2 instances.
	//
	//   - Valkey or Redis OSS Multi-AZ with automatic failover is not supported
	//     on T1 instances.
	//
	//   - The configuration variables appendonly and appendfsync are not supported
	//     on Valkey, or on Redis OSS version 2.8.22 and later.
	CacheNodeType *string `json:"cacheNodeType,omitempty"`
	// The name of the parameter group to associate with this cluster. If this argument
	// is omitted, the default parameter group for the specified engine is used.
	// You cannot use any parameter group which has cluster-enabled='yes' when creating
	// a cluster.
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="Value is immutable once set"
	CacheParameterGroupName *string                                  `json:"cacheParameterGroupName,omitempty"`
	CacheParameterGroupRef  *ackv1alpha1.AWSResourceReferenceWrapper `json:"cacheParameterGroupRef,omitempty"`
	// A list of security group names to associate with this cluster.
	//
	// Use this parameter only when you are creating a cluster outside of an Amazon
	// Virtual Private Cloud (Amazon VPC).
	CacheSecurityGroupNames []*string `json:"cacheSecurityGroupNames,omitempty"`
	// The name of the subnet group to be used for the cluster.
	//
	// Use this parameter only when you are creating a cluster in an Amazon Virtual
	// Private Cloud (Amazon VPC).
	//
	// If you're going to launch your cluster in an Amazon VPC, you need to create
	// a subnet group before you start creating a cluster. For more information,
	// see Subnets and Subnet Groups (https://docs.aws.amazon.com/AmazonElastiCache/latest/dg/SubnetGroups.html).
	CacheSubnetGroupName *string                                  `json:"cacheSubnetGroupName,omitempty"`
	CacheSubnetGroupRef  *ackv1alpha1.AWSResourceReferenceWrapper `json:"cacheSubnetGroupRef,omitempty"`
	// The name of the cache engine to be used for this cluster.
	//
	// Valid values for this parameter are: memcached | redis
	Engine *string `json:"engine,omitempty"`
	// The version number of the cache engine to be used for this cluster. To view
	// the supported cache engine versions, use the DescribeCacheEngineVersions
	// operation.
	//
	// Important: You can upgrade to a newer engine version (see Selecting a Cache
	// Engine and Version (https://docs.aws.amazon.com/AmazonElastiCache/latest/dg/SelectEngine.html#VersionManagement)),
	// but you cannot downgrade to an earlier engine version. If you want to use
	// an earlier engine version, you must delete the existing cluster or replication
	// group and create it anew with the earlier engine version.
	EngineVersion *string `json:"engineVersion,omitempty"`
	// The network type you choose when modifying a cluster, either ipv4 | ipv6.
	// IPv6 is supported for workloads using Valkey 7.2 and above, Redis OSS engine
	// version 6.2 and above or Memcached engine version 1.6.6 and above on all
	// instances built on the Nitro system (http://aws.amazon.com/ec2/nitro/).
	IPDiscovery *string `json:"ipDiscovery,omitempty"`
	// Specifies the destination, format and type of the logs.
	LogDeliveryConfigurations []*LogDeliveryConfigurationRequest `json:"logDeliveryConfigurations,omitempty"`
	// Must be either ipv4 | ipv6 | dual_stack. IPv6 is supported for workloads
	// using Valkey 7.2 and above, Redis OSS engine version 6.2 and above or Memcached
	// engine version 1.6.6 and above on all instances built on the Nitro system
	// (http://aws.amazon.com/ec2/nitro/).
	NetworkType *string `json:"networkType,omitempty"`
	// The Amazon Resource Name (ARN) of the Amazon Simple Notification Service
	// (SNS) topic to which notifications are sent.
	//
	// The Amazon SNS topic owner must be the same as the cluster owner.
	NotificationTopicARN *string                                  `json:"notificationTopicARN,omitempty"`
	NotificationTopicRef *ackv1alpha1.AWSResourceReferenceWrapper `json:"notificationTopicRef,omitempty"`
	// The initial number of cache nodes that the cluster has.
	//
	// For clusters running Valkey or Redis OSS, this value must be 1. For clusters
	// running Memcached, this value must be between 1 and 40.
	//
	// If you need more than 40 nodes for your Memcached cluster, please fill out
	// the ElastiCache Limit Increase Request form at http://aws.amazon.com/contact-us/elasticache-node-limit-request/
	// (http://aws.amazon.com/contact-us/elasticache-node-limit-request/).
	NumCacheNodes *int64 `json:"numCacheNodes,omitempty"`
	// Specifies whether the nodes in the cluster are created in a single outpost
	// or across multiple outposts.
	OutpostMode *string `json:"outpostMode,omitempty"`
	// The port number on which each of the cache nodes accepts connections.
	Port *int64 `json:"port,omitempty"`
	// The EC2 Availability Zone in which the cluster is created.
	//
	// All nodes belonging to this cluster are placed in the preferred Availability
	// Zone. If you want to create your nodes across multiple Availability Zones,
	// use PreferredAvailabilityZones.
	//
	// Default: System chosen Availability Zone.
	PreferredAvailabilityZone *string `json:"preferredAvailabilityZone,omitempty"`
	// A list of the Availability Zones in which cache nodes are created. The order
	// of the zones in the list is not important.
	//
	// This option is only supported on Memcached.
	//
	// If you are creating your cluster in an Amazon VPC (recommended) you can only
	// locate nodes in Availability Zones that are associated with the subnets in
	// the selected subnet group.
	//
	// The number of Availability Zones listed must equal the value of NumCacheNodes.
	//
	// If you want all the nodes in the same Availability Zone, use PreferredAvailabilityZone
	// instead, or repeat the Availability Zone multiple times in the list.
	//
	// Default: System chosen Availability Zones.
	PreferredAvailabilityZones []*string `json:"preferredAvailabilityZones,omitempty"`
	// Specifies the weekly time range during which maintenance on the cluster is
	// performed. It is specified as a range in the format ddd:hh24:mi-ddd:hh24:mi
	// (24H Clock UTC). The minimum maintenance window is a 60 minute period.
	PreferredMaintenanceWindow *string `json:"preferredMaintenanceWindow,omitempty"`
	// The outpost ARN in which the cache cluster is created.
	PreferredOutpostARN *string `json:"preferredOutpostARN,omitempty"`
	// The outpost ARNs in which the cache cluster is created.
	PreferredOutpostARNs []*string `json:"preferredOutpostARNs,omitempty"`
	// The ID of the replication group to which this cluster should belong. If this
	// parameter is specified, the cluster is added to the specified replication
	// group as a read replica; otherwise, the cluster is a standalone primary that
	// is not part of any replication group.
	//
	// If the specified replication group is Multi-AZ enabled and the Availability
	// Zone is not specified, the cluster is created in Availability Zones that
	// provide the best spread of read replicas across Availability Zones.
	//
	// This parameter is only valid if the Engine parameter is redis.
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="Value is immutable once set"
	ReplicationGroupID  *string                                  `json:"replicationGroupID,omitempty"`
	ReplicationGroupRef *ackv1alpha1.AWSResourceReferenceWrapper `json:"replicationGroupRef,omitempty"`
	// One or more VPC security groups associated with the cluster.
	//
	// Use this parameter only when you are creating a cluster in an Amazon Virtual
	// Private Cloud (Amazon VPC).
	SecurityGroupIDs  []*string                                  `json:"securityGroupIDs,omitempty"`
	SecurityGroupRefs []*ackv1alpha1.AWSResourceReferenceWrapper `json:"securityGroupRefs,omitempty"`
	// A single-element string list containing an Amazon Resource Name (ARN) that
	// uniquely identifies a Valkey or Redis OSS RDB snapshot file stored in Amazon
	// S3. The snapshot file is used to populate the node group (shard). The Amazon
	// S3 object name in the ARN cannot contain any commas.
	//
	// This parameter is only valid if the Engine parameter is redis.
	//
	// Example of an Amazon S3 ARN: arn:aws:s3:::my_bucket/snapshot1.rdb
	SnapshotARNs []*string `json:"snapshotARNs,omitempty"`
	// The name of a Valkey or Redis OSS snapshot from which to restore data into
	// the new node group (shard). The snapshot status changes to restoring while
	// the new node group (shard) is being created.
	//
	// This parameter is only valid if the Engine parameter is redis.
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="Value is immutable once set"
	SnapshotName *string                                  `json:"snapshotName,omitempty"`
	SnapshotRef  *ackv1alpha1.AWSResourceReferenceWrapper `json:"snapshotRef,omitempty"`
	// The number of days for which ElastiCache retains automatic snapshots before
	// deleting them. For example, if you set SnapshotRetentionLimit to 5, a snapshot
	// taken today is retained for 5 days before being deleted.
	//
	// This parameter is only valid if the Engine parameter is redis.
	//
	// Default: 0 (i.e., automatic backups are disabled for this cache cluster).
	SnapshotRetentionLimit *int64 `json:"snapshotRetentionLimit,omitempty"`
	// The daily time range (in UTC) during which ElastiCache begins taking a daily
	// snapshot of your node group (shard).
	//
	// Example: 05:00-09:00
	//
	// If you do not specify this parameter, ElastiCache automatically chooses an
	// appropriate time range.
	//
	// This parameter is only valid if the Engine parameter is redis.
	SnapshotWindow *string `json:"snapshotWindow,omitempty"`
	// A list of tags to be added to this resource.
	Tags []*Tag `json:"tags,omitempty"`
	// A flag that enables in-transit encryption when set to true.
	TransitEncryptionEnabled *bool `json:"transitEncryptionEnabled,omitempty"`
}

// CacheClusterStatus defines the observed state of CacheCluster
type CacheClusterStatus struct {
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
	// A flag that enables encryption at-rest when set to true.
	//
	// You cannot modify the value of AtRestEncryptionEnabled after the cluster
	// is created. To enable at-rest encryption on a cluster you must set AtRestEncryptionEnabled
	// to true when you create a cluster.
	//
	// Required: Only available when creating a replication group in an Amazon VPC
	// using Redis OSS version 3.2.6, 4.x or later.
	//
	// Default: false
	// +kubebuilder:validation:Optional
	AtRestEncryptionEnabled *bool `json:"atRestEncryptionEnabled,omitempty"`
	// A flag that enables using an AuthToken (password) when issuing Valkey or
	// Redis OSS commands.
	//
	// Default: false
	// +kubebuilder:validation:Optional
	AuthTokenEnabled *bool `json:"authTokenEnabled,omitempty"`
	// The date the auth token was last modified
	// +kubebuilder:validation:Optional
	AuthTokenLastModifiedDate *metav1.Time `json:"authTokenLastModifiedDate,omitempty"`
	// The date and time when the cluster was created.
	// +kubebuilder:validation:Optional
	CacheClusterCreateTime *metav1.Time `json:"cacheClusterCreateTime,omitempty"`
	// The current state of this cluster, one of the following values: available,
	// creating, deleted, deleting, incompatible-network, modifying, rebooting cluster
	// nodes, restore-failed, or snapshotting.
	// +kubebuilder:validation:Optional
	CacheClusterStatus *string `json:"cacheClusterStatus,omitempty"`
	// A list of cache nodes that are members of the cluster.
	// +kubebuilder:validation:Optional
	CacheNodes []*CacheNode `json:"cacheNodes,omitempty"`
	// Status of the cache parameter group.
	// +kubebuilder:validation:Optional
	CacheParameterGroup *CacheParameterGroupStatus_SDK `json:"cacheParameterGroup,omitempty"`
	// A list of cache security group elements, composed of name and status sub-elements.
	// +kubebuilder:validation:Optional
	CacheSecurityGroups []*CacheSecurityGroupMembership `json:"cacheSecurityGroups,omitempty"`
	// The URL of the web page where you can download the latest ElastiCache client
	// library.
	// +kubebuilder:validation:Optional
	ClientDownloadLandingPage *string `json:"clientDownloadLandingPage,omitempty"`
	// Represents a Memcached cluster endpoint which can be used by an application
	// to connect to any node in the cluster. The configuration endpoint will always
	// have .cfg in it.
	//
	// Example: mem-3.9dvc4r.cfg.usw2.cache.amazonaws.com:11211
	// +kubebuilder:validation:Optional
	ConfigurationEndpoint *Endpoint `json:"configurationEndpoint,omitempty"`
	// Describes a notification topic and its status. Notification topics are used
	// for publishing ElastiCache events to subscribers using Amazon Simple Notification
	// Service (SNS).
	// +kubebuilder:validation:Optional
	NotificationConfiguration *NotificationConfiguration `json:"notificationConfiguration,omitempty"`
	// +kubebuilder:validation:Optional
	PendingModifiedValues *PendingModifiedValues `json:"pendingModifiedValues,omitempty"`
	// A boolean value indicating whether log delivery is enabled for the replication
	// group.
	// +kubebuilder:validation:Optional
	ReplicationGroupLogDeliveryEnabled *bool `json:"replicationGroupLogDeliveryEnabled,omitempty"`
	// A list of VPC Security Groups associated with the cluster.
	// +kubebuilder:validation:Optional
	SecurityGroups []*SecurityGroupMembership `json:"securityGroups,omitempty"`
	// A setting that allows you to migrate your clients to use in-transit encryption,
	// with no downtime.
	// +kubebuilder:validation:Optional
	TransitEncryptionMode *string `json:"transitEncryptionMode,omitempty"`
}

// CacheCluster is the Schema for the CacheClusters API
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="VERSION",type=string,priority=0,JSONPath=`.spec.engineVersion`
// +kubebuilder:printcolumn:name="STATUS",type=string,priority=0,JSONPath=`.status.cacheClusterStatus`
// +kubebuilder:printcolumn:name="ENDPOINT",type=string,priority=1,JSONPath=`.status.configurationEndpoint.address`
// +kubebuilder:printcolumn:name="Synced",type="string",priority=0,JSONPath=".status.conditions[?(@.type==\"ACK.ResourceSynced\")].status"
// +kubebuilder:printcolumn:name="Age",type="date",priority=0,JSONPath=".metadata.creationTimestamp"
type CacheCluster struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              CacheClusterSpec   `json:"spec,omitempty"`
	Status            CacheClusterStatus `json:"status,omitempty"`
}

// CacheClusterList contains a list of CacheCluster
// +kubebuilder:object:root=true
type CacheClusterList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []CacheCluster `json:"items"`
}

func init() {
	SchemeBuilder.Register(&CacheCluster{}, &CacheClusterList{})
}
