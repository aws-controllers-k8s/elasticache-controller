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

"""Test Scenarios loader for Declarative tests framework for custom resources
"""

from declarative_test_fwk import model, helper
import pytest
import os
from typing import Iterable
from pathlib import Path
from acktest.resources import load_resource_file, random_suffix_name


def scenarios(scenarios_directory: Path) -> Iterable:
    """
    Loads scenarios from given directory
    :param scenarios_directory: directory containing scenarios yaml files
    :return: Iterable scenarios
    """
    for scenario_file in os.listdir(scenarios_directory):
        if not scenario_file.endswith(".yaml"):
            continue
        scenario_name = scenario_file.split(".yaml")[0]
        replacements = helper.input_replacements_dict.copy()
        replacements["RANDOM_SUFFIX"] = random_suffix_name("", 32)
        scenario = model.Scenario(load_resource_file(
            scenarios_directory, scenario_name, additional_replacements=replacements))
        yield pytest.param(scenario, marks=marks(scenario))


def idfn(scenario: model.Scenario) -> str:
    """
    Provides scenario test id
    :param scenario: test scenario
    :return: scenario test id string
    """
    return scenario.id()


def marks(scenario: model.Scenario) -> list:
    """
    Provides pytest markers for the scenario
    :param scenario: test scenario
    :return: markers for the scenario
    """
    markers = []
    for usecase in scenario.usecases():
        markers.append(pytest.mark.usecase(arg=usecase))
    for mark in scenario.marks():
        if mark == "canary":
            markers.append(pytest.mark.canary)
        elif mark == "slow":
            markers.append(pytest.mark.slow)
        elif mark == "blocked":
            markers.append(pytest.mark.blocked)
    return markers
