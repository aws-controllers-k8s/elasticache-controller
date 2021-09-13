	if !delta.DifferentAt("Spec.LogDeliveryConfigurations") {
		input.SetLogDeliveryConfigurations(nil)
	}
	if delta.DifferentAt("UserGroupIDs") {
		for _, diff := range delta.Differences {
			if diff.Path.Contains("UserGroupIDs") {
				existingUserGroups := diff.B.([]*string)
				requiredUserGroups := diff.A.([]*string)

				// User groups to add
				{
					var userGroupsToAdd []*string

					for _, requiredUserGroup := range requiredUserGroups {
						found := false
						for _, existingUserGroup := range existingUserGroups {
							if requiredUserGroup == existingUserGroup {
								found = true
								break
							}
						}

						if !found {
							userGroupsToAdd = append(userGroupsToAdd, requiredUserGroup)
						}
					}

					input.SetUserGroupIdsToAdd(userGroupsToAdd)
				}

				// User groups to remove
				{
					var userGroupsToRemove []*string

					for _, existingUserGroup := range existingUserGroups {
						found := false
						for _, requiredUserGroup := range requiredUserGroups {
							if requiredUserGroup == existingUserGroup {
								found = true
								break
							}
						}

						if !found {
							userGroupsToRemove = append(userGroupsToRemove, existingUserGroup)
						}
					}

					input.SetUserGroupIdsToRemove(userGroupsToRemove)
				}
			}
		}
	}