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

PARAMETER_GROUP_FAMILY = "redis7"
INITIAL_DESCRIPTION = "ack e2e test cache parameter group"
TEST_PARAMETER = "maxmemory-policy"
TEST_PARAMETER_VALUE = "allkeys-lru"


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
    def test_description_drift_does_not_loop(self, elasticache_client, simple_cache_parameter_group):
        """ModifyCacheParameterGroup cannot change a group's description, so an
        edit must settle as a no-op rather than producing a delta the controller
        reconciles forever.
        """
        (ref, cr) = simple_cache_parameter_group
        cpg_name = cr["spec"]["cacheParameterGroupName"]

        sleep(CREATE_WAIT_AFTER_SECONDS)
        assert k8s.wait_on_condition(
            ref, condition.CONDITION_TYPE_RESOURCE_SYNCED, "True",
            wait_periods=10, period_length=10,
        )
        assert get_aws_description(elasticache_client, cpg_name) == INITIAL_DESCRIPTION

        synced_before = condition.get_synced_last_transition_time(ref)

        k8s.patch_custom_resource(ref, {"spec": {"description": "an updated description"}})
        sleep(MODIFY_WAIT_AFTER_SECONDS)

        # A fresh reconcile after the edit must still report synced. A resource
        # stuck on an unsatisfiable description delta never gets here.
        assert k8s.wait_on_condition_after(
            ref, condition.CONDITION_TYPE_RESOURCE_SYNCED, "True",
            last_transition_after=synced_before,
            wait_periods=10, period_length=10,
        )

        # The description is create-only, so AWS keeps its original value and
        # the group's parameters are untouched.
        assert get_aws_description(elasticache_client, cpg_name) == INITIAL_DESCRIPTION
        assert get_aws_parameter(
            elasticache_client, cpg_name, TEST_PARAMETER,
        )["Source"] == "system"

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
            elasticache_client, cpg_name, TEST_PARAMETER,
        )["Source"] == "system"

        updates = {
            "spec": {
                "parameterNameValues": [
                    {"parameterName": TEST_PARAMETER, "parameterValue": TEST_PARAMETER_VALUE},
                ],
            },
        }
        k8s.patch_custom_resource(ref, updates)
        sleep(MODIFY_WAIT_AFTER_SECONDS)
        assert k8s.wait_on_condition(
            ref, condition.CONDITION_TYPE_RESOURCE_SYNCED, "True",
            wait_periods=10, period_length=10,
        )

        parameter = get_aws_parameter(elasticache_client, cpg_name, TEST_PARAMETER)
        assert parameter["ParameterValue"] == TEST_PARAMETER_VALUE
        assert parameter["Source"] == "user"

    def test_adopt_description_divergent_group_preserves_parameters(self, elasticache_client):
        """Adopting a group whose real description differs from the manifest must
        still write the observed spec back, so the group's user parameters are
        preserved rather than reset.

        The runtime's adopt branch patches the whole spec from the observed AWS
        state. If that patch is rejected, spec.parameterNameValues is never
        populated and customUpdateCacheParameterGroup resets every parameter to
        its system default.
        """
        cpg_name = random_suffix_name("ack-test-cpg-adopt", 32)

        elasticache_client.create_cache_parameter_group(
            CacheParameterGroupName=cpg_name,
            CacheParameterGroupFamily=PARAMETER_GROUP_FAMILY,
            Description="description set outside of ack",
        )

        ref = None
        try:
            elasticache_client.modify_cache_parameter_group(
                CacheParameterGroupName=cpg_name,
                ParameterNameValues=[
                    {
                        "ParameterName": TEST_PARAMETER,
                        "ParameterValue": TEST_PARAMETER_VALUE,
                    },
                ],
            )
            parameter = get_aws_parameter(elasticache_client, cpg_name, TEST_PARAMETER)
            assert parameter["ParameterValue"] == TEST_PARAMETER_VALUE
            assert parameter["Source"] == "user"

            replacements = REPLACEMENT_VALUES.copy()
            replacements["CACHE_PARAMETER_GROUP_NAME"] = cpg_name
            replacements["CACHE_PARAMETER_GROUP_DESCRIPTION"] = "description set in the manifest"

            resource_data = load_elasticache_resource(
                "cache_parameter_group_adopt",
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

            sleep(CREATE_WAIT_AFTER_SECONDS)
            assert k8s.wait_on_condition(
                ref, condition.CONDITION_TYPE_RESOURCE_SYNCED, "True",
                wait_periods=10, period_length=10,
            )

            # The adopted group's user parameter must survive adoption.
            parameter = get_aws_parameter(elasticache_client, cpg_name, TEST_PARAMETER)
            assert parameter["ParameterValue"] == TEST_PARAMETER_VALUE
            assert parameter["Source"] == "user"

            # The observed spec was written back, which is what keeps the
            # parameters from being reset on the following reconcile.
            adopted = k8s.get_resource(ref)
            adopted_parameters = adopted["spec"].get("parameterNameValues") or []
            assert TEST_PARAMETER in [
                p.get("parameterName") for p in adopted_parameters
            ]
        finally:
            if ref is not None:
                try:
                    k8s.delete_custom_resource(ref, 3, 10)
                except:
                    pass
            try:
                elasticache_client.delete_cache_parameter_group(
                    CacheParameterGroupName=cpg_name,
                )
            except elasticache_client.exceptions.CacheParameterGroupNotFoundFault:
                pass
