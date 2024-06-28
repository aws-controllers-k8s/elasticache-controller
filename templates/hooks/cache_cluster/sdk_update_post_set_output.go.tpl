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
	if err == nil {
		// Setting resource synced condition to false will trigger a requeue of
		// the resource. No need to return a requeue error here.
		ackcondition.SetSynced(&resource{ko}, corev1.ConditionFalse, &condMsgCurrentlyUpdating, nil)
	}
