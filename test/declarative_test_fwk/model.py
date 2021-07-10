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
    indent = "\t\t"

    def __init__(self, config: dict):
        self.config = config

        self.verb = None
        self.input_data = None
        self.expectations = None
        self.resource_helper = None

        # (k8s.CustomResourceReference, ko) to teardown
        self.teardown_list = []

        supported_verbs=["create", "patch", "delete"]
        for verb in supported_verbs:
            if verb not in self.config:
                continue
            self.verb = verb
            self.input_data = self.config.get(verb)
            break

        if self.input_data:
            self.expectations = self.config.get("expect")
            self.resource_helper = helper.get_resource_helper(self.input_data.get("kind"))

    def id(self) -> str:
        return self.config.get("id", "")

    def description(self) -> str:
        return self.config.get("description", "")


class Scenario:
    """
    Represents a declarative test scenario with steps
    """

    def __init__(self, config: dict):
        self.config = config
        self.test_steps = []
        for step in self.config.get("steps", []):
            self.test_steps.append(Step(step))

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
