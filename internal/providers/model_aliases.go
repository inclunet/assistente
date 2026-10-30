package providers

import "assistente/internal/llmcatalog"

func (s *Service) validateModelAliasChanges(aliases, previous map[string]string) error {
	var document llmcatalog.Document
	if s.catalogSnapshot != nil {
		document = s.catalogSnapshot().Document
	}
	return document.ValidateAliasChanges(aliases, previous)
}
