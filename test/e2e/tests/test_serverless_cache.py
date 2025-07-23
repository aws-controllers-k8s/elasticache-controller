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

"""Integration tests for the Elasticache ServerlessCache resource
"""

import pytest
import boto3
import logging
from time import sleep

from acktest.resources import random_suffix_name
from acktest.k8s import resource as k8s
from acktest.k8s import condition
from e2e import service_marker, CRD_GROUP, CRD_VERSION, load_elasticache_resource
from e2e.replacement_values import REPLACEMENT_VALUES

RESOURCE_PLURAL = "serverlesscaches"
MODIFY_WAIT_AFTER_SECONDS = 120
CHECK_STATUS_WAIT_SECONDS = 120


def wait_for_serverless_cache_available(elasticache_client, serverless_cache_name):
    """Wait for serverless cache to reach 'available' state using boto3 waiter.
    """
    waiter = elasticache_client.get_waiter('serverless_cache_available')
    waiter.config.delay = 5
    waiter.config.max_attempts = 240
    waiter.wait(ServerlessCacheName=serverless_cache_name)


def wait_until_deleted(elasticache_client, serverless_cache_name): 
    """Wait for serverless cache to be fully deleted using boto3 waiter.
    """
    waiter = elasticache_client.get_waiter('serverless_cache_deleted')
    waiter.config.delay = 5
    waiter.config.max_attempts = 240
    waiter.wait(ServerlessCacheName=serverless_cache_name)


def get_and_assert_status(ref: k8s.CustomResourceReference, expected_status: str, expected_synced: bool):
    """Get the serverless cache status and assert it matches the expected status.
    """
    cr = k8s.get_resource(ref)
    assert cr is not None
    assert 'status' in cr

    assert cr['status']['status'] == expected_status

    if expected_synced:
        condition.assert_synced(ref)
    else:
        condition.assert_not_synced(ref)


@pytest.fixture(scope="module")
def elasticache_client():
    return boto3.client('elasticache')


def _create_serverless_cache(elasticache_client, name_prefix):
    serverless_cache_name = random_suffix_name(name_prefix, 32)

    replacements = REPLACEMENT_VALUES.copy()
    replacements["SC_NAME"] = serverless_cache_name
    replacements["ENGINE"] = "redis"
    replacements["MAJOR_ENGINE_VERSION"] = "7"
    replacements["ECPU_MIN"] = "10000"
    replacements["ECPU_MAX"] = "100000"

    resource_data = load_elasticache_resource(
        "serverless_cache_basic",
        additional_replacements=replacements,
    )
    logging.debug(resource_data)

    ref = k8s.CustomResourceReference(
        CRD_GROUP, CRD_VERSION, RESOURCE_PLURAL,
        serverless_cache_name, namespace="default",
    )
    _ = k8s.create_custom_resource(ref, resource_data)
    cr = k8s.wait_resource_consumed_by_controller(ref)

    assert cr is not None
    return ref, cr


@pytest.fixture
def simple_serverless_cache(elasticache_client):
    ref, cr = _create_serverless_cache(elasticache_client, "simple-serverless-cache")
    yield ref, cr
    
    # Teardown
    _ = k8s.delete_custom_resource(ref)
    try:
        serverless_cache_name = cr["spec"]["serverlessCacheName"]
        wait_until_deleted(elasticache_client, serverless_cache_name)
    except Exception as e:
        logging.warning(f"Failed to wait for serverless cache deletion: {e}")


@pytest.fixture  
def upgrade_serverless_cache(elasticache_client):
    ref, cr = _create_serverless_cache(elasticache_client, "upgrade-serverless-cache")
    yield ref, cr
    
    # Teardown
    _ = k8s.delete_custom_resource(ref)
    try:
        serverless_cache_name = cr["spec"]["serverlessCacheName"]
        wait_until_deleted(elasticache_client, serverless_cache_name)
    except Exception as e:
        logging.warning(f"Failed to wait for serverless cache deletion: {e}")


@service_marker
class TestServerlessCache:
    def test_create_update_delete_serverless_cache(self, simple_serverless_cache, elasticache_client):
        (ref, _) = simple_serverless_cache
        
        assert k8s.wait_on_condition(
            ref, "ACK.ResourceSynced", "True", wait_periods=90
        )
        get_and_assert_status(ref, "available", True)
        
        cr = k8s.get_resource(ref)
        serverless_cache_name = cr["spec"]["serverlessCacheName"]
        
        try:
            wait_for_serverless_cache_available(elasticache_client, serverless_cache_name)
        except Exception as e:
            logging.warning(f"Failed to wait for serverless cache availability: {e}")
        
        # Test update - modify description, change max to 90000, and add a tag
        new_description = "Updated serverless cache description"
        patch = {
            "spec": {
                "description": new_description,
                "cacheUsageLimits": {
                    "eCPUPerSecond": {
                        "minimum": 10000,
                        "maximum": 90000
                    }
                },
                "tags": [
                    {"key": "Environment", "value": "test"}
                ]
            }
        }
        _ = k8s.patch_custom_resource(ref, patch)
        sleep(MODIFY_WAIT_AFTER_SECONDS)
        
        # Wait for update to be synced
        assert k8s.wait_on_condition(
            ref, "ACK.ResourceSynced", "True", wait_periods=90
        )
        
        # Verify the update was applied
        cr = k8s.get_resource(ref)
        assert cr["spec"]["description"] == new_description
        assert cr["spec"]["cacheUsageLimits"]["eCPUPerSecond"]["maximum"] == 90000
        assert len(cr["spec"]["tags"]) == 1
        assert cr["spec"]["tags"][0]["key"] == "Environment"
        assert cr["spec"]["tags"][0]["value"] == "test"

    def test_upgrade_redis_to_valkey(self, upgrade_serverless_cache, elasticache_client):
        (ref, _) = upgrade_serverless_cache
        
        # Wait for the serverless cache to be created and become available
        assert k8s.wait_on_condition(
            ref, "ACK.ResourceSynced", "True", wait_periods=90
        )
        get_and_assert_status(ref, "available", True)
        
        cr = k8s.get_resource(ref)
        serverless_cache_name = cr["spec"]["serverlessCacheName"]
        
        # Verify initial state - Redis 7
        assert cr["spec"]["engine"] == "redis"
        assert cr["spec"]["majorEngineVersion"] == "7"
        
        try:
            wait_for_serverless_cache_available(elasticache_client, serverless_cache_name)
        except Exception as e:
            logging.warning(f"Failed to wait for serverless cache availability: {e}")
        
        # Upgrade from Redis 7 to Valkey 8
        patch = {
            "spec": {
                "engine": "valkey",
                "majorEngineVersion": "8"
            }
        }
        _ = k8s.patch_custom_resource(ref, patch)
        sleep(MODIFY_WAIT_AFTER_SECONDS)
        
        # Wait for upgrade to be synced
        assert k8s.wait_on_condition(
            ref, "ACK.ResourceSynced", "True", wait_periods=90
        )
        
        # Wait for it to be available again after upgrade
        get_and_assert_status(ref, "available", True)
        
        try:
            wait_for_serverless_cache_available(elasticache_client, serverless_cache_name)
        except Exception as e:
            logging.warning(f"Failed to wait for serverless cache availability after upgrade: {e}")
        
        # Verify the upgrade was applied
        cr = k8s.get_resource(ref)
        assert cr["spec"]["engine"] == "valkey"
        assert cr["spec"]["majorEngineVersion"] == "8"