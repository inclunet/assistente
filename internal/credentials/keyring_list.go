//go:build windows

package credentials

import (
	"fmt"

	"github.com/danieljoos/wincred"
)

// ListKeyringEntries enumera credenciais genéricas do Windows Credential Manager.
func ListKeyringEntries() ([]KeyringEntry, error) {
	creds, err := wincred.List()
	if err != nil {
		return nil, fmt.Errorf("erro ao listar credenciais do Windows: %w", err)
	}

	entries := make([]KeyringEntry, 0, len(creds))
	for _, c := range creds {
		target := c.TargetName
		if target == "" {
			continue
		}
		entries = append(entries, KeyringEntry{
			Target: target,
			User:   c.UserName,
		})
	}
	return entries, nil
}
