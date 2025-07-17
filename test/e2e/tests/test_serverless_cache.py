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
from e2e import service_marker, CRD_GROUP, CRD_VERSION, load_elasticache_resource
from e2e.bootstrap_resources import get_bootstrap_resources
from e2e.util import assert_recoverable_condition_set, wait_serverless_cache_deleted

RESOURCE_PLURAL = "serverlesscaches"
DEFAULT_WAIT_SECS = 120


@pytest.fixture(scope="module")
def elasticache_client():
    return boto3.client("elasticache")




# retrieve resources created in the bootstrap step
@pytest.fixture(scope="module")
def bootstrap_resources():
    return get_bootstrap_resources()


# factory for serverless cache names
@pytest.fixture(scope="module")
def make_sc_name():
    def _make_sc_name(base):
        return random_suffix_name(base, 32)
    return _make_sc_name


# factory for serverless caches
@pytest.fixture(scope="module")
def make_serverless_cache():
    def _make_serverless_cache(yaml_name, input_dict, sc_name):
        sc = load_elasticache_resource(
            yaml_name, additional_replacements=input_dict)
        logging.debug(sc)

        reference = k8s.CustomResourceReference(
            CRD_GROUP, CRD_VERSION, RESOURCE_PLURAL, sc_name, namespace="default")
        _ = k8s.create_custom_resource(reference, sc)
        resource = k8s.wait_resource_consumed_by_controller(
            reference, wait_periods=15, period_length=20)
        assert resource is not None
        return (reference, resource)

    return _make_serverless_cache


@pytest.fixture(scope="module")
def sc_basic_input(make_sc_name):
    return {
        "SC_NAME": make_sc_name("sc-basic"),
        "ENGINE": "redis",
        "MAJOR_ENGINE_VERSION": "7"
    }


@pytest.fixture(scope="module")
def sc_basic(sc_basic_input, make_serverless_cache):
    (reference, resource) = make_serverless_cache(
        "serverless_cache_basic", sc_basic_input, sc_basic_input["SC_NAME"])
    yield reference, resource
    k8s.delete_custom_resource(reference)
    sleep(DEFAULT_WAIT_SECS)
    wait_serverless_cache_deleted(sc_basic_input['SC_NAME'])


@pytest.fixture(scope="module")
def sc_update_input(make_sc_name):
    return {
        "SC_NAME": make_sc_name("sc-update"),
        "ENGINE": "redis",
        "MAJOR_ENGINE_VERSION": "7",
        "DESCRIPTION": "initial description",
        "DAILY_SNAPSHOT_TIME": "05:00",
        "SNAPSHOT_RETENTION_LIMIT": "5"
    }


@pytest.fixture(scope="module")
def sc_update(sc_update_input, make_serverless_cache):
    (reference, resource) = make_serverless_cache(
        "serverless_cache_update", sc_update_input, sc_update_input['SC_NAME'])
    yield reference, resource
    k8s.delete_custom_resource(reference)
    sleep(DEFAULT_WAIT_SECS)
    wait_serverless_cache_deleted(sc_update_input['SC_NAME'])


def wait_for_serverless_cache_available(elasticache_client, sc_name):
    """Wait for serverless cache to reach 'available' state using boto3 waiter.
    """
    waiter = elasticache_client.get_waiter('serverless_cache_available')
    waiter.config.delay = 5
    waiter.config.max_attempts = 240
    waiter.wait(ServerlessCacheName=sc_name)


def retrieve_serverless_cache(sc_name: str):
    """Retrieve serverless cache from AWS API"""
    ec = boto3.client("elasticache")
    response = ec.describe_serverless_caches(ServerlessCacheName=sc_name)
    return response['ServerlessCaches'][0] if response['ServerlessCaches'] else None


def assert_spec_tags(sc_name: str, spec_tags: list):
    """Assert that the serverless cache has the expected tags"""
    sc = retrieve_serverless_cache(sc_name)
    spec_tags_dict = {tag['key']: tag['value'] for tag in spec_tags}
    
    ec = boto3.client("elasticache")
    aws_tag_list = ec.list_tags_for_resource(ResourceName=sc['ARN'])['TagList']
    aws_tags_dict = {tag['Key']: tag['Value'] for tag in aws_tag_list}
    
    # Remove controller-managed tags
    controller_tag_version = "services.k8s.aws/controller-version"
    controller_tag_namespace = "services.k8s.aws/namespace"
    if controller_tag_version in aws_tags_dict:
        del aws_tags_dict[controller_tag_version]
    if controller_tag_namespace in aws_tags_dict:
        del aws_tags_dict[controller_tag_namespace]
    
    assert aws_tags_dict == spec_tags_dict


@service_marker
class TestServerlessCache:
    def test_sc_basic_creation(self, sc_basic, sc_basic_input):
        (reference, _) = sc_basic
        assert k8s.wait_on_condition(
            reference, "ACK.ResourceSynced", "True", wait_periods=90)

        # Assert initial state
        resource = k8s.get_resource(reference)
        assert resource['status']['status'] == "available"
        assert resource['spec']['engine'] == sc_basic_input['ENGINE']
        assert resource['spec']['majorEngineVersion'] == sc_basic_input['MAJOR_ENGINE_VERSION']

    def test_sc_invalid_engine(self, make_sc_name, make_serverless_cache):
        input_dict = {
            "SC_NAME": make_sc_name("sc-invalid-engine"),
            "ENGINE": "invalid-engine",
            "MAJOR_ENGINE_VERSION": "7"
        }
        (reference, resource) = make_serverless_cache(
            "serverless_cache_basic", input_dict, input_dict['SC_NAME'])

        sleep(DEFAULT_WAIT_SECS)
        resource = k8s.get_resource(reference)
        assert_recoverable_condition_set(resource)

        # Cleanup
        k8s.delete_custom_resource(reference)
        sleep(DEFAULT_WAIT_SECS)

    def test_sc_update(self, sc_update_input, sc_update, elasticache_client):
        (reference, _) = sc_update
        assert k8s.wait_on_condition(
            reference, "ACK.ResourceSynced", "True", wait_periods=90)

        # Assert initial state
        sc_name = sc_update_input['SC_NAME']
        resource = k8s.get_resource(reference)
        sc = retrieve_serverless_cache(sc_name)
        
        assert resource['spec']['description'] == sc_update_input['DESCRIPTION']
        assert resource['spec']['dailySnapshotTime'] == sc_update_input['DAILY_SNAPSHOT_TIME']
        assert resource['spec']['snapshotRetentionLimit'] == int(sc_update_input['SNAPSHOT_RETENTION_LIMIT'])
        assert sc['Description'] == sc_update_input['DESCRIPTION']
        assert sc['DailySnapshotTime'] == sc_update_input['DAILY_SNAPSHOT_TIME']
        assert sc['SnapshotRetentionLimit'] == int(sc_update_input['SNAPSHOT_RETENTION_LIMIT'])

        # Update fields
        new_description = "updated description"
        new_snapshot_time = "10:00"
        new_retention_limit = 3
        new_tags = [
            {"key": "Environment", "value": "test"},
            {"key": "Team", "value": "ack"}
        ]

        patch = {"spec": {
            "description": new_description,
            "dailySnapshotTime": new_snapshot_time,
            "snapshotRetentionLimit": new_retention_limit,
            "tags": new_tags
        }}
        _ = k8s.patch_custom_resource(reference, patch)
        sleep(DEFAULT_WAIT_SECS)
        assert k8s.wait_on_condition(
            reference, "ACK.ResourceSynced", "True", wait_periods=90)

        # Assert updated state
        resource = k8s.get_resource(reference)
        sc = retrieve_serverless_cache(sc_name)
        
        assert resource['spec']['description'] == new_description
        assert resource['spec']['dailySnapshotTime'] == new_snapshot_time
        assert resource['spec']['snapshotRetentionLimit'] == new_retention_limit
        assert sc['Description'] == new_description
        assert sc['DailySnapshotTime'] == new_snapshot_time
        assert sc['SnapshotRetentionLimit'] == new_retention_limit
        
        # Assert tags
        assert_spec_tags(sc_name, new_tags)

    def test_sc_creation_deletion(self, make_sc_name, make_serverless_cache, elasticache_client):
        input_dict = {
            "SC_NAME": make_sc_name("sc-delete"),
            "ENGINE": "redis",
            "MAJOR_ENGINE_VERSION": "7"
        }

        (reference, resource) = make_serverless_cache(
            "serverless_cache_basic", input_dict, input_dict["SC_NAME"])

        assert k8s.wait_on_condition(
            reference, "ACK.ResourceSynced", "True", wait_periods=90)

        # Assert initial state
        resource = k8s.get_resource(reference)
        assert resource['status']['status'] == "available"

        # Verify serverless cache exists in AWS
        sc = retrieve_serverless_cache(input_dict["SC_NAME"])
        assert sc is not None
        assert sc['ServerlessCacheName'] == input_dict["SC_NAME"]

        # Delete
        k8s.delete_custom_resource(reference)
        sleep(DEFAULT_WAIT_SECS)

        resource = k8s.get_resource(reference)
        assert resource['metadata']['deletionTimestamp'] is not None

        wait_serverless_cache_deleted(input_dict["SC_NAME"])