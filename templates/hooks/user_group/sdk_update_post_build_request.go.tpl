   if err := rm.updateModifyUserGroupPayload(input, desired, latest, delta); err != nil {
        return nil, ackerr.NewTerminalError(err)
    }