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

"""Model for Declarative tests framework for custom resources
"""

from declarative_test_fwk import helper


class Step:
    """
    Represents a declarative test step
    """

    def __init__(self, config: dict, custom_resource_details: dict, replacements: dict = {}):
        self.config = config
        self.custom_resource_details = custom_resource_details
        self.replacements = replacements

        self.verb = None
        self.input_data = {}
        self.expectations = None

        # (k8s.CustomResourceReference, ko) to teardown
        self.teardown_list = []

        supported_verbs = ["create", "patch", "delete"]
        for verb in supported_verbs:
            if verb not in self.config:
                continue
            self.verb = verb
            self.input_data = self.config.get(verb)
            if type(self.input_data) is str:
                # consider the input as resource name
                # confirm that self.custom_resource_details must be provided with same name
                if self.custom_resource_details["metadata"]["name"] != self.input_data:
                    raise ValueError(f"Unable to determine input data for '{self.verb}' at step: {self.id()}")
                # self.custom_resource_details will be mixed in into self.input_data
                self.input_data = {}
            break

        if len(self.input_data) == 0 and not self.custom_resource_details:
            raise ValueError(f"Unable to determine custom resource at step: {self.id()}")

        if self.custom_resource_details:
            self.input_data = {**self.custom_resource_details, **self.input_data}
        self.expectations = self.config.get("expect")

    def id(self) -> str:
        return self.config.get("id", "")

    def description(self) -> str:
        return self.config.get("description", "")

    def resource_kind(self) -> str:
        return self.input_data.get("kind")

    def __str__(self) -> str:
        return f"Step(id='{self.id()}')"

    def __repr__(self) -> str:
        return str(self)


class Scenario:
    """
    Represents a declarative test scenario with steps
    """

    def __init__(self, config: dict, replacements: dict = {}):
        self.config = config
        self.test_steps = []
        self.replacements = replacements
        custom_resource_details = self.config.get("customResourceReference", {})
        for step in self.config.get("steps", []):
            self.test_steps.append(Step(step, custom_resource_details.copy(), replacements))

    def id(self) -> str:
        return self.config.get("id", "")

    def description(self) -> str:
        return self.config.get("description", "")

    def usecases(self) -> list:
        return self.config.get("usecases", [])

    def marks(self) -> list:
        return self.config.get("marks", [])

    def steps(self):
        return self.test_steps

    def __str__(self) -> str:
        return f"Scenario(id='{self.id()}')"

    def __repr__(self) -> str:
        return str(self)
