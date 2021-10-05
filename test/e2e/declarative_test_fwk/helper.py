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

"""Helper for Declarative tests framework for custom resources
"""

from e2e.declarative_test_fwk import model

import logging
from typing import Tuple
from time import sleep
from acktest.k8s import resource as k8s

# holds custom resource helper references
TEST_HELPERS = dict()


def register_resource_helper(resource_kind: str, resource_plural: str):
    """Decorator to discover Custom Resource Helper

    Args:
        resource_kind: custom resource kind
        resource_plural: custom resource kind plural

    Returns:
        wrapper
    """

    def registrar(cls):
        global TEST_HELPERS
        if issubclass(cls, ResourceHelper):
            TEST_HELPERS[resource_kind.lower()] = cls
            cls.resource_plural = resource_plural.lower()
            logging.info(f"Registered ResourceHelper: {cls.__name__} for custom resource kind: {resource_kind}")
        else:
            msg = f"Unable to register helper for {resource_kind} resource: {cls} is not a subclass of ResourceHelper"
            logging.error(msg)
            raise Exception(msg)
    return registrar


class ResourceHelper:
    """Provides generic verb (create, patch, delete) methods for custom resources.
    Keep its methods stateless. Methods are on instance to allow specialization.
    """

    DEFAULT_WAIT_SECS = 30

    def create(self, input_data: dict, input_replacements: dict = {}) -> Tuple[k8s.CustomResourceReference, dict]:
        """Creates custom resource inside Kubernetes cluster per the specifications in input data.

        Args:
            input_data: custom resource details
            input_replacements: input replacements

        Returns:
            k8s.CustomResourceReference, created custom resource
        """

        reference = self.custom_resource_reference(input_data, input_replacements)
        _ = k8s.create_custom_resource(reference, input_data)
        resource = k8s.wait_resource_consumed_by_controller(reference, wait_periods=10)
        assert resource is not None
        return reference, resource

    def patch(self, input_data: dict, input_replacements: dict = {}) -> Tuple[k8s.CustomResourceReference, dict]:
        """Patches custom resource inside Kubernetes cluster per the specifications in input data.

        Args:
            input_data: custom resource patch details
            input_replacements: input replacements

        Returns:
            k8s.CustomResourceReference, created custom resource
        """

        reference = self.custom_resource_reference(input_data, input_replacements)
        _ = k8s.patch_custom_resource(reference, input_data)
        sleep(self.DEFAULT_WAIT_SECS)  # required as controller has likely not placed the resource in modifying
        resource = k8s.wait_resource_consumed_by_controller(reference, wait_periods=10)
        assert resource is not None
        return reference, resource

    def delete(self, reference: k8s.CustomResourceReference) -> None:
        """Deletes custom resource inside Kubernetes cluster and waits for delete completion

        Args:
            reference: custom resource reference

        Returns:
            None
        """

        resource = k8s.get_resource(reference)
        if not resource:
            logging.warning(f"ResourceReference {reference} not found. Not invoking k8s delete api.")
            return

        k8s.delete_custom_resource(reference, wait_periods=30, period_length=60)  # throws exception if wait fails
        sleep(self.DEFAULT_WAIT_SECS)
        self.wait_for_delete(reference)  # throws exception if wait fails

    def assert_expectations(self, verb: str, input_data: dict, expectations: model.ExpectDict, reference: k8s.CustomResourceReference) -> None:
        """Asserts custom resource reference inside Kubernetes cluster against the supplied expectations

        :param verb: expectations after performing the verb (apply, patch, delete)
        :param input_data: input data to verb
        :param expectations: expectations to assert
        :param reference: custom resource reference
        :return: None
        """
        self._assert_conditions(expectations, reference, wait=False)
        # conditions expectations met, now check current resource against expectations
        resource = k8s.get_resource(reference)
        self.assert_items(expectations.get("status"), resource.get("status"))

        # self._assert_state(expectations.get("spec"), resource)  # uncomment to support spec assertions

    def wait_for(self, wait_expectations: dict, reference: k8s.CustomResourceReference) -> None:
        """Waits for custom resource reference details inside Kubernetes cluster to match supplied config,
        currently supports wait on "status.conditions",
        it can be enhanced later for wait on any/other properties.

        Args:
            wait_expectations: properties to wait for
            reference:  custom resource reference

        Returns:
            None
        """

        # wait for conditions
        self._assert_conditions(wait_expectations, reference, wait=True)

    def _assert_conditions(self, expectations: dict, reference: k8s.CustomResourceReference, wait: bool = True) -> None:
        expect_conditions: dict = {}
        if "status" in expectations and "conditions" in expectations["status"]:
            expect_conditions = expectations["status"]["conditions"]

        default_wait_periods = 60
        # period_length = 1 will result in condition check every second
        default_period_length = 1
        for (condition_name, expected_value) in expect_conditions.items():
            if type(expected_value) is str:
                # Example: ACK.Terminal: "True"
                if wait:
                    assert k8s.wait_on_condition(reference, condition_name, expected_value,
                                                 wait_periods=default_wait_periods, period_length=default_period_length)
                else:
                    actual_condition = k8s.get_resource_condition(reference, condition_name)
                    assert actual_condition is not None
                    assert expected_value == actual_condition.get("status"), f"Condition status mismatch. Expected condition: {condition_name} - {expected_value} but found {actual_condition}"

            elif type(expected_value) is dict:
                # Example:
                # ACK.ResourceSynced:
                #     status: "False"
                #     message: "Expected message ..."
                #     timeout: 60 # seconds
                condition_value = expected_value.get("status")
                condition_message = expected_value.get("message")
                # default wait 60 seconds
                wait_timeout = expected_value.get("timeout", default_wait_periods)

                if wait:
                    assert k8s.wait_on_condition(reference, condition_name, condition_value,
                                                 wait_periods=wait_timeout, period_length=default_period_length)

                actual_condition = k8s.get_resource_condition(reference,
                                                              condition_name)
                assert actual_condition is not None
                assert condition_value == actual_condition.get("status"), f"Condition status mismatch. Expected condition: {condition_name} - {expected_value} but found {actual_condition}"
                if condition_message is not None:
                    assert condition_message == actual_condition.get("message"), f"Condition message mismatch. Expected condition: {condition_name} - {expected_value} but found {actual_condition}"

            else:
                raise Exception(f"Condition {condition_name} is provided with invalid value: {expected_value} ")

    def assert_items(self, expectations: dict, state: dict) -> None:
        """Asserts state against supplied expectations
        Override it as needed for custom verifications

        Args:
            expectations: dictionary with items (expected) to assert in state
            state: dictionary with items (actual)

        Returns:
            None
        """

        if not expectations:
            # nothing to assert as there are no expectations
            return
        if not state:
            # there are expectations but no given state to validate
            # following assert will fail and assert introspection will provide useful information for debugging
            assert expectations == state

        for (key, value) in expectations.items():
            # conditions are processed separately
            if key == "conditions":
                continue
            assert (key, value) == (key, state.get(key))

    def custom_resource_reference(self, input_data: dict, input_replacements: dict = {}) -> k8s.CustomResourceReference:
        """Helper method to provide k8s.CustomResourceReference for supplied input

        Args:
            input_data: custom resource input data
            input_replacements: input replacements

        Returns:
            k8s.CustomResourceReference
        """

        resource_name = input_data.get("metadata").get("name")
        crd_group = input_replacements.get("CRD_GROUP")
        crd_version = input_replacements.get("CRD_VERSION")

        reference = k8s.CustomResourceReference(
            crd_group, crd_version, self.resource_plural, resource_name, namespace="default")
        return reference

    def wait_for_delete(self, reference: k8s.CustomResourceReference) -> None:
        """Override this method to implement custom wail logic on resource delete.

        Args:
            reference: custom resource reference

        Returns:
            None
        """

        logging.debug(f"No-op wait_for_delete()")


def get_resource_helper(resource_kind: str) -> ResourceHelper:
    """Provides ResourceHelper for supplied custom resource kind
    If no helper is registered for the supplied resource kind then returns default ResourceHelper

    Args:
        resource_kind: custom resource kind string

    Returns:
        custom resource helper instance
    """

    helper_cls = TEST_HELPERS.get(resource_kind.lower())
    if helper_cls:
        return helper_cls()
    return ResourceHelper()
