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

"""Declares the structure of the bootstrapped resources and provides a loader
for them.
"""

from dataclasses import dataclass
from e2e import bootstrap_directory
import yaml
import logging
from pathlib import Path

@dataclass
class TestBootstrapResources:
    SnsTopic1: str
    SnsTopic2: str
    SecurityGroup1: str
    SecurityGroup2: str
    UserGroup1: str
    UserGroup2: str
    KmsKeyID: str
    SnapshotName: str
    NonDefaultUser: str
    CWLogGroup1: str
    CWLogGroup2: str
    CPGName: str

    def replacement_dict(self):
        return {
            "SNS_TOPIC_ARN": self.SnsTopic1,
            "SNS_TOPIC_ARN_2": self.SnsTopic2,
            "SG_ID": self.SecurityGroup1,
            "SG_ID_2": self.SecurityGroup2,
            "USERGROUP_ID": self.UserGroup1,
            "USERGROUP_ID_2": self.UserGroup2,
            "KMS_KEY_ID": self.KmsKeyID,
            "SNAPSHOT_NAME": self.SnapshotName,
            "NON_DEFAULT_USER": self.NonDefaultUser,
            "LOG_GROUP": self.CWLogGroup1,
            "LOG_GROUP_2": self.CWLogGroup2,
            "CACHE_PARAMETER_GROUP_NAME": self.CPGName
        }

_bootstrap_resources = None


def get_bootstrap_resources(bootstrap_file_name: str = "bootstrap.yaml"):
    global _bootstrap_resources
    if _bootstrap_resources is None:
        _bootstrap_resources = TestBootstrapResources(
            **read_bootstrap_config(bootstrap_directory, bootstrap_file_name=bootstrap_file_name),
        )
    return _bootstrap_resources


def write_bootstrap_config(bootstrap: dict, output_path: Path, bootstrap_file_name: str = "bootstrap.yaml"):
    """ Dumps the bootstrap object into a YAML file at a given path.

    Args:
        bootstrap: The bootstrap object.
        output_path: The directory in which to dump the bootstrap yaml.
        bootstrap_file_name: The name of the created bootstrap yaml file.
    """
    path =  output_path / bootstrap_file_name
    logging.info(f"Wrote bootstrap to {path}")
    with open(path, "w") as stream:
        yaml.safe_dump(bootstrap, stream)


def read_bootstrap_config(config_dir: Path, bootstrap_file_name: str = "bootstrap.yaml") -> dict:
    """ Reads a bootstrap dictionary from a given bootstrap file.

    Args:
        config_dir: The directory in which the bootstrap yaml exists.
        bootstrap_file_name: The name of the created bootstrap yaml file.

    Returns:
        dict: The bootstrap dictionary read from the file.
    """
    path = config_dir / bootstrap_file_name
    with open(path, "r") as stream:
        bootstrap = yaml.safe_load(stream)
    return bootstrap
