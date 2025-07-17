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

# """Integration tests for the Elasticache UserGroup resource
# """
# import pytest
# from acktest.k8s import resource as k8s
# from acktest.resources import random_suffix_name

# from e2e import service_marker, CRD_GROUP, CRD_VERSION, load_elasticache_resource
# from e2e.bootstrap_resources import get_bootstrap_resources

# RESOURCE_PLURAL = "usergroups"
# KIND_NAME = "UserGroup"


# @pytest.fixture(scope="module")
# def bootstrap_resources():
#     return get_bootstrap_resources()


# @pytest.fixture(scope="module")
# def get_user_group_yaml():
#     def _get_user_group_yaml(user_group_id):
#         input_dict = {
#             "USER_GROUP_ID": user_group_id,
#         }
#         user_group = load_elasticache_resource("usergroup", additional_replacements=input_dict)

#         return user_group
#     return _get_user_group_yaml


# # setup/teardown for test_user_group_create_update
# @pytest.fixture(scope="module")
# def user_group_create(get_user_group_yaml):
#     user_group_id = random_suffix_name("ack-usergroup", 32)

#     reference = k8s.CustomResourceReference(
#         CRD_GROUP, CRD_VERSION, RESOURCE_PLURAL, user_group_id, namespace="default")

#     user_group = get_user_group_yaml(user_group_id)

#     # Create new user group
#     _ = k8s.create_custom_resource(reference, user_group)
#     resource = k8s.wait_resource_consumed_by_controller(reference, wait_periods=10)
#     assert resource is not None
#     yield reference, resource

#     # Teardown
#     _, deleted = k8s.delete_custom_resource(reference)
#     assert deleted is True


# @service_marker
# class TestUserGroup:
#     def test_user_group_create_update(self, user_group_create, get_user_group_yaml, bootstrap_resources):
#         (reference, resource) = user_group_create
#         assert k8s.wait_on_condition(reference, "ACK.ResourceSynced", "True", wait_periods=15)

#         # Update the usergroup to include one more user
#         updated_user_group = get_user_group_yaml(reference.name)
#         updated_user_group["spec"]["userIDs"].append(bootstrap_resources.NonDefaultUser)

#         k8s.patch_custom_resource(reference, updated_user_group)

#         assert k8s.wait_on_condition(reference, "ACK.ResourceSynced", "True", wait_periods=15)
#         resource = k8s.get_resource(reference)
#         assert len(resource["spec"]["userIDs"]) == 2
#         assert resource["status"]["status"] == "active"
