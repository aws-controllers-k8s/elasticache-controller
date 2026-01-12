  if !isActive(latest.ko) {
		return nil, requeueWaitUntilCanModify(latest)
	}