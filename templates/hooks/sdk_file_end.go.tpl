{{ $outputShape := .CRD.GetOutputShape .CRD.Ops.Create }}
// This method copies the data from given {{ $outputShape.ShapeName }} by populating it
// into copy of supplied resource and returns that.
func (rm *resourceManager) set{{ $outputShape.ShapeName }}Output (
	r *resource,
	obj *svcsdk.{{ $outputShape.ShapeName }},
) (*resource, error) {
	if obj == nil ||
		r == nil ||
		r.ko == nil {
		return nil, nil
	}
	resp := &svcsdk.{{ .CRD.Ops.Create.OutputRef.Shape.ShapeName }}{ {{ $outputShape.ShapeName }}:obj }
	// Merge in the information we read from the API call above to the copy of
	// the original Kubernetes object we passed to the function
	ko := r.ko.DeepCopy()
{{ $createCode := GoCodeSetCreateOutput .CRD "resp" "ko" 1 true }}
{{ $createCode }}
	rm.setStatusDefaults(ko)
{{- if $hookCode := Hook .CRD "sdk_file_end_set_output_post_populate" }}
{{ $hookCode }}
{{- end }}
	return &resource{ko}, nil
}
