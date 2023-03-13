
	if len(ko.Spec.SubnetIDs) == 0 {
		for _, subnetIdIter := range ko.Status.Subnets {
			if subnetIdIter.SubnetIdentifier != nil {
				ko.Spec.SubnetIDs = append(ko.Spec.SubnetIDs, subnetIdIter.SubnetIdentifier)
			}
		}
	}
