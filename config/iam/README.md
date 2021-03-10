# Additional IAM policies required for testing
As outlined in the 
[documentation for test setup](https://aws-controllers-k8s.github.io/community/dev-docs/testing/#iam-setup),
the `Admin-k8s` IAM role used to execute the tests requires permission to create and manage resources from the service whose controller
is being tested. For example, attaching the policy specified in the `recommended-policy-arn` file (located in this directory)
to the `Admin-k8s` role ensures the tests can perform all ElastiCache operations.

However, the ElastiCache tests also depend on other services and therefore require additional policies to be attached to
the `Admin-k8s` IAM role, or some tests may fail. See the below sections for instructions to set this up:


## KMS (Key Management Service)
The AWS-managed `AWSKeyManagementServicePowerUser` policy does not contain all the required permissions, so we will have
to manually create and attach a full-access policy:

1. The template for this policy is located in the `KMS-policy.json` file in this directory. Replace the two occurrences of
`<accountId>` with the AWS account ID used to execute these tests. This a complete policy specification.

2. Create the IAM policy by following the directions [here](https://docs.aws.amazon.com/IAM/latest/UserGuide/access_policies_create.html),
providing a name like `KMSFullAccess`. In the AWS console this basically amounts to pasting in
the policy JSON from step 1, but this is also doable in the AWS CLI/SDKs.

3. Attach the newly created policy to the `Admin-k8s` role via the console or the 
[attach-role-policy](https://docs.aws.amazon.com/cli/latest/reference/iam/attach-role-policy.html) operation in the CLI.



