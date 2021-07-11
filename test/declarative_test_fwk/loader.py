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
from os.path import isfile, join
from acktest.resources import load_resource_file, random_suffix_name


def list_scenarios(scenarios_directory: Path) -> Iterable[Path]:
    """
    Lists test scenarios from given directory
    :param scenarios_directory: directory containing scenarios yaml files
    :return: Iterable scenarios
    """
    scenarios_list = []
    for scenario_file in os.listdir(scenarios_directory):
        scenario_file_full_path = join(scenarios_directory, scenario_file)
        if not isfile(scenario_file_full_path) or not scenario_file.endswith(".yaml"):
            continue
        scenarios_list.append(Path(scenario_file_full_path))
    return scenarios_list


def load_scenario(scenario_file: Path, replacements: dict = {}) -> Iterable:
    """
    Loads scenario from given scenario_file
    :param scenario_file: yaml file containing scenarios
    :param replacements: input replacements
    :return: Iterable scenarios
    """
    scenario_name = scenario_file.name.split(".yaml")[0]
    replacements = replacements.copy()
    replacements["RANDOM_SUFFIX"] = random_suffix_name("", 32)
    scenario = model.Scenario(load_resource_file(
        scenario_file.parent, scenario_name, additional_replacements=replacements), replacements)
    yield pytest.param(scenario, marks=marks(scenario))


def idfn(scenario_file_full_path: Path) -> str:
    """
    Provides scenario file name as scenario test id
    :param scenario: test scenario file path
    :return: scenario test id string
    """
    return scenario_file_full_path.name


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
