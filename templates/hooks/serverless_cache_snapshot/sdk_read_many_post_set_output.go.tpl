    // Only fetch tags if the snapshot is available
    // ListTagsForResource fails when snapshot is still creating
    if ko.Status.ACKResourceMetadata != nil && ko.Status.ACKResourceMetadata.ARN != nil && 
        isServerlessCacheSnapshotAvailable(&resource{ko}) {
        resourceARN := (*string)(ko.Status.ACKResourceMetadata.ARN)
        tags, err := rm.getTags(ctx, *resourceARN)
        if err != nil {
            return nil, err
        }
        ko.Spec.Tags = tags
    }