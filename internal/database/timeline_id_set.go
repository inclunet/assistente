package database

import "encoding/hex"

// JSON contém somente hex ASCII; unhex + CAST TEXT restaura os bytes originais
// (inclusive NUL/UTF-8 inválido) no lado direito de IN, preservando os índices.
func EncodeTimelineIDSet(ids []string) string {
	size := 2
	for _, id := range ids {
		size += 3 + 2*len(id)
	}
	encoded := make([]byte, 0, size)
	encoded = append(encoded, '[')
	for i, id := range ids {
		if i > 0 {
			encoded = append(encoded, ',')
		}
		encoded = append(encoded, '"')
		encoded = hex.AppendEncode(encoded, []byte(id))
		encoded = append(encoded, '"')
	}
	encoded = append(encoded, ']')
	return string(encoded)
}
