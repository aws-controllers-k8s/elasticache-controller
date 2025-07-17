# # Copyright Amazon.com Inc. or its affiliates. All Rights Reserved.
# #
# # Licensed under the Apache License, Version 2.0 (the "License"). You may
# # not use this file except in compliance with the License. A copy of the
# # License is located at
# #
# #	 http://aws.amazon.com/apache2.0/
# #
# # or in the "license" file accompanying this file. This file is distributed
# # on an "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either
# # express or implied. See the License for the specific language governing
# # permissions and limitations under the License.

# """CRUD tests for the Elasticache User resource
# """

# import boto3
# import botocore
# import pytest

# from acktest.resources import random_suffix_name
# from acktest.k8s import resource as k8s

# from time import sleep
# from e2e import service_marker, CRD_GROUP, CRD_VERSION, load_elasticache_resource
# from e2e.util import assert_user_deletion

# RESOURCE_PLURAL = "users"
# DEFAULT_WAIT_SECS = 30


# @pytest.fixture(scope="module")
# def elasticache_client():
#     return boto3.client("elasticache")


# # set up input parameters for User
# @pytest.fixture(scope="module")
# def user_nopass_input():
#     return {
#         "USER_ID": random_suffix_name("user-nopass", 32),
#         "ACCESS_STRING": "on ~app::* -@all +@read"
#     }


# @pytest.fixture(scope="module")
# def user_nopass(user_nopass_input, elasticache_client):

#     # inject parameters into yaml; create User in cluster
#     user = load_elasticache_resource("user_nopass", additional_replacements=user_nopass_input)
#     reference = k8s.CustomResourceReference(
#         CRD_GROUP, CRD_VERSION, RESOURCE_PLURAL, user_nopass_input["USER_ID"], namespace="default")
#     _ = k8s.create_custom_resource(reference, user)
#     resource = k8s.wait_resource_consumed_by_controller(reference)
#     assert resource is not None
#     yield (reference, resource)

#     # teardown: delete in k8s, assert user does not exist in AWS
#     k8s.delete_custom_resource(reference)
#     sleep(DEFAULT_WAIT_SECS)
#     assert_user_deletion(user_nopass_input['USER_ID'])


# # create secrets for below user password test
# @pytest.fixture(scope="module")
# def secrets():
#     secrets = {
#         "NAME1": random_suffix_name("first", 32),
#         "NAME2": random_suffix_name("second", 32),
#         "KEY1": "secret1",
#         "KEY2": "secret2"
#     }
#     k8s.create_opaque_secret("default", secrets['NAME1'], secrets['KEY1'], random_suffix_name("password", 32))
#     k8s.create_opaque_secret("default", secrets['NAME2'], secrets['KEY2'], random_suffix_name("password", 32))
#     yield secrets

#     # teardown
#     k8s.delete_secret("default", secrets['NAME1'])
#     k8s.delete_secret("default", secrets['NAME2'])


# # input for test case with Passwords field
# @pytest.fixture(scope="module")
# def user_password_input(secrets):
#     inputs = {
#         "USER_ID": random_suffix_name("user-password", 32),
#         "ACCESS_STRING": "on ~app::* -@all +@read",
#     }
#     return {**secrets, **inputs}


# @pytest.fixture(scope="module")
# def user_password(user_password_input, elasticache_client):

#     # inject parameters into yaml; create User in cluster
#     user = load_elasticache_resource("user_password", additional_replacements=user_password_input)
#     reference = k8s.CustomResourceReference(
#         CRD_GROUP, CRD_VERSION, RESOURCE_PLURAL, user_password_input["USER_ID"], namespace="default")
#     _ = k8s.create_custom_resource(reference, user)
#     resource = k8s.wait_resource_consumed_by_controller(reference)
#     assert resource is not None
#     yield (reference, resource)

#     # teardown: delete in k8s, assert user does not exist in AWS
#     k8s.delete_custom_resource(reference)
#     sleep(DEFAULT_WAIT_SECS)
#     assert_user_deletion(user_password_input['USER_ID'])


# @service_marker
# class TestUser:

#     # CRUD test for User; "create" and "delete" operations implicit in "user" fixture
#     def test_user_nopass(self, user_nopass, user_nopass_input):
#         (reference, resource) = user_nopass
#         assert k8s.get_resource_exists(reference)

#         assert k8s.wait_on_condition(reference, "ACK.ResourceSynced", "True", wait_periods=5)
#         resource = k8s.get_resource(reference)
#         assert resource["status"]["lastRequestedAccessString"] == user_nopass_input["ACCESS_STRING"]

#         new_access_string = "on ~app::* -@all +@read +@write"
#         user_patch = {"spec": {"accessString": new_access_string}}
#         _ = k8s.patch_custom_resource(reference, user_patch)
#         sleep(DEFAULT_WAIT_SECS)

#         assert k8s.wait_on_condition(reference, "ACK.ResourceSynced", "True", wait_periods=5)
#         resource = k8s.get_resource(reference)
#         assert resource["status"]["lastRequestedAccessString"] == new_access_string

#     # test creation with Passwords specified (as k8s secrets)
#     def test_user_password(self, user_password, user_password_input):
#         (reference, resource) = user_password
#         assert k8s.get_resource_exists(reference)

#         assert k8s.wait_on_condition(reference, "ACK.ResourceSynced", "True", wait_periods=5)
#         resource = k8s.get_resource(reference)
#         assert resource["status"]["authentication"] is not None
#         assert resource["status"]["authentication"]["type_"] == "password"
#         assert resource["status"]["authentication"]["passwordCount"] == 2
