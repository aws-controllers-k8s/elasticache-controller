	if !delta.DifferentAt("Spec.LogDeliveryConfigurations") {
		input.LogDeliveryConfigurations = nil
	}
	if !delta.DifferentAt("Spec.TransitEncryptionEnabled") {
		input.TransitEncryptionEnabled = nil
	}
	if delta.DifferentAt("UserGroupIDs") {
		for _, diff := range delta.Differences {
			if diff.Path.Contains("UserGroupIDs") {
				existingUserGroups := diff.B.([]*string)
				requiredUserGroups := diff.A.([]*string)

				// User groups to add
				{
					var userGroupsToAdd []string

					for _, requiredUserGroup := range requiredUserGroups {
						found := false
						for _, existingUserGroup := range existingUserGroups {
							if requiredUserGroup == existingUserGroup {
								found = true
								break
							}
						}

						if !found {
							if requiredUserGroup != nil {
								userGroupsToAdd = append(userGroupsToAdd, *requiredUserGroup)
							}
						}
					}

					input.UserGroupIdsToAdd = userGroupsToAdd
				}

				// User groups to remove
				{
					var userGroupsToRemove []string

					for _, existingUserGroup := range existingUserGroups {
						found := false
						for _, requiredUserGroup := range requiredUserGroups {
							if requiredUserGroup == existingUserGroup {
								found = true
								break
							}
						}

						if !found {
							if existingUserGroup != nil {
								userGroupsToRemove = append(userGroupsToRemove, *existingUserGroup)
							}
						}
					}

					input.UserGroupIdsToRemove = userGroupsToRemove
				}
			}
		}
	}
