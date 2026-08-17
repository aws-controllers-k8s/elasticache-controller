# Copyright Amazon.com Inc. or its affiliates. All Rights Reserved.
#
# Licensed under the Apache License, Version 2.0 (the "License"). You may
# not use this file except in compliance with the License. A copy of the
# License is located at
#
# http://aws.amazon.com/apache2.0/
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
from e2e.util import retrieve_cache_cluster, retrieve_replication_group, assert_recoverable_condition_set, retrieve_replication_group_tags


RESOURCE_PLURAL = "replicationgroups"
DEFAULT_WAIT_SECS = 120


def assert_terminal_condition_from_aws(resource, expected_transition: str):
    """Assert the resource is in a terminal condition raised by the AWS API itself.

    Checking the message ensures the rejection was surfaced from the service rather than
    reimplemented as a client-side validation, which would risk drifting from what AWS
    actually allows. Client-side rejections in this controller are raised with a
    "Please refer to Events for more details" message, so their absence is asserted too.
    """
    conditions = resource.get('status', {}).get('conditions', [])
    terminal = next(
        (c for c in conditions if c['type'] == 'ACK.Terminal'), None)
    assert terminal is not None, \
        f"Expected ACK.Terminal condition for {expected_transition} transition"
    assert terminal['status'] == 'True'
    message = terminal.get('message', '')
    assert "InvalidParameter" in message, \
        f"Expected an AWS InvalidParameter* error for {expected_transition}, got: {message}"
    assert "Please refer to Events for more details" not in message, \
        f"{expected_transition} was rejected client-side rather than by AWS: {message}"


@pytest.fixture(scope="module")
def rg_deletion_waiter():
    ec = boto3.client("elasticache")
    return ec.get_waiter('replication_group_deleted')


@pytest.fixture(scope="module")
def elasticache_client():
    return boto3.client('elasticache')


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
        resource = k8s.wait_resource_consumed_by_controller(
            reference, wait_periods=15, period_length=20)
        assert resource is not None
        return (reference, resource)

    return _make_replication_group


@pytest.fixture(scope="module")
def secrets():
    secrets = {
        "NAME1": random_suffix_name("first", 32),
        "NAME2": random_suffix_name("second", 32),
        "KEY1": "secret1",
        "KEY2": "secret2"
    }
    k8s.create_opaque_secret(
        "default", secrets['NAME1'], secrets['KEY1'], random_suffix_name("token", 32))
    k8s.create_opaque_secret(
        "default", secrets['NAME2'], secrets['KEY2'], random_suffix_name("token", 32))
    yield secrets

    k8s.delete_secret("default", secrets['NAME1'])
    k8s.delete_secret("default", secrets['NAME2'])


@pytest.fixture(scope="module")
def rg_cmd_fromsnapshot(bootstrap_resources, make_rg_name, make_replication_group, rg_deletion_waiter):
    input_dict = {
        "RG_ID": make_rg_name("rg-cmd-fromsnapshot"),
        "SNAPSHOT_NAME": bootstrap_resources.SnapshotName
    }

    (reference, resource) = make_replication_group(
        "replicationgroup_cmd_fromsnapshot", input_dict, input_dict["RG_ID"])
    yield (reference, resource)

    k8s.delete_custom_resource(reference)
    sleep(DEFAULT_WAIT_SECS)
    # throws exception if wait fails
    rg_deletion_waiter.wait(ReplicationGroupId=input_dict['RG_ID'])


@pytest.fixture(scope="module")
def rg_update_input(make_rg_name):
    return {
        "RG_ID": make_rg_name("rg-update-misc"),
        "PMW": "sun:23:00-mon:02:00",
        "DESCRIPTION": "description1",
        "SRL": "5",
        "SW": "05:00-09:00",
        "IP_DISCOVERY": "ipv4",
        "NETWORK_TYPE": "ipv4"
    }


@pytest.fixture(scope="module")
def rg_update(rg_update_input, make_replication_group, rg_deletion_waiter):
    (reference, resource) = make_replication_group(
        "replicationgroup_update", rg_update_input, rg_update_input['RG_ID'])
    yield reference, resource
    k8s.delete_custom_resource(reference)
    sleep(DEFAULT_WAIT_SECS)
    # throws exception if wait fails
    rg_deletion_waiter.wait(ReplicationGroupId=rg_update_input['RG_ID'])


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
def rg_fault_tolerance(rg_fault_tolerance_input, make_replication_group, rg_deletion_waiter):
    (reference, resource) = make_replication_group(
        "replicationgroup_fault_tolerance", rg_fault_tolerance_input, rg_fault_tolerance_input['RG_ID'])
    yield reference, resource
    k8s.delete_custom_resource(reference)
    sleep(DEFAULT_WAIT_SECS)
    # throws exception if wait fails
    rg_deletion_waiter.wait(
        ReplicationGroupId=rg_fault_tolerance_input['RG_ID'])


@service_marker
class TestReplicationGroup:
    def test_rg_cmd_fromsnapshot(self, rg_cmd_fromsnapshot):
        (reference, _) = rg_cmd_fromsnapshot
        assert k8s.wait_on_condition(
            reference, "ACK.ResourceSynced", "True", wait_periods=90)

    def test_rg_invalid_primary(self, make_rg_name, make_replication_group, rg_deletion_waiter):
        input_dict = {
            "RG_ID": make_rg_name("rg-invalid-primary"),
            "PRIMARY_NODE": make_rg_name("node-dne")
        }
        (reference, resource) = make_replication_group(
            "replicationgroup_primary_cluster", input_dict, input_dict['RG_ID'])

        sleep(DEFAULT_WAIT_SECS)
        resource = k8s.get_resource(reference)
        assert_recoverable_condition_set(resource)

        # Cleanup
        k8s.delete_custom_resource(reference)
        sleep(DEFAULT_WAIT_SECS)
        # throws exception if wait fails
        rg_deletion_waiter.wait(ReplicationGroupId=input_dict['RG_ID'])

    # test update of fields that can be changed quickly

    def test_rg_update(self, rg_update_input, rg_update):
        (reference, _) = rg_update
        assert k8s.wait_on_condition(
            reference, "ACK.ResourceSynced", "True", wait_periods=90)

        # desired initial state
        cr = k8s.get_resource(reference)
        assert 'ipDiscovery' in cr['spec']
        assert cr['spec']['ipDiscovery'] == 'ipv4'
        pmw = rg_update_input['PMW']
        description = rg_update_input['DESCRIPTION']
        srl = int(rg_update_input['SRL'])
        sw = rg_update_input['SW']
        ip_discovery = rg_update_input['IP_DISCOVERY']
        network_type = rg_update_input['NETWORK_TYPE']
        tags = [
            {"key": "tag_to_remove",  "value": "should_be_removed"},
            {"key": "tag_to_update", "value": "old_value"}
        ]

        # assert initial state
        rg_id = rg_update_input['RG_ID']

        resource = k8s.get_resource(reference)
        cc = retrieve_cache_cluster(rg_id)
        rg = retrieve_replication_group(rg_id)
        assert cc is not None
        assert cc['PreferredMaintenanceWindow'] == pmw
        assert resource['spec']['description'] == description
        assert rg['SnapshotRetentionLimit'] == srl
        assert rg['SnapshotWindow'] == sw
        assert rg['IpDiscovery'] == ip_discovery
        assert rg['NetworkType'] == network_type
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
        }}
        _ = k8s.patch_custom_resource(reference, patch)
        sleep(DEFAULT_WAIT_SECS)
        assert k8s.wait_on_condition(
            reference, "ACK.ResourceSynced", "True", wait_periods=90)

        # Assert new state
        resource = k8s.get_resource(reference)
        cc = retrieve_cache_cluster(rg_id)
        rg = retrieve_replication_group(rg_id)
        assert cc is not None
        assert cc['PreferredMaintenanceWindow'] == pmw
        assert resource['spec']['description'] == description
        assert rg['SnapshotRetentionLimit'] == srl
        assert rg['SnapshotWindow'] == sw

        patch = {"spec": {
            "tags": new_tags
        }}
        _ = k8s.patch_custom_resource(reference, patch)
        # patching tags can make cluster unavailable for a while(status: modifying)
        LONG_WAIT_SECS = 180
        sleep(LONG_WAIT_SECS)
        assert k8s.wait_on_condition(
            reference, "ACK.ResourceSynced", "True", wait_periods=90)

        # assert new tags
        assert_spec_tags(rg_id, new_tags)

    # test modifying properties related to tolerance: replica promotion, multi AZ, automatic failover
    def test_rg_fault_tolerance(self, rg_fault_tolerance):
        (reference, _) = rg_fault_tolerance
        assert k8s.wait_on_condition(
            reference, "ACK.ResourceSynced", "True", wait_periods=90)

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
        patch = {"spec": {"automaticFailoverEnabled": False,
                          "multiAZEnabled": False}}
        _ = k8s.patch_custom_resource(reference, patch)
        sleep(DEFAULT_WAIT_SECS)
        assert k8s.wait_on_condition(
            reference, "ACK.ResourceSynced", "True", wait_periods=90)

        # assert new state
        resource = k8s.get_resource(reference)
        assert resource['status']['automaticFailover'] == "disabled"
        assert resource['status']['multiAZ'] == "disabled"

        # promote replica to primary, re-enable both multi AZ and AF
        patch = {"spec": {"primaryClusterID": node2,
                          "automaticFailoverEnabled": True, "multiAZEnabled": True}}
        _ = k8s.patch_custom_resource(reference, patch)
        sleep(DEFAULT_WAIT_SECS)
        assert k8s.wait_on_condition(
            reference, "ACK.ResourceSynced", "True", wait_periods=90)

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


    def test_rg_durability_update(self, make_rg_name, make_replication_group, rg_deletion_waiter):
        """Create with async durability, verify read-back and effectiveDurability, then update to sync."""
        rg_id = make_rg_name("rg-durability")
        input_dict = {"RG_ID": rg_id, "DURABILITY": "async"}

        (reference, _) = make_replication_group(
            "replicationgroup_durability", input_dict, rg_id)

        try:
            assert k8s.wait_on_condition(
                reference, "ACK.ResourceSynced", "True", wait_periods=90)

            resource = k8s.get_resource(reference)
            assert resource['spec']['durability'] == "async"
            assert resource['status'].get('effectiveDurability') == "async"
            rg = retrieve_replication_group(rg_id)
            assert rg['Durability'] == "async"

            patch = {"spec": {"durability": "sync"}}
            _ = k8s.patch_custom_resource(reference, patch)
            sleep(DEFAULT_WAIT_SECS)
            assert k8s.wait_on_condition(
                reference, "ACK.ResourceSynced", "True", wait_periods=90)

            resource = k8s.get_resource(reference)
            assert resource['spec']['durability'] == "sync"
            assert resource['status'].get('effectiveDurability') == "sync"
            rg = retrieve_replication_group(rg_id)
            assert rg['Durability'] == "sync"

            # find-only flow: spec.durability is populated from the read path
            # (DescribeReplicationGroups) and drives the generated delta comparison. Once
            # the read converges, desired == observed and no further modify is issued.

            # Confirm the resource stays converged rather than flapping back into a
            # modifying state on a later reconcile. Any flap re-transitions the synced
            # condition, so an unchanged lastTransitionTime proves it held steady.
            synced = k8s.get_resource_condition(reference, "ACK.ResourceSynced")
            assert synced is not None and synced['status'] == "True"
            synced_at = synced.get('lastTransitionTime')
            sleep(DEFAULT_WAIT_SECS)
            synced = k8s.get_resource_condition(reference, "ACK.ResourceSynced")
            assert synced is not None and synced['status'] == "True", \
                "resource did not stay synced after the durability update converged"
            assert synced.get('lastTransitionTime') == synced_at, \
                "resource left the synced state after the durability update converged"
        finally:
            k8s.delete_custom_resource(reference)
            sleep(DEFAULT_WAIT_SECS)
            rg_deletion_waiter.wait(ReplicationGroupId=rg_id)

    def test_rg_durability_drift(self, make_rg_name, make_replication_group, rg_deletion_waiter, elasticache_client):
        """Verify that a durability change made outside of ACK is detected and corrected.

        The durability difference is computed against the value the API reports, so the
        change is observed on the next reconcile and the value declared in the spec is
        re-applied. Note the controller does not poll for drift: it reconciles on its resync
        interval or when the resource changes, so this test edits an unrelated field to
        trigger a reconcile rather than waiting for the resync."""
        rg_id = make_rg_name("rg-durability-drift")
        input_dict = {"RG_ID": rg_id, "DURABILITY": "sync"}

        (reference, _) = make_replication_group(
            "replicationgroup_durability", input_dict, rg_id)

        try:
            assert k8s.wait_on_condition(
                reference, "ACK.ResourceSynced", "True", wait_periods=90)
            assert retrieve_replication_group(rg_id)['Durability'] == "sync"

            # Change durability behind the controller's back.
            elasticache_client.modify_replication_group(
                ReplicationGroupId=rg_id, Durability="async", ApplyImmediately=True)
            sleep(DEFAULT_WAIT_SECS)
            assert retrieve_replication_group(rg_id)['Durability'] == "async"

            # Edit an unrelated field to trigger a reconcile.
            _ = k8s.patch_custom_resource(
                reference, {"spec": {"description": "durability drift test"}})
            sleep(DEFAULT_WAIT_SECS)

            # The controller must notice and restore the durability declared in the spec.
            assert k8s.wait_on_condition(
                reference, "ACK.ResourceSynced", "True", wait_periods=90)
            resource = k8s.get_resource(reference)
            assert resource['spec']['durability'] == "sync"
            rg = retrieve_replication_group(rg_id)
            assert rg['Durability'] == "sync", \
                f"out-of-band durability change was not corrected: {rg['Durability']}"
        finally:
            k8s.delete_custom_resource(reference)
            sleep(DEFAULT_WAIT_SECS)
            rg_deletion_waiter.wait(ReplicationGroupId=rg_id)

    def test_rg_durability_unset(self, make_rg_name, make_replication_group, rg_deletion_waiter):
        """Verify that removing durability from the spec does not result in a the resource 
        being left in a perpetual ACK.ResourceSynced=False state or ACK.Terminal error."""
        rg_id = make_rg_name("rg-durability-unset")
        input_dict = {"RG_ID": rg_id, "DURABILITY": "async"}

        (reference, _) = make_replication_group(
            "replicationgroup_durability", input_dict, rg_id)

        try:
            assert k8s.wait_on_condition(
                reference, "ACK.ResourceSynced", "True", wait_periods=90)

            resource = k8s.get_resource(reference)
            assert resource['spec']['durability'] == "async"

            # Remove durability from the spec. A merge patch requires an explicit null to
            # delete the field rather than leaving it untouched.
            _ = k8s.patch_custom_resource(reference, {"spec": {"durability": None}})
            sleep(DEFAULT_WAIT_SECS)

            # Removing the field must not raise a terminal condition, and must not leave the
            # resource permanently out of sync attempting a modify it cannot express.
            resource = k8s.get_resource(reference)
            conditions = resource.get('status', {}).get('conditions', [])
            terminal = next(
                (c for c in conditions if c['type'] == 'ACK.Terminal'), None)
            assert terminal is None or terminal['status'] != 'True', \
                f"removing durability should not be terminal: {terminal}"
            assert k8s.wait_on_condition(
                reference, "ACK.ResourceSynced", "True", wait_periods=30)

            # The durability already applied to the replication group is left as-is.
            rg = retrieve_replication_group(rg_id)
            assert rg['Durability'] == "async"
        finally:
            k8s.delete_custom_resource(reference)
            sleep(DEFAULT_WAIT_SECS)
            rg_deletion_waiter.wait(ReplicationGroupId=rg_id)

    def test_rg_durability_disabled_guard(self, make_rg_name, make_replication_group, rg_deletion_waiter):
        """Verify that patching durability to 'disabled' is rejected by AWS as a terminal
        error (InvalidParameterCombination/InvalidParameterValue -- both in terminal_codes),
        rather than retried forever. Durability cannot be disabled once a replication group
        has been created with it enabled, which the service enforces itself: the API returns
        "Durability cannot currently be disabled on durable clusters". Note the durability
        mode may still be changed within the enabled values, async to sync and back."""
        rg_id = make_rg_name("rg-durability-guard")
        input_dict = {"RG_ID": rg_id, "DURABILITY": "async"}

        (reference, _) = make_replication_group(
            "replicationgroup_durability", input_dict, rg_id)

        try:
            assert k8s.wait_on_condition(
                reference, "ACK.ResourceSynced", "True", wait_periods=90)

            resource = k8s.get_resource(reference)
            assert resource['spec']['durability'] == "async"

            # Attempt to patch to disabled — must be rejected with a terminal condition
            patch = {"spec": {"durability": "disabled"}}
            _ = k8s.patch_custom_resource(reference, patch)
            assert k8s.wait_on_condition(
                reference, "ACK.Terminal", "True", wait_periods=30)

            resource = k8s.get_resource(reference)
            assert_terminal_condition_from_aws(resource, "async->disabled")

            # The rejected transition must not have altered the AWS-side resource.
            rg = retrieve_replication_group(rg_id)
            assert rg['Durability'] == "async"
        finally:
            k8s.delete_custom_resource(reference)
            sleep(DEFAULT_WAIT_SECS)
            rg_deletion_waiter.wait(ReplicationGroupId=rg_id)

    def test_rg_durability_default_guard(self, make_rg_name, make_replication_group, rg_deletion_waiter):
        """Verify that patching durability from 'default' to an enabled value is rejected by
        AWS as a terminal error, rather than being silently ignored. A replication group
        created with "default" reads back "default" indefinitely, since the API reports the
        requested value and only status.effectiveDurability holds the value the service
        resolved it to, so this difference must be detected and sent to AWS. The service
        treats "default" as non-durable and returns "Durability cannot currently be enabled
        on non-durable clusters"."""
        rg_id = make_rg_name("rg-durability-default-guard")
        input_dict = {"RG_ID": rg_id, "DURABILITY": "default"}

        (reference, _) = make_replication_group(
            "replicationgroup_durability", input_dict, rg_id)

        try:
            assert k8s.wait_on_condition(
                reference, "ACK.ResourceSynced", "True", wait_periods=90)

            resource = k8s.get_resource(reference)
            assert resource['spec']['durability'] == "default"
            assert resource['status'].get('effectiveDurability') == "disabled"

            # Attempt to patch away from default — must be rejected with a terminal
            # condition, not silently ignored.
            patch = {"spec": {"durability": "sync"}}
            _ = k8s.patch_custom_resource(reference, patch)
            assert k8s.wait_on_condition(
                reference, "ACK.Terminal", "True", wait_periods=30)

            resource = k8s.get_resource(reference)
            assert_terminal_condition_from_aws(resource, "default->sync")

            # The rejected transition must not have altered the AWS-side resource.
            rg = retrieve_replication_group(rg_id)
            assert rg['Durability'] == "default"
        finally:
            k8s.delete_custom_resource(reference)
            sleep(DEFAULT_WAIT_SECS)
            rg_deletion_waiter.wait(ReplicationGroupId=rg_id)

    def test_rg_creation_deletion(self, make_rg_name, make_replication_group, rg_deletion_waiter):
        input_dict = {
            "RG_ID": make_rg_name("rg-delete"),
            "ENGINE_VERSION": "6.x",
            "NUM_NODE_GROUPS": "1",
            "REPLICAS_PER_NODE_GROUP": "1"
        }

        (reference, resource) = make_replication_group(
            "replicationgroup_create_delete", input_dict, input_dict["RG_ID"])

        assert k8s.wait_on_condition(
            reference, "ACK.ResourceSynced", "True", wait_periods=90)

        # assertions after initial creation
        resource = k8s.get_resource(reference)
        assert resource['status']['status'] == "available"

        # delete
        k8s.delete_custom_resource(reference)
        sleep(DEFAULT_WAIT_SECS)

        resource = k8s.get_resource(reference)
        assert resource['metadata']['deletionTimestamp'] is not None

        rg_deletion_waiter.wait(ReplicationGroupId=input_dict["RG_ID"])
