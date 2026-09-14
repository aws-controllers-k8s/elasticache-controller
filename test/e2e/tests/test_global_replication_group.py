# Copyright Amazon.com Inc. or its affiliates. All Rights Reserved.
#
# Licensed under the Apache License, Version 2.0 (the "License"). You may
# not use this file except in compliance with the License. A copy of the
# License is located at
#
#     http://aws.amazon.com/apache2.0/
#
# or in the "license" file accompanying this file. This file is distributed
# on an "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either
# express or implied. See the License for the specific language governing
# permissions and limitations under the License.

"""Integration tests for the ElastiCache GlobalReplicationGroup resource.

Scope is deliberately limited to paths that are cheap against real AWS. Shard
scaling is the slowest Global Datastore operation by a wide margin -- ElastiCache
holds the datastore in 'modifying' for the whole slot migration -- so the shard
count decisions are covered by unit tests over the delta classifier in
pkg/resource/global_replication_group/hooks_test.go instead of here, keeping this
suite off the critical path of every elasticache presubmit.
"""

import logging
from time import sleep

import boto3
import pytest
from acktest.k8s import condition
from acktest.k8s import resource as k8s
from acktest.resources import random_suffix_name

from e2e import CRD_GROUP, CRD_VERSION, load_elasticache_resource, service_marker

RESOURCE_PLURAL_GRG = "globalreplicationgroups"
RESOURCE_PLURAL_RG = "replicationgroups"

# The prerequisite primary can take well over a plain 15-minute budget to reach
# 'available' when the suite runs every elasticache resource in parallel. Each wait
# returns as soon as its target state is reached, so a generous ceiling costs
# nothing on the happy path and only absorbs that tail.
RG_AVAILABLE_WAIT_PERIODS = 40
RG_AVAILABLE_PERIOD_LENGTH = 30

CREATE_WAIT_PERIODS = 30
CREATE_PERIOD_LENGTH = 30
MODIFY_WAIT_PERIODS = 30
MODIFY_PERIOD_LENGTH = 30
DELETE_WAIT_PERIODS = 40
DELETE_PERIOD_LENGTH = 30

STEADY_STATES = ("primary-only", "available")

ec = boto3.client("elasticache")


def get_global_replication_group(global_rg_id: str):
    try:
        resp = ec.describe_global_replication_groups(
            GlobalReplicationGroupId=global_rg_id,
            ShowMemberInfo=True,
        )
    except ec.exceptions.GlobalReplicationGroupNotFoundFault:
        return None
    groups = resp.get("GlobalReplicationGroups") or []
    return groups[0] if groups else None


def wait_global_replication_group_status(
    global_rg_id: str,
    target_status: str,
    wait_periods: int = CREATE_WAIT_PERIODS,
    period_length: int = CREATE_PERIOD_LENGTH,
) -> bool:
    """Wait until the datastore reports target_status, or is gone for 'deleted'."""
    for i in range(wait_periods):
        grg = get_global_replication_group(global_rg_id)
        if grg is None:
            return target_status == "deleted"
        current = grg.get("Status")
        logging.debug(
            f"GlobalReplicationGroup {global_rg_id} status={current} "
            f"waiting for {target_status} ({i}/{wait_periods})"
        )
        if current == target_status:
            return True
        sleep(period_length)
    return False


def wait_global_attribute(
    global_rg_id: str,
    attribute: str,
    expected,
    wait_periods: int = MODIFY_WAIT_PERIODS,
    period_length: int = MODIFY_PERIOD_LENGTH,
) -> bool:
    """Wait until an attribute matches AND the datastore has settled.

    Polling on status alone is racy: the datastore keeps its pre-patch steady state
    for a few seconds after the spec is patched, before the controller issues the
    modify and AWS flips the status to 'modifying'. Requiring both the value and a
    steady state means the wait cannot pass on the pre-patch state.
    """
    for i in range(wait_periods):
        grg = get_global_replication_group(global_rg_id)
        if grg is not None:
            actual = grg.get(attribute)
            status = grg.get("Status")
            logging.debug(
                f"GlobalReplicationGroup {global_rg_id} {attribute}={actual} "
                f"status={status} waiting for {expected} ({i}/{wait_periods})"
            )
            if actual == expected and status in STEADY_STATES:
                return True
        sleep(period_length)
    return False


def wait_primary_automatic_failover(
    global_rg_id: str,
    expected_enabled: bool,
    wait_periods: int = MODIFY_WAIT_PERIODS,
    period_length: int = MODIFY_PERIOD_LENGTH,
) -> bool:
    """Wait until the primary member's automatic failover matches and it settled.

    The Describe response has no top-level automatic-failover attribute; it is only
    observable per member, which is why the controller derives it from member state.
    """
    target = {"enabled", "enabling"} if expected_enabled else {"disabled", "disabling"}
    for i in range(wait_periods):
        grg = get_global_replication_group(global_rg_id)
        if grg is not None:
            primary = next(
                (m for m in (grg.get("Members") or []) if m.get("Role") == "PRIMARY"),
                None,
            )
            failover = primary.get("AutomaticFailover") if primary else None
            status = grg.get("Status")
            logging.debug(
                f"GlobalReplicationGroup {global_rg_id} primary failover={failover} "
                f"status={status} waiting for enabled={expected_enabled} ({i}/{wait_periods})"
            )
            if failover in target and status in STEADY_STATES:
                return True
        sleep(period_length)
    return False


def wait_replication_group_available(rg_id: str) -> bool:
    for i in range(RG_AVAILABLE_WAIT_PERIODS):
        try:
            rg = ec.describe_replication_groups(ReplicationGroupId=rg_id)["ReplicationGroups"][0]
            if rg["Status"] == "available":
                return True
        except Exception as e:
            logging.warning(f"error checking ReplicationGroup {rg_id}: {e}")
        logging.debug(f"waiting for ReplicationGroup {rg_id} to be available ({i})")
        sleep(RG_AVAILABLE_PERIOD_LENGTH)
    return False


def wait_k8s_global_id(reference, wait_periods: int = 10, period_length: int = 10):
    """Poll until the controller records the AWS-generated full datastore ID."""
    for _ in range(wait_periods):
        resource = k8s.get_resource(reference)
        global_id = (resource or {}).get("status", {}).get("globalReplicationGroupID")
        if global_id:
            return global_id
        sleep(period_length)
    return None


def assert_condition_not_true(reference, condition_type: str):
    """Assert a condition is absent or not True, as a point-in-time read.

    Only meaningful after a positive wait has already let real time elapse. Written
    as a single read rather than a short wait_on_condition, because a wait with a
    tiny budget would pass simply by not giving the condition time to appear.
    """
    cond = k8s.get_resource_condition(reference, condition_type)
    assert cond is None or cond.get("status") != "True", (
        f"{condition_type} was unexpectedly True: {cond}"
    )


def create_grg(suffix: str, primary_rg_id: str, description: str, extra_spec: dict = None):
    """Create a GlobalReplicationGroup and wait for it to reach primary-only."""
    grg_name = f"grg-{suffix}"
    grg_resource = load_elasticache_resource(
        "global_replication_group_basic",
        additional_replacements={
            "GRG_NAME": grg_name,
            "GRG_SUFFIX": suffix,
            "PRIMARY_RG_ID": primary_rg_id,
            "DESCRIPTION": description,
        },
    )
    if extra_spec:
        grg_resource["spec"].update(extra_spec)

    reference = k8s.CustomResourceReference(
        CRD_GROUP, CRD_VERSION, RESOURCE_PLURAL_GRG, grg_name, namespace="default"
    )
    _ = k8s.create_custom_resource(reference, grg_resource)
    assert k8s.wait_resource_consumed_by_controller(
        reference, wait_periods=15, period_length=20
    ) is not None

    global_id = wait_k8s_global_id(reference)
    assert global_id is not None, "status.globalReplicationGroupID was never populated"
    assert wait_global_replication_group_status(global_id, "primary-only"), (
        f"GlobalReplicationGroup {global_id} did not reach primary-only"
    )
    # IsSynced is status-based, so waiting on the condition guarantees the CR's own
    # status already reflects the state asserted above.
    assert k8s.wait_on_condition(
        reference, condition.CONDITION_TYPE_RESOURCE_SYNCED, "True",
        wait_periods=10, period_length=15,
    ), "resource did not reach ACK.ResourceSynced=True after create"
    return reference, global_id


def delete_grg(reference, global_id: str):
    k8s.delete_custom_resource(reference)
    assert wait_global_replication_group_status(
        global_id, "deleted",
        wait_periods=DELETE_WAIT_PERIODS,
        period_length=DELETE_PERIOD_LENGTH,
    ), f"GlobalReplicationGroup {global_id} was not deleted"


@pytest.fixture(scope="module")
def primary_replication_group():
    """A ReplicationGroup that satisfies the Global Datastore primary requirements."""
    rg_id = random_suffix_name("ack-e2e-grg-primary", 32)
    rg_resource = load_elasticache_resource(
        "replicationgroup_create_delete_grg",
        additional_replacements={
            "RG_ID": rg_id,
            "ENGINE_VERSION": "7.0",
            "NUM_NODE_GROUPS": "1",
            "REPLICAS_PER_NODE_GROUP": "1",
        },
    )
    reference = k8s.CustomResourceReference(
        CRD_GROUP, CRD_VERSION, RESOURCE_PLURAL_RG, rg_id, namespace="default"
    )
    _ = k8s.create_custom_resource(reference, rg_resource)
    assert k8s.wait_resource_consumed_by_controller(
        reference, wait_periods=15, period_length=20
    ) is not None
    assert wait_replication_group_available(rg_id), (
        f"primary ReplicationGroup {rg_id} did not become available"
    )

    yield rg_id

    k8s.delete_custom_resource(reference)
    try:
        ec.get_waiter("replication_group_deleted").wait(
            ReplicationGroupId=rg_id,
            WaiterConfig={"Delay": 30, "MaxAttempts": 40},
        )
    except Exception as e:
        logging.warning(f"cleanup of ReplicationGroup {rg_id} did not complete: {e}")


@service_marker
class TestGlobalReplicationGroupLifecycle:
    def test_create_and_verify(self, primary_replication_group):
        """Create, reach primary-only and Synced, then delete, retaining the primary."""
        rg_id = primary_replication_group
        suffix = random_suffix_name("ack-e2e-grg", 20)

        reference, global_id = create_grg(suffix, rg_id, "ACK E2E basic lifecycle")

        # AWS prepends a region-specific prefix, so the recorded ID is the suffix
        # plus more -- this is the field every later API call keys on.
        assert suffix in global_id
        assert len(global_id) > len(suffix)

        grg = get_global_replication_group(global_id)
        assert grg is not None
        assert grg["Status"] == "primary-only"
        assert len(grg.get("Members") or []) >= 1

        delete_grg(reference, global_id)

        # Deleting the datastore must not delete the ReplicationGroup it only
        # references.
        assert wait_replication_group_available(rg_id), (
            f"primary ReplicationGroup {rg_id} should survive the datastore's deletion"
        )


@service_marker
class TestGlobalReplicationGroupInvalidPrimary:
    def test_invalid_primary_terminal(self):
        """A primary that does not exist must be terminal, not retried forever."""
        suffix = random_suffix_name("ack-e2e-bad", 20)
        grg_name = f"grg-bad-{suffix}"
        grg_resource = load_elasticache_resource(
            "global_replication_group_basic",
            additional_replacements={
                "GRG_NAME": grg_name,
                "GRG_SUFFIX": suffix,
                "PRIMARY_RG_ID": "ack-e2e-nonexistent-primary",
                "DESCRIPTION": "ACK E2E invalid primary",
            },
        )
        reference = k8s.CustomResourceReference(
            CRD_GROUP, CRD_VERSION, RESOURCE_PLURAL_GRG, grg_name, namespace="default"
        )
        _ = k8s.create_custom_resource(reference, grg_resource)
        assert k8s.wait_resource_consumed_by_controller(
            reference, wait_periods=10, period_length=15
        ) is not None

        assert k8s.wait_on_condition(
            reference, condition.CONDITION_TYPE_TERMINAL, "True",
            wait_periods=12, period_length=10,
        ), "an absent primary did not drive the resource to ACK.Terminal=True"

        k8s.delete_custom_resource(reference)


@service_marker
class TestGlobalReplicationGroupModify:
    def test_modify_description(self, primary_replication_group):
        rg_id = primary_replication_group
        suffix = random_suffix_name("ack-e2e-mod", 20)
        reference, global_id = create_grg(suffix, rg_id, "Original description")

        # ACK rewrites the Synced condition on every reconcile, so requiring a newer
        # timestamp proves this change was reconciled rather than reading a stale True.
        synced_before = condition.get_synced_last_transition_time(reference)
        assert synced_before is not None

        k8s.patch_custom_resource(
            reference, {"spec": {"description": "Updated by ACK E2E"}}
        )

        assert wait_global_attribute(
            global_id, "GlobalReplicationGroupDescription", "Updated by ACK E2E"
        ), f"GlobalReplicationGroup {global_id} description was not updated"
        assert k8s.wait_on_condition_after(
            reference, condition.CONDITION_TYPE_RESOURCE_SYNCED, "True",
            last_transition_after=synced_before,
            wait_periods=MODIFY_WAIT_PERIODS,
            period_length=MODIFY_PERIOD_LENGTH,
        ), "resource did not return to ACK.ResourceSynced=True after the modify"

        delete_grg(reference, global_id)

    def test_modify_automatic_failover_is_noop(self, primary_replication_group):
        """Setting a value AWS already reports must converge without a Modify call.

        A Global Datastore inherits automatic failover from its primary, so
        Modify(AutomaticFailoverEnabled=true) is a no-op that AWS rejects. The
        controller derives the current value from live member state, so it must see
        that the value already matches and never issue the call -- neither
        ACK.Terminal nor ACK.Recoverable should ever appear.
        """
        rg_id = primary_replication_group
        suffix = random_suffix_name("ack-e2e-afe", 20)

        # Created without the field so the patch below is the first time it is set,
        # which is the exact trigger for the reconcile loop this guards against.
        reference, global_id = create_grg(suffix, rg_id, "ACK E2E failover no-op")

        k8s.patch_custom_resource(
            reference, {"spec": {"automaticFailoverEnabled": True}}
        )

        assert wait_primary_automatic_failover(global_id, True), (
            f"GlobalReplicationGroup {global_id} primary does not report failover enabled"
        )
        assert not k8s.wait_on_condition(
            reference, condition.CONDITION_TYPE_TERMINAL, "True",
            wait_periods=8, period_length=15,
        ), "a no-op failover change drove the resource to ACK.Terminal=True"
        # Checked as a read, not a wait: the two-minute Terminal window above has
        # already given the controller ample time to surface either condition.
        assert_condition_not_true(reference, condition.CONDITION_TYPE_RECOVERABLE)
        condition.assert_synced(reference)

        delete_grg(reference, global_id)

    def test_engine_version_upgrade(self, primary_replication_group):
        """A minor engine upgrade must send engine and version together and converge.

        ModifyGlobalReplicationGroup rejects an engine version sent on its own, so
        this exercises the grouped engine request. A MAJOR upgrade additionally
        requires spec.cacheParameterGroupName; that branch is covered by unit tests
        because it could not be validated against live AWS here.
        """
        rg_id = primary_replication_group
        suffix = random_suffix_name("ack-e2e-ev", 20)
        reference, global_id = create_grg(
            suffix, rg_id, "ACK E2E engine upgrade",
            extra_spec={"engine": "redis", "engineVersion": "7.0"},
        )

        synced_before = condition.get_synced_last_transition_time(reference)
        assert synced_before is not None

        k8s.patch_custom_resource(
            reference, {"spec": {"engine": "redis", "engineVersion": "7.1"}}
        )

        assert wait_global_attribute(global_id, "EngineVersion", "7.1"), (
            f"GlobalReplicationGroup {global_id} engine version was not upgraded"
        )
        # The upgrade wait above has already elapsed real time, so a read is
        # sufficient here and does not pass merely for lack of elapsed time.
        assert_condition_not_true(reference, condition.CONDITION_TYPE_TERMINAL)
        assert k8s.wait_on_condition_after(
            reference, condition.CONDITION_TYPE_RESOURCE_SYNCED, "True",
            last_transition_after=synced_before,
            wait_periods=MODIFY_WAIT_PERIODS,
            period_length=MODIFY_PERIOD_LENGTH,
        ), "resource did not return to ACK.ResourceSynced=True after the upgrade"

        delete_grg(reference, global_id)
