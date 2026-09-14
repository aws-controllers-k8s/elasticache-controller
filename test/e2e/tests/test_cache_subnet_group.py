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

"""Integration tests for the ElastiCache CacheSubnetGroup resource
"""

import boto3
import logging
from time import sleep

import pytest

from acktest.resources import random_suffix_name
from acktest.k8s import resource as k8s
from acktest.k8s import condition
from e2e import service_marker, CRD_GROUP, CRD_VERSION, load_elasticache_resource
from e2e.replacement_values import REPLACEMENT_VALUES

RESOURCE_PLURAL = "cachesubnetgroups"

# Cache subnet groups are created and modified synchronously, so these waits
# only need to cover the controller's reconcile latency.
CREATE_WAIT_AFTER_SECONDS = 20
MODIFY_WAIT_AFTER_SECONDS = 30
DELETE_WAIT_PERIODS = 10
DELETE_WAIT_PERIOD_LENGTH = 10

INITIAL_DESCRIPTION = "ack e2e test cache subnet group"
UPDATED_DESCRIPTION = "ack e2e test cache subnet group, updated"


@pytest.fixture(scope="module")
def elasticache_client():
    return boto3.client('elasticache')


@pytest.fixture(scope="module")
def default_vpc_subnets():
    """Two distinct subnets from the account's default VPC.

    Every subnet in a cache subnet group has to belong to one VPC, so both are
    taken from the same default VPC that the test bootstrap already requires.
    """
    ec2 = boto3.client('ec2')

    vpcs = ec2.describe_vpcs(
        Filters=[{"Name": "isDefault", "Values": ["true"]}],
    )["Vpcs"]
    assert len(vpcs) > 0, "no default VPC found, which CacheSubnetGroup tests need"
    vpc_id = vpcs[0]["VpcId"]

    subnets = ec2.describe_subnets(
        Filters=[
            {"Name": "vpc-id", "Values": [vpc_id]},
            {"Name": "state", "Values": ["available"]},
        ],
    )["Subnets"]
    subnet_ids = sorted({s["SubnetId"] for s in subnets})
    assert len(subnet_ids) >= 2, (
        f"default VPC {vpc_id} has {len(subnet_ids)} available subnet(s), "
        "but the update case needs two"
    )

    return (vpc_id, subnet_ids[:2])


@pytest.fixture
def simple_cache_subnet_group(default_vpc_subnets):
    (_, subnet_ids) = default_vpc_subnets
    csg_name = random_suffix_name("ack-test-csg", 32)

    replacements = REPLACEMENT_VALUES.copy()
    replacements["CACHE_SUBNET_GROUP_NAME"] = csg_name
    replacements["CACHE_SUBNET_GROUP_DESCRIPTION"] = INITIAL_DESCRIPTION
    replacements["SUBNET_ID"] = subnet_ids[0]

    resource_data = load_elasticache_resource(
        "cache_subnet_group_simple",
        additional_replacements=replacements,
    )
    logging.debug(resource_data)

    ref = k8s.CustomResourceReference(
        CRD_GROUP, CRD_VERSION, RESOURCE_PLURAL,
        csg_name, namespace="default",
    )
    k8s.create_custom_resource(ref, resource_data)
    cr = k8s.wait_resource_consumed_by_controller(ref)

    assert cr is not None
    assert k8s.get_resource_exists(ref)

    yield (ref, cr)

    # The test deletes the resource itself so the delete assertions belong to
    # it; this only cleans up after a test that failed before getting there.
    try:
        k8s.delete_custom_resource(ref, 3, 10)
    except:
        pass


def get_aws_subnet_group(elasticache_client, csg_name):
    aws_res = elasticache_client.describe_cache_subnet_groups(
        CacheSubnetGroupName=csg_name,
    )
    assert len(aws_res["CacheSubnetGroups"]) == 1
    return aws_res["CacheSubnetGroups"][0]


def get_status_subnet_ids(ref):
    resource = k8s.get_resource(ref)
    subnets = resource["status"].get("subnets") or []
    return {s["subnetIdentifier"] for s in subnets}


def assert_subnet_group_deleted(elasticache_client, csg_name):
    """DeleteCacheSubnetGroup reports no lifecycle state, so poll until the
    group is really gone instead of assuming it disappears immediately.
    """
    for _ in range(DELETE_WAIT_PERIODS):
        try:
            elasticache_client.describe_cache_subnet_groups(
                CacheSubnetGroupName=csg_name,
            )
        except elasticache_client.exceptions.CacheSubnetGroupNotFoundFault:
            return
        sleep(DELETE_WAIT_PERIOD_LENGTH)

    pytest.fail(f"cache subnet group {csg_name} still exists after deletion")


@service_marker
class TestCacheSubnetGroup:
    def test_crud(self, elasticache_client, default_vpc_subnets, simple_cache_subnet_group):
        """Create, read back, modify and delete a CacheSubnetGroup.

        ModifyCacheSubnetGroup accepts both the description and the subnet list,
        so a single update exercises every mutable field the resource has.
        """
        (vpc_id, subnet_ids) = default_vpc_subnets
        (ref, cr) = simple_cache_subnet_group
        csg_name = cr["spec"]["cacheSubnetGroupName"]

        sleep(CREATE_WAIT_AFTER_SECONDS)
        assert k8s.wait_on_condition(
            ref, condition.CONDITION_TYPE_RESOURCE_SYNCED, "True",
            wait_periods=10, period_length=10,
        )

        group = get_aws_subnet_group(elasticache_client, csg_name)
        assert group["CacheSubnetGroupDescription"] == INITIAL_DESCRIPTION
        assert group["VpcId"] == vpc_id
        assert {s["SubnetIdentifier"] for s in group["Subnets"]} == {subnet_ids[0]}

        resource = k8s.get_resource(ref)
        assert resource["status"]["vpcID"] == vpc_id
        assert get_status_subnet_ids(ref) == {subnet_ids[0]}

        # Waiting for a NEWER synced transition is what proves the edit was
        # reconciled; a plain wait can observe the pre-patch condition.
        synced_before = condition.get_synced_last_transition_time(ref)
        assert synced_before is not None

        updates = {
            "spec": {
                "cacheSubnetGroupDescription": UPDATED_DESCRIPTION,
                "subnetIDs": subnet_ids,
            },
        }
        k8s.patch_custom_resource(ref, updates)
        sleep(MODIFY_WAIT_AFTER_SECONDS)
        assert k8s.wait_on_condition_after(
            ref, condition.CONDITION_TYPE_RESOURCE_SYNCED, "True",
            last_transition_after=synced_before,
            wait_periods=10, period_length=10,
        )

        group = get_aws_subnet_group(elasticache_client, csg_name)
        assert group["CacheSubnetGroupDescription"] == UPDATED_DESCRIPTION
        # Compared as sets: AWS does not echo the manifest's subnet order back.
        assert {s["SubnetIdentifier"] for s in group["Subnets"]} == set(subnet_ids)
        assert get_status_subnet_ids(ref) == set(subnet_ids)

        _, deleted = k8s.delete_custom_resource(ref, 3, 10)
        assert deleted
        assert_subnet_group_deleted(elasticache_client, csg_name)
