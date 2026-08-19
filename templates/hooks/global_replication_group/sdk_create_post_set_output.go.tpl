// After a successful Create call, capture the full GlobalReplicationGroupId
// (which includes the auto-generated region prefix) into status.
// The user only provides the suffix in spec; this is the canonical ID for all
// subsequent API calls.
	if resp.GlobalReplicationGroup != nil && resp.GlobalReplicationGroup.GlobalReplicationGroupId != nil {
		ko.Status.GlobalReplicationGroupID = resp.GlobalReplicationGroup.GlobalReplicationGroupId
	}
	if resp.GlobalReplicationGroup != nil && resp.GlobalReplicationGroup.Status != nil {
		ko.Status.Status = resp.GlobalReplicationGroup.Status
	}
	// Record the values of fields the Describe API does not return, so future
	// reconciles can detect when the user actually changes them.
	rm.setLastRequestedUnreadableFields(desired, ko)
