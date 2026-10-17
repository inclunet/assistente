package apidto

import "assistente/internal/llmcatalog"

// LLMCatalogResponse é o snapshot tipado do catálogo público e seu estado de
// cache para os consumidores Wails.
type LLMCatalogResponse struct {
	Document   llmcatalog.Document `json:"document"`
	FetchedAt  string              `json:"fetched_at,omitempty"`
	AgeSeconds int64               `json:"age_seconds"`
	FromCache  bool                `json:"from_cache"`
	Stale      bool                `json:"stale"`
	Refreshing bool                `json:"refreshing"`
	ReasonCode string              `json:"reason_code,omitempty"`
}
