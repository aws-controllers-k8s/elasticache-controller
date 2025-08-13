// If tags have changed, sync them
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

// If only tags have changed, we don't need to update the resource
if !delta.DifferentExcept("Spec.Tags") {
    return desired, nil
}