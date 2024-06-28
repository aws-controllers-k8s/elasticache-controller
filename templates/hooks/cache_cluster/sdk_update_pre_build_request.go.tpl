	if delta.DifferentAt("Spec.Tags") {
		if err = rm.syncTags(ctx, desired, latest); err != nil {
			return nil, err
		}
	} else if !delta.DifferentExcept("Spec.Tags") {
		// If the only difference between the desired and latest is in the
		// Spec.Tags field, we can skip the ModifyCacheCluster call.
		return desired, nil
	}
