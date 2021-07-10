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

"""
Tests for custom resources.
Uses declarative tests framework for custom resources.

To add test: add scenario yaml to scenarios/ directory.
"""

from declarative_test_fwk import helper, loader, runner

import pytest
import boto3
import logging

from e2e import service_marker, scenarios_directory, CRD_VERSION, CRD_GROUP, SERVICE_NAME
from e2e.bootstrap_resources import get_bootstrap_resources
from acktest.k8s import resource as k8s


@helper.input_replacements
def input_replacements():
    """
    Input replacements for test scenarios
    """
    replacements = get_bootstrap_resources().replacement_dict()
    replacements["CRD_VERSION"] = CRD_VERSION
    replacements["CRD_GROUP"] = CRD_GROUP
    replacements["SERVICE_NAME"] = SERVICE_NAME
    return replacements


@helper.resource_helper("ReplicationGroup")
class ReplicationGroupHelper(helper.ResourceHelper):
    """
    Helper for replication group scenarios
    """
    def wait_for_delete(self, reference: k8s.CustomResourceReference):
        logging.debug(f"ReplicationGroupHelper - wait_for_delete()")
        ec = boto3.client("elasticache")
        waiter = ec.get_waiter('replication_group_deleted')
        # throws exception if wait fails
        waiter.wait(ReplicationGroupId=reference.name)


@pytest.fixture(params=loader.scenarios(scenarios_directory), ids=loader.idfn)
def scenario(request):
    """
    Parameterized fixture
    Provides scenarios to execute
    Supports parallel execution of scenarios
    """
    s = request.param
    yield s
    runner.teardown(s)


@service_marker
class TestSuite:
    """
    Declarative scenarios based test suite
    """
    def test_scenario(self, scenario):
        runner.run(scenario)
