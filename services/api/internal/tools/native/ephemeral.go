package native

func ephemeralFromInput(input map[string]any) bool {
	if v, ok := input["ephemeral"]; ok {
		if b, ok := v.(bool); ok {
			return b
		}
	}
	return true
}
