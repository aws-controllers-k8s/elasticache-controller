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

"""Integration tests for the Elasticache ServerlessCacheSnapshot resource
"""

import pytest
import boto3
import logging
import time

from acktest.resources import random_suffix_name
from acktest.k8s import resource as k8s
from acktest import tags as tagutil
from e2e import service_marker, CRD_GROUP, CRD_VERSION, load_elasticache_resource
from e2e.replacement_values import REPLACEMENT_VALUES

RESOURCE_PLURAL = "serverlesscachesnapshots"
SERVERLESS_CACHE_PLURAL = "serverlesscaches"
UPDATE_WAIT_SECS = 180


@pytest.fixture(scope="module")
def elasticache_client():
    return boto3.client('elasticache')


@pytest.fixture
def serverless_cache_for_snapshot(elasticache_client):
    """Fixture to create a serverless cache for snapshot testing"""
    serverless_cache_name = random_suffix_name("snapshot-test-sc", 32)

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
        CRD_GROUP, CRD_VERSION, SERVERLESS_CACHE_PLURAL,
        serverless_cache_name, namespace="default",
    )
    _ = k8s.create_custom_resource(ref, resource_data)
    cr = k8s.wait_resource_consumed_by_controller(ref)

    assert cr is not None
    
    # Wait for serverless cache to be available
    assert k8s.wait_on_condition(
        ref, "Ready", "True", wait_periods=90
    )
    
    yield ref, cr
    
    # Teardown
    _ = k8s.delete_custom_resource(ref)


@pytest.fixture
def simple_serverless_cache_snapshot(elasticache_client, serverless_cache_for_snapshot):
    """Fixture to create a simple serverless cache snapshot for testing"""
    sc_ref, sc_cr = serverless_cache_for_snapshot
    serverless_cache_name = sc_cr["spec"]["serverlessCacheName"]
    
    snapshot_name = random_suffix_name("simple-snapshot", 32)

    replacements = REPLACEMENT_VALUES.copy()
    replacements["SNAPSHOT_NAME"] = snapshot_name
    replacements["SC_NAME"] = serverless_cache_name

    resource_data = load_elasticache_resource(
        "serverless_cache_snapshot_basic",
        additional_replacements=replacements,
    )
    logging.debug(resource_data)

    ref = k8s.CustomResourceReference(
        CRD_GROUP, CRD_VERSION, RESOURCE_PLURAL,
        snapshot_name, namespace="default",
    )
    _ = k8s.create_custom_resource(ref, resource_data)
    cr = k8s.wait_resource_consumed_by_controller(ref)

    assert cr is not None
    yield ref, cr
    
    # Teardown
    _ = k8s.delete_custom_resource(ref)


@service_marker
class TestServerlessCacheSnapshot:
    def test_create_delete_serverless_cache_snapshot(self, simple_serverless_cache_snapshot, elasticache_client):
        """Test basic creation and deletion of a serverless cache snapshot"""
        (ref, _) = simple_serverless_cache_snapshot
        
        assert k8s.wait_on_condition(
            ref, "Ready", "True", wait_periods=120
        )
        
        tag_updates = {
            "spec": {
                "tags": [
                    {"key": "Environment", "value": "test"},
                    {"key": "Purpose", "value": "e2e-testing"}
                ]
            }
        }
        
        k8s.patch_custom_resource(ref, tag_updates)
        
        time.sleep(UPDATE_WAIT_SECS)
        
        final_cr = k8s.get_resource(ref)
        snapshot_arn = final_cr['status']['ackResourceMetadata']['arn']
        
        tag_list = elasticache_client.list_tags_for_resource(ResourceName=snapshot_arn)
        aws_tags = tagutil.clean(tag_list['TagList'])
        
        expected_tags = [{"Key": "Environment", "Value": "test"}, {"Key": "Purpose", "Value": "e2e-testing"}]
        assert len(aws_tags) == 2
        assert aws_tags == expected_tags