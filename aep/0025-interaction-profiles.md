# Arquitetura de Perfis de Interação por Voz (v2)

**Status:** Superseded

> **Nota histórica:** os tipos `InteractionProfile` e `InteractionTrigger`
> descritos abaixo não existem no contrato vigente. A arquitetura foi
> substituída por `profiles.Profile`, com `InputConfig` e `TriggerConfig`,
> consolidada pela AEP-0038. O restante deste documento preserva o desenho
> anterior apenas para rastreabilidade.

## Visão Geral

O sistema de Perfis de Interação permite configurar diferentes modos de interação por voz com o assistente. A arquitetura foi redesenhada para ser mais flexível, separando **perfis** (configurações comuns) de **triggers** (formas de ativação).

Um perfil pode ter **múltiplos triggers**, permitindo por exemplo ativar o mesmo perfil por hotkey, wake word ou botão na interface.

```
┌─────────────────────────────────────────────────────────────────────┐
│                     InteractionProfile                               │
│                                                                      │
│  ┌─────────────────────────────────────────────────────────────┐   │
│  │  Configurações Comuns                                        │   │
│  │  • Nome, Descrição, Padrão                                   │   │
│  │  • STT Provider (WebSpeech, Whisper, Vosk)                   │   │
│  │  • Idioma (pt-BR, en-US)                                     │   │
│  │  • Sons de Feedback                                          │   │
│  └─────────────────────────────────────────────────────────────┘   │
│                                                                      │
│  ┌─────────────────────────────────────────────────────────────┐   │
│  │  Triggers (1 ou mais)                                        │   │
│  │                                                              │   │
│  │  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐      │   │
│  │  │   Hotkey     │  │  Wakeword    │  │   Button     │      │   │
│  │  │ Ctrl+Shift+A │  │ "assistente" │  │   Toggle     │      │   │
│  │  │   Toggle     │  │  VAD stop    │  │              │      │   │
│  │  └──────────────┘  └──────────────┘  └──────────────┘      │   │
│  └─────────────────────────────────────────────────────────────┘   │
│                                                                      │
└─────────────────────────────────────────────────────────────────────┘
```

---

## Modelo de Dados

### InteractionProfile

Configurações comuns compartilhadas por todos os triggers do perfil.

| Campo | Tipo | Null | Default | Descrição |
|-------|------|------|---------|-----------|
| `id` | uint | N | auto | PK |
| `created_at` | datetime | N | now | |
| `updated_at` | datetime | N | now | |
| `name` | string | N | | Nome do perfil |
| `description` | string | S | "" | Descrição |
| `is_default` | bool | N | false | Perfil padrão |
| `stt_provider` | string | N | "webspeech" | Provider STT: webspeech, whisper_api, vosk |
| `language` | string | N | "pt-BR" | Idioma do reconhecimento |
| `feedback_sounds` | bool | N | true | Sons de início/fim de gravação |

### InteractionTrigger

Cada trigger define UMA forma de ativar o perfil. Um perfil pode ter N triggers.

| Campo | Tipo | Null | Default | Descrição |
|-------|------|------|---------|-----------|
| `id` | uint | N | auto | PK |
| `created_at` | datetime | N | now | |
| `updated_at` | datetime | N | now | |
| `profile_id` | uint | N | | FK → InteractionProfile |
| `type` | string | N | | Tipo: hotkey, button_ptt, button_toggle, wakeword, vad |
| `enabled` | bool | N | true | Trigger ativo |
| `auto_stop` | bool | N | false | true=para com VAD, false=para manual (só hotkey/button_toggle) |
| **Hotkey (comum para type=hotkey ou toggle de wakeword/vad)** |||||
| `hotkey` | string | S | "" | Combinação de tecla (ex: Ctrl+Shift+A) |
| `hotkey_global` | bool | N | true | true=funciona em qualquer app, false=só com janela focada |
| `hotkey_bring_to_front` | bool | N | true | Trazer janela para frente (só se global) |
| **Wakeword (type=wakeword)** |||||
| `wakeword_keyword` | string | S | "" | Palavra de ativação (ex: assistente) |
| `wakeword_provider` | string | S | "vosk" | Provider: vosk, webspeech |
| `wakeword_sensitivity` | float | N | 0.5 | Sensibilidade 0.0 - 1.0 |
| **VAD (quando auto_stop=true, type=wakeword ou type=vad)** |||||
| `vad_silence_threshold` | float | N | 0.01 | Limiar de silêncio |
| `vad_silence_duration` | int | N | 1500 | Duração do silêncio para parar (ms) |
| `vad_activity_threshold` | float | N | 0.02 | Limiar de atividade |
| `vad_activity_duration` | int | N | 200 | Duração da atividade para iniciar (ms) |

---

## Tipos de Trigger

### Comportamento por Tipo

| type | Como INICIA | Como TERMINA | Campo `hotkey` serve para |
|------|-------------|--------------|---------------------------|
| `hotkey` | Pressiona tecla (toggle) | Pressiona tecla novamente OU VAD (se auto_stop) | Tecla que aciona gravação |
| `button_ptt` | Segura botão | Solta botão | - |
| `button_toggle` | Clica botão | Clica novamente OU VAD (se auto_stop) | - |
| `wakeword` | Fala a palavra | VAD (sempre) | Tecla para ligar/desligar escuta |
| `vad` | Detecta voz automaticamente | VAD (sempre) | Tecla para ligar/desligar escuta |

### Uso do Campo `hotkey`

O campo `hotkey` tem significados diferentes dependendo do tipo:

- **type=hotkey**: A tecla que **aciona** a gravação (toggle)
- **type=wakeword**: A tecla que **liga/desliga** a escuta do wakeword
- **type=vad**: A tecla que **liga/desliga** a escuta contínua
- **type=button_***: Não usa (vazio)

### Exemplos de Configuração

**Perfil "Conversa Rápida":**
```
InteractionProfile:
  name: "Conversa Rápida"
  stt_provider: "whisper_api"
  language: "pt-BR"
  feedback_sounds: true

InteractionTriggers:
  1. type: "hotkey"
     hotkey: "Ctrl+Shift+Space"
     hotkey_global: true
     hotkey_bring_to_front: true
     auto_stop: true (para com VAD)
     
  2. type: "wakeword"
     wakeword_keyword: "assistente"
     wakeword_provider: "vosk"
     hotkey: "Ctrl+W" (liga/desliga wakeword)
     hotkey_global: true
     
  3. type: "button_toggle"
     auto_stop: true
```

**Perfil "Ditado Longo":**
```
InteractionProfile:
  name: "Ditado Longo"
  stt_provider: "webspeech"
  feedback_sounds: true

InteractionTriggers:
  1. type: "hotkey"
     hotkey: "Ctrl+D"
     hotkey_global: true
     auto_stop: false (para manualmente)
     
  2. type: "button_ptt"
```

---

## Relacionamento entre Entidades

```
┌────────────────────────┐
│   InteractionProfile   │
├────────────────────────┤
│ id                     │
│ name                   │
│ description            │
│ is_default             │
│ stt_provider           │
│ language               │
│ feedback_sounds        │
└───────────┬────────────┘
            │
            │ 1:N
            │
            ▼
┌────────────────────────┐
│   InteractionTrigger   │
├────────────────────────┤
│ id                     │
│ profile_id (FK)        │
│ type                   │
│ enabled                │
│ auto_stop              │
│ hotkey                 │
│ hotkey_global          │
│ hotkey_bring_to_front  │
│ wakeword_keyword       │
│ wakeword_provider      │
│ wakeword_sensitivity   │
│ vad_*                  │
└────────────────────────┘
```

---

## Fluxo de Ativação

### Hotkey

```
1. Usuário pressiona Ctrl+Shift+Space
2. Backend detecta hotkey → emite evento
3. Frontend recebe evento
4. Verifica auto_stop:
   - false: Toggle manual (pressiona de novo para parar)
   - true: Inicia VAD, para automaticamente no silêncio
5. Áudio enviado para STT provider do perfil
6. Transcrição enviada para o chat
```

### Wakeword

```
1. Usuário ativa trigger (ou perfil já está ativo)
2. Vosk/WebSpeech escuta continuamente
3. Detecta palavra "assistente"
4. Inicia gravação com VAD
5. VAD detecta silêncio → para gravação
6. Áudio enviado para STT provider do perfil
7. Volta a escutar wakeword
```

### Button PTT

```
1. Usuário pressiona botão (mousedown/touchstart)
2. Inicia gravação
3. Usuário solta botão (mouseup/touchend)
4. Para gravação
5. Áudio enviado para STT provider do perfil
```

### Button Toggle

```
1. Usuário clica botão
2. Inicia gravação
3. Verifica auto_stop:
   - false: Aguarda outro clique para parar
   - true: VAD para automaticamente no silêncio
4. Áudio enviado para STT provider do perfil
```

### VAD Contínuo

```
1. Usuário ativa trigger (hotkey de toggle)
2. Sistema escuta continuamente
3. VAD detecta atividade de voz → inicia gravação
4. VAD detecta silêncio → para gravação
5. Áudio enviado para STT provider do perfil
6. Volta a escutar (loop)
```

---

## API Backend (Go)

### InteractionProfile CRUD

```go
func (a *App) GetInteractionProfiles() ([]database.InteractionProfile, error)
func (a *App) GetInteractionProfile(id uint) (*database.InteractionProfile, error)
func (a *App) CreateInteractionProfile(profile database.InteractionProfile) (*database.InteractionProfile, error)
func (a *App) UpdateInteractionProfile(id uint, profile database.InteractionProfile) (*database.InteractionProfile, error)
func (a *App) DeleteInteractionProfile(id uint) error
func (a *App) SetDefaultInteractionProfile(id uint) error
```

### InteractionTrigger CRUD

```go
func (a *App) GetTriggersByProfile(profileId uint) ([]database.InteractionTrigger, error)
func (a *App) GetInteractionTrigger(id uint) (*database.InteractionTrigger, error)
func (a *App) CreateInteractionTrigger(trigger database.InteractionTrigger) (*database.InteractionTrigger, error)
func (a *App) UpdateInteractionTrigger(id uint, trigger database.InteractionTrigger) (*database.InteractionTrigger, error)
func (a *App) DeleteInteractionTrigger(id uint) error
```

### Eventos Wails

```go
// Hotkey pressionada
runtime.EventsEmit(ctx, "interaction:hotkey:triggered", map[string]interface{}{
    "triggerId":     123,
    "profileId":     456,
    "combination":   "Ctrl+Shift+Space",
    "bringToFront":  true,
})

// Wakeword detectado
runtime.EventsEmit(ctx, "interaction:wakeword:detected", map[string]interface{}{
    "triggerId":  123,
    "profileId":  456,
    "keyword":    "assistente",
})

// Trigger toggle (wakeword/vad ligado/desligado)
runtime.EventsEmit(ctx, "interaction:trigger:toggled", map[string]interface{}{
    "triggerId":  123,
    "enabled":    true,
})
```

---

## Interface do Usuário

### Página de Perfis de Interação

```
┌─────────────────────────────────────────────────────────────────────┐
│  Perfis de Interação                                    [+ Novo]    │
├─────────────────────────────────────────────────────────────────────┤
│                                                                      │
│  ┌─────────────────────────────────────────────────────────────┐   │
│  │ ⭐ Conversa Rápida                              STT: Whisper │   │
│  │    Triggers: Hotkey (Ctrl+Shift+Space), Wakeword, Button     │   │
│  │                                        [Editar] [Excluir]    │   │
│  └─────────────────────────────────────────────────────────────┘   │
│                                                                      │
│  ┌─────────────────────────────────────────────────────────────┐   │
│  │   Ditado Longo                                 STT: WebSpeech │   │
│  │   Triggers: Hotkey (Ctrl+D), Button PTT                      │   │
│  │                                        [Editar] [Excluir]    │   │
│  └─────────────────────────────────────────────────────────────┘   │
│                                                                      │
└─────────────────────────────────────────────────────────────────────┘
```

### Modal de Edição de Perfil

```
┌─────────────────────────────────────────────────────────────────────┐
│  Editar Perfil                                                [X]   │
├─────────────────────────────────────────────────────────────────────┤
│                                                                      │
│  Nome: [Conversa Rápida                                      ]     │
│  Descrição: [Perfil para conversas rápidas com o assistente  ]     │
│                                                                      │
│  ═══════════════════════════════════════════════════════════════    │
│  CONFIGURAÇÕES COMUNS                                               │
│  ═══════════════════════════════════════════════════════════════    │
│                                                                      │
│  Provider STT: [Whisper API                              ▼]        │
│  Idioma:       [Português (Brasil)                       ▼]        │
│  [✓] Sons de feedback (início/fim de gravação)                      │
│  [✓] Definir como perfil padrão                                     │
│                                                                      │
│  ═══════════════════════════════════════════════════════════════    │
│  TRIGGERS                                              [+ Adicionar] │
│  ═══════════════════════════════════════════════════════════════    │
│                                                                      │
│  ┌───────────────────────────────────────────────────────────────┐ │
│  │ 🔤 Hotkey: Ctrl+Shift+Space                          [✓] [🗑️] │ │
│  │    Global, traz janela, para com VAD                          │ │
│  └───────────────────────────────────────────────────────────────┘ │
│                                                                      │
│  ┌───────────────────────────────────────────────────────────────┐ │
│  │ 🎤 Wakeword: "assistente"                            [✓] [🗑️] │ │
│  │    Vosk, toggle: Ctrl+W                                       │ │
│  └───────────────────────────────────────────────────────────────┘ │
│                                                                      │
│  ┌───────────────────────────────────────────────────────────────┐ │
│  │ 🖱️ Button Toggle                                     [✓] [🗑️] │ │
│  │    Para com VAD                                               │ │
│  └───────────────────────────────────────────────────────────────┘ │
│                                                                      │
│                                          [Cancelar] [Salvar]        │
└─────────────────────────────────────────────────────────────────────┘
```

### Modal de Adicionar Trigger

```
┌─────────────────────────────────────────────────────────────────────┐
│  Adicionar Trigger                                            [X]   │
├─────────────────────────────────────────────────────────────────────┤
│                                                                      │
│  Tipo: ( ) Hotkey - Atalho de teclado (toggle)                      │
│        ( ) Button PTT - Segura para gravar                          │
│        ( ) Button Toggle - Clica para alternar                      │
│        ( ) Wakeword - Palavra de ativação                           │
│        ( ) VAD - Detecção contínua de voz                           │
│                                                                      │
│  ─────────────────────────────────────────────────────────────────  │
│  (Campos específicos aparecem baseado no tipo selecionado)          │
│  ─────────────────────────────────────────────────────────────────  │
│                                                                      │
│                                          [Cancelar] [Adicionar]     │
└─────────────────────────────────────────────────────────────────────┘
```

---

## Considerações Técnicas

### Performance

| Componente | CPU | Memória | Nota |
|------------|-----|---------|------|
| VAD (frontend) | ~2% | ~10MB | Apenas quando gravando |
| Vosk wakeword | ~5% | ~50MB | Contínuo quando ativo |
| WebSpeech STT | ~2% | ~5MB | Usa API do browser |
| Whisper API | N/A | N/A | Processamento remoto |

### Privacidade

| Provider | Dados enviados |
|----------|----------------|
| Vosk | Nenhum (100% local) |
| WebSpeech | Áudio para Google |
| Whisper API | Áudio para OpenAI |

---

## Migração da Versão Anterior

A versão anterior tinha `activation_mode` como campo único (manual/hotkey/wakeword). A nova versão separa em triggers independentes, permitindo múltiplas formas de ativação no mesmo perfil.

### Dados a Migrar

| Campo Antigo | Novo Local |
|--------------|------------|
| `activation_mode` | Removido (N triggers por perfil) |
| `hotkey_combination` | `InteractionTrigger.hotkey` |
| `hotkey_bring_to_front` | `InteractionTrigger.hotkey_bring_to_front` |
| `secondary_hotkey_*` | Segundo trigger do tipo `hotkey` |
| `wakeword_*` | Trigger do tipo `wakeword` |
| `recording_mode` | `InteractionTrigger.type` + `auto_stop` |
| `vad_*` | `InteractionTrigger.vad_*` |
| `auto_tts` | Removido (usa VoiceProfile da conversa) |
| `tts_profile_id` | Removido (usa VoiceProfile da conversa) |

---

## Referências

- [SPEECH_ARCHITECTURE.md](./SPEECH_ARCHITECTURE.md) - Arquitetura de voz
- [Vosk API](https://alphacephei.com/vosk/) - STT offline
- [Web Speech API](https://developer.mozilla.org/en-US/docs/Web/API/Web_Speech_API) - STT browser
- [OpenAI Whisper API](https://platform.openai.com/docs/guides/speech-to-text) - STT cloud
