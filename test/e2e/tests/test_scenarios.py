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

from e2e import service_bootstrap, service_cleanup, service_marker, scenarios_directory, CRD_VERSION, CRD_GROUP, SERVICE_NAME
from acktest.k8s import resource as k8s

@helper.resource_helper("ReplicationGroup")
class ReplicationGroupHelper(helper.ResourceHelper):
    def assert_expectations(self, verb: str, input_data: dict, expectations: dict,
                            reference: k8s.CustomResourceReference):
        # default assertions
        super().assert_expectations(verb, input_data, expectations, reference)

        # perform custom server side checks based on:
        # verb, input data to verb, expectations for given resource

    """
    Helper for replication group scenarios
    """
    def wait_for_delete(self, reference: k8s.CustomResourceReference):
        logging.debug(f"ReplicationGroupHelper - wait_for_delete()")
        ec = boto3.client("elasticache")
        waiter = ec.get_waiter('replication_group_deleted')
        # throws exception if wait fails
        waiter.wait(ReplicationGroupId=reference.name)


@pytest.fixture(scope="session")
def input_replacements():
    """
    Session scoped fixture to bootstrap service resources and teardown
    provides input replacements for test scenarios.
    Eliminates the need for:
     - bootstrap.yaml and
     - call to <service_controller_test_dir>/service_bootstrap.py from test-infra
     - call to <service_controller_test_dir>/service_cleanup.py from test-infra
    """

    resources_dict = service_bootstrap.service_bootstrap()
    replacements = {
        "CRD_VERSION": CRD_VERSION,
        "CRD_GROUP": CRD_GROUP,
        "SERVICE_NAME": SERVICE_NAME,
        "SNS_TOPIC_ARN": resources_dict.get("SnsTopicARN"),
        "SG_ID": resources_dict.get("SecurityGroupID"),
        "USERGROUP_ID": resources_dict.get("UserGroupID"),
        "KMS_KEY_ID": resources_dict.get("KmsKeyID"),
        "SNAPSHOT_NAME": resources_dict.get("SnapshotName"),
        "NON_DEFAULT_USER": resources_dict.get("SnapshotName")
    }

    yield replacements
    # teardown
    service_cleanup.service_cleanup(resources_dict)


@pytest.fixture(params=loader.list_scenarios(scenarios_directory), ids=loader.idfn)
def scenario(request, input_replacements):
    """
    Parameterized fixture
    Provides scenarios to execute
    Supports parallel execution of scenarios
    """
    scenario_file_path = request.param
    scenario = loader.load_scenario(scenario_file_path, input_replacements)
    yield scenario
    runner.teardown(scenario)


@service_marker
class TestScenarios:
    """
    Declarative scenarios based test suite
    """
    def test_scenario(self, scenario):
        runner.run(scenario)
