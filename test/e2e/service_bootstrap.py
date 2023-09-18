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
"""Bootstraps the resources required to run Elasticache integration tests.
"""

import boto3
import yaml
import logging
import re
from dataclasses import dataclass

from acktest.aws.identity import get_account_id, get_region
from acktest.resources import random_suffix_name
from acktest import resources
from e2e import bootstrap_directory
from e2e.util import wait_usergroup_active, wait_snapshot_available
from e2e.bootstrap_resources import TestBootstrapResources, write_bootstrap_config

def create_sns_topic() -> str:
    topic_name = random_suffix_name("ack-sns-topic", 32)

    sns = boto3.client("sns")
    response = sns.create_topic(Name=topic_name)
    logging.info(f"Created SNS topic {response['TopicArn']}")

    return response['TopicArn']

# create an EC2 VPC security group from the default VPC (not an ElastiCache security group)
def create_security_group() -> str:
    region = get_region()
    account_id = get_account_id()

    ec2 = boto3.client("ec2")
    vpc_response = ec2.describe_vpcs(Filters=[{"Name": "isDefault", "Values": ["true"]}])
    if len(vpc_response['Vpcs']) == 0:
        raise ValueError(f"Default VPC not found for account {account_id} in region {region}")
    default_vpc_id = vpc_response['Vpcs'][0]['VpcId']

    sg_name = random_suffix_name("ack-security-group", 32)
    sg_description = "Security group for ACK ElastiCache tests"
    sg_response = ec2.create_security_group(GroupName=sg_name, VpcId=default_vpc_id, Description=sg_description)
    logging.info(f"Created VPC Security Group {sg_response['GroupId']}")

    return sg_response['GroupId']

def create_user_group() -> str:
    ec = boto3.client("elasticache")

    usergroup_id = random_suffix_name("ack-ec-usergroup", 32)
    _ = ec.create_user_group(UserGroupId=usergroup_id,
                                    Engine="Redis",
                                    UserIds=["default"])
    logging.info(f"Creating ElastiCache User Group {usergroup_id}")
    assert wait_usergroup_active(usergroup_id)

    return usergroup_id

def create_kms_key() -> str:
    kms = boto3.client("kms")

    response = kms.create_key(Description="Key for ACK ElastiCache tests")
    key_id = response['KeyMetadata']['KeyId']
    logging.info(f"Created KMS key {key_id}")

    return key_id

# create a cache cluster, snapshot it, and return the snapshot name
def create_cc_snapshot():
    ec = boto3.client("elasticache")

    cc_id = random_suffix_name("ack-cache-cluster", 32)
    _ = ec.create_cache_cluster(
        CacheClusterId=cc_id,
        NumCacheNodes=1,
        CacheNodeType="cache.t3.micro",
        Engine="redis"
    )
    waiter = ec.get_waiter('cache_cluster_available')
    waiter.wait(CacheClusterId=cc_id)
    logging.info(f"Created cache cluster {cc_id} for snapshotting")

    snapshot_name = random_suffix_name("ack-cc-snapshot", 32)
    _ = ec.create_snapshot(
        CacheClusterId=cc_id,
        SnapshotName=snapshot_name
    )
    assert wait_snapshot_available(snapshot_name)

    return snapshot_name


def create_non_default_user() -> str:
    ec = boto3.client("elasticache")
    user_id = random_suffix_name("ackecuser", 32)

    _ = ec.create_user(UserId=user_id,
                       UserName="ACKNonDefaultUser",
                       Engine="Redis",
                       NoPasswordRequired=True,
                       AccessString="on -@all")

    logging.info(f"Creating ElastiCache non default User {user_id}")
    return user_id


def create_log_group():
    logs = boto3.client("logs")
    log_group_name = random_suffix_name("ack-cw-log-group", 32)
    logs.create_log_group(logGroupName=log_group_name)

    logging.info(f"Create CW log group {log_group_name}")

    return log_group_name


def create_cpg():
    ec = boto3.client("elasticache")
    cpg_name = random_suffix_name("ack-cpg", 32)
    ec.create_cache_parameter_group(CacheParameterGroupName=cpg_name,
                                    CacheParameterGroupFamily='redis6.x', Description='ACK e2e test')

    logging.info(f"Created ElastiCache cache paramter group {cpg_name}")
    return cpg_name


# perform any cleanup tasks that need to be done before bootstrap and test execution
def pre_bootstrap_cleanup():
    # clear out relevant CW resource policy/policies
    cw = boto3.client("logs")

    deletion_list = []
    response = cw.describe_resource_policies()
    if 'resourcePolicies' in response:
        for policy in response['resourcePolicies']:
            if re.search('logdelivery', policy['policyName'], re.IGNORECASE):
                deletion_list.append(policy['policyName'])

    for policy in deletion_list:
        cw.delete_resource_policy(policyName=policy)
        logging.info(f'deleted resource policy {policy}')


def service_bootstrap() -> dict:
    logging.getLogger().setLevel(logging.INFO)
    pre_bootstrap_cleanup()

    return TestBootstrapResources(
        create_sns_topic(),
        create_sns_topic(),
        create_security_group(),
        create_security_group(),
        create_user_group(),
        create_user_group(),
        create_kms_key(),
        create_cc_snapshot(),
        create_non_default_user(),
        create_log_group(),
        create_log_group(),
        create_cpg()
    ).__dict__


if __name__ == "__main__":
    config = service_bootstrap()
    write_bootstrap_config(config, bootstrap_directory)