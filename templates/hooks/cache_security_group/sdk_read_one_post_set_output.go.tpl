// Get the tags for the CacheSecurityGroup
if ko.Spec.CacheSecurityGroupName != nil {
    ko.Spec.Tags = rm.getTags(ctx, *ko.Spec.CacheSecurityGroupName)
}