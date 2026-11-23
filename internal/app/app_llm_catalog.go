package app

import (
	"assistente/internal/llmcatalog"
	"assistente/internal/wailsapi"
)

// LLMCatalogAPI permite que o entrypoint CLI use o mesmo contrato autenticado.
func LLMCatalogAPI(a *App) *wailsapi.LLMCatalog { return a.llmCatalogAPI }

func (a *App) initLLMCatalog() {
	if a.llmCatalog == nil {
		a.llmCatalog = llmcatalog.New(llmcatalog.Config{OnChanged: func() {
			if a.emitter != nil {
				a.emitter.Emit("llm_catalog:changed", nil)
			}
		}})
	}
	// O chat da CLI carrega o cache sob demanda após validar sua sessão.
	if !a.conversationCLI {
		a.llmCatalog.Start(a.appContext())
	}
}

func (a *App) wireLLMCatalog() {
	if a.llmCatalogAPI != nil {
		wailsapi.AttachLLMCatalog(a.llmCatalogAPI, wailsSession{app: a}, a.llmCatalog)
	}
}
