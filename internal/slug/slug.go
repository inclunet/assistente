// Package slug fornece a implementação canônica de geração de slugs do
// projeto. Centraliza a normalização usada por profiles, skills e jobs para
// que slugs sejam consistentes entre todos os domínios e correções de
// normalização sejam feitas em um único lugar.
package slug

import (
	"regexp"
	"strings"
	"unicode"

	"golang.org/x/text/unicode/norm"
)

// nonAlphanumeric casa qualquer sequência de caracteres que não sejam letras
// minúsculas ASCII ou dígitos. Compilado uma única vez por performance.
var nonAlphanumeric = regexp.MustCompile(`[^a-z0-9]+`)

// Slugify converte um nome arbitrário em um slug seguro para uso como nome de
// arquivo, diretório ou identificador estável.
//
// O algoritmo canônico:
//  1. Normaliza o texto em Unicode NFD para separar caracteres base de acentos.
//  2. Remove marcas diacríticas (acentos), ex.: "ã" -> "a", "ç" -> "c".
//  3. Converte para minúsculas.
//  4. Substitui qualquer sequência de caracteres não-alfanuméricos por um hífen.
//  5. Remove hífens das extremidades.
//
// Se o resultado for vazio (ex.: entrada vazia ou só com símbolos), retorna o
// fallback informado. O fallback NÃO é normalizado; cabe a quem chama fornecer
// um valor já válido (ex.: "perfil", "skill" ou "" para preservar vazio).
func Slugify(name, fallback string) string {
	normalized := norm.NFD.String(name)

	var builder strings.Builder
	builder.Grow(len(normalized))
	for _, r := range normalized {
		if unicode.Is(unicode.Mn, r) {
			continue // Pula combining marks (acentos).
		}
		builder.WriteRune(r)
	}

	result := strings.ToLower(builder.String())
	result = nonAlphanumeric.ReplaceAllString(result, "-")
	result = strings.Trim(result, "-")

	if result == "" {
		return fallback
	}

	return result
}
