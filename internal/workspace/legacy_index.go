package workspace

import (
	"bytes"
	"fmt"
	"gopkg.in/yaml.v3"
	"io"
	"os"
)

func readLegacyWorkspaceIndex(path string) (*Index, []byte, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, fmt.Errorf("read workspace index: %w", err)
	}
	var index Index
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	decoder.KnownFields(true)
	if err := decoder.Decode(&index); err != nil {
		return nil, nil, fmt.Errorf("parse workspace index: %w", err)
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return nil, nil, fmt.Errorf("parse workspace index: multiple YAML documents")
		}
		return nil, nil, fmt.Errorf("parse workspace index: %w", err)
	}
	return &index, data, nil
}
