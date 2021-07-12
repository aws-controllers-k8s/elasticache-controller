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

import logging
from time import sleep
from acktest.k8s import resource as k8s

TEST_HELPERS = dict()


def resource_helper(resource_kind: str):
    """
    Decorator to discover Custom Resource Helper
    :param resource_kind: custom resource kind
    """
    def registrar(cls):
        TEST_HELPERS[resource_kind.lower()] = cls
        logging.info(f"Registered ResourceHepler: {cls.__name__} for custom resource kind: {resource_kind}")
    return registrar


class ResourceHelper:
    """
    Provides generic verb (create, patch, delete) methods for custom resources.
    Keep its methods stateless. Methods are on instance to allow specialization.
    """
    DEFAULT_WAIT_SECS = 30

    def create(self, input_data: dict, input_replacements: dict = {}):
        """
        Creates custom resource
        :param input_data: resource details
        :param input_replacements: input replacements
        :return: k8s.CustomResourceReference, created custom resource
        """
        reference = self.custom_resource_reference(input_data, input_replacements)
        _ = k8s.create_custom_resource(reference, input_data)
        resource = k8s.wait_resource_consumed_by_controller(reference, wait_periods=10)
        assert resource is not None
        return (reference, resource)

    def patch(self, input_data: dict, input_replacements: dict = {}):
        """
        Patches custom resource
        :param input_data: resource patch details
        :param input_replacements: input replacements
        :return: k8s.CustomResourceReference, created custom resource
        """
        reference = self.custom_resource_reference(input_data, input_replacements)
        _ = k8s.patch_custom_resource(reference, input_data)
        sleep(self.DEFAULT_WAIT_SECS)  # required as controller has likely not placed the resource in modifying
        resource = k8s.wait_resource_consumed_by_controller(reference, wait_periods=10)
        assert resource is not None
        return (reference, resource)

    def delete(self, reference: k8s.CustomResourceReference):
        """
        Deletes custom resource and waits for delete completion
        :param reference: resource reference
        :return: None
        """
        resource = k8s.get_resource(reference)
        if not resource:
            logging.warning(f"ResourceReference {reference} not found. Not invoking k8s delete api.")
            return

        k8s.delete_custom_resource(reference)
        sleep(self.DEFAULT_WAIT_SECS)
        self.wait_for_delete(reference) # throws exception if wait fails

    def assert_expectations(self, verb: str, input_data: dict, expectations: dict, reference: k8s.CustomResourceReference):
        """
        Asserts custom resource reference against supplied expectations
        :param verb: expectations after performing the verb (apply, patch, delete)
        :param input_data: input data to verb
        :param expectations: expectations to assert
        :param reference: custom resource reference
        :return: None
        """
        # condition assertion contains wait logic
        self._assert_conditions(expectations, reference)

        # conditions expectations met, now check current resource against expectations
        resource = k8s.get_resource(reference)
        self.assert_items(expectations.get("status"), resource.get("status"))

        # self._assert_state(expectations.get("spec"), resource)  # uncomment to support spec assertions

    def _assert_conditions(self, expectations: dict, reference: k8s.CustomResourceReference):
        expect_conditions: dict = {}
        if "status" in expectations and "conditions" in expectations["status"]:
            expect_conditions = expectations["status"]["conditions"]

        for (condition_name, condition_value) in expect_conditions.items():
            assert k8s.wait_on_condition(reference, condition_name, condition_value, wait_periods=30)

    def assert_items(self, expectations: dict, state: dict):
        """
        Asserts state against supplied expectations
        Override it as needed for custom verifications
        :param expectations: dictionary with items to assert in state
        :param state: dictionary with items
        :return: None
        """
        if not expectations:
            return
        if not state:
            assert expectations == state

        for (property, value) in expectations.items():
            # conditions are processed separately
            if property == "conditions":
                continue
            assert (property, value) == (property, state.get(property))

    def custom_resource_reference(self, input_data: dict, input_replacements: dict = {}) -> k8s.CustomResourceReference:
        """
        Helper method to provide k8s.CustomResourceReference for supplied input
        :param input_data: custom resource input data
        :param input_replacements: input replacements
        :return: k8s.CustomResourceReference
        """
        resource_plural = self.resource_plural(input_data.get("kind"))
        resource_name = input_data.get("metadata").get("name")
        crd_group = input_replacements.get("CRD_GROUP")
        crd_version = input_replacements.get("CRD_VERSION")

        reference = k8s.CustomResourceReference(
            crd_group, crd_version, resource_plural, resource_name, namespace="default")
        return reference

    def wait_for_delete(self, reference: k8s.CustomResourceReference):
        """
        Override this method to implement custom resource delete logic.
        :param reference: custom resource reference
        :return: None
        """
        logging.debug(f"No-op wait_for_delete()")

    def resource_plural(self, resource_kind: str) -> str:
        """
        Provide plural string for supplied custom resource kind
        Override as needed
        :param resource_kind: custom resource kind
        :return: plural string
        """
        return resource_kind.lower() + "s"


def get_resource_helper(resource_kind: str) -> ResourceHelper:
    """
    Provides ResourceHelper for supplied custom resource kind
    If no helper is registered for the supplied resource kind then returns default ResourceHelper
    :param resource_kind: custom resource kind
    :return: custom resource helper
    """
    helper_cls = TEST_HELPERS.get(resource_kind.lower())
    if helper_cls:
        return helper_cls()
    return ResourceHelper()
