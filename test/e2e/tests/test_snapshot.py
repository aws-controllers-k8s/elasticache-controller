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

"""Integration tests for the Elasticache Snapshot resource
"""

import pytest
import logging
import boto3
from time import sleep

from acktest.resources import random_suffix_name
from acktest.k8s import resource as k8s
from e2e import service_marker, CRD_GROUP, CRD_VERSION, load_elasticache_resource
from e2e.bootstrap_resources import get_bootstrap_resources
from e2e.util import wait_snapshot_deleted

RESOURCE_PLURAL = "snapshots"
DEFAULT_WAIT_SECS = 30

@pytest.fixture(scope="module")
def ec_client():
    ec = boto3.client("elasticache")
    return ec

# retrieve resources created in the bootstrap step
@pytest.fixture(scope="module")
def bootstrap_resources():
    return get_bootstrap_resources()

# factory for snapshots
@pytest.fixture(scope="module")
def make_snapshot():
    def _make_snapshot(yaml_name, input_dict, snapshot_name):
        snapshot = load_elasticache_resource(
            yaml_name, additional_replacements=input_dict)
        logging.debug(snapshot)

        reference = k8s.CustomResourceReference(
            CRD_GROUP, CRD_VERSION, RESOURCE_PLURAL, snapshot_name, namespace="default")
        _ = k8s.create_custom_resource(reference, snapshot)
        resource = k8s.wait_resource_consumed_by_controller(reference, wait_periods=10)
        assert resource is not None
        return (reference, resource)

    return _make_snapshot

# setup/teardown for test_snapshot_kms
@pytest.fixture(scope="module")
def snapshot_kms(ec_client, bootstrap_resources, make_snapshot):
    response = ec_client.describe_snapshots(SnapshotName=bootstrap_resources.SnapshotName)
    cc_id = response['Snapshots'][0]['CacheClusterId']

    snapshot_name = random_suffix_name("ack-snapshot-kms", 32)

    input_dict = {
        "SNAPSHOT_NAME": snapshot_name,
        "CC_ID": cc_id,
        "KMS_KEY_ID": bootstrap_resources.KmsKeyID,
    }

    (reference, resource) = make_snapshot("snapshot_kms", input_dict, input_dict['SNAPSHOT_NAME'])
    yield (reference, resource)

    # teardown
    k8s.delete_custom_resource(reference)
    assert wait_snapshot_deleted(snapshot_name)

@service_marker
class TestSnapshot:

    # test create of snapshot while providing KMS key
    def test_snapshot_kms(self, snapshot_kms):
        (reference, _) = snapshot_kms
        assert k8s.wait_on_condition(reference, "Ready", "True", wait_periods=15)
