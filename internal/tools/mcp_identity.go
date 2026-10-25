package tools

// MCPToolIdentity é declarada pela ponte, independentemente do nome publicado.
type MCPToolIdentity struct {
	ServerSlug string
	ToolName   string
}

type MCPIdentityProvider interface {
	MCPIdentity() MCPToolIdentity
}

func MCPIdentityOf(tool Tool) (MCPToolIdentity, bool) {
	provider, ok := tool.(MCPIdentityProvider)
	if !ok {
		return MCPToolIdentity{}, false
	}
	identity := provider.MCPIdentity()
	return identity, identity.ServerSlug != "" && identity.ToolName != ""
}
