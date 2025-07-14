// Check if Tags are set in the spec and mark the resource as needing to be synced
// if they are. This will trigger a requeue and allow the tags to be synced in the
// next reconciliation loop.
if ko.Spec.Tags != nil {
    ackcondition.SetSynced(&resource{ko}, corev1.ConditionFalse, nil, nil)
}