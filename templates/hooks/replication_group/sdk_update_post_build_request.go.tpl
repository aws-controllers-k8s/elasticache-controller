	if !delta.DifferentAt("Spec.LogDeliveryConfigurations") {
		input.LogDeliveryConfigurations = nil
	}
	if !delta.DifferentAt("Spec.TransitEncryptionEnabled") {
		input.TransitEncryptionEnabled = nil
	}
	if !delta.DifferentAt("Spec.Durability") {
		input.Durability = ""
	}
	rm.updateUserGroupIDsPayload(input, desired, latest, delta)
