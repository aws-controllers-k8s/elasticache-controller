// A durability update is accepted while the replication group stays in the
// "available" state, and the durability reported by the API (and thus the spec
// populated from it on the read path) is briefly stale after the modify. Left alone
// the resource would report ACK.ResourceSynced=True immediately while Status.EffectiveDurability
// remained stale.
if ko != nil && delta.DifferentAt("Spec.Durability") {
    ackcondition.SetSynced(&resource{ko}, corev1.ConditionFalse, &condMsgDurabilityModifying, nil)
}