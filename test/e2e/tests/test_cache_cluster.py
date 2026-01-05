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

"""Integration tests for the ElastiCache CacheCluster resource
"""

import boto3
import logging
from time import sleep

import pytest

from acktest.resources import random_suffix_name
from acktest.k8s import resource as k8s, condition
from acktest.k8s import condition
from acktest import tags as tagutil
from e2e import service_marker, CRD_GROUP, CRD_VERSION, load_elasticache_resource
from e2e.bootstrap_resources import get_bootstrap_resources
from e2e.replacement_values import REPLACEMENT_VALUES

RESOURCE_PLURAL = "cacheclusters"

# Time to wait after modifying the CR for the status to change
MODIFY_WAIT_AFTER_SECONDS = 120

# Time to wait after the cluster has changed status, for the CR to update
CHECK_STATUS_WAIT_SECONDS = 120

TAGS_PATCH_WAIT_TIME = 120


def wait_for_cache_cluster_available(elasticache_client, cache_cluster_id):
    """Wait for cache cluster to reach 'available' state using boto3 waiter.
    """
    waiter = elasticache_client.get_waiter(
        'cache_cluster_available',
    )
    waiter.config.delay = 5
    waiter.config.max_attempts = 240
    waiter.wait(CacheClusterId=cache_cluster_id)


def wait_until_deleted(elasticache_client, cache_cluster_id):
    """Wait for cache cluster to be fully deleted using boto3 waiter.
    """
    waiter = elasticache_client.get_waiter(
        'cache_cluster_deleted',
    )
    waiter.config.delay = 5
    waiter.config.max_attempts = 240
    waiter.wait(CacheClusterId=cache_cluster_id)


def get_and_assert_status(ref: k8s.CustomResourceReference, expected_status: str, expected_synced: bool):
    """Get the cache cluster status and assert it matches the expected status.
    """
    cr = k8s.get_resource(ref)
    assert cr is not None
    assert 'status' in cr

    assert cr['status']['cacheClusterStatus'] == expected_status

    if expected_synced:
        condition.assert_synced(ref)
    else:
        condition.assert_not_synced(ref)


@pytest.fixture(scope="module")
def elasticache_client():
    return boto3.client('elasticache')

@pytest.fixture(scope="module")
def bootstrap_resources():
    return get_bootstrap_resources()


@pytest.fixture
def simple_cache_cluster(elasticache_client):
    cache_cluster_id = random_suffix_name("simple-cache-cluster", 32)

    replacements = REPLACEMENT_VALUES.copy()
    replacements["CACHE_CLUSTER_ID"] = cache_cluster_id

    resource_data = load_elasticache_resource(
        "cache_cluster_simple",
        additional_replacements=replacements,
    )
    logging.debug(resource_data)

    # Create the k8s resource
    ref = k8s.CustomResourceReference(
        CRD_GROUP, CRD_VERSION, RESOURCE_PLURAL,
        cache_cluster_id, namespace="default",
    )
    k8s.create_custom_resource(ref, resource_data)
    cr = k8s.wait_resource_consumed_by_controller(ref, wait_periods=15, period_length=20)

    logging.info("resource consumed by controller")

    assert cr is not None
    assert k8s.get_resource_exists(ref)

    res = k8s.get_resource(ref)
    print(res)

    yield (ref, cr)

    # Try to delete, if doesn't already exist
    try:
        _, deleted = k8s.delete_custom_resource(ref, 3, 10)
        assert deleted
        wait_until_deleted(elasticache_client, cache_cluster_id)
    except:
        pass


@service_marker
@pytest.mark.canary
class TestCacheCluster:
    def test_create_update_delete_cache_cluster(self, elasticache_client, simple_cache_cluster):
        (ref, cr) = simple_cache_cluster

        cache_cluster_id = cr["spec"]["cacheClusterID"]

        logging.info("starting cache cluster test")
        logging.info(cache_cluster_id)
        try:
            aws_res = elasticache_client.describe_cache_clusters(CacheClusterId=cache_cluster_id)
            assert len(aws_res["CacheClusters"]) == 1
            print(aws_res['CacheClusters'])
        except elasticache_client.exceptions.CacheClusterNotFoundFault:
            pytest.fail(f"Could not find cache cluster '{cache_cluster_id}' in ElastiCache")

        logging.info("waiting for cluster to become available")
        wait_for_cache_cluster_available(elasticache_client, cache_cluster_id)

        updates = {
            "spec": {
                "numCacheNodes": 3,
                "autoMinorVersionUpgrade": True
            }
        }
        k8s.patch_custom_resource(ref, updates)
        logging.info("patched resource")
        print(updates)
        sleep(MODIFY_WAIT_AFTER_SECONDS)

        # Ensure status is updating properly and set as not synced
        get_and_assert_status(ref, 'modifying', False)

        # Wait for the status to become available again
        wait_for_cache_cluster_available(elasticache_client, cache_cluster_id)
        logging.info("update complete")

        # Ensure status is updated properly once it has become active
        sleep(CHECK_STATUS_WAIT_SECONDS)
        get_and_assert_status(ref, 'available', True)

        aws_res = elasticache_client.describe_cache_clusters(CacheClusterId=cache_cluster_id)
        assert len(aws_res["CacheClusters"]) == 1
        cache_cluster = aws_res["CacheClusters"][0]
        assert cache_cluster['NumCacheNodes'] == 3
        assert cache_cluster['AutoMinorVersionUpgrade']

        updates = {
            "spec": {
                "numCacheNodes": 4,
                "preferredAvailabilityZones": ["us-west-2a"]
            }
        }

        k8s.patch_custom_resource(ref, updates)
        sleep(MODIFY_WAIT_AFTER_SECONDS)
        get_and_assert_status(ref, 'modifying', False)

        wait_for_cache_cluster_available(elasticache_client, cache_cluster_id)

        aws_res = elasticache_client.describe_cache_clusters(CacheClusterId=cache_cluster_id)
        assert len(aws_res['CacheClusters']) == 1
        cache_cluster = aws_res['CacheClusters'][0]
        assert cache_cluster['NumCacheNodes'] == 4

        updates = {
            "spec": {
                "tags": [
                    {
                        "key": "k1",
                        "value": "v1"
                    },
                    {
                        "key": "k2",
                        "value": "v2"
                    }
                ]
            }
        }

        k8s.patch_custom_resource(ref, updates)
        sleep(TAGS_PATCH_WAIT_TIME)
        tag_list = elasticache_client.list_tags_for_resource(ResourceName=cr['status']['ackResourceMetadata']['arn'])
        tags = tagutil.clean(tag_list['TagList'])
        assert len(tags) == 2
        assert tags == [{"Key": "k1", "Value": "v1"}, {"Key": "k2", "Value": "v2"}]

        k8s.delete_custom_resource(ref)
        wait_until_deleted(elasticache_client, cache_cluster_id)

    def test_update_security_group_ids(self, elasticache_client, bootstrap_resources, simple_cache_cluster):
        (ref, cr) = simple_cache_cluster
        security_group_1 = bootstrap_resources.SecurityGroup1

        cache_cluster_id = cr["spec"]["cacheClusterID"]
        assert "securityGroupIDs" not in cr["spec"]

        logging.info("waiting for cluster to become available")
        wait_for_cache_cluster_available(elasticache_client, cache_cluster_id)

        aws_res = elasticache_client.describe_cache_clusters(CacheClusterId=cache_cluster_id)
        assert len(aws_res['CacheClusters']) == 1
        cache_cluster = aws_res['CacheClusters'][0]
        assert 'SecurityGroups' not in cache_cluster or len(cache_cluster['SecurityGroups']) == 0

        updates = {
            "spec": {
                "securityGroupIDs": [security_group_1]
            }
        }

        k8s.patch_custom_resource(ref, updates)
        sleep(MODIFY_WAIT_AFTER_SECONDS)

        wait_for_cache_cluster_available(elasticache_client, cache_cluster_id)
        assert k8s.wait_on_condition(ref, condition.CONDITION_TYPE_RESOURCE_SYNCED, "True", wait_periods=10)

        cr = k8s.get_resource(ref)
        assert cr is not None
        assert 'spec' in cr
        assert 'securityGroupIDs' in cr['spec']
        assert len(cr['spec']['securityGroupIDs']) == 1
        assert cr['spec']['securityGroupIDs'][0] == security_group_1

        aws_res = elasticache_client.describe_cache_clusters(CacheClusterId=cache_cluster_id)
        assert len(aws_res['CacheClusters']) == 1
        cache_cluster = aws_res['CacheClusters'][0]
        assert len(cache_cluster['SecurityGroups']) == 1
        assert cache_cluster['SecurityGroups'][0]['SecurityGroupId'] == security_group_1






