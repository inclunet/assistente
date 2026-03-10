//go:build !windows

package credentials

// ListKeyringEntries retorna lista vazia em plataformas sem suporte a enumeração.
func ListKeyringEntries() ([]KeyringEntry, error) {
	return []KeyringEntry{}, nil
}
