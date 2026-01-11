  res := desired.ko.DeepCopy()
	res.Status = latest.ko.Status
  
  if !isActive(latest.ko) {
		msg := "User group cannot be modifed while in '" + *latest.ko.Status.Status + "' status"
		ackcondition.SetSynced(&resource{res}, corev1.ConditionFalse, &msg, nil)
		return &resource{res}, requeueWaitUntilCanModify(latest)
	}