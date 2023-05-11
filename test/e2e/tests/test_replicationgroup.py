# Copyright Amazon.com Inc. or its affiliates. All Rights Reserved.
#
# Licensed under the Apache License, Version 2.0 (the "License"). You may
# not use this file except in compliance with the License. A copy of the
# License is located at
#
#	 http://aws.amazon.com/apache2.0/
#
# or in the "license" file accompanying this file. This file is distributed
# on an "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either
# express or implied. See the License for the specific language governing
# permissions and limitations under the License.

"""Integration tests for the Elasticache ReplicationGroup resource
"""

import pytest
import boto3
import logging
from time import sleep

from acktest.resources import random_suffix_name
from acktest.k8s import resource as k8s
from e2e import service_marker, CRD_GROUP, CRD_VERSION, load_elasticache_resource
from e2e.bootstrap_resources import get_bootstrap_resources
from e2e.util import retrieve_cache_cluster, assert_even_shards_replica_count, retrieve_replication_group, \
    assert_recoverable_condition_set, retrieve_replication_group_tags


RESOURCE_PLURAL = "replicationgroups"
DEFAULT_WAIT_SECS = 30


@pytest.fixture(scope="module")
def rg_deletion_waiter():
    ec = boto3.client("elasticache")
    return ec.get_waiter('replication_group_deleted')


# delete the replication group using the provided k8s reference, and use the elasticache deletion waiter
# to wait for server-side deletion
@pytest.fixture(scope="module")
def perform_teardown(rg_deletion_waiter):
    def _perform_teardown(reference, rg_id):
        k8s.delete_custom_resource(reference)
        sleep(DEFAULT_WAIT_SECS)
        rg_deletion_waiter.wait(ReplicationGroupId=rg_id)  # throws exception if wait fails

    return _perform_teardown


# retrieve resources created in the bootstrap step
@pytest.fixture(scope="module")
def bootstrap_resources():
    return get_bootstrap_resources()


# factory for replication group names
@pytest.fixture(scope="module")
def make_rg_name():
    def _make_rg_name(base):
        return random_suffix_name(base, 32)

    return _make_rg_name


# factory for replication groups
@pytest.fixture(scope="module")
def make_replication_group():
    def _make_replication_group(yaml_name, input_dict, rg_name):
        rg = load_elasticache_resource(
            yaml_name, additional_replacements=input_dict)
        logging.debug(rg)

        reference = k8s.CustomResourceReference(
            CRD_GROUP, CRD_VERSION, RESOURCE_PLURAL, rg_name, namespace="default")
        _ = k8s.create_custom_resource(reference, rg)
        resource = k8s.wait_resource_consumed_by_controller(reference, wait_periods=10)
        assert resource is not None
        return (reference, resource)

    return _make_replication_group


@pytest.fixture(scope="module")
def rg_input_coverage(bootstrap_resources, make_rg_name, make_replication_group, perform_teardown):
    input_dict = {
        "RG_ID": make_rg_name("rg-input-coverage"),
        "KMS_KEY_ID": bootstrap_resources.KmsKeyID,
        "SNS_TOPIC_ARN": bootstrap_resources.SnsTopic1,
        "SG_ID": bootstrap_resources.SecurityGroup1,
        "USERGROUP_ID": bootstrap_resources.UserGroup1,
        "LOG_GROUP": bootstrap_resources.CWLogGroup1
    }

    (reference, resource) = make_replication_group("replicationgroup_input_coverage", input_dict, input_dict["RG_ID"])
    yield (reference, resource)

    perform_teardown(reference, input_dict['RG_ID'])


@pytest.fixture(scope="module")
def secrets():
    secrets = {
        "NAME1": random_suffix_name("first", 32),
        "NAME2": random_suffix_name("second", 32),
        "KEY1": "secret1",
        "KEY2": "secret2"
    }
    k8s.create_opaque_secret("default", secrets['NAME1'], secrets['KEY1'], random_suffix_name("token", 32))
    k8s.create_opaque_secret("default", secrets['NAME2'], secrets['KEY2'], random_suffix_name("token", 32))
    yield secrets

    # teardown
    k8s.delete_secret("default", secrets['NAME1'])
    k8s.delete_secret("default", secrets['NAME2'])


@pytest.fixture(scope="module")
def rg_auth_token(make_rg_name, make_replication_group, perform_teardown, secrets):
    input_dict = {
        "RG_ID": make_rg_name("rg-auth-token"),
        "NAME": secrets['NAME1'],
        "KEY": secrets['KEY1']
    }
    (reference, resource) = make_replication_group("replicationgroup_authtoken", input_dict, input_dict["RG_ID"])
    yield (reference, resource)

    perform_teardown(reference, input_dict['RG_ID'])


@pytest.fixture(scope="module")
def rg_invalid_primary(make_rg_name, make_replication_group, perform_teardown):
    input_dict = {
        "RG_ID": make_rg_name("rg-invalid-primary"),
        "PRIMARY_NODE": make_rg_name("node-dne")
    }
    (reference, resource) = make_replication_group("replicationgroup_primary_cluster", input_dict, input_dict['RG_ID'])
    yield reference, resource
    perform_teardown(reference, input_dict['RG_ID'])


@pytest.fixture(scope="module")
def rg_cmd_fromsnapshot(bootstrap_resources, make_rg_name, make_replication_group, perform_teardown):
    input_dict = {
        "RG_ID": make_rg_name("rg-cmd-fromsnapshot"),
        "SNAPSHOT_NAME": bootstrap_resources.SnapshotName
    }

    (reference, resource) = make_replication_group("replicationgroup_cmd_fromsnapshot", input_dict, input_dict["RG_ID"])
    yield (reference, resource)

    perform_teardown(reference, input_dict['RG_ID'])


@pytest.fixture(scope="module")
def rg_cme_uneven_shards_input(make_rg_name):
    return {
        "RG_ID": make_rg_name("rg-cme-uneven-shards"),
        "NGID1": '"1111"',
        "NGID2": '"2222"'
    }


@pytest.fixture(scope="module")
def rg_cme_uneven_shards(rg_cme_uneven_shards_input, make_replication_group, perform_teardown):
    (reference, resource) = make_replication_group("replicationgroup_cme_ngc", rg_cme_uneven_shards_input,
                                                   rg_cme_uneven_shards_input['RG_ID'])
    yield reference, resource
    perform_teardown(reference, rg_cme_uneven_shards_input['RG_ID'])


@pytest.fixture(scope="module")
def rg_cme_even_shards_input(make_rg_name):
    return {
        "RG_ID": make_rg_name("rg-cme-even-shards"),
        "NUM_NODE_GROUPS": "2",
        "REPLICAS_PER_NODE_GROUP": "2"
    }


@pytest.fixture(scope="module")
def rg_cme_even_shards(rg_cme_even_shards_input, make_replication_group, perform_teardown):
    (reference, resource) = make_replication_group("replicationgroup_rpng", rg_cme_even_shards_input,
                                                   rg_cme_even_shards_input['RG_ID'])
    yield reference, resource
    perform_teardown(reference, rg_cme_even_shards_input['RG_ID'])


@pytest.fixture(scope="module")
def rg_upgrade_ev_input(make_rg_name):
    return {
        "RG_ID": make_rg_name("rg-upgrade-ev"),
        "ENGINE_VERSION": "5.0.0",
        "NUM_NODE_GROUPS": "1",
        "REPLICAS_PER_NODE_GROUP": "1"
    }


@pytest.fixture(scope="module")
def rg_upgrade_ev(rg_upgrade_ev_input, make_replication_group, perform_teardown):
    input_dict = rg_upgrade_ev_input

    (reference, resource) = make_replication_group("replicationgroup_cmd_update", input_dict, input_dict["RG_ID"])
    yield (reference, resource)

    perform_teardown(reference, input_dict['RG_ID'])


@pytest.fixture(scope="module")
def rg_update_misc_input(make_rg_name):
    return {
        "RG_ID": make_rg_name("rg-update-misc"),
        "PMW": "sun:23:00-mon:02:00",
        "DESCRIPTION": "description1",
        "SRL": "5",
        "SW": "05:00-09:00",
    }


@pytest.fixture(scope="module")
def rg_update_misc(rg_update_misc_input, make_replication_group, perform_teardown):
    (reference, resource) = make_replication_group("replicationgroup_cme_misc", rg_update_misc_input,
                                                   rg_update_misc_input['RG_ID'])
    yield reference, resource
    perform_teardown(reference, rg_update_misc_input['RG_ID'])


# for test rg_update_misc: retrieve latest state and assert desired state
def assert_misc_fields(reference, rg_id, pmw, description, srl, sw):
    resource = k8s.get_resource(reference)
    cc = retrieve_cache_cluster(rg_id)
    rg = retrieve_replication_group(rg_id)
    assert cc is not None
    assert cc['PreferredMaintenanceWindow'] == pmw
    assert resource['spec']['description'] == description
    assert rg['SnapshotRetentionLimit'] == srl
    assert rg['SnapshotWindow'] == sw


def assert_spec_tags(rg_id: str, spec_tags: list):
    rg = retrieve_replication_group(rg_id)
    spec_tags_dict = {tag['key']: tag['value'] for tag in spec_tags}

    print("spec:", spec_tags_dict)
    aws_tag_list = retrieve_replication_group_tags(rg['ARN'])
    aws_tags_dict = {tag['Key']: tag['Value'] for tag in aws_tag_list}
    controller_tag_version = "services.k8s.aws/controller-version"
    controller_tag_namespace = "services.k8s.aws/namespace"
    del aws_tags_dict[controller_tag_version]
    del aws_tags_dict[controller_tag_namespace]

    print("aws", aws_tags_dict)
    assert aws_tags_dict == spec_tags_dict


@pytest.fixture(scope="module")
def rg_fault_tolerance_input(make_rg_name):
    return {
        "RG_ID": make_rg_name("rg-fault-tolerance"),
        "AF_ENABLED": "true",
        "MAZ_ENABLED": "true"
    }


@pytest.fixture(scope="module")
def rg_fault_tolerance(rg_fault_tolerance_input, make_replication_group, perform_teardown):
    (reference, resource) = make_replication_group("replicationgroup_fault_tolerance", rg_fault_tolerance_input,
                                                   rg_fault_tolerance_input['RG_ID'])
    yield reference, resource
    perform_teardown(reference, rg_fault_tolerance_input['RG_ID'])


@pytest.fixture(scope="module")
def rg_associate_resources_input(make_rg_name):
    return {
        "RG_ID": make_rg_name("rg-associate-resources"),
        "NUM_NODE_GROUPS": "1",
        "REPLICAS_PER_NODE_GROUP": "1"
    }


@pytest.fixture(scope="module")
def rg_associate_resources(rg_associate_resources_input, make_replication_group, perform_teardown):
    (reference, resource) = make_replication_group("replicationgroup_rpng", rg_associate_resources_input,
                                                   rg_associate_resources_input['RG_ID'])

    yield reference, resource
    perform_teardown(reference, rg_associate_resources_input['RG_ID'])


# for test rg_associate_resources
def assert_associated_resources(rg_id, sg_list, sns_topic, ug_list):
    rg = retrieve_replication_group(rg_id)
    cc = retrieve_cache_cluster(rg_id)
    assert len(cc['SecurityGroups']) == len(sg_list)
    for sg in cc['SecurityGroups']:
        assert sg['SecurityGroupId'] in sg_list
    assert cc['NotificationConfiguration']['TopicArn'] == sns_topic
    for ug_id in rg['UserGroupIds']:
        assert ug_id in ug_list


@pytest.fixture(scope="module")
def rg_update_cpg_input(make_rg_name):
    return {
        "RG_ID": make_rg_name("rg-update-cpg"),
        "ENGINE_VERSION": "6.x",
        "NUM_NODE_GROUPS": "1",
        "REPLICAS_PER_NODE_GROUP": "1"
    }


@pytest.fixture(scope="module")
def rg_update_cpg(rg_update_cpg_input, make_replication_group, perform_teardown):
    input_dict = rg_update_cpg_input

    (reference, resource) = make_replication_group("replicationgroup_cmd_update", input_dict, input_dict['RG_ID'])
    yield reference, resource

    perform_teardown(reference, input_dict['RG_ID'])


@pytest.fixture(scope="module")
def rg_scale_vertically_input(make_rg_name):
    return {
        "RG_ID": make_rg_name("rg-scale-vertically"),
        "NUM_NODE_GROUPS": "2",
        "REPLICAS_PER_NODE_GROUP": "1"
    }


@pytest.fixture(scope="module")
def rg_scale_vertically(rg_scale_vertically_input, make_replication_group, perform_teardown):
    (reference, resource) = make_replication_group("replicationgroup_rpng", rg_scale_vertically_input,
                                                   rg_scale_vertically_input['RG_ID'])

    yield reference, resource
    perform_teardown(reference, rg_scale_vertically_input['RG_ID'])


@pytest.fixture(scope="module")
def rg_scale_horizontally_input(make_rg_name):
    return {
        "RG_ID": make_rg_name("rg-scale-horizontally"),
        "NUM_NODE_GROUPS": "2",
        "REPLICAS_PER_NODE_GROUP": "1"
    }


@pytest.fixture(scope="module")
def rg_scale_horizontally(rg_scale_horizontally_input, make_replication_group, perform_teardown):
    (reference, resource) = make_replication_group("replicationgroup_rpng", rg_scale_horizontally_input,
                                                   rg_scale_horizontally_input['RG_ID'])

    yield reference, resource
    perform_teardown(reference, rg_scale_horizontally_input['RG_ID'])


@pytest.fixture(scope="module")
def rg_log_delivery_input(make_rg_name):
    return {
        "RG_ID": make_rg_name("rg-log-delivery"),
        "NUM_NODE_GROUPS": "1",
        "REPLICAS_PER_NODE_GROUP": "1"
    }


@pytest.fixture(scope="module")
def rg_log_delivery(rg_log_delivery_input, make_replication_group, perform_teardown):
    (reference, resource) = make_replication_group("replicationgroup_rpng", rg_log_delivery_input,
                                                   rg_log_delivery_input['RG_ID'])
    yield reference, resource
    perform_teardown(reference, rg_log_delivery_input['RG_ID'])


# assert that the latest state of the replication group matches the desired configuration
def assert_log_delivery_config(reference, config):
    resource = k8s.get_resource(reference)

    # if log delivery is disabled, logDeliveryConfigurations should be empty or none
    if not config['enabled']:
        assert 'logDeliveryConfigurations' not in resource['status']
    else:
        latest = resource['status']['logDeliveryConfigurations'][0]
        assert latest['status'] == "active"
        assert latest['destinationDetails'] == config['destinationDetails']
        assert latest['destinationType'] == config['destinationType']
        assert latest['logFormat'] == config['logFormat']
        assert latest['logType'] == config['logType']


@pytest.fixture(scope="module")
def rg_deletion_input(make_rg_name):
    return {
        "RG_ID": make_rg_name("rg-delete"),
        "ENGINE_VERSION": "6.x",
        "NUM_NODE_GROUPS": "1",
        "REPLICAS_PER_NODE_GROUP": "1"
    }


@pytest.fixture(scope="module")
def rg_deletion(rg_deletion_input, make_replication_group):
    input_dict = rg_deletion_input

    (reference, resource) = make_replication_group("replicationgroup_cmd_update", input_dict, input_dict["RG_ID"])
    return (reference, resource)  # no teardown, as the teardown is part of the actual test


@service_marker
class TestReplicationGroup:

    def test_rg_input_coverage(self, rg_input_coverage):
        (reference, _) = rg_input_coverage
        assert k8s.wait_on_condition(reference, "ACK.ResourceSynced", "True", wait_periods=90)

    def test_rg_cmd_fromsnapshot(self, rg_cmd_fromsnapshot):
        (reference, _) = rg_cmd_fromsnapshot
        assert k8s.wait_on_condition(reference, "ACK.ResourceSynced", "True", wait_periods=90)

    # if primaryClusterID is a nonexistent node, the recoverable condition should be set
    def test_rg_invalid_primary(self, rg_invalid_primary):
        (reference, _) = rg_invalid_primary
        sleep(DEFAULT_WAIT_SECS)

        resource = k8s.get_resource(reference)
        assert_recoverable_condition_set(resource)

    # increase and decrease replica counts per-shard in a CME RG
    @pytest.mark.blocked  # TODO: remove when passing
    def test_rg_cme_uneven_shards(self, rg_cme_uneven_shards, rg_cme_uneven_shards_input):
        (reference, _) = rg_cme_uneven_shards
        assert k8s.wait_on_condition(reference, "ACK.ResourceSynced", "True", wait_periods=90)
        ngid1 = rg_cme_uneven_shards_input['NGID1'][1:-1]  # need to strip double quotes off node group ID
        ngid2 = rg_cme_uneven_shards_input['NGID2'][1:-1]

        # assert initial state
        resource = k8s.get_resource(reference)
        assert len(resource['status']['nodeGroups']) == 2
        for ng in resource['status']['nodeGroups']:
            if ng['nodeGroupID'] == ngid1:
                assert len(ng['nodeGroupMembers']) == 3
            elif ng['nodeGroupID'] == ngid2:
                assert len(ng['nodeGroupMembers']) == 3
            else:  # node group with unknown ID
                assert False

        # increase replica count of first shard, decrease replica count of second, and wait for resource to sync
        patch = {"spec": {"nodeGroupConfiguration": [
                    {
                        "nodeGroupID": ngid1,
                        "primaryAvailabilityZone": "us-west-2a",
                        "replicaAvailabilityZones": ["us-west-2b", "us-west-2c", "us-west-2a"],
                        "replicaCount": 3
                    },
                    {
                        "nodeGroupID": ngid2,
                        "primaryAvailabilityZone": "us-west-2b",
                        "replicaAvailabilityZones": ["us-west-2c"],
                        "replicaCount": 1
                    }
                ]
            }
        }
        _ = k8s.patch_custom_resource(reference, patch)
        sleep(DEFAULT_WAIT_SECS)
        assert k8s.wait_on_condition(reference, "ACK.ResourceSynced", "True", wait_periods=90)

        # assert new state
        resource = k8s.get_resource(reference)
        assert len(resource['status']['nodeGroups']) == 2
        for ng in resource['status']['nodeGroups']:
            if ng['nodeGroupID'] == ngid1:
                assert len(ng['nodeGroupMembers']) == 4
            elif ng['nodeGroupID'] == ngid2:
                assert len(ng['nodeGroupMembers']) == 2
            else:  # node group with unknown ID
                assert False

    # increase and decrease replica count evenly across all shards in a CME RG
    def test_rg_cme_even_shards(self, rg_cme_even_shards, rg_cme_even_shards_input):
        (reference, _) = rg_cme_even_shards
        assert k8s.wait_on_condition(reference, "ACK.ResourceSynced", "True", wait_periods=90)
        nng = int(rg_cme_even_shards_input['NUM_NODE_GROUPS'])
        rpng = int(rg_cme_even_shards_input['REPLICAS_PER_NODE_GROUP'])

        # assert initial state
        resource = k8s.get_resource(reference)
        assert len(resource['status']['nodeGroups']) == nng
        assert_even_shards_replica_count(resource, rpng)

        # increase replica count, wait for resource to sync
        rpng += 1
        patch = {"spec": {"replicasPerNodeGroup": rpng}}
        _ = k8s.patch_custom_resource(reference, patch)
        sleep(DEFAULT_WAIT_SECS)
        assert k8s.wait_on_condition(reference, "ACK.ResourceSynced", "True", wait_periods=90)

        # assert replica count has increased
        resource = k8s.get_resource(reference)
        assert len(resource['status']['nodeGroups']) == nng
        assert_even_shards_replica_count(resource, rpng)

        # decrease replica count, wait for resource to sync
        rpng -= 2
        patch = {"spec": {"replicasPerNodeGroup": rpng}}
        _ = k8s.patch_custom_resource(reference, patch)
        sleep(DEFAULT_WAIT_SECS)
        assert k8s.wait_on_condition(reference, "ACK.ResourceSynced", "True", wait_periods=90)

        # assert replica count has decreased
        resource = k8s.get_resource(reference)
        assert len(resource['status']['nodeGroups']) == nng
        assert_even_shards_replica_count(resource, rpng)

    # test update behavior of controller (engine version and replica count)
    def test_rg_upgrade_ev(self, rg_upgrade_ev_input, rg_upgrade_ev):
        (reference, _) = rg_upgrade_ev
        assert k8s.wait_on_condition(reference, "ACK.ResourceSynced", "True", wait_periods=90)

        # assert initial state
        cc = retrieve_cache_cluster(rg_upgrade_ev_input['RG_ID'])
        assert cc is not None
        assert cc['EngineVersion'] == rg_upgrade_ev_input['ENGINE_VERSION']

        # upgrade engine version, wait for resource to sync
        desired_engine_version = "5.0.6"
        patch = {"spec": {"engineVersion": desired_engine_version}}
        _ = k8s.patch_custom_resource(reference, patch)
        sleep(DEFAULT_WAIT_SECS)
        assert k8s.wait_on_condition(reference, "ACK.ResourceSynced", "True", wait_periods=90)

        # assert new state after upgrading engine version
        resource = k8s.get_resource(reference)
        assert resource['status']['status'] == "available"
        assert resource['spec']['engineVersion'] == desired_engine_version
        cc = retrieve_cache_cluster(rg_upgrade_ev_input['RG_ID'])
        assert cc is not None
        assert cc['EngineVersion'] == desired_engine_version

    # test update of fields that can be changed quickly
    def test_rg_update_misc(self, rg_update_misc_input, rg_update_misc):
        (reference, _) = rg_update_misc
        assert k8s.wait_on_condition(reference, "ACK.ResourceSynced", "True", wait_periods=90)

        # desired initial state
        pmw = rg_update_misc_input['PMW']
        description = rg_update_misc_input['DESCRIPTION']
        srl = int(rg_update_misc_input['SRL'])
        sw = rg_update_misc_input['SW']
        tags = [
            {"key": "tag_to_remove",  "value": "should_be_removed"},
            {"key": "tag_to_update", "value": "old_value"}
        ]

        # assert initial state
        rg_id = rg_update_misc_input['RG_ID']
        assert_misc_fields(reference, rg_id, pmw, description, srl, sw)
        assert_spec_tags(rg_id, tags)

        # change field values, wait for resource to sync
        pmw = "wed:10:00-wed:14:00"
        description = "description2"
        srl = 0
        sw = "15:00-17:00"
        new_tags = [
            {"key": "tag_to_update", "value": "new value"},
            {"key": "tag_to_add", "value": "add"}
        ]
        patch = {"spec": {
            "preferredMaintenanceWindow": pmw,
            "description": description,
            "snapshotRetentionLimit": srl,
            "snapshotWindow": sw,
        }
        }
        _ = k8s.patch_custom_resource(reference, patch)
        sleep(DEFAULT_WAIT_SECS)
        assert k8s.wait_on_condition(reference, "ACK.ResourceSynced", "True", wait_periods=90)

        # assert new state
        assert_misc_fields(reference, rg_id, pmw, description, srl, sw)

        patch = {"spec": {
               "tags": new_tags
            }
        }
        _ = k8s.patch_custom_resource(reference, patch)
        # patching tags can make cluster unavailable for a while(status: modifying)
        LONG_WAIT_SECS = 180
        sleep(LONG_WAIT_SECS)
        assert k8s.wait_on_condition(reference, "ACK.ResourceSynced", "True", wait_periods=90)

        # assert new tags
        assert_spec_tags(rg_id, new_tags)

    # test modifying properties related to tolerance: replica promotion, multi AZ, automatic failover
    def test_rg_fault_tolerance(self, rg_fault_tolerance):
        (reference, _) = rg_fault_tolerance
        assert k8s.wait_on_condition(reference, "ACK.ResourceSynced", "True", wait_periods=90)

        # assert initial state
        resource = k8s.get_resource(reference)
        assert resource['status']['automaticFailover'] == "enabled"
        assert resource['status']['multiAZ'] == "enabled"

        # retrieve current names of primary (currently node1) and replica (currently node2)
        members = resource['status']['nodeGroups'][0]['nodeGroupMembers']
        assert len(members) == 2
        node1 = None
        node2 = None
        for node in members:
            if node['currentRole'] == 'primary':
                node1 = node['cacheClusterID']
            elif node['currentRole'] == 'replica':
                node2 = node['cacheClusterID']
        assert node1 is not None and node2 is not None

        # disable both fields, wait for resource to sync
        patch = {"spec": {"automaticFailoverEnabled": False, "multiAZEnabled": False}}
        _ = k8s.patch_custom_resource(reference, patch)
        sleep(DEFAULT_WAIT_SECS)
        assert k8s.wait_on_condition(reference, "ACK.ResourceSynced", "True", wait_periods=90)

        # assert new state
        resource = k8s.get_resource(reference)
        assert resource['status']['automaticFailover'] == "disabled"
        assert resource['status']['multiAZ'] == "disabled"

        # promote replica to primary, re-enable both multi AZ and AF
        patch = {"spec": {"primaryClusterID": node2, "automaticFailoverEnabled": True, "multiAZEnabled": True}}
        _ = k8s.patch_custom_resource(reference, patch)
        sleep(DEFAULT_WAIT_SECS)
        assert k8s.wait_on_condition(reference, "ACK.ResourceSynced", "True", wait_periods=90)

        # assert roles
        resource = k8s.get_resource(reference)
        members = resource['status']['nodeGroups'][0]['nodeGroupMembers']
        assert len(members) == 2
        for node in members:
            if node['cacheClusterID'] == node1:
                assert node['currentRole'] == 'replica'
            elif node['cacheClusterID'] == node2:
                assert node['currentRole'] == 'primary'
            else:
                raise AssertionError(f"Unknown node {node['cacheClusterID']}")

        # assert AF and multi AZ
        assert resource['status']['automaticFailover'] == "enabled"
        assert resource['status']['multiAZ'] == "enabled"

    # test association and disassociation of other resources (VPC security groups, SNS topic, user groups)
    @pytest.mark.blocked  # TODO: remove when passing
    def test_rg_associate_resources(self, rg_associate_resources_input, rg_associate_resources, bootstrap_resources):
        (reference, _) = rg_associate_resources
        assert k8s.wait_on_condition(reference, "ACK.ResourceSynced", "True", wait_periods=90)

        # associate resources, wait for RG to sync
        sg_list = [bootstrap_resources.SecurityGroup1, bootstrap_resources.SecurityGroup2]
        sns_topic = bootstrap_resources.SnsTopic1
        ug_list = [bootstrap_resources.UserGroup1]
        patch = {"spec": {"securityGroupIDs": sg_list, "notificationTopicARN": sns_topic, "userGroupIDs": ug_list}}
        _ = k8s.patch_custom_resource(reference, patch)
        sleep(DEFAULT_WAIT_SECS)
        assert k8s.wait_on_condition(reference, "ACK.ResourceSynced", "True", wait_periods=90)

        # assert new state
        assert_associated_resources(rg_associate_resources_input['RG_ID'], sg_list, sns_topic, ug_list)

        # change associated resources
        sg_list = [bootstrap_resources.SecurityGroup2]
        sns_topic = bootstrap_resources.SnsTopic2
        ug_list = [bootstrap_resources.UserGroup2]
        patch = {"spec": {"securityGroupIDs": sg_list, "notificationTopicARN": sns_topic, "userGroupIDs": ug_list}}
        _ = k8s.patch_custom_resource(reference, patch)
        sleep(DEFAULT_WAIT_SECS)
        assert k8s.wait_on_condition(reference, "ACK.ResourceSynced", "True", wait_periods=90)

        # assert new state
        assert_associated_resources(rg_associate_resources_input['RG_ID'], sg_list, sns_topic, ug_list)

    def test_rg_update_cpg(self, rg_update_cpg_input, rg_update_cpg, bootstrap_resources):
        # wait for resource to sync and retrieve initial state
        (reference, _) = rg_update_cpg
        assert k8s.wait_on_condition(reference, "ACK.ResourceSynced", "True", wait_periods=90)

        # update, wait for resource to sync
        patch = {"spec": {"cacheParameterGroupName": bootstrap_resources.CPGName}}
        _ = k8s.patch_custom_resource(reference, patch)
        sleep(DEFAULT_WAIT_SECS)
        assert k8s.wait_on_condition(reference, "ACK.ResourceSynced", "True", wait_periods=5)  # should be immediate

        # assert new state
        cc = retrieve_cache_cluster(rg_update_cpg_input['RG_ID'])
        assert cc['CacheParameterGroup']['CacheParameterGroupName'] == bootstrap_resources.CPGName

    @pytest.mark.blocked  # TODO: remove when passing
    def test_rg_scale_vertically(self, rg_scale_vertically_input, rg_scale_vertically):
        (reference, _) = rg_scale_vertically
        assert k8s.wait_on_condition(reference, "ACK.ResourceSynced", "True", wait_periods=90)

        # assert initial state
        rg = retrieve_replication_group(rg_scale_vertically_input['RG_ID'])
        assert rg['CacheNodeType'] == "cache.t3.micro"

        # scale up
        cnt = "cache.t3.medium"
        patch = {"spec": {"cacheNodeType": cnt}}
        _ = k8s.patch_custom_resource(reference, patch)
        sleep(DEFAULT_WAIT_SECS)
        assert k8s.wait_on_condition(reference, "ACK.ResourceSynced", "True", wait_periods=90)

        # assert scale up complete
        rg = retrieve_replication_group(rg_scale_vertically_input['RG_ID'])
        assert rg['CacheNodeType'] == cnt

        # scale down
        cnt = "cache.t3.small"
        patch = {"spec": {"cacheNodeType": cnt}}
        _ = k8s.patch_custom_resource(reference, patch)
        sleep(DEFAULT_WAIT_SECS)
        assert k8s.wait_on_condition(reference, "ACK.ResourceSynced", "True", wait_periods=90)

        # assert scale down complete
        rg = retrieve_replication_group(rg_scale_vertically_input['RG_ID'])
        assert rg['CacheNodeType'] == cnt

    @pytest.mark.blocked  # TODO: remove when passing
    def test_rg_scale_horizontally(self, rg_scale_horizontally_input, rg_scale_horizontally):
        (reference, _) = rg_scale_horizontally
        assert k8s.wait_on_condition(reference, "ACK.ResourceSynced", "True", wait_periods=90)

        # assert initial state
        rg = retrieve_replication_group(rg_scale_horizontally_input['RG_ID'])
        nng = int(rg_scale_horizontally_input['NUM_NODE_GROUPS'])
        assert len(rg['NodeGroups']) == nng

        # scale out
        nng += 1
        patch = {"spec": {"numNodeGroups": nng}}
        _ = k8s.patch_custom_resource(reference, patch)
        sleep(DEFAULT_WAIT_SECS)
        assert k8s.wait_on_condition(reference, "ACK.ResourceSynced", "True", wait_periods=90)

        # assert scale out complete
        rg = retrieve_replication_group(rg_scale_horizontally_input['RG_ID'])
        assert len(rg['NodeGroups']) == nng

        # scale in
        nng -= 2
        patch = {"spec": {"numNodeGroups": nng}}
        _ = k8s.patch_custom_resource(reference, patch)
        sleep(DEFAULT_WAIT_SECS)
        assert k8s.wait_on_condition(reference, "ACK.ResourceSynced", "True", wait_periods=90)

        # assert scale in complete
        rg = retrieve_replication_group(rg_scale_horizontally_input['RG_ID'])
        assert len(rg['NodeGroups']) == nng

    # add and modify log delivery configuration to replication group
    def test_rg_log_delivery(self, rg_log_delivery_input, rg_log_delivery, bootstrap_resources):
        (reference, _) = rg_log_delivery
        assert k8s.wait_on_condition(reference, "ACK.ResourceSynced", "True", wait_periods=90)

        # add log delivery config
        config = {
            "destinationDetails": {
                "cloudWatchLogsDetails": {
                    "logGroup": bootstrap_resources.CWLogGroup1
                }
            },
            "destinationType": "cloudwatch-logs",
            "enabled": True,
            "logFormat": "json",
            "logType": "slow-log"
        }
        patch = {"spec": {"logDeliveryConfigurations": [config]}}
        k8s.patch_custom_resource(reference, patch)
        sleep(DEFAULT_WAIT_SECS)
        assert k8s.wait_on_condition(reference, "ACK.ResourceSynced", "True", wait_periods=90)

        # assert log delivery added
        assert_log_delivery_config(reference, config)

        # change target log group and log format
        config['destinationDetails']['cloudWatchLogsDetails']['logGroup'] = bootstrap_resources.CWLogGroup2
        config['logFormat'] = "text"
        patch = {"spec": {"logDeliveryConfigurations": [config]}}
        k8s.patch_custom_resource(reference, patch)
        sleep(DEFAULT_WAIT_SECS)
        assert k8s.wait_on_condition(reference, "ACK.ResourceSynced", "True", wait_periods=90)

        # assert configuration modified
        assert_log_delivery_config(reference, config)

        # change to nonexistent log group and ensure error status/message found
        config['destinationDetails']['cloudWatchLogsDetails']['logGroup'] = random_suffix_name("dne", 16)
        patch = {"spec": {"logDeliveryConfigurations": [config]}}
        k8s.patch_custom_resource(reference, patch)
        sleep(DEFAULT_WAIT_SECS)
        assert k8s.wait_on_condition(reference, "ACK.ResourceSynced", "True", wait_periods=90)

        # assert error message present
        resource = k8s.get_resource(reference)
        latest = resource['status']['logDeliveryConfigurations'][0]
        assert 'does not exist' in latest['message']

        # disable log delivery
        config = {"logType": "slow-log", "enabled": False}
        patch = {"spec": {"logDeliveryConfigurations": [config]}}
        k8s.patch_custom_resource(reference, patch)
        sleep(DEFAULT_WAIT_SECS)
        assert k8s.wait_on_condition(reference, "ACK.ResourceSynced", "True", wait_periods=90)

        # assert log delivery disabled
        assert_log_delivery_config(reference, config)

    def test_rg_auth_token(self, rg_auth_token, secrets):
        (reference, _) = rg_auth_token
        assert k8s.wait_on_condition(reference, "ACK.ResourceSynced", "True", wait_periods=90)

        patch = {"spec": {"authToken": {"name": secrets['NAME2'], "key": secrets['KEY2']}}}
        k8s.patch_custom_resource(reference, patch)
        sleep(DEFAULT_WAIT_SECS)
        assert k8s.wait_on_condition(reference, "ACK.ResourceSynced", "True", wait_periods=90)

    def test_rg_deletion(self, rg_deletion_input, rg_deletion, rg_deletion_waiter):
        (reference, _) = rg_deletion
        assert k8s.wait_on_condition(reference, "ACK.ResourceSynced", "True", wait_periods=90)

        # assertions after initial creation
        resource = k8s.get_resource(reference)
        assert resource['status']['status'] == "available"

        # delete
        k8s.delete_custom_resource(reference)
        sleep(DEFAULT_WAIT_SECS)

        resource = k8s.get_resource(reference)
        assert resource['metadata']['deletionTimestamp'] is not None
        # TODO: uncomment when reconciler->cleanup() invokes patchResource()
        # assert k8s.wait_on_condition(reference, "ACK.ResourceSynced", "False", wait_periods=1)

        rg_deletion_waiter.wait(ReplicationGroupId=rg_deletion_input["RG_ID"])
