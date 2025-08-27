// If the Tags field has changed, sync the tags
if delta.DifferentAt("Spec.Tags") {
    err := rm.syncTags(
        ctx,
        desired,
        latest,
    )
    if err != nil {
        return nil, err
    }
}

// If the only difference is in the Tags field, we don't need to make an update call
if !delta.DifferentExcept("Spec.Tags") {
    return desired, nil
}