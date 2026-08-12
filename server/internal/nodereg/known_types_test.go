package nodereg

// knownTypes returns all registered node type ids (test helper).
func knownTypes() []string {
	out := make([]string, 0, len(registry))
	for t := range registry {
		out = append(out, t)
	}
	return out
}
