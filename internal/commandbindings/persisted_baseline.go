package commandbindings

import (
	"fmt"
	"strings"
)

// WithPersistedBaseline attaches the host-computed immutable identity of the
// persisted command/authority base. It is not caller authority; it only lets
// HostState distinguish a job-claim projection from a real config change.
func (c *Configuration) WithPersistedBaseline(fingerprint string) (*Configuration, error) {
	if c == nil || strings.TrimSpace(fingerprint) == "" || strings.TrimSpace(fingerprint) != fingerprint {
		return nil, fmt.Errorf("baseline persistido inválido")
	}
	clone := *c
	clone.persistedBaseline = fingerprint
	return &clone, nil
}

func (c *Configuration) PersistedBaseline() string {
	if c == nil {
		return ""
	}
	return c.persistedBaseline
}
