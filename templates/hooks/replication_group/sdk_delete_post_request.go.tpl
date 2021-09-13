	// delete call successful
	if err == nil {
		rp, _ := rm.setReplicationGroupOutput(r, resp.ReplicationGroup)
		// Setting resource synced condition to false will trigger a requeue of
		// the resource. No need to return a requeue error here.
		ackcondition.SetSynced(
			r,
			corev1.ConditionFalse,
			&condMsgCurrentlyDeleting,
			nil,
		)
		return rp, nil
    }
