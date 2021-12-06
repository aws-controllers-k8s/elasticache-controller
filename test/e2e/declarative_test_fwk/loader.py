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

from e2e.declarative_test_fwk import model
import pytest
import os
import glob
from typing import Iterable, List
from pathlib import Path
from os.path import isfile, join, isdir
from acktest.resources import load_resource_file, random_suffix_name


def list_scenarios(scenarios_directory: Path) -> Iterable:
    """Lists test scenarios from given directory

    Args:
        scenarios_directory: directory containing scenarios yaml files

    Returns:
        Iterable scenarios for pytest parameterized fixture
    """

    scenarios_list = []
    scenario_files = glob.glob(str(scenarios_directory) + "/**/*.yaml", recursive=True)

    for scenario_file in scenario_files:
        scenarios_list.append(pytest.param(Path(scenario_file), marks=marks(Path(scenario_file))))

    return scenarios_list


def load_scenario(scenario_file: Path, resource_directory: Path = None, replacements: dict = {}) -> model.Scenario:
    """Loads scenario from given scenario_file

    Args:
        scenario_file: yaml file containing scenario
        resource_directory: Path to custom resources directory
        replacements: input replacements

    Returns:
        Scenario reference
    """

    scenario_name = scenario_file.stem
    replacements = replacements.copy()
    replacements["RANDOM_SUFFIX"] = random_suffix_name("", 16)
    scenario = model.Scenario(resource_directory, load_resource_file(
        scenario_file.parent, scenario_name, additional_replacements=replacements), replacements)
    return scenario


def idfn(scenario_file_full_path: Path) -> str:
    """Provides scenario file name as scenario test id

    Args:
        scenario_file_full_path: test scenario file path

    Returns:
        scenario test id string
    """

    return scenario_file_full_path.name


def marks(scenario_file_path: Path) -> List:
    """Provides pytest markers for the given scenario

    Args:
        scenario_file_path: test scenario file path

    Returns:
        pytest markers for the scenario
    """

    scenario_config = load_resource_file(
        scenario_file_path.parent, scenario_file_path.stem)

    markers = []
    for mark in scenario_config.get("marks", []):
        if mark == "canary":
            markers.append(pytest.mark.canary)
        elif mark == "slow":
            markers.append(pytest.mark.slow)
        elif mark == "blocked":
            markers.append(pytest.mark.blocked)
    return markers
