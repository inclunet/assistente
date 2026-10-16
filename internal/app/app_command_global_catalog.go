package app

import "assistente/internal/commandcatalog"

const (
	commandGlobalVoiceID = "voice.input.activate"
	commandGlobalJobID   = "job.run"
	commandGlobalLayerID = "application.global_hotkeys"
)

func commandGlobalRegistrations() []commandcatalog.Registration {
	voice := commandcatalog.Definition{
		ID: commandGlobalVoiceID, Effect: commandcatalog.Read, Decision: commandcatalog.NoDecision,
		AllowedSources: []commandcatalog.Source{commandcatalog.KeyboardGlobal},
		Context:        commandcatalog.ContextPolicy{Facts: []commandcatalog.ContextFact{{Provider: "workspace", Fact: "active_tab", Mode: commandcatalog.ExactVersion}}},
		ArgumentsSchema: &commandcatalog.Schema{Type: commandcatalog.SchemaObject, Properties: map[string]commandcatalog.Schema{
			"profile_slug": {Type: commandcatalog.SchemaString}, "trigger_type": {Type: commandcatalog.SchemaString}, "bring_to_front": {Type: commandcatalog.SchemaBoolean},
		}, Required: []string{"profile_slug", "trigger_type", "bring_to_front"}},
		ResultSchema: &commandcatalog.Schema{Type: commandcatalog.SchemaObject},
		// Only configured profile/trigger metadata, never audio or transcription.
		Risk: commandcatalog.RiskLow, Persistence: commandcatalog.PersistencePolicy{Arguments: commandcatalog.PersistenceRedacted, Result: commandcatalog.PersistenceNever, Audit: commandcatalog.PersistenceRedacted},
		Scopes: []commandcatalog.Scope{commandcatalog.ScopeSession}, Availability: commandcatalog.Availability{Status: commandcatalog.Available},
		HandlerRoute: "ui/voice/input/activate", HandlerClassification: commandcatalog.HandlerUI,
		Presentation: &commandcatalog.Presentation{Version: "global-voice-v1", Locales: map[string]commandcatalog.LocalizedMetadata{
			"pt-BR": {Name: "Acionar voz do perfil", Description: "Aciona a voz na superfície capturada do perfil ativo", Category: "Voz"},
			"en":    {Name: "Activate profile voice", Description: "Activates voice on the captured surface of the active profile", Category: "Voice"},
			"es":    {Name: "Activar voz del perfil", Description: "Activa la voz en la superficie capturada del perfil activo", Category: "Voz"},
		}},
	}
	job := commandcatalog.Definition{
		ID: commandGlobalJobID, Effect: commandcatalog.Destructive, HasMutableTarget: true, Decision: commandcatalog.Interactive,
		AllowedSources:  []commandcatalog.Source{commandcatalog.KeyboardGlobal},
		Context:         commandcatalog.ContextPolicy{Facts: []commandcatalog.ContextFact{{Provider: "workspace", Fact: "workspace", Mode: commandcatalog.ExactVersion}}},
		ArgumentsSchema: &commandcatalog.Schema{Type: commandcatalog.SchemaObject, Properties: map[string]commandcatalog.Schema{"job_id": {Type: commandcatalog.SchemaString}}, Required: []string{"job_id"}},
		ResultSchema: &commandcatalog.Schema{Type: commandcatalog.SchemaObject, Properties: map[string]commandcatalog.Schema{
			"job_id": {Type: commandcatalog.SchemaString}, "run_id": {Type: commandcatalog.SchemaString}, "status": {Type: commandcatalog.SchemaString},
		}, Required: []string{"job_id", "run_id", "status"}},
		Risk: commandcatalog.RiskHigh, Persistence: commandcatalog.PersistencePolicy{Arguments: commandcatalog.PersistenceRedacted, Result: commandcatalog.PersistenceRedacted, Audit: commandcatalog.PersistenceRedacted},
		Scopes: []commandcatalog.Scope{commandcatalog.ScopeSession}, Availability: commandcatalog.Availability{Status: commandcatalog.Available},
		HandlerRoute: "internal/jobs/run", HandlerClassification: commandcatalog.HandlerJob,
		Presentation: &commandcatalog.Presentation{Version: "global-job-v1", Locales: map[string]commandcatalog.LocalizedMetadata{
			"pt-BR": {Name: "Executar job", Description: "Executa o job configurado após confirmar a operação", Category: "Jobs"},
			"en":    {Name: "Run job", Description: "Runs the configured job after confirming the operation", Category: "Jobs"},
			"es":    {Name: "Ejecutar job", Description: "Ejecuta el job configurado tras confirmar la operación", Category: "Jobs"},
		}},
	}
	var result []commandcatalog.Registration
	for _, definition := range []commandcatalog.Definition{voice, job} {
		result = append(result, commandcatalog.Registration{Definition: definition, Handler: commandcatalog.HandlerContract{
			Effect: definition.Effect, HasMutableTarget: definition.HasMutableTarget, Classification: definition.HandlerClassification, Route: definition.HandlerRoute,
		}})
	}
	return result
}
