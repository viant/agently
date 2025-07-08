package overlay

// Overlay represents a declarative schema augmentation loaded from a YAML or
// JSON document located under "<workspace>/elicitation".  The structure is a
// generalisation of the preset format already used by the elicitation UI.

type Overlay struct {
	Match struct {
		Fields []string `json:"fields" yaml:"fields"`
	} `json:"match" yaml:"match"`

	Fields []map[string]interface{} `json:"fields" yaml:"fields"`
}

// Apply merges the overlay’s field-level overrides into the supplied
// properties map (which must be the JSON-Schema "properties" object). The map
// is modified in-place.
func (o *Overlay) Apply(props map[string]any) {
	if o == nil || props == nil {
		return
	}
	for _, fld := range o.Fields {
		nameRaw, ok := fld["name"]
		if !ok {
			continue
		}
		name, ok := nameRaw.(string)
		if !ok || name == "" {
			continue
		}
		propAny, ok := props[name]
		if !ok {
			continue // base schema lacks this property
		}
		prop, ok := propAny.(map[string]any)
		if !ok {
			continue
		}
		for k, v := range fld {
			if k == "name" {
				continue
			}
			prop[k] = v
		}
	}
}

// FieldsMatch reports whether every name in wanted exists as a key in props.
// If exact is true, the set must match exactly; otherwise wanted must be a
// subset. Only key presence is evaluated.
func FieldsMatch(props map[string]any, wanted []string, exact bool) bool {
	if len(wanted) == 0 || props == nil {
		return false
	}
	if exact && len(props) != len(wanted) {
		return false
	}
	for _, k := range wanted {
		if _, ok := props[k]; !ok {
			return false
		}
	}
	return true
}
