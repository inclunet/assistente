package llm

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

var (
	cloudflareRayIDHTMLRe = regexp.MustCompile(`(?is)Cloudflare Ray ID:\s*<strong[^>]*>([^<]+)</strong>`)
	errorCodeHTMLRe       = regexp.MustCompile(`(?is)Error code\s*([0-9]{3})`)
)

func summarizeHTTPError(statusCode int, body []byte) string {
	text := strings.TrimSpace(string(body))
	if text == "" {
		return fmt.Sprintf("Erro na API (%d): resposta vazia", statusCode)
	}

	// Cloudflare costuma devolver uma página HTML com o código 52x e detalhes.
	lower := strings.ToLower(text)
	isHTML := strings.Contains(lower, "<!doctype html") || strings.Contains(lower, "<html")
	looksLikeCloudflare := strings.Contains(lower, "cloudflare") || strings.Contains(lower, "cf-error") || strings.Contains(lower, "cdn-cgi")

	if isHTML && looksLikeCloudflare {
		code := statusCode
		if code == 0 {
			code = parseErrorCodeFromHTML(text)
		} else if code < 400 || code > 599 {
			// Em alguns cenários intermediários o status pode não bater com o HTML.
			if parsed := parseErrorCodeFromHTML(text); parsed != 0 {
				code = parsed
			}
		}

		ray := parseCloudflareRayID(text)
		raySuffix := ""
		if ray != "" {
			raySuffix = fmt.Sprintf(" (Ray ID: %s)", ray)
		}

		if code == 524 {
			return "Erro na API (524): Cloudflare timeout — o servidor de origem não respondeu a tempo" + raySuffix +
				"\nDicas: tente reduzir o tamanho do prompt/max_tokens, verifique carga/logs do servidor LLM e, se esta API está atrás do proxy do Cloudflare, considere usar DNS-only (grey cloud) para o subdomínio da API."
		}

		if code != 0 {
			return fmt.Sprintf("Erro na API (%d): erro retornado pelo Cloudflare%s", code, raySuffix)
		}

		return "Erro na API: erro retornado pelo Cloudflare" + raySuffix
	}

	// Texto não-HTML ou não-Cloudflare: evita jogar payload gigante no UI.
	const maxLen = 2000
	if len(text) > maxLen {
		text = text[:maxLen] + "… (truncado)"
	}
	if statusCode > 0 {
		return fmt.Sprintf("Erro na API (%d): %s", statusCode, text)
	}
	return "Erro na API: " + text
}

func parseCloudflareRayID(html string) string {
	m := cloudflareRayIDHTMLRe.FindStringSubmatch(html)
	if len(m) >= 2 {
		return strings.TrimSpace(m[1])
	}
	return ""
}

func parseErrorCodeFromHTML(html string) int {
	m := errorCodeHTMLRe.FindStringSubmatch(html)
	if len(m) >= 2 {
		n, err := strconv.Atoi(m[1])
		if err == nil {
			return n
		}
	}
	return 0
}
