// Package installpath mantém somente a entrada de PATH pertencente ao instalador.
package installpath

import "strings"

func sameEntry(entry, directory string) bool {
	normalize := func(s string) string {
		return strings.TrimRight(strings.ReplaceAll(strings.Trim(strings.TrimSpace(s), `"`), "/", `\`), `\`)
	}
	return strings.EqualFold(normalize(entry), normalize(directory))
}

func addEntry(value, directory string) (string, bool) {
	for _, entry := range strings.Split(value, ";") {
		if sameEntry(entry, directory) {
			return value, false
		}
	}
	if value == "" {
		return directory, true
	}
	return value + ";" + directory, true
}

func removeEntry(value, directory string) string {
	entries := strings.Split(value, ";")
	kept := entries[:0]
	for _, entry := range entries {
		if !sameEntry(entry, directory) {
			kept = append(kept, entry)
		}
	}
	return strings.Join(kept, ";")
}
