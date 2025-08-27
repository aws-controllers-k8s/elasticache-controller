// Check if Tags are specified in the resource and mark the resource as
// needing to be synced if so.
if ko.Spec.Tags != nil {
    ackcondition.SetSynced(&resource{ko}, corev1.ConditionFalse, nil, nil)
}