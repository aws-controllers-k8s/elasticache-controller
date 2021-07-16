	// if resource is already deleting, return requeue error; otherwise, initiate deletion
	if isDeleting(r) {
		return r, requeueWaitWhileDeleting
	}
