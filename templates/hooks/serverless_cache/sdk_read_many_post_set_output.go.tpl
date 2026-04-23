// Get the ARN from the resource metadata
if ko.Status.ACKResourceMetadata != nil && ko.Status.ACKResourceMetadata.ARN != nil {
    // Retrieve the tags for the resource
    resourceARN := string(*ko.Status.ACKResourceMetadata.ARN)
    tags, err := rm.getTags(ctx, resourceARN)
    if err != nil {
        return nil, err
    }
    ko.Spec.Tags = tags
}