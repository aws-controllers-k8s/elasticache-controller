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

"""Integration tests for the ElastiCache CacheParameterGroup resource
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

RESOURCE_PLURAL = "cacheparametergroups"

# Cache parameter groups are created synchronously, so these waits only need to
# cover the controller's reconcile latency.
CREATE_WAIT_AFTER_SECONDS = 20
MODIFY_WAIT_AFTER_SECONDS = 30

INITIAL_DESCRIPTION = "ack e2e test cache parameter group"


@pytest.fixture(scope="module")
def elasticache_client():
    return boto3.client('elasticache')


@pytest.fixture
def simple_cache_parameter_group():
    cpg_name = random_suffix_name("ack-test-cpg", 32)

    replacements = REPLACEMENT_VALUES.copy()
    replacements["CACHE_PARAMETER_GROUP_NAME"] = cpg_name
    replacements["CACHE_PARAMETER_GROUP_DESCRIPTION"] = INITIAL_DESCRIPTION

    resource_data = load_elasticache_resource(
        "cache_parameter_group_simple",
        additional_replacements=replacements,
    )
    logging.debug(resource_data)

    ref = k8s.CustomResourceReference(
        CRD_GROUP, CRD_VERSION, RESOURCE_PLURAL,
        cpg_name, namespace="default",
    )
    k8s.create_custom_resource(ref, resource_data)
    cr = k8s.wait_resource_consumed_by_controller(ref)

    assert cr is not None
    assert k8s.get_resource_exists(ref)

    yield (ref, cr)

    try:
        _, deleted = k8s.delete_custom_resource(ref, 3, 10)
        assert deleted
    except:
        pass


def get_aws_description(elasticache_client, cpg_name):
    aws_res = elasticache_client.describe_cache_parameter_groups(
        CacheParameterGroupName=cpg_name,
    )
    assert len(aws_res["CacheParameterGroups"]) == 1
    return aws_res["CacheParameterGroups"][0]["Description"]


def get_aws_parameter(elasticache_client, cpg_name, parameter_name):
    aws_res = elasticache_client.describe_cache_parameters(
        CacheParameterGroupName=cpg_name,
    )
    matches = [
        p for p in aws_res["Parameters"]
        if p["ParameterName"] == parameter_name
    ]
    assert len(matches) == 1
    return matches[0]


@service_marker
class TestCacheParameterGroup:
    def test_description_is_immutable(self, elasticache_client, simple_cache_parameter_group):
        """ModifyCacheParameterGroup cannot change a group's description, so the
        CRD rejects the edit at admission instead of the controller reconciling a
        delta it can never resolve.
        """
        (ref, cr) = simple_cache_parameter_group
        cpg_name = cr["spec"]["cacheParameterGroupName"]

        sleep(CREATE_WAIT_AFTER_SECONDS)
        assert k8s.wait_on_condition(
            ref, condition.CONDITION_TYPE_RESOURCE_SYNCED, "True",
            wait_periods=10, period_length=10,
        )
        assert get_aws_description(elasticache_client, cpg_name) == INITIAL_DESCRIPTION

        with pytest.raises(k8s.ApiException) as exc:
            k8s.patch_custom_resource(ref, {"spec": {"description": "an updated description"}})
        assert "Value is immutable once set" in str(exc.value.body)

        # The rejected patch never reached the cluster, so there is no drift to
        # reconcile and the resource stays synced on its original description.
        assert k8s.get_resource(ref)["spec"]["description"] == INITIAL_DESCRIPTION
        assert get_aws_description(elasticache_client, cpg_name) == INITIAL_DESCRIPTION
        assert k8s.wait_on_condition(
            ref, condition.CONDITION_TYPE_RESOURCE_SYNCED, "True",
            wait_periods=3, period_length=10,
        )

    def test_parameters_remain_updatable(self, elasticache_client, simple_cache_parameter_group):
        """Excluding the description from the delta must not suppress a genuine
        parameter update, which is the one thing this resource can modify.
        """
        (ref, cr) = simple_cache_parameter_group
        cpg_name = cr["spec"]["cacheParameterGroupName"]

        sleep(CREATE_WAIT_AFTER_SECONDS)
        assert k8s.wait_on_condition(
            ref, condition.CONDITION_TYPE_RESOURCE_SYNCED, "True",
            wait_periods=10, period_length=10,
        )
        assert get_aws_parameter(
            elasticache_client, cpg_name, "maxmemory-policy",
        )["Source"] == "system"

        updates = {
            "spec": {
                "parameterNameValues": [
                    {"parameterName": "maxmemory-policy", "parameterValue": "allkeys-lru"},
                ],
            },
        }
        k8s.patch_custom_resource(ref, updates)
        sleep(MODIFY_WAIT_AFTER_SECONDS)
        assert k8s.wait_on_condition(
            ref, condition.CONDITION_TYPE_RESOURCE_SYNCED, "True",
            wait_periods=10, period_length=10,
        )

        parameter = get_aws_parameter(elasticache_client, cpg_name, "maxmemory-policy")
        assert parameter["ParameterValue"] == "allkeys-lru"
        assert parameter["Source"] == "user"
