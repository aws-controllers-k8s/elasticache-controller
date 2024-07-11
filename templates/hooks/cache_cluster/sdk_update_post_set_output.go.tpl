    if pendingModifications := resp.CacheCluster.PendingModifiedValues; pendingModifications != nil {
		if pendingModifications.NumCacheNodes != nil {
			ko.Spec.NumCacheNodes = pendingModifications.NumCacheNodes
		}
		if pendingModifications.CacheNodeType != nil {
			ko.Spec.CacheNodeType = pendingModifications.CacheNodeType
		}
		if pendingModifications.TransitEncryptionEnabled != nil {
			ko.Spec.TransitEncryptionEnabled = pendingModifications.TransitEncryptionEnabled
		}
	}
