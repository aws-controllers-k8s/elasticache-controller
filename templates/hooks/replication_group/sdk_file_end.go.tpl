// This method copies the data from given {{ .CRD.Names.Camel }} by populating it
// into copy of supplied resource and returns that.
func (rm *resourceManager) set{{ .CRD.Names.Camel }}Output (
	r *resource,
	obj *svcsdk.{{ .CRD.Names.Camel }},
) (*resource, error) {
	if obj == nil ||
		r == nil ||
		r.ko == nil {
		return nil, nil
	}
	resp := &svcsdk.{{ .CRD.Ops.Create.OutputRef.Shape.ShapeName }}{ {{ .CRD.Names.Camel }}:obj }
	// Merge in the information we read from the API call above to the copy of
	// the original Kubernetes object we passed to the function
	ko := r.ko.DeepCopy()
{{ $createCode := GoCodeSetCreateOutput .CRD "resp" "ko" 1 }}
{{ $createCode }}
	rm.setStatusDefaults(ko)
{{- if $hookCode := Hook .CRD "sdk_file_end_set_output_post_populate" }}
{{ $hookCode }}
{{- end }}
	return &resource{ko}, nil
}
