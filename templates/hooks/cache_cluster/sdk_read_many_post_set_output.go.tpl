    if pendingModifications := ko.Status.PendingModifiedValues; pendingModifications != nil {
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
	
	if len(ko.Status.SecurityGroups) > 0 {
		sgIDs := make([]*string, len(ko.Status.SecurityGroups))
		for i, sg := range ko.Status.SecurityGroups {
			id := *sg.SecurityGroupID
			sgIDs[i] = &id
		}
		ko.Spec.SecurityGroupIDs = sgIDs
	} else {
		ko.Spec.SecurityGroupIDs = nil
	}

	if isAvailable(r) {
		ackcondition.SetSynced(&resource{ko}, corev1.ConditionTrue, nil, nil)
	} else {
		// Setting resource synced condition to false will trigger a requeue of
		// the resource. No need to return a requeue error here.
		ackcondition.SetSynced(&resource{ko}, corev1.ConditionFalse, nil, nil)
		return &resource{ko}, nil
	}
    if ko.Status.ACKResourceMetadata != nil && ko.Status.ACKResourceMetadata.ARN != nil {
        resourceARN := (*string)(ko.Status.ACKResourceMetadata.ARN)
        tags, err := rm.getTags(ctx, *resourceARN)
        if err != nil {
            return nil, err
        }
        ko.Spec.Tags = tags
    }
