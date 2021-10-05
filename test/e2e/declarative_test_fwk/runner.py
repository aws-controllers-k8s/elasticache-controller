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

from e2e.declarative_test_fwk import model, helper
import pytest
import sys
import logging
from time import sleep
from acktest.k8s import resource as k8s


def run(scenario: model.Scenario) -> None:
    """Runs steps in the given scenario

    Args:
        scenario: the scenario to run

    Returns:
        None
    """

    logging.info(f"Execute: {scenario}")
    for step in scenario.steps:
        run_step(step)


def teardown(scenario: model.Scenario) -> None:
    """Teardown steps in the given scenario in reverse run order

    Args:
        scenario: the scenario to teardown

    Returns:
        None
    """

    logging.info(f"Teardown: {scenario}")
    teardown_failures = []
    # tear down steps in reverse order
    for step in reversed(scenario.steps):
        try:
            teardown_step(step)
        except:
            error = f"Failed to teardown: {step}. " \
                    f"Unexpected error: {sys.exc_info()[0]}"
            teardown_failures.append(error)

    if len(teardown_failures) != 0:
        teardown_failures.insert(0, f"Failures during teardown: {scenario}")
        failures = "\n\t- ".join(teardown_failures)
        logging.error(failures)
        pytest.fail(failures)


def run_step(step: model.Step) -> None:
    """Runs a test scenario step

    Args:
        step: the step to run

    Returns:
        None
    """

    logging.info(f"Execute: {step}")
    if step.verb == model.Verb.create:
        create_resource(step)
    elif step.verb == model.Verb.patch:
        patch_resource(step)
    elif step.verb == model.Verb.delete:
        delete_resource(step)
    wait(step)
    assert_expectations(step)


def create_resource(step: model.Step) -> None:
    """Perform the Verb "create" for given test step.
    It results in creating custom resource inside Kubernetes cluster per the specification from the step.

    Args:
        step: test step

    Returns:
        None
    """

    logging.debug(f"create: {step}")
    if not step.input_data:
        return
    resource_helper = helper.get_resource_helper(step.resource_kind)
    (reference, ko) = resource_helper.create(step.input_data, step.replacements)
    # track created reference to teardown later
    step.teardown_list.append((reference, ko))


def patch_resource(step: model.Step) -> None:
    """Perform the Verb "patch" for given test step.
    It results in patching custom resource inside Kubernetes cluster per the specification from the step.

    Args:
        step: test step

    Returns:
        None
    """

    logging.debug(f"patch: {step}")
    if not step.input_data:
        return

    resource_helper = helper.get_resource_helper(step.resource_kind)
    (reference, ko) = resource_helper.patch(step.input_data, step.replacements)
    # no need to teardown patched reference, its creator should tear it down.


def delete_resource(step: model.Step, reference: k8s.CustomResourceReference = None) -> None:
    """Perform the Verb "delete" for given custom resource reference in given test step.
    It results in deleting the custom resource inside Kubernetes cluster.

    Args:
        step: test step
        reference: custom resource reference to delete

    Returns:
        None
    """

    resource_helper = helper.get_resource_helper(step.resource_kind)
    if not reference:
        logging.debug(f"delete:  {step}")
        reference = resource_helper.custom_resource_reference(step.input_data, step.replacements)
    if k8s.get_resource_exists(reference):
        logging.debug(f"deleting resource:  {reference}")
        resource_helper.delete(reference)
    else:
        logging.info(f"Resource already deleted:  {reference}")


def wait(step: model.Step) -> None:
    """Performs wait logic for the given step.
    The step provides the wait details (properties/conditions values to wait for)

    Args:
        step: test step

    Returns:
        None
    """

    logging.debug(f"wait: {step}")
    if not step.wait:
        return

    if type(step.wait) is int:
        interval_seconds = step.wait
        logging.debug(f"Going to sleep for {interval_seconds} seconds during step {step}")
        sleep(interval_seconds)
        return

    resource_helper = helper.get_resource_helper(step.resource_kind)
    reference = resource_helper.custom_resource_reference(step.input_data, step.replacements)
    try:
        resource_helper.wait_for(step.wait, reference)
    except AssertionError as ae:
        logging.error(f"Wait failed, AssertionError at {step}")
        raise ae
    except Exception as e:
        logging.error(f"Wait failed, Exception at {step}")
        raise e


def assert_expectations(step: model.Step) -> None:
    """Asserts expectations as specified in the Step.

    Args:
        step: test step

    Returns:
        None
    """

    logging.info(f"assert:  {step}")
    if not step.expectations:
        return

    resource_helper = helper.get_resource_helper(step.resource_kind)
    reference = resource_helper.custom_resource_reference(step.input_data, step.replacements)
    try:
        resource_helper.assert_expectations(step.verb, step.input_data, step.expectations, reference)
    except AssertionError as ae:
        logging.error(f"AssertionError at {step}")
        raise ae


def teardown_step(step: model.Step) -> None:
    """Teardown custom resources that were created during step execution (run) inside Kubernetes cluster.

    Args:
        step: test step

    Returns:
        None
    """

    if not step or len(step.teardown_list) == 0:
        return

    logging.info(f"teardown: {step}")

    for (reference, _) in step.teardown_list:
        if reference:
            delete_resource(step, reference)

    # clear list
    step.teardown_list = []
