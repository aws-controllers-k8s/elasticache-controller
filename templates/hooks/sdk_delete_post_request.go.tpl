	// delete call successful
	if err == nil {
		rp, _ := rm.provideUpdatedResource(r, resp.ReplicationGroup)
		return rp, requeueWaitWhileDeleting
    }