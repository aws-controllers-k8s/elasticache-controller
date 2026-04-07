	rm.setSyncedCondition(resp.Users[0].Status, &resource{ko})
	if len(resp.Users) > 0 && resp.Users[0].Authentication != nil {
		if ko.Spec.AuthenticationMode == nil {
			ko.Spec.AuthenticationMode = &svcapitypes.AuthenticationMode{}
		}
		authType := string(resp.Users[0].Authentication.Type)
		ko.Spec.AuthenticationMode.Type = &authType
	}
