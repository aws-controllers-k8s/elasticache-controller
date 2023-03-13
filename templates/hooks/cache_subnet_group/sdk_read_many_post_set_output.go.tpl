
	subnets := make([]*string, 0, len(ko.Status.Subnets))
	for _, subnetIdIter := range ko.Status.Subnets {
		if subnetIdIter.SubnetIdentifier != nil {
			subnets = append(subnets, subnetIdIter.SubnetIdentifier)
		}
	}
	ko.Spec.SubnetIDs = subnets
