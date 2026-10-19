	if !delta.DifferentAt("Spec.LogDeliveryConfigurations") {
		input.LogDeliveryConfigurations = nil
	}
	if !delta.DifferentAt("Spec.TransitEncryptionEnabled") {
		input.TransitEncryptionEnabled = nil
	}
	if !delta.DifferentAt("Spec.Durability") {
		input.Durability = ""
	}
	if delta.DifferentAt("Spec.UserGroupIDs") {
		input.UserGroupIdsToAdd, input.UserGroupIdsToRemove = userGroupIDsDifference(
			desired.ko.Spec.UserGroupIDs, latest.ko.Spec.UserGroupIDs,
		)
	}
