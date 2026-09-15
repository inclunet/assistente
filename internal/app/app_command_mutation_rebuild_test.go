package app

import (
	"errors"
	"slices"
	"testing"

	"assistente/internal/commandconfig"
)

func TestCommandMutationActiveLayerSet(t *testing.T) {
	for _, test := range []struct {
		name           string
		before, after  []string
		equal, invalid bool
	}{
		{name: "nil-vazio", before: nil, after: []string{}, equal: true},
		{name: "vazio-nil", before: []string{}, after: nil, equal: true},
		{name: "ordem", before: []string{"b", "a"}, after: []string{"a", "b"}, equal: true},
		{name: "adicionada", before: []string{"a"}, after: []string{"a", "b"}},
		{name: "removida", before: []string{"a", "b"}, after: []string{"a"}},
		{name: "substituída", before: []string{"a"}, after: []string{"b"}},
		{name: "duplicada", before: []string{"a"}, after: []string{"a", "a"}, invalid: true},
		{name: "id-vazio", after: []string{""}, invalid: true},
		{name: "whitespace", after: []string{"a "}, invalid: true},
		{name: "nul", after: []string{"a\x00"}, invalid: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			originalBefore, originalAfter := slices.Clone(test.before), slices.Clone(test.after)
			before, err := commandMutationActiveLayerSet(test.before)
			if err != nil {
				t.Fatal(err)
			}
			after, err := commandMutationActiveLayerSet(test.after)
			if test.invalid {
				if !errors.Is(err, commandconfig.ErrInvalid) {
					t.Fatalf("erro = %v", err)
				}
			} else if err != nil || slices.Equal(before, after) != test.equal {
				t.Fatalf("conjuntos %v/%v, erro=%v", before, after, err)
			}
			if !slices.Equal(originalBefore, test.before) || !slices.Equal(originalAfter, test.after) {
				t.Fatal("alterou slice do provider")
			}
		})
	}
}
