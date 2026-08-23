# Arquitetura de Voz (TTS/STT)

**Status:** Done

## Visão Geral

O sistema de voz permite interação por áudio com o assistente, suportando múltiplos provedores de TTS (Text-to-Speech) e STT (Speech-to-Text), incluindo APIs de nuvem, Web Speech API do navegador, SAPI5 do Windows e modelos multimodais.

```
┌─────────────────────────────────────────────────────────────────┐
│                    CAMADA 1: INTERFACE                          │
│                                                                 │
│  ┌─────────────┐  ┌─────────────┐  ┌─────────────────────────┐  │
│  │    PTT      │  │  Wake Word  │  │    Voice Activity       │  │
│  │ Push-to-Talk│  │ (Porcupine) │  │   Detection (VAD)       │  │
│  │  🟢 Fase 1  │  │  🔮 Fase 4  │  │   🔮 Fase 4             │  │
│  └──────┬──────┘  └──────┬──────┘  └───────────┬─────────────┘  │
│         └────────────────┴─────────────────────┘                │
│                          │ áudio                                │
└──────────────────────────┼──────────────────────────────────────┘
                           ▼
┌─────────────────────────────────────────────────────────────────┐
│                    CAMADA 2: SPEECH AGENT                       │
│                                                                 │
│  ┌───────────────────────────────────────────────────────────┐  │
│  │                    SpeechService                          │  │
│  │                                                           │  │
│  │  STT Provider:  [WebSpeech | Whisper | Azure | Google]   │  │
│  │  TTS Provider:  [WebSpeech | SAPI5 | OpenAI | Azure]     │  │
│  │  Multimodal:    [GPT-4o Realtime | Gemini 2.0]           │  │
│  │                                                           │  │
│  └───────────────────────────────────────────────────────────┘  │
│                                                                 │
└─────────────────────────────────────────────────────────────────┘
                           │
                           ▼
┌─────────────────────────────────────────────────────────────────┐
│                    CAMADA 3: PROVEDORES                         │
│                                                                 │
│  ┌─────────────────────────────────────────────────────────────┐│
│  │ STT Providers (Speech-to-Text)                              ││
│  │ ┌───────────┐ ┌───────────┐ ┌───────────┐ ┌───────────────┐ ││
│  │ │ WebSpeech │ │  Whisper  │ │  Azure    │ │ Google Cloud  │ ││
│  │ │ (Browser) │ │  (OpenAI) │ │  Speech   │ │ Speech-to-Text│ ││
│  │ │  🆓 Grátis│ │  💰 API   │ │  💰 API   │ │   💰 API      │ ││
│  │ └───────────┘ └───────────┘ └───────────┘ └───────────────┘ ││
│  └─────────────────────────────────────────────────────────────┘│
│                                                                 │
│  ┌─────────────────────────────────────────────────────────────┐│
│  │ TTS Providers (Text-to-Speech)                              ││
│  │ ┌───────────┐ ┌───────────┐ ┌───────────┐ ┌───────────────┐ ││
│  │ │ WebSpeech │ │  SAPI5    │ │ OpenAI TTS│ │    Azure      │ ││
│  │ │ (Browser) │ │ (Windows) │ │ (tts-1)   │ │   Neural      │ ││
│  │ │  🆓 Grátis│ │ 🆓 Local  │ │  💰 API   │ │   💰 API      │ ││
│  │ └───────────┘ └───────────┘ └───────────┘ └───────────────┘ ││
│  │ ┌───────────┐                                               ││
│  │ │ElevenLabs │                                               ││
│  │ │ (Premium) │                                               ││
│  │ │  💰 API   │                                               ││
│  │ └───────────┘                                               ││
│  └─────────────────────────────────────────────────────────────┘│
│                                                                 │
│  ┌─────────────────────────────────────────────────────────────┐│
│  │ 🚀 Multimodal Providers (Áudio Nativo)                      ││
│  │ ┌───────────────────────────┐ ┌───────────────────────────┐ ││
│  │ │   OpenAI Realtime API     │ │   Gemini 2.0 Flash        │ ││
│  │ │   (GPT-4o via WebSocket)  │ │   (Live API)              │ ││
│  │ │   🎤→🤖→🔊 Direto         │ │   🎤→🤖→🔊 Direto         │ ││
│  │ └───────────────────────────┘ └───────────────────────────┘ ││
│  └─────────────────────────────────────────────────────────────┘│
└─────────────────────────────────────────────────────────────────┘
```

---

## SAPI5 (Speech API 5)

### Por que SAPI5?

SAPI5 é a API de síntese de voz nativa do Windows, usada por:
- **Leitores de tela**: NVDA, JAWS, Narrator
- **Vozes do sistema**: Microsoft David, Zira, Maria
- **Sintetizadores de terceiros**: Vocalizer, eSpeak, Balabolka voices

### Vantagens

| Vantagem | Descrição |
|----------|-----------|
| **Acessibilidade** | Usuários de leitores de tela já têm vozes configuradas |
| **Offline** | Funciona sem internet |
| **Baixa latência** | Processamento local |
| **Custo zero** | Usa vozes já instaladas |
| **Familiar** | Mesma voz que o usuário usa no sistema |

### Implementação

```go
// internal/speech/tts_sapi5_windows.go
// +build windows

package speech

import (
    "github.com/AnotherKnight/go-sapi5"
)

type SAPI5Provider struct {
    voice    *sapi5.Voice
    rate     int  // -10 a +10
    volume   int  // 0 a 100
}

func (s *SAPI5Provider) Speak(text string) error {
    return s.voice.Speak(text, sapi5.SVSFDefault)
}

func (s *SAPI5Provider) GetVoices() ([]string, error) {
    return sapi5.GetVoices()
}

func (s *SAPI5Provider) SetVoice(name string) error {
    return s.voice.SetVoice(name)
}
```

### Vozes SAPI5 Comuns

| Voz | Idioma | Origem |
|-----|--------|--------|
| Microsoft Maria | pt-BR | Windows 10+ |
| Microsoft Daniel | pt-BR | Windows 10+ |
| Microsoft Zira | en-US | Windows 10+ |
| NVDA eSpeak | Multi | NVDA |
| Vocalizer Expressive | Multi | Nuance |

---

## Modos de Operação

| Modo | Fluxo | Latência | Custo | Uso |
|------|-------|----------|-------|-----|
| **Text-Bridge** | STT → LLM (texto) → TTS | ~2-5s | Médio | Padrão |
| **Multimodal Native** | Áudio → GPT-4o → Áudio | ~0.5-1s | Alto | Premium |
| **Hybrid** | WebSpeech STT → LLM → SAPI5 | ~1-2s | Grátis | Acessibilidade |
| **Local** | WebSpeech STT → LLM → SAPI5 | ~1-2s | Grátis | Offline |

---

## Interface do Usuário

### Botão PTT (Push-to-Talk)

O botão de microfone aparece **no lugar do botão de envio** quando o campo de texto está vazio:

```
┌─────────────────────────────────────────────────────────────────┐
│  Campo VAZIO - Mostra botão de microfone                        │
│  ┌─────────────────────────────────────────────────────────────┐│
│  │ [Digite sua mensagem...                              ] [🎤] ││
│  └─────────────────────────────────────────────────────────────┘│
│                                                                 │
│  Campo com TEXTO - Mostra botão de envio                        │
│  ┌─────────────────────────────────────────────────────────────┐│
│  │ [Olá, como você está?                                ] [📤] ││
│  └─────────────────────────────────────────────────────────────┘│
│                                                                 │
│  GRAVANDO - Mostra indicador visual                             │
│  ┌─────────────────────────────────────────────────────────────┐│
│  │ [🔴 Gravando... solte para enviar                    ] [⏹️] ││
│  └─────────────────────────────────────────────────────────────┘│
└─────────────────────────────────────────────────────────────────┘
```

### Comportamento do Botão

| Estado | Ação | Resultado |
|--------|------|-----------|
| Campo vazio | Click e segura 🎤 | Inicia gravação |
| Campo vazio | Solta 🎤 | Para, transcreve, envia |
| Campo com texto | Click 📤 | Envia mensagem texto |
| Gravando | Click ⏹️ | Cancela gravação |

### Feedback Visual e Sonoro

```
┌─────────────────────────────────────────────────────────────────┐
│  Estados visuais do botão:                                      │
│                                                                 │
│  [🎤] Idle          - Azul, aguardando                          │
│  [🔴] Recording     - Vermelho pulsando + waveform              │
│  [⏳] Processing    - Amarelo, transcrevendo                    │
│  [🔊] Speaking      - Verde, reproduzindo resposta              │
│  [❌] Error         - Vermelho, erro no processo                │
└─────────────────────────────────────────────────────────────────┘
```

### Acessibilidade

| Recurso | Implementação |
|---------|---------------|
| **ARIA Labels** | `aria-label="Segure para falar"` dinâmico |
| **Live Region** | Anuncia estados: "Gravando", "Processando", "Resposta" |
| **Atalho** | `Alt+M` para ativar microfone |
| **Screen Reader** | Compatível com NVDA/JAWS |
| **SAPI5** | Usa voz do leitor de tela se configurado |

---

## Configuração

### Modelo de Dados

```json
{
  "speech": {
    "enabled": true,
    "mode": "ptt",
    
    "stt": {
      "provider": "webspeech",
      "language": "pt-BR",
      "continuous": false,
      "interim_results": true
    },
    
    "tts": {
      "provider": "sapi5",
      "voice": "Microsoft Maria",
      "rate": 0,
      "volume": 100,
      "auto_play": true
    },
    
    "multimodal": {
      "enabled": false,
      "provider": "openai_realtime",
      "voice": "alloy"
    },
    
    "wakeword": {
      "enabled": false,
      "word": "assistente",
      "sensitivity": 0.5
    }
  }
}
```

### Variáveis de Ambiente

```bash
# STT
OPENAI_API_KEY=sk-xxx          # Whisper API
AZURE_SPEECH_KEY=xxx           # Azure Speech
AZURE_SPEECH_REGION=eastus
GOOGLE_SPEECH_KEY=xxx          # Google Cloud

# TTS
OPENAI_API_KEY=sk-xxx          # OpenAI TTS (mesmo key)
AZURE_SPEECH_KEY=xxx           # Azure Neural TTS
ELEVENLABS_API_KEY=xxx         # ElevenLabs

# Multimodal
OPENAI_API_KEY=sk-xxx          # GPT-4o Realtime
```

---

## Comparativo de Provedores

### STT (Speech-to-Text)

| Provider | Qualidade | Latência | Preço | Offline | PT-BR |
|----------|-----------|----------|-------|---------|-------|
| WebSpeech API | ⭐⭐⭐ | ~100ms | Grátis | ❌ | ✅ |
| Whisper (OpenAI) | ⭐⭐⭐⭐⭐ | ~2-3s | $0.006/min | ❌ | ✅ |
| Azure Speech | ⭐⭐⭐⭐ | ~500ms | $1/hora | ✅ | ✅ |
| Google Cloud | ⭐⭐⭐⭐ | ~500ms | $0.006/15s | ❌ | ✅ |

### TTS (Text-to-Speech)

| Provider | Naturalidade | Latência | Preço | Offline | PT-BR |
|----------|--------------|----------|-------|---------|-------|
| WebSpeech API | ⭐⭐ | Instant | Grátis | ❌ | ✅ |
| **SAPI5** | ⭐⭐⭐ | Instant | Grátis | ✅ | ✅ |
| OpenAI TTS | ⭐⭐⭐⭐ | ~1s | $15/1M chars | ❌ | ✅ |
| Azure Neural | ⭐⭐⭐⭐⭐ | ~500ms | $16/1M chars | ❌ | ✅ |
| ElevenLabs | ⭐⭐⭐⭐⭐ | ~1s | $5-330/mês | ❌ | ✅ |

### Multimodal

| Provider | Latência | Preço | Naturalidade | Recursos |
|----------|----------|-------|--------------|----------|
| OpenAI Realtime | ~300ms | $0.06/min | ⭐⭐⭐⭐⭐ | Interrupção, emoção |
| Gemini 2.0 Live | ~400ms | TBD | ⭐⭐⭐⭐ | Visão + áudio |

---

## Arquivos

### Backend (Go)

```
internal/speech/
├── speech_service.go         # Serviço principal
├── stt_provider.go           # Interface STT
├── stt_webspeech.go          # Wrapper (delega ao browser)
├── stt_whisper.go            # OpenAI Whisper API
├── stt_azure.go              # Azure Speech-to-Text
├── stt_google.go             # Google Cloud STT
├── tts_provider.go           # Interface TTS
├── tts_webspeech.go          # Wrapper (delega ao browser)
├── tts_sapi5_windows.go      # SAPI5 (Windows only)
├── tts_sapi5_stub.go         # Stub para outros OS
├── tts_openai.go             # OpenAI TTS (tts-1, tts-1-hd)
├── tts_azure.go              # Azure Neural TTS
├── tts_elevenlabs.go         # ElevenLabs (opcional)
├── realtime_openai.go        # GPT-4o Realtime WebSocket
└── realtime_gemini.go        # Gemini 2.0 Live API
```

### Frontend (Svelte)

```
frontend/src/
├── components/
│   ├── Chat.svelte           # Atualizado com PTT
│   ├── VoiceButton.svelte    # Componente de voz
│   ├── VoiceSettings.svelte  # Configurações de voz
│   └── AudioVisualizer.svelte # Visualização de áudio
└── lib/
    ├── speech/
    │   ├── index.js          # Exporta tudo
    │   ├── stt-webspeech.js  # WebSpeech STT
    │   ├── stt-whisper.js    # Whisper via backend
    │   ├── tts-webspeech.js  # WebSpeech TTS
    │   ├── tts-backend.js    # TTS via backend (SAPI5, etc)
    │   ├── audio-recorder.js # MediaRecorder wrapper
    │   └── audio-player.js   # Reprodução de áudio
    └── wakeword/
        └── porcupine.js      # Picovoice Porcupine (futuro)
```

---

## Fases de Implementação

### Fase 1: PTT com WebSpeech ✅ Concluída

**Objetivo:** Implementar Push-to-Talk básico usando APIs do navegador.

**Tarefas:**
- [x] Criar componente `VoiceButton.svelte`
- [x] Integrar `SpeechRecognition` API para STT
- [x] Integrar `SpeechSynthesis` API para TTS
- [x] Modificar `Chat.svelte` para alternar botão envio/microfone
- [x] Adicionar feedback visual (gravando, processando)
- [x] Adicionar feedback sonoro (beeps de início/fim)
- [x] Implementar atalho `Alt+M` para microfone
- [ ] Testes de acessibilidade com NVDA

**Arquivos criados:**
```
frontend/src/lib/speech/
├── index.js              # Exporta módulos
├── stt-webspeech.js      # SpeechRecognition wrapper
├── tts-webspeech.js      # SpeechSynthesis wrapper
└── audio-recorder.js     # MediaRecorder wrapper (para Fase 3)

frontend/src/components/
├── VoiceButton.svelte    # Componente PTT
└── Chat.svelte           # Atualizado com integração de voz
```

**Recursos de UI:**
- Botão 🎤 aparece quando campo está vazio, 📤 quando tem texto
- Seletor de vozes TTS na toolbar (igual ao de modelos)
- Toolbar navegável por setas (←/→)
- Foco retorna ao campo de input após enviar por voz
- Feedback visual por estados (idle/recording/speaking)

**Resultado:** Usuário pode falar com o assistente usando o navegador, sem custo.

---

### Fase 2: SAPI5 para Windows ✅ Concluída

**Objetivo:** Suporte a vozes SAPI5 do Windows para TTS.

**Tarefas:**
- [x] Implementar `sapi5_windows.go` usando go-ole (COM nativo)
- [x] Criar stub `sapi5_other.go` para Linux/Mac
- [x] API para listar vozes SAPI5 disponíveis
- [x] API para falar texto via SAPI5
- [x] Configurar voz, velocidade e volume
- [x] Adicionar opção SAPI5 no VoicePicker
- [x] Modal de configurações de voz (volume, velocidade, auto-speak)

**Arquivos criados:**
```
internal/speech/
├── sapi5_windows.go    # Implementação COM nativa (go-ole)
└── sapi5_other.go      # Stub para Linux/Mac
```

**Resultado:** Respostas do assistente podem usar vozes SAPI5 locais sem latência.

---

### Fase 3: Provedores de API (Whisper, OpenAI TTS) ✅ Concluída

**Objetivo:** Adicionar suporte a APIs de nuvem para maior qualidade.

**Tarefas:**
- [x] Implementar `openai_whisper.go` (OpenAI Whisper API)
- [x] Implementar `openai_tts.go` (tts-1, tts-1-hd)
- [x] Implementar `speech_manager.go` (gerenciador unificado)
- [x] Endpoint `TranscribeWhisper` para transcrição de áudio
- [x] Endpoint `SynthesizeOpenAIWithVoice` para síntese de voz
- [x] Vozes OpenAI no VoicePicker (alloy, echo, fable, onyx, nova, shimmer)
- [x] Fallback automático (API falha → WebSpeech)

**Arquivos criados:**
```
internal/speech/
├── openai_whisper.go   # Cliente Whisper API (STT)
├── openai_tts.go       # Cliente OpenAI TTS (TTS)
└── speech_manager.go   # Gerenciador unificado
```

**Vozes OpenAI disponíveis:**
| Voz | Descrição | Gênero |
|-----|-----------|--------|
| alloy | Neutra e balanceada | Neutro |
| echo | Masculina e profunda | Masculino |
| fable | Expressiva, narrativa | Neutro |
| onyx | Masculina, autoritária | Masculino |
| nova | Feminina, jovem e energética | Feminino |
| shimmer | Feminina, clara e expressiva | Feminino |

**Resultado:** Usuário pode escolher vozes OpenAI de alta qualidade (premium) ou locais (gratuito).

---

### Fase 4: Hotkey Global ✅ Concluída (alternativa a Wake Word)

**Objetivo:** Permitir ativação do assistente de qualquer janela via atalho de teclado.

**Decisão de arquitetura:** Wake word (Porcupine) foi descartado por:
- Licenciamento proprietário
- Complexidade de integração
- Alternativas open source são muito complexas

**Solução implementada:** Hotkey global usando API nativa do Windows.

**Tarefas:**
- [x] Implementar `hotkey_windows.go` usando RegisterHotKey API
- [x] Criar stub `hotkey_other.go` para Linux/Mac
- [x] Integrar com Wails runtime (eventos + WindowShow)
- [x] Frontend escuta evento `global:hotkey:voice`
- [x] VoiceButton expõe `startRecording()` para ativação externa
- [x] API para configurar/desativar hotkey

**Arquivos criados:**
```
internal/hotkey/
└── hotkey.go   # Wrapper cross-platform (golang.design/x/hotkey)
```

**Biblioteca utilizada:** `golang.design/x/hotkey`
- Windows: ✅ Suportado (RegisterHotKey API)
- Linux: ✅ Suportado (X11 - XGrabKey)
- macOS: ✅ Suportado (Carbon Events)
- Linux Wayland: ❌ Não suportado (limitação do protocolo)

**Hotkey padrão:** `Ctrl+Shift+A`

**Fluxo:**
1. Usuário pressiona `Ctrl+Shift+A` em qualquer janela
2. Janela do assistente aparece e ganha foco
3. Microfone ativa automaticamente
4. Usuário fala → STT → LLM → TTS
5. Mensagens salvas normalmente no histórico

**APIs expostas:**
```go
IsGlobalHotkeySupported() bool
GetGlobalHotkeys() []HotkeyInfo
SetVoiceHotkey(modifiers, key string) error
DisableVoiceHotkey() error
EnableVoiceHotkey() error
```

**Resultado:** Usuário pode ativar o assistente de qualquer lugar com Ctrl+Shift+A.

---

### Fase 5: Multimodal (FUTURO)

**Status:** 🔮 Planejamento futuro - não será implementado agora.

**Motivo:** Requer implementação de cliente WebSocket próprio, independente do LiteLLM.

> ⚠️ **Nota importante sobre LiteLLM e Multimodal:**
> 
> O LiteLLM abstrai APIs HTTP (chat completion, embeddings, imagens no prompt).
> Porém, APIs de áudio realtime (OpenAI Realtime, Gemini Live) usam **WebSocket**,
> que o LiteLLM **não suporta**. Seria necessário implementar clientes WebSocket
> específicos para cada provedor.

**Consulte:** [MULTIMODAL_ROADMAP.md](./MULTIMODAL_ROADMAP.md) para o plano completo.

---

## Fluxo Detalhado

### PTT (Push-to-Talk) - Fase 1

```
1. Usuário PRESSIONA botão 🎤
   ├─ Toca som de início (beep)
   ├─ Inicia SpeechRecognition
   ├─ Mostra indicador visual 🔴
   └─ aria-live: "Gravando"

2. Usuário SOLTA botão
   ├─ Para SpeechRecognition
   ├─ Toca som de fim (beep)
   └─ Mostra indicador ⏳

3. Transcrição recebida
   ├─ Preenche campo de texto
   ├─ Envia automaticamente (handleSubmit)
   └─ aria-live: "Enviado: [texto]"

4. Resposta do LLM
   ├─ Streaming normal no chat
   └─ Ao finalizar, passa para TTS

5. TTS reproduz resposta
   ├─ Se SAPI5: Go chama SAPI5
   ├─ Se WebSpeech: browser fala
   ├─ Mostra indicador 🔊
   └─ aria-live: "Resposta: [preview]"

6. Retorna ao estado idle
   └─ Mostra botão 🎤
```

### Com APIs de Nuvem - Fase 3

```
1. Usuário PRESSIONA botão 🎤
   ├─ Inicia MediaRecorder
   └─ Grava áudio em WebM/Opus

2. Usuário SOLTA botão
   ├─ Para MediaRecorder
   └─ Obtém Blob de áudio

3. Envia áudio para backend
   └─> POST /api/speech/transcribe
       ├─ Go recebe áudio
       ├─ Chama Whisper API
       └─ Retorna texto

4. Resposta do LLM
   └─ Ao finalizar, chama TTS

5. Backend gera áudio
   └─> POST /api/speech/speak
       ├─ Go recebe texto
       ├─ Chama OpenAI TTS ou SAPI5
       └─ Retorna áudio MP3/WAV

6. Frontend reproduz áudio
   └─ Audio element ou SAPI5 local
```

---

## Considerações

### Performance

| Aspecto | Solução |
|---------|---------|
| Latência STT | WebSpeech é mais rápido, Whisper mais preciso |
| Latência TTS | SAPI5 é instantâneo, APIs têm ~1s delay |
| Memória | Porcupine usa ~3MB para wake word |
| CPU | WebSpeech usa API do OS, eficiente |

### Acessibilidade

| Requisito | Implementação |
|-----------|---------------|
| Leitor de tela | ARIA labels dinâmicos, live regions |
| Sem mouse | `Alt+M` ativa microfone, `Esc` cancela |
| Voz familiar | SAPI5 usa voz do sistema/leitor |
| Feedback | Sons distintos para cada estado |

### Privacidade

| Aspecto | Consideração |
|---------|--------------|
| Wake word | Processa localmente (Porcupine) |
| Áudio para API | Só quando usuário inicia PTT |
| Transcrição | Opção para usar só WebSpeech (local) |
| Armazenamento | Áudio não é salvo por padrão |

### Custos

| Uso | Custo Estimado |
|-----|----------------|
| 100 mensagens/dia WebSpeech + SAPI5 | $0 |
| 100 mensagens/dia Whisper + OpenAI TTS | ~$0.50 |
| 100 mensagens/dia GPT-4o Realtime | ~$6.00 |

---

## Design System - Componentes Reutilizáveis

### Componentes Base

| Componente | Descrição | Uso |
|------------|-----------|-----|
| `ComboboxPicker` | Picker acessível com filtro e teclado | Base para pickers |
| `Toolbar` | Toolbar com roving tabindex | Navegação por setas |
| `Modal` | Modal acessível com trap de foco | Dialogs |

### Componentes Especializados

| Componente | Extends | Descrição |
|------------|---------|-----------|
| `ModelPicker` | `ComboboxPicker` | Seletor de modelos LLM com auto-load |
| `VoicePicker` | `ComboboxPicker` | Seletor de vozes TTS com opção "Desativada" |
| `VoiceButton` | - | Botão PTT com estados visuais |

### Lógica de Leitura (TTS vs Leitor de Telas)

O `VoicePicker` inclui uma opção **"Desativada (usar leitor de telas)"** que evita duplicação:

| Voz Selecionada | Comportamento |
|-----------------|---------------|
| 🔇 Desativada | Envia para `aria-live` → Leitor de telas lê |
| Qualquer voz | Usa TTS → **NÃO** envia para `aria-live` |

**Implementação técnica:**
- `VOICE_DISABLED` é exportado via `<script context="module">` para ser importável
- A região `aria-live` só é renderizada quando TTS está desativado
- Mensagens em streaming têm `aria-hidden="true"` quando TTS está ativo
- O container tem `aria-busy="true"` durante loading quando TTS está ativo

Isso é importante para acessibilidade:
- Usuários de leitores de tela não ouvem duplicado (TTS + leitor)
- Usuários sem leitor podem usar vozes de síntese
- O padrão é "Desativada" para priorizar acessibilidade

### Variants

Os componentes `ModelPicker` e `VoicePicker` suportam duas variantes:

- **`toolbar`** (padrão): Compacto, sem label externa, para toolbars
- **`form`**: Com label, help text, estados de loading/error

```svelte
<!-- Toolbar (compacto) -->
<ModelPicker bind:value={model} label={model || 'Modelo'} />

<!-- Form (com label e help) -->
<ModelPicker 
  bind:value={model} 
  variant="form"
  label="Modelo de Chat"
  helpText="Selecione o modelo LLM"
/>
```

---

## Próximos Passos

1. ✅ **Fase 1** - Implementar PTT básico com WebSpeech
2. ✅ **Fase 2** - Adicionar suporte SAPI5 para Windows
3. **Fase 3** - Integrar APIs de nuvem como opção premium
4. **Fase 4** - Wake word com Porcupine
5. **Fase 5** - Modo multimodal com GPT-4o Realtime

---

## Implementação SAPI5 (Fase 2 - Concluída)

### Arquitetura

```
┌─────────────────────────────────────────────────────────────┐
│                       Frontend                               │
│  VoicePicker.svelte                                          │
│  - loadWebSpeechVoices() → speechSynthesis API              │
│  - loadSAPI5Voices() → GetSAPI5Voices() (Wails)             │
│  - Combina ambas as listas                                  │
│  - Identifica source: 'webspeech' | 'sapi5'                 │
└─────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────────┐
│                       Backend (Go)                           │
│  internal/speech/                                            │
│  ├── sapi5_windows.go      (Windows - go-ole COM nativo)    │
│  └── sapi5_other.go        (Linux/Mac - stubs vazios)       │
│                                                              │
│  App Methods:                                                │
│  - GetSAPI5Voices() → lista vozes instaladas                │
│  - SpeakSAPI5(text, voiceName) → sintetiza texto            │
│  - StopSAPI5() → para síntese                               │
│  - SetSAPI5Volume(0-100) → define volume                    │
│  - SetSAPI5Rate(-10 a 10) → define velocidade               │
│  - IsSAPI5Speaking() → verifica se está falando             │
└─────────────────────────────────────────────────────────────┘
```

### Tecnologia: go-ole (COM Nativo)

Usamos a biblioteca **go-ole** para acessar SAPI5 diretamente via COM:

```go
import (
    "github.com/go-ole/go-ole"
    "github.com/go-ole/go-ole/oleutil"
)

// Cria instância do SpVoice
unknown, _ := oleutil.CreateObject("SAPI.SpVoice")
spVoice, _ := unknown.QueryInterface(ole.IID_IDispatch)

// Fala texto de forma assíncrona
oleutil.CallMethod(spVoice, "Speak", text, 1) // 1 = async
```

### Vantagens vs PowerShell

| Aspecto | PowerShell | go-ole |
|---------|------------|--------|
| Latência | ~500ms (inicia processo) | ~10ms (direto) |
| Controle | Limitado | Total (volume, velocidade, eventos) |
| Dependência | PowerShell instalado | Nenhuma |
| Código | "Gambiarra" | Solução correta |

### Build Tags

- `//go:build windows` - Código SAPI5 real com go-ole
- `//go:build !windows` - Stubs vazios (retorna lista vazia, não faz nada)

### Graceful Degradation

- Em Linux/Mac: SAPI5 retorna lista vazia sem erros
- Se COM falhar: retorna erro tratável
- Se voz SAPI5 falhar ao falar: fallback para WebSpeech

### Modal de Configurações de Voz

O modal de configurações de voz permite ajustar:

| Parâmetro | Intervalo | Descrição |
|-----------|-----------|-----------|
| Volume | 0-100% | Volume da síntese |
| Velocidade | -10 a +10 | Taxa de fala |
| Auto-falar | On/Off | Fala respostas automaticamente |

As configurações são aplicadas tanto ao WebSpeech quanto ao SAPI5, com conversão automática dos intervalos.

você tinha comentado que tinha implementado wisper, como uso?
