// Get the tags for the resource
if ko.Status.ACKResourceMetadata != nil && ko.Status.ACKResourceMetadata.ARN != nil {
    tags, err := util.GetTags(ctx, rm.sdkapi, rm.metrics, *ko.Status.ACKResourceMetadata.ARN)
    if err == nil {
        ko.Spec.Tags = tags
    }
}