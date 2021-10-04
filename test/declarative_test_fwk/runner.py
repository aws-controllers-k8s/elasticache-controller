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

"""Runner for Declarative tests framework scenarios for custom resources
"""

from declarative_test_fwk import model, helper
import pytest
import sys
import logging
from acktest.k8s import resource as k8s


def run(scenario: model.Scenario) -> None:
    """
    Runs steps in the scenario
    """
    if not scenario:
        return

    logging.info(f"Execute: {scenario}")
    for step in scenario.steps():
        run_step(step)


def teardown(scenario: model.Scenario) -> None:
    """
    Teardown steps in the scenario in reverse order
    """
    if not scenario:
        return

    logging.info(f"Teardown: {scenario}")
    teardown_failures = []
    # tear down steps in reverse order
    for step in reversed(scenario.steps()):
        try:
            teardown_step(step)
        except:
            error = f"Failed to teardown: {step}. " \
                    f"Unexpected error: {sys.exc_info()[0]}"
            teardown_failures.append(error)
            logging.debug(error)

    if len(teardown_failures) != 0:
        teardown_failures.insert(0, f"Failures during teardown: {scenario}")
        failures = "\n\t- ".join(teardown_failures)
        logging.error(failures)
        pytest.fail(failures)


def run_step(step: model.Step) -> None:
    """
    Runs step
    """
    if not step:
        return

    if not step.verb:
        logging.warning(
            f"skipped: {step}. No matching verb found."
            f" Supported verbs: create, patch, delete.")
        return

    logging.info(f"Execute: {step}")
    if step.verb == "create":
        create_resource(step)
    elif step.verb == "patch":
        patch_resource(step)
    elif step.verb == "delete":
        delete_resource(step)
    assert_expectations(step)


def create_resource(step: model.Step) -> None:
    logging.debug(f"create:  {step}")
    if not step.input_data:
        return
    resource_helper = helper.get_resource_helper(step.resource_kind())
    (reference, ko) = resource_helper.create(step.input_data, step.replacements)
    # track created reference to teardown later
    step.teardown_list.append((reference, ko))


def patch_resource(step: model.Step) -> None:
    logging.debug(f"patch:   {step}")
    if not step.input_data:
        return

    resource_helper = helper.get_resource_helper(step.resource_kind())
    (reference, ko) = resource_helper.patch(step.input_data, step.replacements)
    # no need to teardown patched reference, its creator should tear it down.


def delete_resource(step: model.Step, reference: k8s.CustomResourceReference = None) -> None:
    resource_helper = helper.get_resource_helper(step.resource_kind())
    if not reference:
        logging.debug(f"delete:  {step}")
        reference = resource_helper.custom_resource_reference(step.input_data, step.replacements)

    resource_helper.delete(reference)


def assert_expectations(step: model.Step) -> None:
    logging.info(f"assert:  {step}")
    if not step.expectations:
        return

    resource_helper = helper.get_resource_helper(step.resource_kind())
    reference = resource_helper.custom_resource_reference(step.input_data, step.replacements)
    try:
        resource_helper.assert_expectations(step.verb, step.input_data, step.expectations, reference)
    except AssertionError as ae:
        logging.error(f"AssertionError at {step}")
        raise ae


def teardown_step(step: model.Step) -> None:
    """
    Teardown resources from the step
    """
    if not step or len(step.teardown_list) == 0:
        return

    logging.info(f"teardown: {step}")

    for (reference, _) in step.teardown_list:
        if reference:
            delete_resource(reference)

    # clear list
    step.teardown_list = []
