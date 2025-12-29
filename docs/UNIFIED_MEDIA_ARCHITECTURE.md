# Arquitetura Unificada de Mídia

> **Status Geral:** 🟡 Em andamento

Este documento descreve a arquitetura para unificação do sistema de envio de arquivos, modos de gravação de áudio e geração de imagens.

---

## Resumo Rápido de Status

| Fase | Frontend | Backend | Observação |
|------|----------|---------|------------|
| **U1: Menu Unificado** | 🔲 | N/A | Uma opção de arquivo + submenu de modos |
| **U2: Detecção Automática** | 🔲 | 🔲 | Detecta tipo e processa (imagem→vision, áudio→transcreve, etc.) |
| **U3: Modos de Gravação** | 🔲 | N/A | PTT, Toggle, VAD-Silêncio, VAD-Atividade |
| **U4: DALL-E** | 🔲 | 🔲 | Geração de imagens |

---

## Visão Geral

### Problema Atual

O menu de mídia atual tem opções separadas para cada tipo:
- 🖼️ Enviar imagem
- 🎵 Enviar arquivo de áudio
- 📄 Enviar documento
- 📸 Capturar tela
- 📷 Capturar webcam
- 🎙️ Gravar áudio

E o botão de voz só tem um modo (PTT).

### Solução Proposta

**1. Menu simplificado** - apenas 1 opção de arquivo:

```
┌─────────────────────────────────────────┐
│  📎 Enviar arquivo                      │  ← Abre seletor, detecta tipo auto
│  📸 Capturar tela                       │  ← Screenshot
│  📷 Capturar webcam                     │  ← Foto da câmera
│  ─────────────────────────────────────  │
│  🎙️ Modo de gravação               ►   │  ← Submenu de modos
│      ├─ 🎤 Push-to-Talk (segurar)       │
│      ├─ ⏺️ Toggle (clique p/ iniciar/parar) │
│      ├─ 🔇 VAD Silêncio (clique + auto-stop) │
│      └─ 🎯 VAD Atividade (full auto)    │
└─────────────────────────────────────────┘
```

**2. Detecção automática** - sistema identifica tipo pelo MIME/extensão e processa:
- Imagem → Vision API (ou modelo auxiliar)
- Áudio → Transcreve com Whisper (ou envia nativo se modelo suportar)
- Documento → Extrai texto
- Dados → Parseia e formata

**3. Modos de gravação de áudio (4 modos):**
- **PTT**: Segura para gravar, solta para parar
- **Toggle**: Clica para iniciar, clica para parar
- **VAD Silêncio**: Clica para iniciar, detecta silêncio para parar
- **VAD Atividade**: Detecta início de fala para começar E silêncio para parar (totalmente automático)

**4. Hotkey global (Ctrl+Shift+A)**: Sempre usa modo VAD Atividade (full auto)

### Drag & Drop e Colar

Além do menu, o usuário pode:
- **Arrastar e soltar** qualquer arquivo no chat
- **Colar (Ctrl+V)** imagens, texto ou arquivos
- Ambos usam a mesma lógica de detecção automática

---

## Fase U1: Menu Unificado

**Objetivo:** Simplificar o menu de mídia para uma única opção de arquivo.

### Antes vs Depois

```
ANTES (6 opções):                  DEPOIS (4 opções + submenu):
┌─────────────────────────┐        ┌─────────────────────────┐
│ 🖼️ Enviar imagem        │        │ 📎 Enviar arquivo       │
│ 🎵 Enviar áudio         │        │ 📸 Capturar tela        │
│ 📄 Enviar documento     │        │ 📷 Capturar webcam      │
│ ─────────────────────── │        │ ─────────────────────── │
│ 📸 Capturar tela        │        │ 🎙️ Modo gravação    ►   │
│ 📷 Capturar webcam      │        └─────────────────────────┘
│ 🎙️ Gravar áudio         │
└─────────────────────────┘
```

### Tarefas

- [ ] Atualizar `MediaMenu.svelte` para menu simplificado
- [ ] Adicionar submenu para modos de gravação
- [ ] Input de arquivo aceita todos os tipos
- [ ] Manter compatibilidade com eventos existentes

### Código

```svelte
<!-- MediaMenu.svelte atualizado -->
<script>
  import { createEventDispatcher } from 'svelte';
  
  export let recordingMode = 'ptt'; // 'ptt', 'toggle', 'vad'
  
  const dispatch = createEventDispatcher();
  
  const menuItems = [
    { id: 'file', label: 'Enviar arquivo', icon: '📎' },
    { id: 'screenshot', label: 'Capturar tela', icon: '📸' },
    { id: 'webcam', label: 'Capturar webcam', icon: '📷' },
    { separator: true },
    { 
      id: 'recording_mode', 
      label: 'Modo de gravação', 
      icon: '🎙️',
      submenu: [
        { 
          id: 'mode_ptt', 
          label: 'Push-to-Talk (segurar)', 
          icon: '🎤',
          checked: recordingMode === 'ptt'
        },
        { 
          id: 'mode_toggle', 
          label: 'Toggle (clique)', 
          icon: '⏺️',
          checked: recordingMode === 'toggle'
        },
        { 
          id: 'mode_vad', 
          label: 'Detecção de silêncio', 
          icon: '🔇',
          checked: recordingMode === 'vad'
        }
      ]
    }
  ];
  
  // Aceita todos os tipos de arquivo
  const acceptTypes = 'image/*,audio/*,video/*,.pdf,.doc,.docx,.txt,.md,.csv,.json,.xml,.xls,.xlsx,.ppt,.pptx';
</script>
```

---

## Fase U3: Modos de Gravação de Áudio

**Objetivo:** Oferecer diferentes modos de gravação para diferentes situações.

### Modos Disponíveis (4 modos)

| Modo | Ícone | Início | Fim | Quando usar |
|------|-------|--------|-----|-------------|
| **PTT** | 🎤 | Pressiona botão | Solta botão | Mensagens curtas, precisão |
| **Toggle** | ⏺️ | Clica botão | Clica botão | Mensagens longas, mãos livres |
| **VAD Silêncio** | 🔇 | Clica botão | Detecta silêncio | Semi-automático |
| **VAD Atividade** | 🎯 | Detecta voz | Detecta silêncio | Totalmente automático |

### Comportamento do Botão por Modo

```
┌─────────────────────────────────────────────────────────────────┐
│                     MODO PTT (Push-to-Talk)                      │
├─────────────────────────────────────────────────────────────────┤
│                                                                 │
│  [🎤] ─► Pressionar e segurar ─► Gravando... ─► Soltar ─► Envia │
│                                                                 │
│  - Comportamento atual (padrão)                                 │
│  - Mais controle sobre quando começa/termina                    │
│  - Ideal para ambientes com ruído                               │
│                                                                 │
└─────────────────────────────────────────────────────────────────┘

┌─────────────────────────────────────────────────────────────────┐
│                     MODO TOGGLE (Clique)                         │
├─────────────────────────────────────────────────────────────────┤
│                                                                 │
│  [🎤] ─► Clique ─► [⏺️ Gravando...] ─► Clique ─► Envia          │
│                                                                 │
│  - Não precisa segurar o botão                                  │
│  - Bom para mensagens longas                                    │
│  - Usuário tem controle total                                   │
│                                                                 │
└─────────────────────────────────────────────────────────────────┘

┌─────────────────────────────────────────────────────────────────┐
│                     MODO VAD SILÊNCIO                            │
├─────────────────────────────────────────────────────────────────┤
│                                                                 │
│  [🎤] ─► Clique ─► [🔇 Gravando...] ─► Fala ─► Silêncio ─► Envia │
│                                                                 │
│  - Usuário clica para começar                                   │
│  - Detecta silêncio no fim da fala para parar automaticamente   │
│  - Timeout configurável (ex: 1.5s de silêncio)                  │
│  - Bom equilíbrio entre controle e conveniência                 │
│                                                                 │
└─────────────────────────────────────────────────────────────────┘

┌─────────────────────────────────────────────────────────────────┐
│                     MODO VAD ATIVIDADE (Full Auto)               │
├─────────────────────────────────────────────────────────────────┤
│                                                                 │
│  [🎤] ─► Clique ─► [🎯 Aguardando...] ─► Detecta voz ─►         │
│                    [🔊 Gravando...] ─► Silêncio ─► Envia         │
│                                                                 │
│  - Usuário clica para ativar modo de escuta                     │
│  - Sistema aguarda início de atividade de voz para gravar       │
│  - Detecta silêncio no fim para parar e enviar                  │
│  - Totalmente automático após ativar                            │
│  - Usado pelo Ctrl+Shift+A (hotkey global)                      │
│                                                                 │
└─────────────────────────────────────────────────────────────────┘
```

### Hotkey Global (Ctrl+Shift+A)

O atalho global **sempre** usa modo **VAD Atividade** (full auto), independente do modo selecionado no menu:

```
Ctrl+Shift+A (de qualquer janela)
      │
      ▼
┌─────────────────────────────────────────────────────────────────┐
│ 1. Janela do assistente aparece e ganha foco                    │
│ 2. Microfone ativa em modo VAD Atividade                        │
│ 3. Sistema aguarda detecção de voz                              │
│ 4. Usuário fala                                                 │
│ 5. Detecta silêncio → transcreve → envia                        │
│ 6. Assistente responde (TTS se habilitado)                      │
└─────────────────────────────────────────────────────────────────┘
```

### Implementação do VAD

```javascript
// lib/speech/vad.js
export class VoiceActivityDetector {
  constructor(options = {}) {
    this.silenceThreshold = options.silenceThreshold || 0.01;
    this.silenceTimeout = options.silenceTimeout || 1500; // ms
    this.minSpeechDuration = options.minSpeechDuration || 500; // ms
    
    // Modo: 'silence' (VAD Silêncio) ou 'activity' (VAD Atividade)
    this.mode = options.mode || 'silence';
    
    this.analyser = null;
    this.audioContext = null;
    this.silenceTimer = null;
    this.speechStartTime = null;
    this.isSpeaking = false;
    this.isRecording = false;
  }
  
  /**
   * Inicia detecção de atividade de voz
   * @param {MediaStream} stream - Stream de áudio do microfone
   * @param {Object} callbacks - { onSpeechStart, onSpeechEnd, onRecordingStart }
   */
  start(stream, callbacks = {}) {
    const { onSpeechStart, onSpeechEnd, onRecordingStart } = callbacks;
    
    this.audioContext = new AudioContext();
    const source = this.audioContext.createMediaStreamSource(stream);
    this.analyser = this.audioContext.createAnalyser();
    this.analyser.fftSize = 256;
    source.connect(this.analyser);
    
    const dataArray = new Uint8Array(this.analyser.frequencyBinCount);
    
    // No modo 'silence', já começa gravando
    if (this.mode === 'silence') {
      this.isRecording = true;
      onRecordingStart?.();
    }
    
    const checkAudio = () => {
      if (!this.analyser) return;
      
      this.analyser.getByteFrequencyData(dataArray);
      const average = dataArray.reduce((a, b) => a + b) / dataArray.length;
      const normalized = average / 255;
      
      if (normalized > this.silenceThreshold) {
        // Som/voz detectada
        if (!this.isSpeaking) {
          this.isSpeaking = true;
          this.speechStartTime = Date.now();
          
          // No modo 'activity', inicia gravação quando detecta voz
          if (this.mode === 'activity' && !this.isRecording) {
            this.isRecording = true;
            onSpeechStart?.();
            onRecordingStart?.();
          }
        }
        
        // Reseta timer de silêncio
        clearTimeout(this.silenceTimer);
        this.silenceTimer = null;
      } else if (this.isSpeaking && this.isRecording && !this.silenceTimer) {
        // Silêncio detectado após fala
        this.silenceTimer = setTimeout(() => {
          const duration = Date.now() - this.speechStartTime;
          if (duration >= this.minSpeechDuration) {
            this.isSpeaking = false;
            this.isRecording = false;
            onSpeechEnd?.();
          }
        }, this.silenceTimeout);
      }
      
      requestAnimationFrame(checkAudio);
    };
    
    checkAudio();
  }
  
  stop() {
    clearTimeout(this.silenceTimer);
    this.audioContext?.close();
    this.analyser = null;
    this.audioContext = null;
    this.isSpeaking = false;
    this.isRecording = false;
  }
}

// Uso:
// VAD Silêncio: clica para iniciar, detecta silêncio para parar
const vadSilence = new VoiceActivityDetector({ mode: 'silence' });
vadSilence.start(stream, {
  onRecordingStart: () => console.log('Gravando...'),
  onSpeechEnd: () => console.log('Silêncio detectado, enviando...')
});

// VAD Atividade: detecta voz para iniciar, silêncio para parar
const vadActivity = new VoiceActivityDetector({ mode: 'activity' });
vadActivity.start(stream, {
  onSpeechStart: () => console.log('Voz detectada!'),
  onRecordingStart: () => console.log('Gravando...'),
  onSpeechEnd: () => console.log('Silêncio detectado, enviando...')
});
```

### Persistência do Modo

O modo selecionado é salvo nas preferências do usuário:

```javascript
// Salva preferência
localStorage.setItem('recordingMode', 'toggle');

// Carrega na inicialização
const savedMode = localStorage.getItem('recordingMode') || 'ptt';
```

### Feedback Visual por Modo

| Estado | PTT | Toggle | VAD Silêncio | VAD Atividade |
|--------|-----|--------|--------------|---------------|
| Idle | 🎤 | 🎤 | 🎤 | 🎤 |
| Aguardando voz | - | - | - | 🎯 amarelo pulsando |
| Gravando | 🔴 pulsando | ⏺️ vermelho | 🔇 gravando | 🔊 verde pulsando |
| Detectando silêncio | - | - | 🔇 amarelo | 🔇 amarelo |
| Processando | ⏳ | ⏳ | ⏳ | ⏳ |

### Tarefas

- [ ] Implementar `VoiceActivityDetector` em `lib/speech/vad.js` com 2 modos (silence, activity)
- [ ] Atualizar `VoiceButton.svelte` para suportar 4 modos (ptt, toggle, vad_silence, vad_activity)
- [ ] Adicionar submenu de modos no `MediaMenu.svelte`
- [ ] Persistir modo selecionado em localStorage
- [ ] Hotkey global (Ctrl+Shift+A) sempre usa VAD Atividade (full auto)
- [ ] Feedback visual diferenciado por modo
- [ ] Configuração de timeout de silêncio
- [ ] Configuração de threshold de detecção de voz

---

## Fase U2: Detecção Automática de Tipo e Processamento

**Objetivo:** Identificar automaticamente o que fazer com cada arquivo e processá-lo corretamente.

### Categorias de Arquivo

| Categoria | MIME Types | Extensões | Ação |
|-----------|------------|-----------|------|
| **Imagem** | `image/*` | `.jpg`, `.png`, `.gif`, `.webp`, `.svg` | Vision API ou modelo auxiliar |
| **Áudio** | `audio/*` | `.mp3`, `.wav`, `.m4a`, `.ogg`, `.webm` | Transcreve (Whisper) ou envia nativo |
| **Vídeo** | `video/*` | `.mp4`, `.webm`, `.mov` | Extrai frames + transcreve áudio |
| **Documento** | `application/pdf`, `text/*` | `.pdf`, `.txt`, `.md`, `.doc` | Extrai texto |
| **Dados** | `application/json`, `text/csv` | `.json`, `.csv`, `.xml` | Parseia e formata |
| **Outros** | - | - | Erro ou converte para texto |

### Lógica de Detecção

```javascript
// lib/media-detector.js
export function detectMediaType(file) {
  const mimeType = file.type;
  const extension = file.name.split('.').pop().toLowerCase();
  
  // Imagens
  if (mimeType.startsWith('image/')) {
    return { category: 'image', action: 'vision', icon: '🖼️' };
  }
  
  // Áudio
  if (mimeType.startsWith('audio/')) {
    return { category: 'audio', action: 'transcribe_or_native', icon: '🎵' };
  }
  
  // Vídeo
  if (mimeType.startsWith('video/')) {
    return { category: 'video', action: 'extract_and_transcribe', icon: '🎬' };
  }
  
  // PDF
  if (mimeType === 'application/pdf' || extension === 'pdf') {
    return { category: 'document', action: 'extract_text', icon: '📄' };
  }
  
  // Documentos Office
  if (['doc', 'docx', 'xls', 'xlsx', 'ppt', 'pptx'].includes(extension)) {
    return { category: 'document', action: 'extract_text', icon: '📄' };
  }
  
  // Texto puro
  if (mimeType.startsWith('text/') || ['txt', 'md', 'json', 'csv', 'xml'].includes(extension)) {
    return { category: 'text', action: 'read_content', icon: '📝' };
  }
  
  // Fallback
  return { category: 'unknown', action: 'error', icon: '❓' };
}
```

### Processamento de Áudio (incluído nesta fase)

Quando um arquivo de áudio é detectado:

```
┌─────────────────────────────────────────────────────────────────┐
│                     PROCESSAMENTO DE ÁUDIO                       │
├─────────────────────────────────────────────────────────────────┤
│                                                                 │
│  Arquivo de áudio detectado (.mp3, .wav, .m4a, etc.)           │
│                                                                 │
│                        │                                        │
│                        ▼                                        │
│  ┌─────────────────────────────────────────────────────────────┐│
│  │ Modelo suporta áudio nativo?                                ││
│  │ (GPT-4o, GPT-4o-mini, Gemini 1.5)                           ││
│  └─────────────────────────────────────────────────────────────┘│
│                │                      │                         │
│               SIM                    NÃO                        │
│                │                      │                         │
│                ▼                      ▼                         │
│  ┌───────────────────┐    ┌────────────────────────────────────┐│
│  │ Envia áudio       │    │ Transcreve com Whisper            ││
│  │ direto para API   │    │ Envia transcrição como texto      ││
│  │ (input_audio)     │    │                                    ││
│  └───────────────────┘    └────────────────────────────────────┘│
│                                                                 │
└─────────────────────────────────────────────────────────────────┘
```

#### Formato de Mensagem com Áudio Nativo

```javascript
// Para modelos que suportam áudio (GPT-4o, Gemini)
{
  role: "user",
  content: [
    { type: "text", text: "O que está sendo dito neste áudio?" },
    { 
      type: "input_audio", 
      input_audio: {
        data: "<base64-audio>",
        format: "mp3"  // ou "wav", "m4a"
      }
    }
  ]
}
```

#### Fallback com Transcrição

```javascript
// Para modelos que NÃO suportam áudio
{
  role: "user",
  content: [
    { type: "text", text: "O que está sendo dito neste áudio?" },
    { type: "text", text: "[Transcrição do áudio]:\n\"Olá, como vai você?\"" }
  ]
}
```

#### Modelos com Suporte Nativo a Áudio

| Provedor | Modelo | Áudio Input |
|----------|--------|-------------|
| OpenAI | gpt-4o | ✅ |
| OpenAI | gpt-4o-mini | ✅ |
| OpenAI | gpt-4o-audio-preview | ✅ |
| Google | gemini-1.5-pro | ✅ |
| Google | gemini-1.5-flash | ✅ |
| Google | gemini-2.0-flash | ✅ |

### Ícones por Categoria

| Categoria | Ícone | Cor Preview |
|-----------|-------|-------------|
| Imagem | 🖼️ | - (mostra thumbnail) |
| Áudio | 🎵 | Azul (com player) |
| Vídeo | 🎬 | Roxo |
| Documento | 📄 | Laranja |
| Texto | 📝 | Verde |
| Dados | 📊 | Ciano |
| Desconhecido | ❓ | Vermelho |

### Tarefas

**Frontend:**
- [ ] Criar `lib/media-detector.js`
- [ ] Atualizar `Chat.svelte` para usar detecção automática
- [ ] Atualizar preview de arquivos pendentes com ícones corretos
- [ ] Preview de áudio com player embutido
- [ ] Validar tamanho máximo por tipo
- [ ] Mensagens de erro amigáveis

**Backend:**
- [ ] Adicionar campo `supports_audio` em `ModelCapability`
- [ ] Implementar `ProcessAudioMessage()` em `openai.go`
- [ ] Atualizar `streamChat()` para suportar `input_audio`
- [ ] Detectar capacidade de áudio automaticamente (aprendizado)

---

## Fase U4: Geração de Imagens (DALL-E)

**Objetivo:** Permitir que o assistente gere imagens via DALL-E com alt-text independente para acessibilidade.

### Acessibilidade: Alt-Text Independente

**Problema:** A descrição que vem do modelo que gerou a imagem pode ser enviesada ou superficial ("Gerei uma bela imagem de um gato"). Uma pessoa cega precisa saber **exatamente** o que está na imagem para verificar se foi gerada conforme solicitou.

**Solução:** Após gerar a imagem, usar um **modelo de visão independente** (GPT-4o, Gemini Vision) para analisar a imagem e gerar uma descrição objetiva e detalhada.

```
┌─────────────────────────────────────────────────────────────────┐
│                     FLUXO COM ALT-TEXT INDEPENDENTE              │
├─────────────────────────────────────────────────────────────────┤
│                                                                 │
│  1. Usuário: "Gere uma imagem de um gato astronauta na lua"    │
│                                                                 │
│  2. DALL-E gera imagem                                          │
│     └─► Retorna: imagem em base64                               │
│                                                                 │
│  3. Modelo de visão INDEPENDENTE analisa a imagem              │
│     └─► GPT-4o Vision (ou outro modelo com visão)               │
│     └─► Prompt: "Descreva esta imagem de forma objetiva e      │
│                  detalhada para uma pessoa cega. Inclua:       │
│                  - O que aparece na imagem                      │
│                  - Cores, posições, expressões                  │
│                  - Se há texto, transcreva                      │
│                  - Qualidade e estilo da imagem"                │
│                                                                 │
│  4. Resultado final:                                            │
│     ├─► Imagem gerada                                           │
│     ├─► Alt-text independente e objetivo                        │
│     └─► Usuário cego pode verificar se está correto             │
│                                                                 │
└─────────────────────────────────────────────────────────────────┘
```

### Por que Alt-Text Independente?

| Fonte | Problema | Exemplo |
|-------|----------|---------|
| **DALL-E** | Pode ser vago ou promocional | "Gerei uma imagem incrível do seu pedido" |
| **Modelo de chat** | Pode assumir que funcionou | "Aqui está o gato astronauta que você pediu!" |
| **Visão independente** | Descreve objetivamente | "A imagem mostra um gato laranja usando um traje espacial branco, flutuando sobre a superfície lunar cinza. A Terra aparece ao fundo..." |

### Arquitetura

```
┌─────────────────────────────────────────────────────────────────┐
│                     GERAÇÃO DE IMAGENS                          │
├─────────────────────────────────────────────────────────────────┤
│                                                                 │
│  Usuário: "Gere uma imagem de um gato astronauta"              │
│                                                                 │
│                        │                                        │
│                        ▼                                        │
│  ┌─────────────────────────────────────────────────────────────┐│
│  │ Orquestrador detecta intenção de gerar imagem               ││
│  │ → Delega para Image Generator Agent                         ││
│  └─────────────────────────────────────────────────────────────┘│
│                        │                                        │
│                        ▼                                        │
│  ┌─────────────────────────────────────────────────────────────┐│
│  │ Image Generator Agent                                       ││
│  │ Tools:                                                      ││
│  │   - generate_image(prompt, size, quality, style)           ││
│  │   - edit_image(image, prompt, mask)                        ││
│  │   - create_variation(image, n)                             ││
│  └─────────────────────────────────────────────────────────────┘│
│                        │                                        │
│                        ▼                                        │
│  ┌─────────────────────────────────────────────────────────────┐│
│  │ DALL-E 3 API                                                ││
│  │ POST /v1/images/generations                                 ││
│  │ → Retorna imagem em base64                                  ││
│  └─────────────────────────────────────────────────────────────┘│
│                        │                                        │
│                        ▼                                        │
│  ┌─────────────────────────────────────────────────────────────┐│
│  │ 🔍 MODELO DE VISÃO INDEPENDENTE (GPT-4o Vision)             ││
│  │                                                             ││
│  │ Analisa a imagem gerada e cria alt-text objetivo:           ││
│  │ "A imagem mostra um gato laranja vestindo um traje          ││
│  │  espacial branco, flutuando sobre a superfície lunar..."   ││
│  └─────────────────────────────────────────────────────────────┘│
│                        │                                        │
│                        ▼                                        │
│  ┌─────────────────────────────────────────────────────────────┐│
│  │ Resposta com imagem + alt-text acessível                    ││
│  │ [Imagem gerada] [alt="descrição objetiva"]                  ││
│  │ [Download] [Copiar] [Variação]                              ││
│  └─────────────────────────────────────────────────────────────┘│
│                                                                 │
└─────────────────────────────────────────────────────────────────┘
```

### Image Generator Agent

```go
// internal/agents/image_agent.go

type ImageAgent struct {
    BaseAgent
    Client *openai.Client
}

func NewImageAgent(apiKey, baseURL string) *ImageAgent {
    return &ImageAgent{
        BaseAgent: BaseAgent{
            Name:        "image_generator",
            DisplayName: "Gerador de Imagens",
            Description: "Gera, edita e cria variações de imagens usando DALL-E",
            AgentType:   "internal",
            Model:       "gpt-4o-mini",
            Enabled:     true,
        },
    }
}

func (a *ImageAgent) GetTools() []Tool {
    return []Tool{
        {
            Type: "function",
            Function: ToolFunction{
                Name:        "generate_image",
                Description: "Gera uma nova imagem a partir de uma descrição textual",
                Parameters: map[string]interface{}{
                    "type": "object",
                    "properties": map[string]interface{}{
                        "prompt": map[string]interface{}{
                            "type":        "string",
                            "description": "Descrição detalhada da imagem a ser gerada",
                        },
                        "size": map[string]interface{}{
                            "type":        "string",
                            "enum":        []string{"1024x1024", "1792x1024", "1024x1792"},
                            "description": "Tamanho da imagem (padrão: 1024x1024)",
                        },
                        "quality": map[string]interface{}{
                            "type":        "string",
                            "enum":        []string{"standard", "hd"},
                            "description": "Qualidade da imagem (padrão: standard)",
                        },
                        "style": map[string]interface{}{
                            "type":        "string",
                            "enum":        []string{"vivid", "natural"},
                            "description": "Estilo visual (padrão: vivid)",
                        },
                    },
                    "required": []string{"prompt"},
                },
            },
        },
    }
}

func (a *ImageAgent) ExecuteTool(toolCall ToolCall) (string, error) {
    switch toolCall.Function.Name {
    case "generate_image":
        return a.generateImage(toolCall.Function.Arguments)
    default:
        return "", fmt.Errorf("tool desconhecida: %s", toolCall.Function.Name)
    }
}

func (a *ImageAgent) generateImage(argsJSON string) (string, error) {
    var args struct {
        Prompt  string `json:"prompt"`
        Size    string `json:"size"`
        Quality string `json:"quality"`
        Style   string `json:"style"`
    }
    json.Unmarshal([]byte(argsJSON), &args)
    
    // Defaults
    if args.Size == "" {
        args.Size = "1024x1024"
    }
    if args.Quality == "" {
        args.Quality = "standard"
    }
    if args.Style == "" {
        args.Style = "vivid"
    }
    
    // 1. Chama API DALL-E para gerar a imagem
    resp, err := a.Client.CreateImage(context.Background(), openai.ImageRequest{
        Model:          "dall-e-3",
        Prompt:         args.Prompt,
        Size:           args.Size,
        Quality:        args.Quality,
        Style:          args.Style,
        N:              1,
        ResponseFormat: openai.CreateImageResponseFormatB64JSON,
    })
    
    if err != nil {
        return "", err
    }
    
    imageBase64 := resp.Data[0].B64JSON
    
    // 2. Gera alt-text INDEPENDENTE usando modelo de visão
    altText, err := a.generateIndependentAltText(imageBase64, args.Prompt)
    if err != nil {
        // Se falhar, usa descrição genérica (mas avisa)
        altText = fmt.Sprintf("Imagem gerada a partir do prompt: %s (descrição automática indisponível)", args.Prompt)
    }
    
    // 3. Retorna imagem + alt-text com marcador especial
    // Formato: [GENERATED_IMAGE:alt_text_base64:image_base64]
    altTextB64 := base64.StdEncoding.EncodeToString([]byte(altText))
    return fmt.Sprintf("[GENERATED_IMAGE:%s:%s]", altTextB64, imageBase64), nil
}

// generateIndependentAltText usa um modelo de visão para descrever a imagem
// de forma objetiva e detalhada, independente do modelo que a gerou
func (a *ImageAgent) generateIndependentAltText(imageBase64, originalPrompt string) (string, error) {
    // Prompt cuidadosamente elaborado para acessibilidade
    prompt := `Você é um especialista em acessibilidade criando descrições de imagens para pessoas cegas.

Analise esta imagem e forneça uma descrição OBJETIVA e DETALHADA. A pessoa que pediu esta imagem precisa saber se ela foi gerada corretamente.

Sua descrição deve incluir:
1. O que aparece na imagem (pessoas, objetos, animais, cenário)
2. Cores predominantes
3. Posições e composição
4. Expressões faciais ou emoções (se aplicável)
5. Qualquer texto visível (transcreva)
6. Estilo artístico (fotorealista, cartoon, pintura, etc.)
7. Qualidade geral da imagem

NÃO inclua:
- Opiniões subjetivas ("linda", "incrível")
- Suposições sobre o que deveria estar na imagem
- Referências ao prompt original

Seja factual e objetivo. A pessoa precisa verificar se a imagem corresponde ao que ela solicitou.

O prompt original era: "` + originalPrompt + `"

Descreva o que você VÊ na imagem:`

    // Chama modelo de visão (GPT-4o ou similar)
    messages := []Message{
        {
            Role: "user",
            Content: []interface{}{
                map[string]string{"type": "text", "text": prompt},
                map[string]interface{}{
                    "type": "image_url",
                    "image_url": map[string]string{
                        "url":    "data:image/png;base64," + imageBase64,
                        "detail": "high",
                    },
                },
            },
        },
    }
    
    // Usa modelo de visão configurado (padrão: gpt-4o)
    visionModel := a.getVisionModel() // "gpt-4o" ou configurado
    
    resp, err := a.LLMClient.Chat(context.Background(), visionModel, messages)
    if err != nil {
        return "", err
    }
    
    return resp.Content, nil
}
```

### Exibição no Chat

```svelte
<!-- Chat.svelte - Detecta imagens geradas -->
{#if content.includes('[GENERATED_IMAGE:')}
  {@const imageData = extractGeneratedImage(content)}
  <div class="generated-image" role="figure" aria-labelledby="img-desc-{imageData.id}">
    <img 
      src="data:image/png;base64,{imageData.base64}" 
      alt="{imageData.altText}"
    />
    
    <!-- Descrição visível para todos (acessibilidade) -->
    <details class="image-description">
      <summary>📖 Descrição da imagem</summary>
      <p id="img-desc-{imageData.id}">{imageData.altText}</p>
    </details>
    
    <div class="image-actions">
      <button on:click={() => downloadImage(imageData)} aria-label="Download da imagem">
        💾 Download
      </button>
      <button on:click={() => copyImage(imageData)} aria-label="Copiar imagem">
        📋 Copiar
      </button>
      <button on:click={() => generateVariation(imageData)} aria-label="Gerar variação">
        🔄 Variação
      </button>
    </div>
  </div>
{/if}

<script>
  // Extrai imagem e alt-text do marcador
  // Formato: [GENERATED_IMAGE:alt_text_base64:image_base64]
  function extractGeneratedImage(content) {
    const match = content.match(/\[GENERATED_IMAGE:([^:]+):([^\]]+)\]/);
    if (!match) return null;
    
    const altTextBase64 = match[1];
    const imageBase64 = match[2];
    
    // Decodifica alt-text
    const altText = atob(altTextBase64);
    
    return {
      id: Date.now(),
      base64: imageBase64,
      altText: altText
    };
  }
</script>

<style>
  .generated-image {
    max-width: 512px;
    border-radius: 8px;
    overflow: hidden;
    background: var(--bg-secondary);
  }
  
  .generated-image img {
    width: 100%;
    display: block;
  }
  
  .image-description {
    padding: 0.75rem;
    font-size: 0.9rem;
    color: var(--text-secondary);
    border-top: 1px solid var(--border);
  }
  
  .image-description summary {
    cursor: pointer;
    font-weight: 500;
  }
  
  .image-description p {
    margin-top: 0.5rem;
    line-height: 1.5;
  }
  
  .image-actions {
    display: flex;
    gap: 0.5rem;
    padding: 0.5rem 0.75rem;
    border-top: 1px solid var(--border);
  }
</style>
```

### Acessibilidade da Imagem Gerada

| Elemento | Implementação |
|----------|---------------|
| **Alt-text** | Descrição objetiva gerada por modelo de visão independente |
| **Descrição visível** | Expandível com `<details>` para todos verem |
| **Aria labels** | Botões com labels descritivos |
| **Role figure** | Semântica correta para leitores de tela |
| **Foco** | Botões navegáveis por teclado |

### Tamanhos e Preços DALL-E 3

| Tamanho | Aspecto | Quality Standard | Quality HD |
|---------|---------|------------------|------------|
| 1024x1024 | Quadrado | $0.040 | $0.080 |
| 1792x1024 | Paisagem | $0.080 | $0.120 |
| 1024x1792 | Retrato | $0.080 | $0.120 |

### Tarefas

**Backend:**
- [ ] Criar `internal/agents/image_agent.go`
- [ ] Implementar `generateImage()` usando API DALL-E
- [ ] Implementar `generateIndependentAltText()` usando modelo de visão
- [ ] Configurar modelo de visão para alt-text (padrão: gpt-4o)
- [ ] Registrar no AgentRegistry
- [ ] Adicionar configuração de agente no banco

**Frontend:**
- [ ] Detectar marcador `[GENERATED_IMAGE:alt:img]` no conteúdo
- [ ] Decodificar alt-text do base64
- [ ] Exibir imagem com alt-text acessível
- [ ] Descrição expandível visível para todos
- [ ] Botões com aria-labels
- [ ] Botão de download
- [ ] Botão de copiar para clipboard
- [ ] Persistir imagem + alt-text no histórico

---

## Fluxo Unificado de Mídia

### Diagrama Completo

```
┌─────────────────────────────────────────────────────────────────┐
│                     ENTRADA DE MÍDIA                             │
├─────────────────────────────────────────────────────────────────┤
│                                                                 │
│   ┌──────────────┐  ┌──────────────┐  ┌──────────────────────┐  │
│   │ 📎 Menu      │  │ Drag & Drop  │  │ Ctrl+V (Colar)       │  │
│   │    Arquivo   │  │              │  │                      │  │
│   └──────┬───────┘  └──────┬───────┘  └──────────┬───────────┘  │
│          │                 │                      │             │
│          └─────────────────┼──────────────────────┘             │
│                            │                                    │
│                            ▼                                    │
│  ┌─────────────────────────────────────────────────────────────┐│
│  │              detectMediaType(file)                          ││
│  │                                                             ││
│  │  Analisa: MIME type + Extensão                              ││
│  │  Retorna: { category, action, icon }                        ││
│  └─────────────────────────────────────────────────────────────┘│
│                            │                                    │
│          ┌─────────────────┼─────────────────┐                  │
│          │                 │                 │                  │
│          ▼                 ▼                 ▼                  │
│    ┌───────────┐    ┌───────────┐    ┌───────────────┐         │
│    │  Imagem   │    │   Áudio   │    │   Documento   │         │
│    │  🖼️       │    │   🎵      │    │   📄          │         │
│    └─────┬─────┘    └─────┬─────┘    └───────┬───────┘         │
│          │                │                  │                  │
│          ▼                ▼                  ▼                  │
│    ┌───────────┐    ┌───────────┐    ┌───────────────┐         │
│    │ Preview   │    │ Preview   │    │ Preview       │         │
│    │ thumbnail │    │ + player  │    │ ícone + nome  │         │
│    └───────────┘    └───────────┘    └───────────────┘         │
│                                                                 │
└─────────────────────────────────────────────────────────────────┘
                            │
                            ▼
┌─────────────────────────────────────────────────────────────────┐
│                     ENVIO DA MENSAGEM                            │
├─────────────────────────────────────────────────────────────────┤
│                                                                 │
│  Usuário clica enviar (ou Enter)                               │
│                                                                 │
│  Para cada mídia pendente:                                      │
│                                                                 │
│  ┌───────────────────────────────────────────────────────────┐  │
│  │  Imagem →  Modelo suporta visão?                          │  │
│  │            SIM: envia image_url                           │  │
│  │            NÃO: usa modelo auxiliar + descrição           │  │
│  ├───────────────────────────────────────────────────────────┤  │
│  │  Áudio →   Modelo suporta áudio?                          │  │
│  │            SIM: envia input_audio                         │  │
│  │            NÃO: transcreve com Whisper                    │  │
│  ├───────────────────────────────────────────────────────────┤  │
│  │  Documento → Extrai texto (PDF, DOC, TXT)                 │  │
│  │              Adiciona como contexto na mensagem           │  │
│  └───────────────────────────────────────────────────────────┘  │
│                                                                 │
└─────────────────────────────────────────────────────────────────┘
```

---

## Preview Unificado de Mídia

### Componente MediaPreview

```svelte
<!-- components/MediaPreview.svelte -->
<script>
  export let items = []; // Array de { file, category, icon, preview }
  export let onRemove = () => {};
  
  function formatSize(bytes) {
    if (bytes < 1024) return bytes + ' B';
    if (bytes < 1024 * 1024) return (bytes / 1024).toFixed(1) + ' KB';
    return (bytes / (1024 * 1024)).toFixed(1) + ' MB';
  }
</script>

<div class="media-preview-container">
  {#each items as item, index}
    <div class="media-item {item.category}">
      {#if item.category === 'image'}
        <img src={item.preview} alt={item.file.name} />
      {:else if item.category === 'audio'}
        <div class="audio-preview">
          <span class="icon">🎵</span>
          <audio controls src={item.preview} />
        </div>
      {:else if item.category === 'video'}
        <video src={item.preview} controls muted />
      {:else}
        <div class="file-preview">
          <span class="icon">{item.icon}</span>
          <span class="name">{item.file.name}</span>
        </div>
      {/if}
      
      <div class="meta">
        <span class="size">{formatSize(item.file.size)}</span>
      </div>
      
      <button class="remove" on:click={() => onRemove(index)} aria-label="Remover">
        ✕
      </button>
    </div>
  {/each}
</div>

<style>
  .media-preview-container {
    display: flex;
    flex-wrap: wrap;
    gap: 0.5rem;
    padding: 0.5rem;
    background: var(--bg-secondary);
    border-radius: 0.5rem;
    margin-bottom: 0.5rem;
  }
  
  .media-item {
    position: relative;
    border-radius: 0.25rem;
    overflow: hidden;
    background: var(--bg-tertiary);
  }
  
  .media-item.image img {
    max-width: 150px;
    max-height: 100px;
    object-fit: cover;
  }
  
  .media-item.audio {
    padding: 0.5rem;
    display: flex;
    align-items: center;
    gap: 0.5rem;
  }
  
  .media-item.audio audio {
    height: 32px;
    max-width: 200px;
  }
  
  .file-preview {
    padding: 0.75rem 1rem;
    display: flex;
    align-items: center;
    gap: 0.5rem;
  }
  
  .icon {
    font-size: 1.5rem;
  }
  
  .remove {
    position: absolute;
    top: 2px;
    right: 2px;
    width: 20px;
    height: 20px;
    border-radius: 50%;
    background: rgba(0, 0, 0, 0.6);
    color: white;
    border: none;
    cursor: pointer;
    font-size: 12px;
    display: flex;
    align-items: center;
    justify-content: center;
  }
</style>
```

---

## Limitações e Validações

### Limites de Tamanho

| Tipo | Limite Frontend | Limite API |
|------|-----------------|------------|
| Imagem | 20 MB | 20 MB (OpenAI) |
| Áudio | 25 MB | 25 MB (Whisper) |
| Documento | 10 MB | - |
| Total por mensagem | 50 MB | - |

### Validações

```javascript
// lib/media-validator.js
export const LIMITS = {
  image: { maxSize: 20 * 1024 * 1024, maxCount: 10 },
  audio: { maxSize: 25 * 1024 * 1024, maxCount: 1 },
  document: { maxSize: 10 * 1024 * 1024, maxCount: 5 },
  total: { maxSize: 50 * 1024 * 1024 }
};

export function validateMedia(file, existingMedia = []) {
  const { category } = detectMediaType(file);
  const limits = LIMITS[category] || LIMITS.document;
  
  // Tamanho individual
  if (file.size > limits.maxSize) {
    return { valid: false, error: `Arquivo muito grande (máx: ${formatSize(limits.maxSize)})` };
  }
  
  // Contagem por tipo
  const sameTypeCount = existingMedia.filter(m => m.category === category).length;
  if (sameTypeCount >= limits.maxCount) {
    return { valid: false, error: `Máximo de ${limits.maxCount} arquivo(s) deste tipo` };
  }
  
  // Total
  const totalSize = existingMedia.reduce((sum, m) => sum + m.file.size, 0) + file.size;
  if (totalSize > LIMITS.total.maxSize) {
    return { valid: false, error: `Tamanho total excede ${formatSize(LIMITS.total.maxSize)}` };
  }
  
  return { valid: true };
}
```

---

## Cronograma

| Fase | Estimativa | Prioridade |
|------|------------|------------|
| **U1: Menu Unificado** | 0.5 dia | Alta |
| **U2: Detecção Automática + Áudio** | 2 dias | Alta |
| **U3: Modos de Gravação** | 1 dia | Alta |
| **U4: DALL-E** | 2 dias | Média |

**Total estimado:** 5-6 dias

---

## Ordem de Implementação

```
1. U1 + U2 (juntos)
   └─► Menu simplificado (1 opção de arquivo)
   └─► Detecção automática de tipo (imagem, áudio, documento, etc.)
   └─► Processamento de áudio (transcrição ou envio nativo)
   └─► Preview unificado com player de áudio
   
2. U3: Modos de Gravação
   └─► 4 modos: PTT, Toggle, VAD Silêncio, VAD Atividade
   └─► Submenu de seleção de modo
   └─► Ctrl+Shift+A sempre usa VAD Atividade (full auto)
   
3. U4: DALL-E
   └─► Agente de geração de imagens
```

---

## Referências

- [OpenAI Audio API](https://platform.openai.com/docs/guides/audio)
- [OpenAI DALL-E API](https://platform.openai.com/docs/guides/images)
- [Gemini Multimodal](https://ai.google.dev/gemini-api/docs/audio)

