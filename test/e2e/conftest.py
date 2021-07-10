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

import pytest
from acktest import k8s


def pytest_addoption(parser):
    parser.addoption("--runslow", action="store_true", default=False, help="run slow tests")
    parser.addoption("--runblocked", action="store_true", default=False, help="run blocked tests")


def pytest_configure(config):
    config.addinivalue_line(
        "markers", "canary: mark test to also run in canary tests"
    )
    config.addinivalue_line(
        "markers", "service(arg): mark test associated with a given service"
    )
    config.addinivalue_line(
        "markers", "usecase(arg): mark test associated with a given usecase"
    )
    config.addinivalue_line(
        "markers", "slow: mark test as slow to run"
    )
    config.addinivalue_line(
        "markers", "blocked: mark test as failing due to unresolved issue"
    )


def pytest_collection_modifyitems(config, items):
    # create skip markers
    skip_slow = pytest.mark.skip(reason="need --runslow option to run")
    skip_blocked = pytest.mark.skip(reason="need --runblocked option to run")

    # add skip markers to tests if the relevant command line option was not specified
    for item in items:
        if "slow" in item.keywords and not config.getoption("--runslow"):
            item.add_marker(skip_slow)
        if "blocked" in item.keywords and not config.getoption("--runblocked"):
            item.add_marker(skip_blocked)
    # TODO: choose test per 'usecase' selector


# Provide a k8s client to interact with the integration test cluster
@pytest.fixture(scope='class')
def k8s_client():
    return k8s._get_k8s_api_client()
