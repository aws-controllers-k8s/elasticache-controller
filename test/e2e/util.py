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
"""ElastiCache-specific test utility functions
"""

import logging
import boto3

from time import sleep

ec = boto3.client("elasticache")


def wait_usergroup_active(usergroup_id: str,
                          wait_periods: int = 10,
                          period_length: int = 60) -> bool:
    for i in range(wait_periods):
        logging.debug(f"Waiting for user group {usergroup_id} to be active ({i})")
        response = ec.describe_user_groups(UserGroupId=usergroup_id)
        user_group = response['UserGroups'][0]

        if not user_group:
            logging.error(f"Could not find User Group {usergroup_id}")
            return False

        if user_group['Status'] == "active":
            logging.info(f"User Group {usergroup_id} is active, continuing...")
            return True

        sleep(period_length)

    logging.error(f"Wait for User Group {usergroup_id} to be active timed out")
    return False


def wait_snapshot_available(snapshot_name: str,
                            wait_periods: int = 10,
                            period_length: int = 60) -> bool:
    for i in range(wait_periods):
        logging.debug(f"Waiting for snapshot {snapshot_name} to be available ({i})")
        response = ec.describe_snapshots(SnapshotName=snapshot_name)
        snapshot = response['Snapshots'][0]

        if not snapshot:
            logging.error(f"Could not find snapshot {snapshot_name}")
            return False

        if snapshot['SnapshotStatus'] == "available":
            logging.info(f"Snapshot {snapshot_name} is available, continuing...")
            return True

        sleep(period_length)

    logging.error(f"Wait for snapshot {snapshot_name} to be available timed out")
    return False


def wait_snapshot_deleted(snapshot_name: str,
                          wait_periods: int = 10,
                          period_length: int = 60) -> bool:
    for i in range(wait_periods):
        logging.debug(f"Waiting for snapshot {snapshot_name} to be deleted ({i})")
        response = ec.describe_snapshots(SnapshotName=snapshot_name)

        if len(response['Snapshots']) == 0:
            return True

        sleep(period_length)

    logging.error(f"Wait for snapshot {snapshot_name} to be deleted timed out")
    return False


# assert that either: 1) deletion has been initiated, or 2) deletion has been completed
#   on the service-side
def assert_user_deletion(user_id: str):
    try:
        resp = ec.describe_users(UserId=user_id)
        assert len(resp['Users']) == 1
        assert resp['Users'][0]['Status'] == 'deleting'  # at this point, deletion is a server-side responsibility
    except ec.exceptions.UserNotFoundFault:
        pass  # we only expect this particular exception (if deletion has already completed)

# TODO: move to common repository
# given the latest state of the resource, assert that the terminal condition is set
def assert_terminal_condition_set(resource):
    terminal = None
    for cond in resource['status']['conditions']:
        if cond['type'] == "ACK.Terminal":
            terminal = cond

    assert terminal is not None
    assert terminal['status'] == "True"


# given the latest state of the resource, assert that the recoverable condition is set
def assert_recoverable_condition_set(resource):
    recoverable = None
    for cond in resource['status']['conditions']:
        if cond['type'] == "ACK.Recoverable":
            recoverable = cond

    assert recoverable is not None
    assert recoverable['status'] == "True"


# provide a basic nodeGroupConfiguration object of desired size
def provide_node_group_configuration(size: int):
    ngc = []
    for i in range(1, size + 1):
        ngc.append({"nodeGroupID": str(i).rjust(4, '0')})
    return ngc


# retrieve first cache cluster found from specified replication group
def retrieve_cache_cluster(rg_id: str):
    rg = retrieve_replication_group(rg_id)
    if len(rg['MemberClusters']) == 0:
        logging.debug(f"No member clusters found for replication group {rg_id}")
        return None

    cc_response = ec.describe_cache_clusters(CacheClusterId=rg['MemberClusters'][0])
    return cc_response['CacheClusters'][0]


def retrieve_replication_group(rg_id: str):
    rg_response = ec.describe_replication_groups(ReplicationGroupId=rg_id)
    return rg_response['ReplicationGroups'][0]


def retrieve_replication_group_tags(rg_arn: str):
    taglist_response = ec.list_tags_for_resource(ResourceName=rg_arn)
    return taglist_response['TagList']


def wait_serverless_cache_deleted(serverless_cache_name: str,
                                  wait_periods: int = 60,
                                  period_length: int = 10) -> bool:
    for i in range(wait_periods):
        logging.debug(f"Waiting for serverless cache {serverless_cache_name} to be deleted ({i})")
        try:
            response = ec.describe_serverless_caches(ServerlessCacheName=serverless_cache_name)
            if len(response['ServerlessCaches']) == 0:
                return True
        except ec.exceptions.ServerlessCacheNotFoundFault:
            return True
        except Exception as e:
            if "ServerlessCacheNotFoundFault" in str(e):
                return True
        
        sleep(period_length)

    logging.error(f"Wait for serverless cache {serverless_cache_name} to be deleted timed out")
    return False