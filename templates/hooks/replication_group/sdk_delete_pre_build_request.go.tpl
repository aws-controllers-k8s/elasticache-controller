	if isDeleting(r) {
		// Setting resource synced condition to false will trigger a requeue of
		// the resource.
		ackcondition.SetSynced(
			r,
			corev1.ConditionFalse,
			&condMsgCurrentlyDeleting,
			nil,
		)
		// Need to return a requeue error here, otherwise:
		// - reconciler.deleteResource() marks the resource unmanaged
		// - reconciler.HandleReconcileError() does not update status for unmanaged resource
		// - reconciler.handleRequeues() is not invoked for delete code path.
		// TODO: return err as nil when reconciler is updated.
		return r, requeueWaitWhileDeleting
	}
	if isModifying(r) {
		// Setting resource synced condition to false will trigger a requeue of
		// the resource.
		ackcondition.SetSynced(
			r,
			corev1.ConditionFalse,
			&condMsgNoDeleteWhileModifying,
			nil,
		)
		// Need to return a requeue error here, otherwise:
		// - reconciler.deleteResource() marks the resource unmanaged
		// - reconciler.HandleReconcileError() does not update status for unmanaged resource
		// - reconciler.handleRequeues() is not invoked for delete code path.
		// TODO: return err as nil when reconciler is updated.
		return r, requeueWaitWhileModifying
	}
