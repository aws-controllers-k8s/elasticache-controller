// If the Tags field is different, sync the tags
if delta.DifferentAt("Spec.Tags") {
    err := rm.syncTags(
        ctx,
        latest,
        desired,
    )
    if err != nil {
        return nil, err
    }
}

// If the only difference is in the Tags field, return the desired object and nil
// to skip the update operation since we've already synced the tags
if !delta.DifferentExcept("Spec.Tags") {
    return desired, nil
}