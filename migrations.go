package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"assistente/internal/database"

	"gopkg.in/yaml.v3"
)

// ==================== Memory & FAQ Migration ====================

// MigrateMemoryToFiles exporta memórias do banco para ~/.assistente/memory/core.md
// Retorna o número de memórias migradas
func (a *App) MigrateMemoryToFiles() (int, error) {
	memories, err := database.GetAllMemories()
	if err != nil {
		return 0, fmt.Errorf("erro ao buscar memórias: %w", err)
	}

	if len(memories) == 0 {
		log.Println("[Migration] Nenhuma memória para migrar")
		return 0, nil
	}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		return 0, fmt.Errorf("erro ao obter home dir: %w", err)
	}

	memoryDir := filepath.Join(homeDir, ".assistente", "memory")
	if err := os.MkdirAll(memoryDir, 0755); err != nil {
		return 0, fmt.Errorf("erro ao criar diretório: %w", err)
	}

	filePath := filepath.Join(memoryDir, "core.md")

	// Se o arquivo já existe, faz backup
	if _, err := os.Stat(filePath); err == nil {
		backupPath := filepath.Join(memoryDir, fmt.Sprintf("core.backup.%s.md", time.Now().Format("20060102_150405")))
		data, _ := os.ReadFile(filePath)
		os.WriteFile(backupPath, data, 0644)
		log.Printf("[Migration] Backup criado: %s", backupPath)
	}

	var sb strings.Builder
	sb.WriteString("# Core Memories\n\n")
	sb.WriteString("Informações importantes sobre o usuário e preferências.\n")
	sb.WriteString("O assistente pode ler e editar este arquivo para manter a memória atualizada.\n\n")

	// Agrupa por categoria
	categories := make(map[string][]database.Memory)
	for _, mem := range memories {
		cat := mem.Category
		if cat == "" {
			cat = "general"
		}
		categories[cat] = append(categories[cat], mem)
	}

	for cat, mems := range categories {
		sb.WriteString(fmt.Sprintf("## %s\n\n", strings.Title(cat)))
		for _, mem := range mems {
			if mem.Title != "" {
				sb.WriteString(fmt.Sprintf("- **%s**: %s\n", mem.Title, mem.Content))
			} else {
				sb.WriteString(fmt.Sprintf("- %s\n", mem.Content))
			}
		}
		sb.WriteString("\n")
	}

	if err := os.WriteFile(filePath, []byte(sb.String()), 0644); err != nil {
		return 0, fmt.Errorf("erro ao escrever arquivo: %w", err)
	}

	log.Printf("[Migration] %d memórias migradas para %s", len(memories), filePath)
	return len(memories), nil
}

// MigrateFAQToFiles exporta FAQs do banco para ~/.assistente/memory/faq.md
// Retorna o número de FAQs migradas
func (a *App) MigrateFAQToFiles() (int, error) {
	faqs, err := database.GetAllFAQs()
	if err != nil {
		return 0, fmt.Errorf("erro ao buscar FAQs: %w", err)
	}

	if len(faqs) == 0 {
		log.Println("[Migration] Nenhuma FAQ para migrar")
		return 0, nil
	}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		return 0, fmt.Errorf("erro ao obter home dir: %w", err)
	}

	memoryDir := filepath.Join(homeDir, ".assistente", "memory")
	if err := os.MkdirAll(memoryDir, 0755); err != nil {
		return 0, fmt.Errorf("erro ao criar diretório: %w", err)
	}

	filePath := filepath.Join(memoryDir, "faq.md")

	// Se o arquivo já existe, faz backup
	if _, err := os.Stat(filePath); err == nil {
		backupPath := filepath.Join(memoryDir, fmt.Sprintf("faq.backup.%s.md", time.Now().Format("20060102_150405")))
		data, _ := os.ReadFile(filePath)
		os.WriteFile(backupPath, data, 0644)
		log.Printf("[Migration] Backup criado: %s", backupPath)
	}

	var sb strings.Builder
	sb.WriteString("# FAQ - Perguntas Frequentes\n\n")
	sb.WriteString("Respostas padronizadas para perguntas comuns.\n")
	sb.WriteString("O assistente deve consultar este arquivo antes de responder perguntas recorrentes.\n\n")

	// Agrupa por tags
	tagged := make(map[string][]database.FAQ)
	for _, faq := range faqs {
		tag := faq.Tags
		if tag == "" {
			tag = "geral"
		}
		tagged[tag] = append(tagged[tag], faq)
	}

	for tag, items := range tagged {
		sb.WriteString(fmt.Sprintf("## %s\n\n", strings.Title(tag)))
		for _, faq := range items {
			sb.WriteString(fmt.Sprintf("### %s\n\n", faq.Question))
			sb.WriteString(faq.Answer)
			sb.WriteString("\n\n")
		}
	}

	if err := os.WriteFile(filePath, []byte(sb.String()), 0644); err != nil {
		return 0, fmt.Errorf("erro ao escrever arquivo: %w", err)
	}

	log.Printf("[Migration] %d FAQs migradas para %s", len(faqs), filePath)
	return len(faqs), nil
}

// RunMemoryMigration executa a migração completa de Memory + FAQ para arquivos
// Chamado automaticamente no startup se detectar dados no banco
func (a *App) RunMemoryMigration() error {
	memCount, err := a.MigrateMemoryToFiles()
	if err != nil {
		return fmt.Errorf("erro na migração de memórias: %w", err)
	}

	faqCount, err := a.MigrateFAQToFiles()
	if err != nil {
		return fmt.Errorf("erro na migração de FAQs: %w", err)
	}

	if memCount > 0 || faqCount > 0 {
		log.Printf("[Migration] Migração concluída: %d memórias, %d FAQs", memCount, faqCount)
	}

	// Cria a skill de memória se não existir
	if err := a.ensureMemorySkill(); err != nil {
		log.Printf("[Migration] Aviso: não foi possível criar skill de memória: %v", err)
	}

	return nil
}

// ensureMemorySkill cria a skill de memória se ela não existir
func (a *App) ensureMemorySkill() error {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return err
	}

	skillDir := filepath.Join(homeDir, ".assistente", "skills", "memory")
	skillPath := filepath.Join(skillDir, "SKILL.md")

	// Se já existe, não sobrescreve
	if _, err := os.Stat(skillPath); err == nil {
		return nil
	}

	if err := os.MkdirAll(skillDir, 0755); err != nil {
		return err
	}

	skillContent := `---
name: Memory Manager
description: Gerencia a memória de longo prazo do assistente (core memories e FAQ)
auto_load: true
tools: [file_read, file_write, file_append, file_replace]
---

# Memory Manager

Você tem acesso a memória de longo prazo através de arquivos Markdown.

## Arquivos de Memória

Os arquivos de memória ficam em ` + "`~/.assistente/memory/`" + `:

- **core.md** — Informações importantes sobre o usuário (nome, preferências, contexto)
- **faq.md** — Perguntas frequentes com respostas padronizadas

## Quando Salvar Memórias

Salve informações em core.md quando o usuário:
- Compartilhar informações pessoais (nome, profissão, preferências)
- Pedir para "lembrar" algo
- Corrigir algo que você disse errado sobre ele
- Definir preferências de como quer ser atendido

## Quando Consultar FAQ

Antes de responder perguntas que parecem recorrentes ou padronizadas:
1. Leia o arquivo faq.md
2. Se encontrar uma resposta relevante, use-a como base

## Como Editar

Use as tools ` + "`file_read`" + ` para ler e ` + "`file_replace`" + ` ou ` + "`file_append`" + ` para editar.

### Adicionar nova memória em core.md:
Use ` + "`file_append`" + ` para adicionar um novo item na categoria apropriada.

### Adicionar nova FAQ:
Use ` + "`file_append`" + ` para adicionar uma nova seção Q&A no faq.md.

### Atualizar memória existente:
Use ` + "`file_replace`" + ` para substituir o texto antigo pelo novo.
`

	return os.WriteFile(skillPath, []byte(skillContent), 0644)
}

// ==================== Profile Migration (Fase 4) ====================

// UnifiedProfile representa o profile unificado em YAML
type UnifiedProfile struct {
	Name        string              `yaml:"name"`
	Description string              `yaml:"description,omitempty"`
	Icon        string              `yaml:"icon,omitempty"`
	Chat        ProfileChatSection  `yaml:"chat"`
	Voice       ProfileVoiceSection `yaml:"voice,omitempty"`
	Interaction ProfileInteractionSection `yaml:"interaction,omitempty"`
}

// ProfileChatSection contém configurações de chat/modelo
type ProfileChatSection struct {
	Model               string   `yaml:"model,omitempty"`
	Temperature         float64  `yaml:"temperature,omitempty"`
	MaxTokens           int      `yaml:"max_tokens,omitempty"`
	TopP                float64  `yaml:"top_p,omitempty"`
	ResponseTimeout     int      `yaml:"response_timeout,omitempty"`
	EnableThinking      bool     `yaml:"enable_thinking,omitempty"`
	UseTools            bool     `yaml:"use_tools"`
	ToolsList           []string `yaml:"tools_list,omitempty"`
	SystemPrompt        string   `yaml:"system_prompt,omitempty"`
	SystemPromptPosition string  `yaml:"system_prompt_position,omitempty"`
	IncludeCoreMemories bool     `yaml:"include_core_memories"`
	EmbeddingsModel     string   `yaml:"embeddings_model,omitempty"`
	EmbeddingsDimensions int     `yaml:"embeddings_dimensions,omitempty"`
	ImageModel          string   `yaml:"image_model,omitempty"`
	ShowInternalMessages bool    `yaml:"show_internal_messages,omitempty"`
}

// ProfileVoiceSection contém configurações de TTS
type ProfileVoiceSection struct {
	Provider        string  `yaml:"provider,omitempty"`
	VoiceID         string  `yaml:"voice_id,omitempty"`
	Rate            float64 `yaml:"rate,omitempty"`
	Pitch           float64 `yaml:"pitch,omitempty"`
	Volume          float64 `yaml:"volume,omitempty"`
	EnabledForAgent bool    `yaml:"enabled_for_agent,omitempty"`
	EnabledForUser  bool    `yaml:"enabled_for_user,omitempty"`
}

// ProfileInteractionSection contém configurações de interação por voz
type ProfileInteractionSection struct {
	STTProvider    string           `yaml:"stt_provider,omitempty"`
	Language       string           `yaml:"language,omitempty"`
	FeedbackSounds bool            `yaml:"feedback_sounds,omitempty"`
	Triggers       []ProfileTrigger `yaml:"triggers,omitempty"`
}

// ProfileTrigger representa um trigger de interação
type ProfileTrigger struct {
	Type                string  `yaml:"type"`
	Enabled             bool    `yaml:"enabled"`
	AutoStop            bool    `yaml:"auto_stop,omitempty"`
	Hotkey              string  `yaml:"hotkey,omitempty"`
	HotkeyGlobal        bool   `yaml:"hotkey_global,omitempty"`
	HotkeyBringToFront   bool   `yaml:"hotkey_bring_to_front,omitempty"`
	WakewordKeyword     string  `yaml:"wakeword_keyword,omitempty"`
	WakewordSensitivity float64 `yaml:"wakeword_sensitivity,omitempty"`
	VADSilenceThreshold float64 `yaml:"vad_silence_threshold,omitempty"`
	VADSilenceDuration  int     `yaml:"vad_silence_duration,omitempty"`
}

// MigrateProfilesToFiles exporta profiles do banco para ~/.assistente/profiles/*.yaml
func (a *App) MigrateProfilesToFiles() (int, error) {
	chatProfiles, err := database.GetAllChatProfiles()
	if err != nil {
		return 0, fmt.Errorf("erro ao buscar chat profiles: %w", err)
	}

	if len(chatProfiles) == 0 {
		log.Println("[Migration] Nenhum chat profile para migrar")
		return 0, nil
	}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		return 0, fmt.Errorf("erro ao obter home dir: %w", err)
	}

	profilesDir := filepath.Join(homeDir, ".assistente", "profiles")
	if err := os.MkdirAll(profilesDir, 0755); err != nil {
		return 0, fmt.Errorf("erro ao criar diretório: %w", err)
	}

	// Busca voice e interaction profiles default para preencher o profile unificado default
	defaultVoice, _ := database.GetDefaultVoiceProfile()
	defaultInteraction, _ := database.GetDefaultInteractionProfile()

	count := 0
	for _, cp := range chatProfiles {
		profile := UnifiedProfile{
			Name:        cp.Name,
			Description: cp.Description,
			Icon:        cp.Icon,
			Chat: ProfileChatSection{
				Model:               cp.Model,
				Temperature:         cp.Temperature,
				MaxTokens:           cp.MaxTokens,
				TopP:                cp.TopP,
				ResponseTimeout:     cp.ResponseTimeout,
				EnableThinking:      cp.EnableThinking,
				UseTools:            cp.UseTools,
				SystemPrompt:        cp.SystemPrompt,
				SystemPromptPosition: cp.SystemPromptPosition,
				IncludeCoreMemories: cp.IncludeCoreMemories,
				EmbeddingsModel:     cp.EmbeddingsModel,
				EmbeddingsDimensions: cp.EmbeddingsDimensions,
				ImageModel:          cp.ImageModel,
				ShowInternalMessages: cp.ShowInternalMessages,
			},
		}

		// Preenche tools_list
		if cp.ToolsList != "" {
			var tools []string
			if err := json.Unmarshal([]byte(cp.ToolsList), &tools); err == nil {
				profile.Chat.ToolsList = tools
			}
		}

		// Para o profile default, inclui voice e interaction
		if cp.IsDefault {
			if defaultVoice != nil {
				profile.Voice = ProfileVoiceSection{
					Provider:        defaultVoice.Provider,
					VoiceID:         defaultVoice.VoiceID,
					Rate:            defaultVoice.Rate,
					Pitch:           defaultVoice.Pitch,
					Volume:          defaultVoice.Volume,
					EnabledForAgent: defaultVoice.EnabledForAgent,
					EnabledForUser:  defaultVoice.EnabledForUser,
				}
			}

			if defaultInteraction != nil {
				profile.Interaction = ProfileInteractionSection{
					STTProvider:    defaultInteraction.STTProvider,
					Language:       defaultInteraction.Language,
					FeedbackSounds: defaultInteraction.FeedbackSounds,
				}
				for _, t := range defaultInteraction.Triggers {
					profile.Interaction.Triggers = append(profile.Interaction.Triggers, ProfileTrigger{
						Type:                t.Type,
						Enabled:             t.Enabled,
						AutoStop:            t.AutoStop,
						Hotkey:              t.Hotkey,
						HotkeyGlobal:        t.HotkeyGlobal,
						HotkeyBringToFront:  t.HotkeyBringToFront,
						WakewordKeyword:     t.WakewordKeyword,
						WakewordSensitivity: t.WakewordSensitivity,
						VADSilenceThreshold: t.VADSilenceThreshold,
						VADSilenceDuration:  t.VADSilenceDuration,
					})
				}
			}
		}

		// Gera slug para nome do arquivo
		slug := strings.ToLower(strings.ReplaceAll(cp.Name, " ", "-"))
		slug = strings.Map(func(r rune) rune {
			if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
				return r
			}
			return '-'
		}, slug)

		filePath := filepath.Join(profilesDir, slug+".yaml")

		// Se já existe, não sobrescreve
		if _, err := os.Stat(filePath); err == nil {
			log.Printf("[Migration] Profile %s já existe, pulando", slug)
			continue
		}

		data, err := yaml.Marshal(profile)
		if err != nil {
			log.Printf("[Migration] Erro ao serializar profile %s: %v", cp.Name, err)
			continue
		}

		if err := os.WriteFile(filePath, data, 0644); err != nil {
			log.Printf("[Migration] Erro ao escrever profile %s: %v", cp.Name, err)
			continue
		}

		count++
		log.Printf("[Migration] Profile migrado: %s → %s", cp.Name, filePath)
	}

	return count, nil
}
