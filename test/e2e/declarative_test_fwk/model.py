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


from enum import Enum, auto
from typing import TypedDict, Dict, Union, List
from pathlib import Path
from os.path import join
from acktest.resources import load_resource_file


class Verb(Enum):
    """
    Verb for custom resource in a test step.
    """
    create = auto()
    patch = auto()
    delete = auto()


# fields for 'resource' field in a test Scenario
class ResourceDict(TypedDict, total=False):
    apiVersion: str
    kind: str
    metadata: Dict


# fields for 'create' Verb in a test step
class CreateDict(ResourceDict):
    spec: Dict


# fields for 'patch' Verb in a test step
class PatchDict(ResourceDict):
    spec: Dict


# fields for 'delete' Verb in a test step
class DeleteDict(ResourceDict):
    pass


# fields for 'expect' field in a test step
class ExpectDict(TypedDict, total=False):
    spec: Dict
    status: Dict


# fields in a test step
class StepDict(TypedDict, total=False):
    id: str
    description: str
    create: Union[str, CreateDict]
    patch: Union[str, PatchDict]
    delete: Union[str, DeleteDict]
    wait: Union[int, Dict]
    expect: ExpectDict


class Step:
    """
    Represents a declarative test step
    """

    def __init__(self, resource_directory: Path, config: StepDict, custom_resource_details: dict, replacements: dict = {}):
        self.config = config
        self.custom_resource_details = custom_resource_details
        self.replacements = replacements

        self.verb = None
        self.input_data = {}
        self.expectations: ExpectDict = None

        # (k8s.CustomResourceReference, ko) to teardown
        self.teardown_list = []

        # validate: only one verb per step
        step_verb = None
        for verb in list(Verb):
            if verb.name in self.config:
                if not step_verb:
                    step_verb = verb
                else:
                    raise ValueError(f"Multiple verbs specified for step: {self.id}."
                                     f" Please specify only one verb from"
                                     f" supported verbs: { {verb.name for verb in list(Verb)} }.")

        # a step with no verb can be used to assert preconditions
        # thus, verb is optional.
        if step_verb:
            self.verb = step_verb
            self.input_data = self.config.get(step_verb.name)
            if type(self.input_data) is str:
                if self.input_data.endswith(".yaml"):
                    # load input data from resource file
                    resource_file_name = self.input_data
                    resource_file_path = Path(join(resource_directory, resource_file_name))
                    self.input_data = load_resource_file(
                        resource_file_path.parent, resource_file_path.stem, additional_replacements=replacements)
                else:
                    # consider the input as resource name string
                    # confirm that self.custom_resource_details must be provided with same name
                    if self.custom_resource_details["metadata"]["name"] != self.input_data:
                        raise ValueError(f"Unable to determine input data for '{self.verb}' at step: {self.id}")
                    # self.custom_resource_details will be mixed in into self.input_data
                    self.input_data = {}

        if len(self.input_data) == 0 and not self.custom_resource_details:
            raise ValueError(f"Unable to determine custom resource at step: {self.id}")

        if self.custom_resource_details:
            self.input_data = {**self.custom_resource_details, **self.input_data}

        self.wait = self.config.get("wait")
        self.expectations = self.config.get("expect")

    @property
    def id(self) -> str:
        return self.config.get("id", "")

    @property
    def description(self) -> str:
        return self.config.get("description", "")

    @property
    def resource_kind(self) -> str:
        return self.input_data.get("kind")

    def __str__(self) -> str:
        return f"Step(id='{self.id}')"

    def __repr__(self) -> str:
        return str(self)


# fields in a test scenario
class ScenarioDict(TypedDict, total=False):
    id: str
    description: str
    marks: List[str]
    resource: ResourceDict
    steps: List[StepDict]


class Scenario:
    """
    Represents a declarative test scenario with steps
    """

    def __init__(self, resource_directory: Path, config: ScenarioDict, replacements: dict = {}):
        self.config = config
        self.test_steps = []
        self.replacements = replacements
        custom_resource_details = self.config.get("resource", {})
        for step_config in self.config.get("steps", []):
            self.test_steps.append(Step(resource_directory, step_config, custom_resource_details.copy(), replacements))

    @property
    def id(self) -> str:
        return self.config.get("id", "")

    @property
    def description(self) -> str:
        return self.config.get("description", "")

    @property
    def steps(self):
        return self.test_steps

    def __str__(self) -> str:
        return f"Scenario(id='{self.id}')"

    def __repr__(self) -> str:
        return str(self)
