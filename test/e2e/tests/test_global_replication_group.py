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

"""Integration tests for the ElastiCache GlobalReplicationGroup resource
"""

import pytest
import boto3
import logging
from time import sleep

from acktest.resources import random_suffix_name
from acktest.k8s import resource as k8s
from acktest.k8s import condition
from e2e import service_marker, CRD_GROUP, CRD_VERSION, load_elasticache_resource

RESOURCE_PLURAL_GRG = "globalreplicationgroups"
RESOURCE_PLURAL_RG = "replicationgroups"

# Timeouts (in seconds)
CREATE_WAIT_PERIODS = 30
CREATE_PERIOD_LENGTH = 30  # 30 * 30s = 15 min max
MODIFY_WAIT_PERIODS = 30
MODIFY_PERIOD_LENGTH = 30
DELETE_WAIT_PERIODS = 40
DELETE_PERIOD_LENGTH = 30  # 40 * 30s = 20 min max
# The prerequisite primary ReplicationGroup can take considerably longer than a
# plain 15-min create budget to reach 'available' when the CI suite runs all
# resources in parallel (many ElastiCache resources provisioning at once, plus
# the controller pod restarting on its periodic credential rotation, both of
# which delay when the group settles). Observed ~13 min in one PR #228 CI run,
# landing right at the old 15-min ceiling and flaking the module fixture. This
# wait returns as soon as the group is available, so a generous ceiling costs
# nothing on the happy path and only absorbs that tail.
RG_AVAILABLE_WAIT_PERIODS = 40
RG_AVAILABLE_PERIOD_LENGTH = 30  # 40 * 30s = 20 min max
# Node group scaling is by far the slowest GRG operation: ElastiCache migrates
# slots between shards and holds the GRG in 'modifying' the whole time. On a
# decrease the shard count only drops at the very end. Measured ~27 min for a
# 3->2 decrease against live AWS, so this budget is deliberately generous --
# the wait returns as soon as the target state is reached, so a large ceiling
# costs nothing on the happy path and only prevents flakes.
SCALE_WAIT_PERIODS = 80
SCALE_PERIOD_LENGTH = 30  # 80 * 30s = 40 min max

ec = boto3.client("elasticache")


# =============================================================================
# HELPERS
# =============================================================================

def wait_k8s_resource_global_id(reference, wait_periods=10, period_length=10) -> str:
    """Poll until .status.globalReplicationGroupID is populated."""
    for _ in range(wait_periods):
        global_id = get_k8s_resource_global_id(reference)
        if global_id:
            return global_id
        sleep(period_length)
    return None


def wait_global_replication_group_status(
    global_rg_id: str,
    target_status: str,
    wait_periods: int = CREATE_WAIT_PERIODS,
    period_length: int = CREATE_PERIOD_LENGTH,
) -> bool:
    """Wait until the GlobalReplicationGroup reaches the target status."""
    for i in range(wait_periods):
        logging.debug(
            f"Waiting for GlobalReplicationGroup {global_rg_id} "
            f"to reach '{target_status}' ({i}/{wait_periods})"
        )
        try:
            response = ec.describe_global_replication_groups(
                GlobalReplicationGroupId=global_rg_id,
                ShowMemberInfo=True,
            )
            if len(response["GlobalReplicationGroups"]) == 0:
                if target_status == "deleted":
                    return True
                logging.warning(f"GlobalReplicationGroup {global_rg_id} not found")
                return False

            grg = response["GlobalReplicationGroups"][0]
            current_status = grg.get("Status", "unknown")
            logging.debug(f"  Current status: {current_status}")

            if current_status == target_status:
                logging.info(
                    f"GlobalReplicationGroup {global_rg_id} reached "
                    f"'{target_status}', continuing..."
                )
                return True

        except ec.exceptions.GlobalReplicationGroupNotFoundFault:
            if target_status == "deleted":
                return True
            logging.warning(f"GlobalReplicationGroup {global_rg_id} not found (404)")
            return False

        sleep(period_length)

    logging.error(
        f"Timed out waiting for GlobalReplicationGroup {global_rg_id} "
        f"to reach '{target_status}'"
    )
    return False


def wait_replication_group_available(rg_id: str) -> bool:
    """Wait until a ReplicationGroup is available (needed as GRG prerequisite)."""
    for i in range(RG_AVAILABLE_WAIT_PERIODS):
        logging.debug(f"Waiting for ReplicationGroup {rg_id} to be available ({i})")
        try:
            response = ec.describe_replication_groups(ReplicationGroupId=rg_id)
            rg = response["ReplicationGroups"][0]
            if rg["Status"] == "available":
                logging.info(f"ReplicationGroup {rg_id} is available")
                return True
        except Exception as e:
            logging.warning(f"Error checking RG status: {e}")
        sleep(RG_AVAILABLE_PERIOD_LENGTH)
    return False


def get_global_replication_group(global_rg_id: str):
    """Describe a GlobalReplicationGroup from the AWS API directly."""
    try:
        response = ec.describe_global_replication_groups(
            GlobalReplicationGroupId=global_rg_id,
            ShowMemberInfo=True,
        )
        if len(response["GlobalReplicationGroups"]) > 0:
            return response["GlobalReplicationGroups"][0]
    except ec.exceptions.GlobalReplicationGroupNotFoundFault:
        pass
    return None


def wait_global_node_group_count(
    global_rg_id: str,
    expected_count: int,
    wait_periods: int = SCALE_WAIT_PERIODS,
    period_length: int = SCALE_PERIOD_LENGTH,
) -> bool:
    """Wait until the GRG has `expected_count` node groups and has settled.

    Scaling is complete only when both the shard count matches AND the status
    has returned to a steady state. Polling on status alone is racy: the GRG
    stays in its pre-patch steady state for a few seconds after the spec is
    patched, before the controller issues the scale call and AWS flips the
    status to 'modifying' -- so a status-only wait can return immediately and
    spuriously succeed.
    """
    for i in range(wait_periods):
        grg = get_global_replication_group(global_rg_id)
        if grg is not None:
            count = len(grg.get("GlobalNodeGroups") or [])
            status = grg.get("Status")
            logging.debug(
                f"Waiting for GlobalReplicationGroup {global_rg_id} to have "
                f"{expected_count} node groups and settle: "
                f"count={count} status={status} ({i}/{wait_periods})"
            )
            if count == expected_count and status in ("primary-only", "available"):
                return True
        sleep(period_length)
    return False


def get_k8s_resource_status(reference) -> str:
    """Get the .status.status field from the K8s resource."""
    resource = k8s.get_resource(reference)
    if resource is None:
        return None
    return resource.get("status", {}).get("status")


def get_k8s_resource_global_id(reference) -> str:
    """Get the .status.globalReplicationGroupID from the K8s resource."""
    resource = k8s.get_resource(reference)
    if resource is None:
        return None
    return resource.get("status", {}).get("globalReplicationGroupID")


def create_grg(
    suffix: str,
    primary_rg_id: str,
    description: str,
    resource_template: str = "global_replication_group_basic",
    target_status: str = "primary-only",
    extra_replacements: dict = None,
):
    """Create a GlobalReplicationGroup and wait until it reaches target_status.

    Returns (reference, global_id).
    """
    grg_name = f"grg-{suffix}"
    replacements = {
        "GRG_NAME": grg_name,
        "GRG_SUFFIX": suffix,
        "PRIMARY_RG_ID": primary_rg_id,
        "DESCRIPTION": description,
    }
    if extra_replacements:
        replacements.update(extra_replacements)

    grg_resource = load_elasticache_resource(
        resource_template, additional_replacements=replacements
    )

    reference = k8s.CustomResourceReference(
        CRD_GROUP, CRD_VERSION, RESOURCE_PLURAL_GRG, grg_name, namespace="default"
    )
    _ = k8s.create_custom_resource(reference, grg_resource)
    resource = k8s.wait_resource_consumed_by_controller(
        reference, wait_periods=15, period_length=20
    )
    assert resource is not None

    global_id = wait_k8s_resource_global_id(reference)
    assert global_id is not None, "globalReplicationGroupID not populated in status"

    assert wait_global_replication_group_status(
        global_id, target_status,
        wait_periods=CREATE_WAIT_PERIODS,
        period_length=CREATE_PERIOD_LENGTH,
    ), f"GRG did not reach '{target_status}'"

    return reference, global_id


def delete_grg(reference, global_id: str):
    """Delete a GlobalReplicationGroup and wait for it to be gone."""
    k8s.delete_custom_resource(reference)
    wait_global_replication_group_status(
        global_id, "deleted",
        wait_periods=DELETE_WAIT_PERIODS,
        period_length=DELETE_PERIOD_LENGTH,
    )


def wait_global_description(
    global_rg_id: str,
    expected_description: str,
    wait_periods: int = MODIFY_WAIT_PERIODS,
    period_length: int = MODIFY_PERIOD_LENGTH,
) -> bool:
    """Wait until the GRG description matches and the group has settled.

    Polling on status alone is racy: the GRG stays in its pre-patch steady
    state for a few seconds after the spec is patched, before the controller
    issues the modify call and AWS flips the status to 'modifying'.
    """
    for i in range(wait_periods):
        grg = get_global_replication_group(global_rg_id)
        if grg is not None:
            desc = grg.get("GlobalReplicationGroupDescription")
            status = grg.get("Status")
            logging.debug(
                f"Waiting for GlobalReplicationGroup {global_rg_id} description "
                f"'{expected_description}': desc='{desc}' status={status} "
                f"({i}/{wait_periods})"
            )
            if desc == expected_description and status in ("primary-only", "available"):
                return True
        sleep(period_length)
    return False


def wait_global_automatic_failover(
    global_rg_id: str,
    expected_enabled: bool,
    wait_periods: int = MODIFY_WAIT_PERIODS,
    period_length: int = MODIFY_PERIOD_LENGTH,
) -> bool:
    """Wait until the primary member's automatic failover matches and the
    group has settled.

    AutomaticFailoverEnabled isn't a top-level GRG attribute in the Describe
    response -- it's read off the primary member's `AutomaticFailover` field
    (enabled/disabled/enabling/disabling). Matches terraform-provider-aws's
    flattenGlobalReplicationGroupAutomaticFailoverEnabled.
    """
    target = {"enabled", "enabling"} if expected_enabled else {"disabled", "disabling"}
    for i in range(wait_periods):
        grg = get_global_replication_group(global_rg_id)
        if grg is not None:
            status = grg.get("Status")
            primary_failover = None
            for member in grg.get("Members") or []:
                if member.get("Role") == "PRIMARY":
                    primary_failover = member.get("AutomaticFailover")
                    break
            logging.debug(
                f"Waiting for GlobalReplicationGroup {global_rg_id} automatic "
                f"failover enabled={expected_enabled}: primary_failover="
                f"{primary_failover} status={status} ({i}/{wait_periods})"
            )
            if primary_failover in target and status in ("primary-only", "available"):
                return True
        sleep(period_length)
    return False


def wait_global_node_type(
    global_rg_id: str,
    expected_node_type: str,
    wait_periods: int = SCALE_WAIT_PERIODS,
    period_length: int = SCALE_PERIOD_LENGTH,
) -> bool:
    """Wait until the GRG's node type matches and the group has settled.

    Changing the node type scales every member and holds the GRG in
    'modifying' for the duration, so this uses the generous scaling budget.
    Requiring a steady status (not just the target node type) avoids the same
    race as the other waiters, where the GRG briefly reports its pre-patch
    state before the controller issues the modify.
    """
    for i in range(wait_periods):
        grg = get_global_replication_group(global_rg_id)
        if grg is not None:
            nt = grg.get("CacheNodeType")
            status = grg.get("Status")
            logging.debug(
                f"Waiting for GlobalReplicationGroup {global_rg_id} node type "
                f"'{expected_node_type}': nodeType='{nt}' status={status} "
                f"({i}/{wait_periods})"
            )
            if nt == expected_node_type and status in ("primary-only", "available"):
                return True
        sleep(period_length)
    return False


# =============================================================================
# FIXTURES
# =============================================================================

@pytest.fixture(scope="module")
def primary_replication_group():
    """Create a ReplicationGroup to serve as the primary for GlobalReplicationGroup tests.

    This is created once per test module and cleaned up at the end.
    """
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
    resource = k8s.wait_resource_consumed_by_controller(
        reference, wait_periods=15, period_length=20
    )
    assert resource is not None

    # Wait for the RG to be available on the AWS side
    assert wait_replication_group_available(rg_id), (
        f"Primary ReplicationGroup {rg_id} did not become available"
    )

    yield rg_id

    # Cleanup: delete the primary RG
    k8s.delete_custom_resource(reference)
    # Wait for it to be deleted
    try:
        waiter = ec.get_waiter("replication_group_deleted")
        waiter.wait(
            ReplicationGroupId=rg_id,
            WaiterConfig={"Delay": 30, "MaxAttempts": 40},
        )
    except Exception:
        pass


# =============================================================================
# TESTS
# =============================================================================

@service_marker
class TestGlobalReplicationGroupBasicLifecycle:
    """Test basic create → describe → delete lifecycle."""

    def test_create_and_verify(self, primary_replication_group):
        """Create a GlobalReplicationGroup and verify it reaches 'primary-only'."""
        rg_id = primary_replication_group
        grg_suffix = random_suffix_name("ack-e2e-grg", 20)
        grg_name = f"grg-{grg_suffix}"

        grg_resource = load_elasticache_resource(
            "global_replication_group_basic",
            additional_replacements={
                "GRG_NAME": grg_name,
                "GRG_SUFFIX": grg_suffix,
                "PRIMARY_RG_ID": rg_id,
                "DESCRIPTION": "ACK E2E test - basic lifecycle",
            },
        )

        reference = k8s.CustomResourceReference(
            CRD_GROUP, CRD_VERSION, RESOURCE_PLURAL_GRG, grg_name, namespace="default"
        )

        # Create the resource
        _ = k8s.create_custom_resource(reference, grg_resource)
        resource = k8s.wait_resource_consumed_by_controller(
            reference, wait_periods=15, period_length=20
        )
        assert resource is not None

        # Get the full global ID from status (has region prefix)
        global_id = wait_k8s_resource_global_id(reference)
        assert global_id is not None, "globalReplicationGroupID not populated in status"
        assert grg_suffix in global_id, (
            f"Expected suffix '{grg_suffix}' in full ID '{global_id}'"
        )
        assert len(global_id) > len(grg_suffix), (
            "Full ID should be longer than suffix (has region prefix)"
        )

        # Wait for primary-only on the AWS side (no secondaries = primary-only, not available)
        assert wait_global_replication_group_status(global_id, "primary-only"), (
            f"GlobalReplicationGroup {global_id} did not become primary-only"
        )

        # Wait for the controller's own reconcile loop to catch up and mark the
        # resource Synced. IsSynced() only returns true once status.status is
        # 'available'/'primary-only', so this guarantees the status field below
        # already reflects the target state -- checking status.status directly
        # without this wait is racy, since the AWS-side wait above can observe
        # the transition slightly ahead of the controller's next poll cycle.
        assert k8s.wait_on_condition(
            reference, condition.CONDITION_TYPE_RESOURCE_SYNCED, "True",
            wait_periods=10, period_length=15,
        ), "resource did not reach ACK.ResourceSynced=True after create"

        # Verify K8s resource status
        assert get_k8s_resource_status(reference) == "primary-only"

        # Verify AWS-side resource exists and matches
        grg = get_global_replication_group(global_id)
        assert grg is not None
        assert grg["Status"] == "primary-only"
        assert len(grg.get("Members", [])) >= 1  # At least the primary

        # --- DELETE ---
        k8s.delete_custom_resource(reference)

        # The delete is multi-step: disassociate secondaries (none here), then delete
        assert wait_global_replication_group_status(
            global_id, "deleted",
            wait_periods=DELETE_WAIT_PERIODS,
            period_length=DELETE_PERIOD_LENGTH,
        ), f"GlobalReplicationGroup {global_id} was not deleted"

        # Verify the primary RG still exists (was retained)
        assert wait_replication_group_available(rg_id), (
            f"Primary ReplicationGroup {rg_id} should still exist after GRG deletion"
        )


@service_marker
class TestGlobalReplicationGroupInvalidPrimary:
    """Test that creating with a nonexistent primary produces a terminal condition."""

    def test_invalid_primary_terminal(self):
        grg_suffix = random_suffix_name("ack-e2e-bad", 20)
        grg_name = f"grg-bad-{grg_suffix}"

        grg_resource = load_elasticache_resource(
            "global_replication_group_basic",
            additional_replacements={
                "GRG_NAME": grg_name,
                "GRG_SUFFIX": grg_suffix,
                "PRIMARY_RG_ID": "nonexistent-rg-that-does-not-exist-12345",
                "DESCRIPTION": "ACK E2E test - should fail",
            },
        )

        reference = k8s.CustomResourceReference(
            CRD_GROUP, CRD_VERSION, RESOURCE_PLURAL_GRG, grg_name, namespace="default"
        )

        _ = k8s.create_custom_resource(reference, grg_resource)
        resource = k8s.wait_resource_consumed_by_controller(
            reference, wait_periods=10, period_length=15
        )
        assert resource is not None

        # An invalid primary must drive the resource terminal
        assert k8s.wait_on_condition(
            reference, condition.CONDITION_TYPE_TERMINAL, "True",
            wait_periods=12, period_length=10,
        ), "resource did not reach ACK.Terminal=True for an invalid primary"

        # Cleanup
        k8s.delete_custom_resource(reference)


@service_marker
class TestGlobalReplicationGroupModify:
    """Test modifying a GlobalReplicationGroup (description change)."""

    def test_modify_description(self, primary_replication_group):
        rg_id = primary_replication_group
        grg_suffix = random_suffix_name("ack-e2e-mod", 20)

        reference, global_id = create_grg(
            grg_suffix, rg_id, "Original description", target_status="primary-only"
        )

        # Capture the Synced condition's lastTransitionTime before patching.
        # ACK rewrites it on every reconcile, so requiring a newer value proves
        # the controller actually reconciled this change rather than us reading
        # a stale 'True' left over from the previous reconcile.
        synced_before = condition.get_synced_last_transition_time(reference)

        # Modify the description
        k8s.patch_custom_resource(reference, {
            "spec": {"description": "Updated description via ACK E2E"}
        })

        # Wait for the modify to land in AWS and the group to settle
        assert wait_global_description(
            global_id, "Updated description via ACK E2E"
        ), f"GlobalReplicationGroup {global_id} description was not updated"

        # Verify on AWS side
        grg = get_global_replication_group(global_id)
        assert grg is not None
        assert grg.get("GlobalReplicationGroupDescription") == "Updated description via ACK E2E"

        # The controller must drive the CR back to Synced via a fresh reconcile
        assert k8s.wait_on_condition_after(
            reference, condition.CONDITION_TYPE_RESOURCE_SYNCED, "True",
            last_transition_after=synced_before,
            wait_periods=MODIFY_WAIT_PERIODS,
            period_length=MODIFY_PERIOD_LENGTH,
        ), "resource did not return to ACK.ResourceSynced=True after modify"

        # Cleanup
        delete_grg(reference, global_id)

    def test_modify_automatic_failover_is_noop(self, primary_replication_group):
        """Setting automaticFailoverEnabled to a value AWS already reports must
        converge, not loop.

        A global datastore's primary always has automatic failover enabled, and
        the GRG inherits it, so Modify(AutomaticFailoverEnabled=true) is a no-op
        that AWS rejects ("Requested product type is the same current product
        type"). The controller derives the current failover state from live
        member info (not an annotation), so it must recognize the value already
        matches and reach Synced without ever issuing that Modify or landing in
        ACK.Terminal. Regression test for the standalone-AFE reconcile loop.
        """
        rg_id = primary_replication_group
        grg_suffix = random_suffix_name("ack-e2e-afe", 20)

        # Create without automaticFailoverEnabled so the patch below is the
        # first time the field is set -- the exact trigger for the old loop.
        reference, global_id = create_grg(
            grg_suffix, rg_id, "Original description", target_status="primary-only"
        )

        k8s.patch_custom_resource(reference, {
            "spec": {"automaticFailoverEnabled": True}
        })

        # AWS already reports failover enabled on the primary (inherited).
        assert wait_global_automatic_failover(
            global_id, True
        ), f"GlobalReplicationGroup {global_id} primary failover not enabled"

        # The no-op must never drive the resource terminal, and -- crucially --
        # must never surface a recoverable error. The old annotation-based code
        # issued Modify(AutomaticFailoverEnabled=true), which AWS rejects as a
        # no-op, flipping ACK.Recoverable=True and looping. Deriving the value
        # from live member state means no Modify is issued at all, so neither
        # condition should ever appear across several reconcile cycles.
        assert not k8s.wait_on_condition(
            reference, condition.CONDITION_TYPE_TERMINAL, "True",
            wait_periods=8, period_length=15,
        ), "resource unexpectedly reached ACK.Terminal=True on a no-op AFE change"
        assert not k8s.wait_on_condition(
            reference, condition.CONDITION_TYPE_RECOVERABLE, "True",
            wait_periods=1, period_length=1,
        ), "resource surfaced ACK.Recoverable=True -- controller issued the no-op Modify"

        # And it must be Synced (it never left the synced state, since no
        # Modify was needed).
        condition.assert_synced(reference)

        delete_grg(reference, global_id)

    def test_modify_multiple_fields_converges(self, primary_replication_group):
        """Changing two independent modify-eligible fields in one patch must
        converge over multiple reconciles, not fail.

        AWS's ModifyGlobalReplicationGroup rejects a request that sets more
        than one field at once. Patching description and cacheNodeType in the
        same apply creates a genuine two-field delta in a single reconcile:
        description is a fast metadata change and cacheNodeType is a real
        scaling change (cache.r6g.large -> cache.r6g.xlarge). The controller
        must send one field per Modify call and let the other field's delta
        persist into the next reconcile, converging on both values without
        ever landing in ACK.Terminal.
        """
        rg_id = primary_replication_group
        grg_suffix = random_suffix_name("ack-e2e-multi", 20)

        reference, global_id = create_grg(
            grg_suffix, rg_id, "Original description", target_status="primary-only"
        )

        synced_before = condition.get_synced_last_transition_time(reference)

        # Two independent fields changed in the same patch.
        k8s.patch_custom_resource(reference, {
            "spec": {
                "description": "Updated via multi-field E2E",
                "cacheNodeType": "cache.r6g.xlarge",
            }
        })

        # Both must eventually land on the AWS side, across however many
        # reconciles it takes -- never simultaneously in one Modify call.
        assert wait_global_description(
            global_id, "Updated via multi-field E2E"
        ), f"GlobalReplicationGroup {global_id} description was not updated"
        assert wait_global_node_type(
            global_id, "cache.r6g.xlarge"
        ), f"GlobalReplicationGroup {global_id} node type was not updated"

        # Must never have landed in a terminal state along the way.
        assert not k8s.wait_on_condition(
            reference, condition.CONDITION_TYPE_TERMINAL, "True",
            wait_periods=1, period_length=1,
        ), "resource unexpectedly reached ACK.Terminal=True during multi-field modify"

        assert k8s.wait_on_condition_after(
            reference, condition.CONDITION_TYPE_RESOURCE_SYNCED, "True",
            last_transition_after=synced_before,
            wait_periods=SCALE_WAIT_PERIODS,
            period_length=SCALE_PERIOD_LENGTH,
        ), "resource did not return to ACK.ResourceSynced=True after multi-field modify"

        # Cleanup
        delete_grg(reference, global_id)


@service_marker
class TestGlobalReplicationGroupNodeGroupScaling:
    """Test increasing and decreasing node groups.

    Uses a separate cluster-mode fixture with 2 shards.
    Verified against live AWS (account 585008087740, us-west-2):
    - IncreaseNodeGroups 2→3
    - DecreaseNodeGroups 3→2 (retains first N node groups)
    """

    @pytest.fixture(scope="class")
    def cluster_mode_primary(self):
        """Create a cluster-mode ReplicationGroup with 2 node groups."""
        rg_id = random_suffix_name("ack-e2e-grg-cm", 32)

        rg_resource = load_elasticache_resource(
            "replicationgroup_create_delete_grg",
            additional_replacements={
                "RG_ID": rg_id,
                "ENGINE_VERSION": "7.0",
                "NUM_NODE_GROUPS": "2",
                "REPLICAS_PER_NODE_GROUP": "1",
            },
        )

        reference = k8s.CustomResourceReference(
            CRD_GROUP, CRD_VERSION, RESOURCE_PLURAL_RG, rg_id, namespace="default"
        )
        _ = k8s.create_custom_resource(reference, rg_resource)
        resource = k8s.wait_resource_consumed_by_controller(
            reference, wait_periods=15, period_length=20
        )
        assert resource is not None
        assert wait_replication_group_available(rg_id), (
            f"Cluster-mode ReplicationGroup {rg_id} did not become available"
        )

        yield rg_id

        # Cleanup
        k8s.delete_custom_resource(reference)
        try:
            waiter = ec.get_waiter("replication_group_deleted")
            waiter.wait(
                ReplicationGroupId=rg_id,
                WaiterConfig={"Delay": 30, "MaxAttempts": 40},
            )
        except Exception:
            pass

    def test_scale_up_and_down(self, cluster_mode_primary):
        """Scale node groups 2→3→2 via nodeGroupCount spec field."""
        rg_id = cluster_mode_primary
        grg_suffix = random_suffix_name("ack-e2e-scale", 20)

        reference, global_id = create_grg(
            grg_suffix, rg_id, "ACK E2E test - scaling",
            resource_template="global_replication_group_scale",
            target_status="primary-only",
            extra_replacements={"NODE_GROUP_COUNT": "2"},
        )

        # Scale UP: 2→3
        synced_before = condition.get_synced_last_transition_time(reference)
        k8s.patch_custom_resource(reference, {"spec": {"nodeGroupCount": 3}})

        assert wait_global_node_group_count(global_id, 3), (
            f"GlobalReplicationGroup {global_id} did not scale up to 3 node groups"
        )
        assert k8s.wait_on_condition_after(
            reference, condition.CONDITION_TYPE_RESOURCE_SYNCED, "True",
            last_transition_after=synced_before,
            wait_periods=SCALE_WAIT_PERIODS,
            period_length=SCALE_PERIOD_LENGTH,
        ), "resource did not return to ACK.ResourceSynced=True after scale up"

        # Scale DOWN: 3→2
        synced_before = condition.get_synced_last_transition_time(reference)
        k8s.patch_custom_resource(reference, {"spec": {"nodeGroupCount": 2}})

        assert wait_global_node_group_count(global_id, 2), (
            f"GlobalReplicationGroup {global_id} did not scale down to 2 node groups"
        )
        assert k8s.wait_on_condition_after(
            reference, condition.CONDITION_TYPE_RESOURCE_SYNCED, "True",
            last_transition_after=synced_before,
            wait_periods=SCALE_WAIT_PERIODS,
            period_length=SCALE_PERIOD_LENGTH,
        ), "resource did not return to ACK.ResourceSynced=True after scale down"

        # Cleanup
        delete_grg(reference, global_id)
