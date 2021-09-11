	if isDeleting(r) {
		// Setting resource synced condition to false will trigger a requeue of
		// the resource. No need to return a requeue error here.
		ackcondition.SetSynced(
			r,
			corev1.ConditionFalse,
			&condMsgCurrentlyDeleting,
			nil,
		)
		return r, nil
	}
	if isModifying(r) {
		// Setting resource synced condition to false will trigger a requeue of
		// the resource. No need to return a requeue error here.
		ackcondition.SetSynced(
			r,
			corev1.ConditionFalse,
			&condMsgNoDeleteWhileModifying,
			nil,
		)
		return r, nil
	}
