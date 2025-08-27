// Get the resource's tags
if ko.Status.ACKResourceMetadata != nil && ko.Status.ACKResourceMetadata.ARN != nil {
    resourceARN := string(*ko.Status.ACKResourceMetadata.ARN)
    tags, err := rm.getTags(ctx, resourceARN)
    if err == nil {
        ko.Spec.Tags = tags
    }
}