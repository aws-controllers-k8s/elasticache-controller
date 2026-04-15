	rm.setSyncedCondition(resp.Status, &resource{ko})
	if resp.Authentication != nil && ko.Spec.AuthenticationMode != nil {
		authType := string(resp.Authentication.Type)
		ko.Spec.AuthenticationMode.Type = &authType
	}
