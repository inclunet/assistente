package mcp

// Compare the repository projection, not transient editor/runtime fields or
// nil-versus-empty collection representations produced by JSON round-trips.
func persistedLegacyConfig(cfg ServerConfig) ServerConfig {
	row, _ := serverConfigToModel(cfg) // maps/slices contain only strings
	result, _ := serverModelToConfig(row)
	if len(result.Env) == 0 {
		result.Env = nil
	}
	if len(result.Args) == 0 {
		result.Args = nil
	}
	if len(result.OAuth2Scopes) == 0 {
		result.OAuth2Scopes = nil
	}
	return result
}
