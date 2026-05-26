	if len(resp.Users) > 0 {
		rm.setSyncedCondition(resp.Users[0].Status, &resource{ko})
		if resp.Users[0].Authentication != nil && ko.Spec.AuthenticationMode != nil {
			authType := string(resp.Users[0].Authentication.Type)
			ko.Spec.AuthenticationMode.Type = &authType
		}
	}
