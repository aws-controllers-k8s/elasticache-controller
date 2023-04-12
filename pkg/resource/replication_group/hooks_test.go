package replication_group

import (
	"context"
	"testing"

	ackv1alpha1 "github.com/aws-controllers-k8s/runtime/apis/core/v1alpha1"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/stretchr/testify/mock"

	svcapitypes "github.com/aws-controllers-k8s/elasticache-controller/apis/v1alpha1"
	mocksvcsdkapi "github.com/aws-controllers-k8s/elasticache-controller/mocks/aws-sdk-go/elasticache"
)

func Test_resourceManager_syncTags(t *testing.T) {
	testhelper := func() (*resourceManager, *mocksvcsdkapi.ElastiCacheAPI) {
		mocksdkapi := &mocksvcsdkapi.ElastiCacheAPI{}
		mocksdkapi.On("RemoveTagsFromResourceWithContext", mock.Anything, mock.Anything).Return(nil, nil)
		mocksdkapi.On("AddTagsToResourceWithContext", mock.Anything, mock.Anything).Return(nil, nil)
		rm := provideResourceManagerWithMockSDKAPI(mocksdkapi)
		return rm, mocksdkapi
	}
	t.Run("add and remove tags, only execute add tags", func(t *testing.T) {
		rm, mocksdkapi := testhelper()
		_ = rm.syncTags(context.Background(),
			&resource{ko: &svcapitypes.ReplicationGroup{
				Spec: svcapitypes.ReplicationGroupSpec{
					Tags: []*svcapitypes.Tag{
						{
							Key:   aws.String("add 1"),
							Value: aws.String("to add"),
						},
						{
							Key:   aws.String("add 2"),
							Value: aws.String("to add"),
						},
					},
				},
			}},
			&resource{ko: &svcapitypes.ReplicationGroup{
				Spec: svcapitypes.ReplicationGroupSpec{
					Tags: []*svcapitypes.Tag{
						{
							Key:   aws.String("remove"),
							Value: aws.String("to remove"),
						},
					},
				},
				Status: svcapitypes.ReplicationGroupStatus{
					ACKResourceMetadata: &ackv1alpha1.ResourceMetadata{
						ARN: (*ackv1alpha1.AWSResourceName)(aws.String("testARN")),
					},
				},
			}})
		mocksdkapi.AssertNumberOfCalls(t, "RemoveTagsFromResourceWithContext", 0)
		mocksdkapi.AssertNumberOfCalls(t, "AddTagsToResourceWithContext", 1)
	})

	t.Run("remove tags", func(t *testing.T) {
		rm, mocksdkapi := testhelper()
		_ = rm.syncTags(context.Background(),
			&resource{ko: &svcapitypes.ReplicationGroup{
				Spec: svcapitypes.ReplicationGroupSpec{
					Tags: []*svcapitypes.Tag{},
				},
			}},
			&resource{ko: &svcapitypes.ReplicationGroup{
				Spec: svcapitypes.ReplicationGroupSpec{
					Tags: []*svcapitypes.Tag{
						{
							Key:   aws.String("remove 1"),
							Value: aws.String("to remove"),
						},
						{
							Key:   aws.String("remove 2"),
							Value: aws.String("to remove"),
						},
					},
				},
				Status: svcapitypes.ReplicationGroupStatus{
					ACKResourceMetadata: &ackv1alpha1.ResourceMetadata{
						ARN: (*ackv1alpha1.AWSResourceName)(aws.String("testARN")),
					},
				},
			}})
		mocksdkapi.AssertNumberOfCalls(t, "RemoveTagsFromResourceWithContext", 1)
		mocksdkapi.AssertNumberOfCalls(t, "AddTagsToResourceWithContext", 0)
	})

	t.Run("modify existent tags, not remove call", func(t *testing.T) {
		rm, mocksdkapi := testhelper()
		_ = rm.syncTags(context.Background(),
			&resource{ko: &svcapitypes.ReplicationGroup{
				Spec: svcapitypes.ReplicationGroupSpec{
					Tags: []*svcapitypes.Tag{
						{
							Key:   aws.String("key1"),
							Value: aws.String("new value1"),
						},
						{
							Key:   aws.String("key2"),
							Value: aws.String("new value2"),
						},
					},
				},
			}},
			&resource{ko: &svcapitypes.ReplicationGroup{
				Spec: svcapitypes.ReplicationGroupSpec{
					Tags: []*svcapitypes.Tag{
						{
							Key:   aws.String("key1"),
							Value: aws.String("value1"),
						},
						{
							Key:   aws.String("key2"),
							Value: aws.String("value2"),
						},
					},
				},
				Status: svcapitypes.ReplicationGroupStatus{
					ACKResourceMetadata: &ackv1alpha1.ResourceMetadata{
						ARN: (*ackv1alpha1.AWSResourceName)(aws.String("testARN")),
					},
				},
			}})
		mocksdkapi.AssertNumberOfCalls(t, "RemoveTagsFromResourceWithContext", 0)
		mocksdkapi.AssertNumberOfCalls(t, "AddTagsToResourceWithContext", 1)
	})
}
