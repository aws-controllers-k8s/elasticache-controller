	rm.updateSpecFields(ctx, resp.ReplicationGroups[0], &resource{ko})
	if isDeleting(r) {
		// Setting resource synced condition to false will trigger a requeue of
		// the resource. No need to return a requeue error here.
		ackcondition.SetSynced(
			&resource{ko},
			corev1.ConditionFalse,
			&condMsgCurrentlyDeleting,
			nil,
		)
		return &resource{ko}, nil
	}
	if isModifying(r) {
		// Setting resource synced condition to false will trigger a requeue of
		// the resource. No need to return a requeue error here.
		ackcondition.SetSynced(
			&resource{ko},
			corev1.ConditionFalse,
			&condMsgNoDeleteWhileModifying,
			nil,
		)
		return &resource{ko}, nil
	}
	if isCreateFailed(r) {
        // This is a terminal state and by setting a Terminal condition on the
        // resource, we will prevent it from being requeued.
		ackcondition.SetTerminal(
			&resource{ko},
			corev1.ConditionTrue,
			&condMsgTerminalCreateFailed,
			nil,
		)
		return &resource{ko}, nil
	}
