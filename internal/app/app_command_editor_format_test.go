package app

import (
	"errors"
	"strings"
	"testing"

	"assistente/internal/commandcatalog"
	"assistente/internal/commandexecution"
)

func TestEditorFormatCommandsUseAuditedUIContracts(t *testing.T) {
	a := readyCommandProduct(t)
	registry, handlers, err := a.commandProductCatalog()
	if err != nil {
		t.Fatal(err)
	}
	wantSlideNames := map[string][3]string{
		commandEditorSlideInsertBasicID:      {"Inserir slide básico", "Insert basic slide", "Insertar diapositiva básica"},
		commandEditorSlideInsertTitleID:      {"Inserir slide de título", "Insert title slide", "Insertar diapositiva de título"},
		commandEditorSlideInsertTwoColumnsID: {"Inserir slide de duas colunas", "Insert two-column slide", "Insertar diapositiva de dos columnas"},
		commandEditorSlideInsertImageRightID: {"Inserir slide com imagem à direita", "Insert slide with image on right", "Insertar diapositiva con imagen a la derecha"},
		commandEditorSlideInsertImageLeftID:  {"Inserir slide com imagem à esquerda", "Insert slide with image on left", "Insertar diapositiva con imagen a la izquierda"},
		commandEditorSlideInsertSectionID:    {"Inserir slide de seção", "Insert section slide", "Insertar diapositiva de sección"},
		commandEditorSlideInsertAgendaID:     {"Inserir slide de agenda", "Insert agenda slide", "Insertar diapositiva de agenda"},
		commandEditorSlideInsertQuoteID:      {"Inserir slide de citação", "Insert quote slide", "Insertar diapositiva de cita"},
		commandEditorSlideInsertComparisonID: {"Inserir slide de comparação", "Insert comparison slide", "Insertar diapositiva de comparación"},
		commandEditorSlideInsertCodeID:       {"Inserir slide de código", "Insert code slide", "Insertar diapositiva de código"},
		commandEditorSlideInsertDiagramID:    {"Inserir slide de diagrama", "Insert diagram slide", "Insertar diapositiva de diagrama"},
	}
	wantMarkdownDescriptions := map[string][3]string{
		commandEditorFormatTableInsertID:     {"Aplica formatação ao conteúdo do editor ativo, inclusive em Markdown", "Applies formatting to active editor content, including Markdown", "Aplica formato al contenido del editor activo, incluido Markdown"},
		commandEditorFormatCodeBlockInsertID: {"Aplica formatação ao conteúdo do editor ativo, inclusive em Markdown", "Applies formatting to active editor content, including Markdown", "Aplica formato al contenido del editor activo, incluido Markdown"},
		commandEditorFormatMermaidInsertID:   {"Aplica formatação ao conteúdo do editor ativo, inclusive em Markdown", "Applies formatting to active editor content, including Markdown", "Aplica formato al contenido del editor activo, incluido Markdown"},
		commandEditorFormatListBulletID:      {"Aplica formatação ao conteúdo do editor ativo, inclusive em Markdown", "Applies formatting to active editor content, including Markdown", "Aplica formato al contenido del editor activo, incluido Markdown"},
		commandEditorFormatListOrderedID:     {"Aplica formatação ao conteúdo do editor ativo, inclusive em Markdown", "Applies formatting to active editor content, including Markdown", "Aplica formato al contenido del editor activo, incluido Markdown"},
		commandEditorFormatBlockquoteID:      {"Aplica formatação ao conteúdo do editor ativo, inclusive em Markdown", "Applies formatting to active editor content, including Markdown", "Aplica formato al contenido del editor activo, incluido Markdown"},
	}
	for _, id := range commandEditorFormatIDs {
		definition, ok := registry.Lookup(id)
		if !ok {
			t.Fatalf("comando ausente: %s", id)
		}
		if definition.Effect != commandcatalog.Write || definition.Decision != commandcatalog.NoDecision || !definition.HasMutableTarget || definition.HandlerClassification != commandcatalog.HandlerUI || commandExecutionClassForDefinition(definition) != commandExecutionAuditedUI {
			t.Fatalf("contrato de %s inválido: %+v", id, definition)
		}
		if len(definition.Context.Facts) != 1 || definition.Context.Facts[0].Provider != "workspace" || definition.Context.Facts[0].Fact != "active_tab" || definition.Context.Facts[0].Mode != commandcatalog.ExactVersion {
			t.Fatalf("contexto de %s inválido: %+v", id, definition.Context)
		}
		if definition.ArgumentsSchema.Type != commandcatalog.SchemaObject || definition.ResultSchema.Type != commandcatalog.SchemaObject || definition.Persistence.Arguments != commandcatalog.PersistenceNever || definition.Persistence.Result != commandcatalog.PersistenceNever {
			t.Fatalf("persistência/schema de %s permitem conteúdo: %+v", id, definition)
		}
		if handlers[id].Start == nil || handlers[id].Contract.Classification != commandcatalog.HandlerUI {
			t.Fatalf("handler UI ausente para %s: %+v", id, handlers[id])
		}
		if definition.Presentation == nil || len(definition.Presentation.Locales) != 3 {
			t.Fatalf("presentation incompleta para %s: %+v", id, definition.Presentation)
		}
		wantRoute := "ui/editor/format/" + strings.TrimPrefix(id, "editor.format.")
		wantVersion, wantCategory := "editor-format-v2", "Editor"
		if isMermaidMutation(id) {
			wantRoute = "ui/editor/mermaid/" + strings.TrimPrefix(id, "editor.mermaid.")
			wantVersion = "editor-mermaid-v1"
		}
		if strings.HasPrefix(id, "editor.slide.insert.") {
			wantRoute = "ui/editor/slide/insert/" + strings.TrimPrefix(id, "editor.slide.insert.")
			wantVersion, wantCategory = "editor-slide-v1", "Slides"
		}
		if definition.HandlerRoute != wantRoute || handlers[id].Contract.Route != wantRoute || definition.Presentation.Version != wantVersion {
			t.Fatalf("rota/versão de %s: definition=%q handler=%q version=%q", id, definition.HandlerRoute, handlers[id].Contract.Route, definition.Presentation.Version)
		}
		for locale, metadata := range definition.Presentation.Locales {
			if metadata.Name == "" || metadata.Description == "" || metadata.Category != wantCategory {
				t.Fatalf("metadata incompleto em %s/%s: %+v", id, locale, metadata)
			}
		}
		if names, ok := wantSlideNames[id]; ok {
			for i, locale := range []string{"pt-BR", "en", "es"} {
				if got := definition.Presentation.Locales[locale].Name; got != names[i] {
					t.Fatalf("label %s/%s = %q, esperado %q", id, locale, got, names[i])
				}
			}
		}
		if descriptions, ok := wantMarkdownDescriptions[id]; ok {
			for i, locale := range []string{"pt-BR", "en", "es"} {
				if got := definition.Presentation.Locales[locale].Description; got != descriptions[i] {
					t.Fatalf("descrição Markdown %s/%s = %q, esperada %q", id, locale, got, descriptions[i])
				}
			}
		}
		if !commandDeckUICommand(id) || !localKeyboardCommandAllowed(id) {
			t.Fatalf("origens Deck/teclado não aceitaram %s", id)
		}
	}
}

func TestEditorFormatBeginTakeCompleteAndRepeatDenied(t *testing.T) {
	for _, commandID := range commandEditorFormatIDs {
		t.Run(commandID, func(t *testing.T) {
			a := readyCommandProduct(t)
			reservation := beginUICommand(t, a, commandID)
			waitCommandUIAdmission(t, a, reservation)
			handoff := takeUICommandFor(t, a, reservation.Ticket, commandID)
			if _, err := a.TakeUICommand(reservation.Ticket); err == nil {
				t.Fatal("Take repetido liberou o mesmo handoff")
			}
			if err := a.CompleteUICommand(reservation.Ticket, handoff.HandoffID, "succeeded"); err != nil {
				t.Fatalf("CompleteUICommand: %v", err)
			}
			result := getUIResultEventually(t, a, reservation.Ticket)
			if result.Status != "succeeded" {
				t.Fatalf("resultado de formatação: %+v", result)
			}
			assertUICommandLedgerStatus(t, reservation.InvocationID, "succeeded")
		})
	}

	a := readyCommandProduct(t)

	view, err := a.GetLocalCommandKeyboardMap()
	if err != nil {
		t.Fatal(err)
	}
	binding := commandKeyboardBindingFor(t, view, LocalCommandShortcut{Version: 1, Code: "KeyB", Modifiers: []string{"Control"}})
	if binding.CommandID != commandEditorFormatBoldID || binding.Handler != "ui" {
		t.Fatalf("Ctrl+B não foi projetado como UI auditada: %+v", binding)
	}
	first, err := a.BeginLocalCommandUIKey(view.Generation, binding.Shortcut, false)
	if err != nil || first == nil {
		t.Fatalf("Begin Ctrl+B: reservation=%+v err=%v", first, err)
	}
	if repeat, err := a.BeginLocalCommandUIKey(view.Generation, binding.Shortcut, true); !errors.Is(err, commandexecution.ErrDenied) || repeat != nil {
		t.Fatalf("repeat Ctrl+B foi aceito: reservation=%+v err=%v", repeat, err)
	}
	keyboardHandoff := takeUICommandFor(t, a, first.Ticket, commandEditorFormatBoldID)
	if err := a.CompleteUICommand(first.Ticket, keyboardHandoff.HandoffID, "cancelled"); err != nil {
		t.Fatalf("cancelamento Ctrl+B: %v", err)
	}
	if result := getUIResultEventually(t, a, first.Ticket); result.Status != "cancelled" {
		t.Fatalf("resultado Ctrl+B = %+v", result)
	}
}
