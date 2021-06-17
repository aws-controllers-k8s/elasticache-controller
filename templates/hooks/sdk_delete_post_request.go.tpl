	// delete call successful
	if err == nil {
		rp, _ := rm.setReplicationGroupOutput(r, resp.ReplicationGroup)
		return rp, requeueWaitWhileDeleting
    }