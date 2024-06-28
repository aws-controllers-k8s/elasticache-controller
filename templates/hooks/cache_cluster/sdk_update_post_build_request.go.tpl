    if err := rm.updateCacheClusterPayload(input, desired, latest, delta); err != nil {
        return nil, ackerr.NewTerminalError(err)
    }
