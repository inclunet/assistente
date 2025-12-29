# Roadmap Multimodal

> **Status Geral:** 🟡 Em andamento - Frontend quase completo, Backend pendente

Este documento detalha o plano de implementação para suporte multimodal completo no assistente.

---

## Resumo Rápido de Status

| Fase | Frontend | Backend | Observação |
|------|----------|---------|------------|
| **Design System** | ✅ | N/A | ContextMenu, ActionBar, InteractiveContainer |
| **M0: Infraestrutura** | ✅ | ✅ | VoiceButton, MediaMenu, ImageModelPicker, capacidades |
| **M1: Imagens** | ✅ | ✅ | Upload, preview, streaming, fallback, aprendizado |
| **UX: Mensagens** | ✅ | N/A | Hover actions, menu contexto, TTS, código, tabelas |
| **M2: DALL-E** | 🔮 | 🔮 | Geração de imagens (futuro) |
| **M3: Áudio prompt** | 🔮 | 🔮 | Upload áudio (futuro) |
| **M4: Realtime** | 🔮 | 🔮 | WebSocket streaming (futuro) |

**🎉 Fases DS, M0, M1 e UX estão 100% concluídas!**

---

## Visão Geral

O termo "multimodal" abrange diferentes tipos de entrada/saída:

| Tipo | Entrada | Saída | LiteLLM suporta? |
|------|---------|-------|------------------|
| **Texto** | ✅ Texto | ✅ Texto | ✅ Sim |
| **Imagem** | ✅ Texto + Imagem | ✅ Texto | ✅ Sim (HTTP) |
| **Áudio Realtime** | ✅ Áudio streaming | ✅ Áudio streaming | ❌ Não (WebSocket) |

---

## Arquitetura de Transporte

```
┌─────────────────────────────────────────────────────────────────────┐
│                        TIPOS DE MULTIMODAL                          │
├─────────────────────────────────┬───────────────────────────────────┤
│      Imagem + Texto (Vision)    │      Áudio Streaming (Realtime)   │
├─────────────────────────────────┼───────────────────────────────────┤
│                                 │                                   │
│  ┌────────┐    HTTP    ┌─────┐  │  ┌────────┐  WebSocket  ┌──────┐  │
│  │Frontend│ ─────────► │Lite │  │  │Frontend│ ═══════════►│OpenAI│  │
│  └────────┘            │LLM  │  │  └────────┘             │API   │  │
│                        └──┬──┘  │                         └──────┘  │
│                           │     │                                   │
│                    ┌──────┴───┐ │  ❌ LiteLLM não participa         │
│                    │ OpenAI   │ │     (não suporta WebSocket)       │
│                    │ Google   │ │                                   │
│                    │ Anthropic│ │                                   │
│                    └──────────┘ │                                   │
│                                 │                                   │
│  ✅ Pode implementar agora     │  🔮 Implementação futura           │
│                                 │                                   │
└─────────────────────────────────┴───────────────────────────────────┘
```

---

## Arquitetura do Botão de Mídia

O botão PTT atual será expandido para ter **dupla função**:

### Comportamento do Botão

```
┌─────────────────────────────────────────────────────────────────┐
│                      BOTÃO DE MÍDIA (🎤)                        │
├─────────────────────────────────────────────────────────────────┤
│                                                                 │
│   SEGURAR (>300ms)                                              │
│   └─► PTT: Grava voz e envia (comportamento atual)             │
│                                                                 │
│   CLICK RÁPIDO (<300ms)                                         │
│   └─► Abre menu de mídia:                                       │
│       ┌─────────────────────────────┐                           │
│       │ 📷 Enviar imagem            │                           │
│       │ 🎤 Enviar arquivo de áudio  │                           │
│       │ 📄 Enviar documento         │                           │
│       │ 📹 Enviar vídeo             │                           │
│       │ 📸 Capturar tela            │                           │
│       │ 📷 Capturar webcam          │                           │
│       └─────────────────────────────┘                           │
│                                                                 │
└─────────────────────────────────────────────────────────────────┘
```

### Implementação do Duplo Comportamento

```javascript
// VoiceButton.svelte
let pressTimer = null;
let isPTTMode = false;

function handlePointerDown(event) {
  pressTimer = setTimeout(() => {
    // Segurou mais de 300ms → Modo PTT
    isPTTMode = true;
    startRecording();
  }, 300);
}

function handlePointerUp(event) {
  clearTimeout(pressTimer);
  
  if (isPTTMode) {
    // Estava gravando → Finaliza PTT
    stopRecording();
    isPTTMode = false;
  } else {
    // Click rápido → Abre menu de mídia
    openMediaMenu();
  }
}
```

### Menu de Mídia

```svelte
<!-- MediaMenu.svelte -->
<div class="media-menu" role="menu" aria-label="Opções de mídia">
  <button role="menuitem" on:click={selectImage}>
    📷 Enviar imagem
  </button>
  <button role="menuitem" on:click={selectAudio}>
    🎤 Enviar arquivo de áudio
  </button>
  <button role="menuitem" on:click={selectDocument}>
    📄 Enviar documento
  </button>
  <button role="menuitem" on:click={captureScreen}>
    📸 Capturar tela
  </button>
</div>
```

---

## Arquitetura de Modelos Auxiliares

Quando o modelo de chat principal não suporta um tipo de mídia, usamos um **modelo auxiliar** específico.

### Toolbar com Modelos Auxiliares

```
┌─────────────────────────────────────────────────────────────────────┐
│                            TOOLBAR                                   │
├─────────────────────────────────────────────────────────────────────┤
│                                                                     │
│  [📦 Chat: gpt-3.5-turbo ▼]     ← Modelo principal (texto)         │
│                                                                     │
│  [🖼️ Imagem: gpt-4o ▼]          ← Modelo para processar imagens    │
│    └─ Só aparece se modelo chat não suporta imagem                 │
│                                                                     │
│  [🔊 Voz: alloy ▼]              ← Voz TTS (já existe)              │
│                                                                     │
│  [🎤 Transcrição: Whisper ▼]    ← STT (já existe)                  │
│                                                                     │
└─────────────────────────────────────────────────────────────────────┘
```

### Lógica de Seleção de Modelo

```
Usuário envia mensagem
        │
        ▼
┌───────────────────────┐
│ Tem mídia anexada?    │
└───────────────────────┘
        │
   ┌────┴────┐
   │         │
  NÃO       SIM
   │         │
   ▼         ▼
┌───────┐  ┌────────────────────────┐
│ Usa   │  │ Qual tipo de mídia?    │
│ modelo│  └────────────────────────┘
│ chat  │         │
└───────┘    ┌────┼────┬────┐
             │    │    │    │
          Imagem Audio Doc Video
             │    │    │    │
             ▼    ▼    ▼    ▼
        ┌─────────────────────────┐
        │ Modelo chat suporta?    │
        └─────────────────────────┘
                  │
           ┌──────┴──────┐
           │             │
          SIM           NÃO
           │             │
           ▼             ▼
      ┌─────────┐   ┌─────────────────────┐
      │ Usa     │   │ Modelo auxiliar     │
      │ modelo  │   │ configurado?        │
      │ chat    │   └─────────────────────┘
      └─────────┘         │
                    ┌─────┴─────┐
                    │           │
                   SIM         NÃO
                    │           │
                    ▼           ▼
              ┌─────────┐  ┌──────────────────┐
              │ Usa     │  │ Erro amigável:   │
              │ modelo  │  │ "Configure um    │
              │ auxiliar│  │ modelo de imagem │
              └─────────┘  │ nas preferências"│
                           └──────────────────┘
```

### Configuração de Modelos

```go
// internal/config/models.go
type ModelCapabilities struct {
    SupportsImage    bool
    SupportsAudio    bool
    SupportsVideo    bool
    SupportsDocument bool
}

type ChatConfig struct {
    // Modelo principal
    ChatModel string
    
    // Modelos auxiliares (opcional)
    ImageModel    string  // Usado se ChatModel não suporta imagem
    AudioModel    string  // Usado se ChatModel não suporta áudio
    DocumentModel string  // Usado se ChatModel não suporta documento
}
```

### Fluxo de Processamento com Modelo Auxiliar

```
┌─────────────────────────────────────────────────────────────────┐
│ Usuário: "O que tem nesta foto?" + 🖼️ imagem.jpg               │
└─────────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────────────┐
│ Modelo chat: gpt-3.5-turbo (não suporta imagem)                │
│ Modelo imagem: gpt-4o (suporta)                                 │
└─────────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────────────┐
│ 1. Envia para gpt-4o: "Descreva esta imagem" + 🖼️              │
│ 2. gpt-4o responde: "A imagem mostra um gato laranja..."       │
│ 3. Envia para gpt-3.5-turbo:                                    │
│    "O usuário perguntou sobre esta imagem.                      │
│     Descrição da imagem: A imagem mostra um gato laranja...     │
│     Pergunta do usuário: O que tem nesta foto?"                 │
│ 4. gpt-3.5-turbo responde normalmente                           │
└─────────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────────────┐
│ Assistente: "Na foto você compartilhou há um gato laranja       │
│ deitado em um sofá azul. Ele parece estar dormindo..."          │
└─────────────────────────────────────────────────────────────────┘
```

### Vantagens desta Arquitetura

| Aspecto | Benefício |
|---------|-----------|
| **Economia** | Usa modelos baratos para texto, premium só para mídia |
| **Flexibilidade** | Usuário escolhe qual modelo usar para cada tipo |
| **Compatibilidade** | Qualquer modelo de chat funciona com mídia |
| **Transparência** | Processo é invisível para o usuário |

### Configurações Sugeridas

| Cenário | Modelo Chat | Modelo Imagem | Custo |
|---------|-------------|---------------|-------|
| **Econômico** | gpt-3.5-turbo | gpt-4o-mini | 💰 Baixo |
| **Balanceado** | gpt-4o-mini | gpt-4o | 💰💰 Médio |
| **Premium** | gpt-4o | gpt-4o | 💰💰💰 Alto |
| **Local** | llama3 (Ollama) | gpt-4o | 💰 Só paga imagem |

---

## Fase M1: Imagens no Chat (Vision)

**Status:** ✅ Concluído

**Objetivo:** Permitir enviar imagens junto com mensagens de texto.

### Funcionalidades

- [x] Upload de imagem no chat
- [x] Colar imagem (Ctrl+V)
- [x] Arrastar e soltar imagem
- [x] Preview da imagem antes de enviar
- [x] Suporte a múltiplas imagens por mensagem
- [x] Captura de tela (screenshot)
- [x] Captura de webcam

### Modelos compatíveis

| Provedor | Modelo | Suporta imagem? |
|----------|--------|-----------------|
| OpenAI | gpt-4o | ✅ |
| OpenAI | gpt-4o-mini | ✅ |
| OpenAI | gpt-4-turbo | ✅ |
| Google | gemini-1.5-pro | ✅ |
| Google | gemini-1.5-flash | ✅ |
| Anthropic | claude-3.5-sonnet | ✅ |
| Anthropic | claude-3-opus | ✅ |

### Implementação técnica

```javascript
// Frontend - preparar mensagem com imagem
const message = {
  role: "user",
  content: [
    { type: "text", text: userMessage },
    { 
      type: "image_url", 
      image_url: { 
        url: `data:image/jpeg;base64,${base64Image}`,
        detail: "auto"  // ou "low", "high"
      }
    }
  ]
};
```

```go
// Backend - já suportado pelo LiteLLM
// Apenas repassar a mensagem com o formato correto
```

### UI proposta

```
┌─────────────────────────────────────────────────────────────────────┐
│                              TOOLBAR                                 │
├─────────────────────────────────────────────────────────────────────┤
│ [📦 Chat ▼] [🖼️ Imagem ▼] [🔊 Voz ▼] [🎤 STT ▼] [⚙️] [➕]          │
│      │            │                                                 │
│      │            └─ Só aparece se modelo chat não suporta imagem  │
│      └─ Modelo principal                                            │
└─────────────────────────────────────────────────────────────────────┘

┌─────────────────────────────────────────────────────┐
│                      Chat                           │
├─────────────────────────────────────────────────────┤
│                                                     │
│  [Você]                                             │
│  O que tem nesta imagem?                            │
│  ┌─────────────┐                                    │
│  │   🖼️        │  ← Preview da imagem              │
│  │  imagem.jpg │                                    │
│  │      ✕      │  ← Botão para remover             │
│  └─────────────┘                                    │
│                                                     │
│  [Assistente]                                       │
│  A imagem mostra um gato dormindo...                │
│                                                     │
├─────────────────────────────────────────────────────┤
│ [Digite sua mensagem...                    ] [🎤/➤] │
│                                                  │   │
│            Segurar = PTT │ Click = Menu de mídia ───┘
└─────────────────────────────────────────────────────┘

Menu de mídia (ao clicar no botão):
┌─────────────────────────┐
│ 📷 Enviar imagem        │
│ 🎤 Enviar áudio         │
│ 📄 Enviar documento     │
│ 📸 Capturar tela        │
└─────────────────────────┘
```

### Tarefas

#### Botão de Mídia com Dupla Função
- [x] Modificar `VoiceButton.svelte` para detectar click vs segurar
- [x] Timer de 300ms para diferenciar PTT de click
- [x] Componente `MediaMenu.svelte` (menu popup)
- [x] Navegação por teclado no menu (ArrowUp/Down, Enter, Escape)
- [x] Acessibilidade (role="menu", aria-label)

#### Modelos Auxiliares
- [x] Criar `ImageModelPicker.svelte` (similar ao ModelPicker)
- [x] Adicionar campo `imageModel` na configuração do backend
- [x] Lógica para detectar se modelo chat suporta imagem (ModelCapability)
- [x] Mostrar/esconder picker de modelo de imagem na toolbar
- [x] `GenerateImageDescription()` já funciona para alt text

#### Upload de Imagem
- [x] Handler de paste (Ctrl+V) em `Chat.svelte`
- [x] Handler de drag-and-drop em `Chat.svelte`
- [x] Conversão para base64
- [x] Preview com opção de remover (`pendingMedia`)
- [x] Exibir imagens no histórico (`message-media`)
- [x] Acessibilidade (alt text gerado automaticamente via LLM)
- [x] Estrutura `ContentPart` com `image_url` já existe no backend
- [x] Usar imagens no streaming de chat (`streamChat`)
- [x] Fallback para modelo auxiliar se modelo principal não suportar visão
- [x] Aprendizado automático de capacidades (detecta erros de visão)
- [x] Persistência de mídias no banco SQLite (`ChatMessage.Media`)
- [x] Modo WAL ativado para melhor performance com arquivos grandes

---

## Fase M2: Geração de Imagens

**Status:** 🟡 Pode ser implementado

**Objetivo:** Permitir que o assistente gere imagens via DALL-E ou similar.

### Funcionalidades

- [ ] Gerar imagem a partir de descrição
- [ ] Editar imagem existente
- [ ] Variações de imagem
- [ ] Diferentes tamanhos e qualidades

### Implementação

```go
// Novo agente: ImageGeneratorAgent
type ImageGeneratorAgent struct {
    client *openai.Client
}

func (a *ImageGeneratorAgent) GenerateImage(prompt string) (string, error) {
    resp, err := a.client.CreateImage(ctx, openai.ImageRequest{
        Prompt:         prompt,
        Model:          "dall-e-3",
        N:              1,
        Size:           "1024x1024",
        ResponseFormat: openai.CreateImageResponseFormatB64JSON,
    })
    return resp.Data[0].B64JSON, err
}
```

### Tarefas

- [ ] `ImageGeneratorAgent` no backend
- [ ] Tool para chamar geração de imagem
- [ ] Exibir imagem gerada no chat
- [ ] Opções de tamanho/qualidade
- [ ] Download da imagem

---

## Fase M3: Áudio no Prompt (não Realtime)

**Status:** 🟡 Pode ser implementado (extensão do que já temos)

**Objetivo:** Enviar áudio pré-gravado junto com texto.

> Diferente de Realtime (streaming). Aqui é upload de arquivo de áudio.

### Funcionalidades

- [ ] Upload de arquivo de áudio (.mp3, .wav, .m4a)
- [ ] Gravar áudio (como PTT, mas salva arquivo)
- [ ] Transcrição automática antes de enviar (Whisper)
- [ ] Opção de enviar áudio direto para modelos que suportam

### Modelos compatíveis

| Provedor | Modelo | Áudio no prompt? |
|----------|--------|------------------|
| OpenAI | gpt-4o | ✅ |
| OpenAI | gpt-4o-mini | ✅ |
| Google | gemini-1.5-pro | ✅ |

### Fluxo

```
Opção A: Transcrever primeiro (compatível com todos os modelos)
┌─────────┐     ┌─────────┐     ┌─────────┐
│ Áudio   │ ──► │ Whisper │ ──► │ Texto   │ ──► LLM
└─────────┘     └─────────┘     └─────────┘

Opção B: Enviar áudio direto (só modelos compatíveis)
┌─────────┐                     ┌─────────┐
│ Áudio   │ ──────────────────► │ GPT-4o  │
└─────────┘                     └─────────┘
```

### Tarefas

- [ ] Componente `AudioUpload.svelte`
- [ ] Detecção de modelo compatível
- [ ] Toggle: transcrever vs enviar direto
- [ ] Conversão de formato se necessário

---

## Fase M4: Áudio Realtime (WebSocket)

**Status:** 🔮 Futuro - requer implementação significativa

**Objetivo:** Conversação por voz em tempo real com o modelo.

> ⚠️ **Não passa pelo LiteLLM** - requer cliente WebSocket próprio.

### Pré-requisitos

- [ ] Chave API OpenAI com acesso a Realtime
- [ ] (Opcional) Chave API Google para Gemini Live

### Provedores

| Provedor | API | Vozes | Status |
|----------|-----|-------|--------|
| OpenAI | Realtime API | 8 hardcoded | 🎯 Prioridade |
| Google | Gemini Live | Google Cloud TTS | 🔮 Futuro |

### Vozes OpenAI Realtime (hardcoded)

| ID | Nome | Gênero | Descrição |
|----|------|--------|-----------|
| alloy | Alloy | Neutro | Balanceada |
| echo | Echo | Masculino | Profunda |
| shimmer | Shimmer | Feminino | Expressiva |
| ash | Ash | Masculino | Nova geração |
| ballad | Ballad | Feminino | Suave |
| coral | Coral | Feminino | Energética |
| sage | Sage | Neutro | Calma |
| verse | Verse | Masculino | Narrativa |

> ❌ Não existe API para listar vozes. Lista atualizada manualmente.

### Arquitetura proposta

```
┌─────────────────────────────────────────────────────────────────┐
│                    CAMADA DE AGENTES (inalterada)               │
│   AgentManager → Agent → Tools → Funções Go → Resultado        │
└─────────────────────────────────────────────────────────────────┘
                              ▲
                              │ mesma interface interna
                              │
┌─────────────────────────────────────────────────────────────────┐
│                    ADAPTER DE TRANSPORTE (~100 LOC)             │
│   Converte eventos WebSocket ↔ formato atual dos agentes       │
└─────────────────────────────────────────────────────────────────┘
                              ▲
                              │
              ┌───────────────┴───────────────┐
              │                               │
       Chat API (HTTP)              Realtime API (WebSocket)
       modo texto/STT+TTS           modo multimodal
```

### Mudanças na UI

**Toolbar quando modelo Realtime ativo:**
```
ANTES (texto/STT+TTS):
[📦 Modelo ▼] [🎤 Transcrição ▼] [🔊 Voz ▼] [⚙️] [➕]

DEPOIS (Realtime):
[📦 Modelo ▼] [🔊 Voz Realtime ▼] [⚙️] [➕]
              ↑
        Só vozes do modelo (esconde STT picker)
```

**Botão de voz muda comportamento:**

| Modo | Comportamento |
|------|---------------|
| Texto/STT+TTS | PTT: segura para gravar |
| Realtime | Toggle: clica para iniciar/parar conversa |

### Configuração de capacidades do modelo

```go
type ModelConfig struct {
    Name           string
    AudioInput     bool     // Suporta enviar áudio
    AudioOutput    bool     // Suporta receber áudio (Realtime)
    RealtimeVoices []string // Vozes disponíveis
}
```

- [ ] Checkboxes nas configurações do modelo
- [ ] Fallback gracioso ao detectar erro da API
- [ ] Desmarcar checkbox automaticamente se API rejeitar

### Tarefas técnicas

**Backend (Go):**
- [ ] Cliente WebSocket para OpenAI Realtime API
- [ ] `RealtimeToolAdapter` (~100 linhas)
- [ ] Gerenciamento de sessão
- [ ] Proxy de áudio WebSocket ↔ Frontend

**Frontend (Svelte):**
- [ ] Componente `RealtimeSession.svelte`
- [ ] Captura de áudio contínua
- [ ] Reprodução de áudio em streaming
- [ ] Detecção de modelo Realtime → mudar UI

### Fallbacks

| Cenário | Comportamento |
|---------|---------------|
| Modelo não suporta áudio | Usa STT + TTS (modo atual) |
| WebSocket falha | Fallback para modo texto |
| Erro de API | Desmarca checkbox, notifica usuário |

---

## Ordem de Implementação Recomendada

```
┌─────────────────────────────────────────────────────────────────┐
│                                                                 │
│   M0: Botão de Mídia ──────► M1: Imagens no Chat               │
│   (base para tudo)              (Vision)                        │
│                                      │                          │
│                                      ▼                          │
│                         M2: Geração de Imagens (DALL-E)        │
│                                      │                          │
│                                      ▼                          │
│                         M3: Áudio no Prompt (Upload)           │
│                                      │                          │
│                                      ▼                          │
│                         M4: Áudio Realtime (WebSocket)         │
│                                                                 │
└─────────────────────────────────────────────────────────────────┘

Prioridade sugerida:
1. M0 - Botão de Mídia + Menu (base para todas as outras fases)
2. M1 - Imagens no Chat + Modelos Auxiliares (mais pedido)
3. M3 - Áudio no Prompt (extensão natural do que já temos)
4. M2 - Geração de Imagens (novo agente, mas simples)
5. M4 - Áudio Realtime (mais complexo, deixar por último)
```

---

## Fase M0: Botão de Mídia e Infraestrutura Base

**Status:** ✅ Concluído

**Objetivo:** Criar a infraestrutura base para envio de mídias.

### Tarefas

**Frontend:**
- [x] Modificar `VoiceButton.svelte` para dupla função (PTT + Menu)
- [x] Criar componente `MediaMenu.svelte`
- [x] Criar componente `ImageModelPicker.svelte`
- [x] Integrar ImageModelPicker no Settings.svelte

**Backend:**
- [x] Adicionar campo `imageModel` na configuração (`config.go`)
- [x] Funções `GetImageModel()`, `SetImageModel()`, `HasImages()` (`openai.go`)
- [x] Tabela `ModelCapability` com capacidades dinâmicas (`database.go`)
- [x] Funções para capacidades: `SetModelVisionSupport()`, `ModelSupportsVision()`, etc.

### Componentes Criados

| Componente | Descrição |
|------------|-----------|
| `MediaMenu.svelte` | Menu popup com opções: imagem, áudio, documento, screenshot, webcam |
| `ImageModelPicker.svelte` | Seletor de modelo com visão, usa capacidades dinâmicas do banco |
| `VoiceButton.svelte` | Agora com dupla função: segurar >300ms = PTT, click rápido = menu |
| `Chat.svelte` | Atualizado com handlers de mídia e preview de arquivos pendentes |

### Sistema de Capacidades Dinâmicas (Aprendizado Sob Demanda)

Em vez de hardcode ou heurísticas, o sistema **aprende** capacidades conforme o uso:

| Tabela | Campos | Descrição |
|--------|--------|-----------|
| `model_capabilities` | `model_name`, `supports_vision`, `supports_audio`, `supports_tools`, etc. | Capacidades aprendidas |

**Princípios:**
- **Zero heurísticas** - Nomes de modelos mudam, versões novas surgem
- **Aprendizado orgânico** - Sistema aprende conforme usuário usa
- **Desconhecido é OK** - Modelos não testados podem ser usados (vai descobrir se funciona)

**Fluxo de aprendizado:**
1. Usuário envia imagem com modelo X (status: `unknown`)
2. Se API processa com sucesso → marca `supports_vision = true`
3. Se API retorna erro de imagem → marca `supports_vision = false`
4. Próxima vez que usar modelo X, já sabe a capacidade

**Estados no frontend:**
| Status | Exibição | Descrição |
|--------|----------|-----------|
| `unknown` | (vazio) | Nunca testado |
| `supported` | ✓ Visão | Confirmado que funciona |
| `not_supported` | ✗ Sem visão | Confirmado que não funciona |
| `error` | ⚠️ Erro | Último uso falhou |

### Comportamento do VoiceButton

| Ação | Resultado |
|------|-----------|
| **Segurar (>300ms)** | Inicia PTT (comportamento existente) |
| **Click rápido (<300ms)** | Abre menu de mídia |
| **Espaço (teclado)** | PTT enquanto segura |
| **Enter (teclado)** | Abre menu de mídia |

### Eventos do VoiceButton

| Evento | Payload | Descrição |
|--------|---------|-----------|
| `transcript` | `{ text }` | Transcrição de voz |
| `media` | `{ type, files }` | Mídia selecionada (imagem, áudio, etc.) |
| `mediaerror` | `{ type, error }` | Erro ao capturar mídia |

---

## Design System: Componentes Base Reutilizáveis

Antes de implementar features específicas, criar componentes base que serão reutilizados em todo o sistema.

### DS1: ContextMenu (Menu de Contexto)

Componente genérico de menu de contexto, reutilizável em qualquer parte do sistema.

**Uso previsto:**
- Mensagens do chat
- Grids de memória, FAQ, agentes
- Campo de texto de entrada
- Células de tabela
- Blocos de código
- Qualquer elemento que precise de ações contextuais

**API do componente:**

```svelte
<!-- ContextMenu.svelte - Componente base -->
<script>
  export let items = [];       // Array de itens do menu
  export let x = 0;            // Posição X
  export let y = 0;            // Posição Y
  export let visible = false;  // Mostrar/esconder

  // Eventos: on:select, on:close
</script>
```

**Estrutura de item do menu:**

```typescript
interface MenuItem {
  id: string;              // Identificador único
  label: string;           // Texto exibido
  icon?: string;           // Emoji ou ícone
  shortcut?: string;       // Atalho (ex: "Ctrl+C")
  disabled?: boolean;      // Desabilitado
  separator?: boolean;     // É um separador (ignora outros campos)
  danger?: boolean;        // Estilo de ação perigosa (vermelho)
  submenu?: MenuItem[];    // Submenu (opcional)
}
```

**Exemplo de uso:**

```svelte
<script>
  import ContextMenu from './ContextMenu.svelte';
  
  let menuVisible = false;
  let menuX = 0;
  let menuY = 0;
  
  const menuItems = [
    { id: 'copy', label: 'Copiar', icon: '📋', shortcut: 'Ctrl+C' },
    { id: 'paste', label: 'Colar', icon: '📋', shortcut: 'Ctrl+V' },
    { separator: true },
    { id: 'delete', label: 'Excluir', icon: '🗑️', danger: true }
  ];
  
  function handleContextMenu(event) {
    event.preventDefault();
    menuX = event.clientX;
    menuY = event.clientY;
    menuVisible = true;
  }
  
  function handleSelect(event) {
    const { id } = event.detail;
    switch (id) {
      case 'copy': copyContent(); break;
      case 'paste': pasteContent(); break;
      case 'delete': deleteItem(); break;
    }
  }
</script>

<div on:contextmenu={handleContextMenu}>
  Conteúdo com menu de contexto
</div>

<ContextMenu 
  {items}={menuItems}
  bind:visible={menuVisible}
  x={menuX}
  y={menuY}
  on:select={handleSelect}
  on:close={() => menuVisible = false}
/>
```

**Funcionalidades do componente:**
- [ ] Posicionamento automático (evita sair da tela)
- [ ] Navegação por teclado (ArrowUp/Down, Enter, Escape)
- [ ] Suporte a separadores
- [ ] Suporte a submenus
- [ ] Atalhos de teclado visíveis
- [ ] Itens desabilitados (estilo cinza)
- [ ] Itens perigosos (estilo vermelho)
- [ ] Fechar ao clicar fora
- [ ] Fechar ao pressionar Escape
- [ ] Acessibilidade (role="menu", role="menuitem", aria-*)
- [ ] Anúncio para screen readers

**Acessibilidade:**

```svelte
<div 
  role="menu"
  aria-label="Menu de contexto"
  tabindex="-1"
>
  {#each items as item}
    <div
      role="menuitem"
      tabindex="-1"
      aria-disabled={item.disabled}
      class:disabled={item.disabled}
      class:danger={item.danger}
    >
      {#if item.icon}<span aria-hidden="true">{item.icon}</span>{/if}
      <span>{item.label}</span>
      {#if item.shortcut}<span class="shortcut">{item.shortcut}</span>{/if}
    </div>
  {/each}
</div>
```

---

### DS2: ContextMenuTrigger (Wrapper de Conveniência)

Componente wrapper que adiciona menu de contexto a qualquer elemento filho.

```svelte
<!-- ContextMenuTrigger.svelte -->
<script>
  export let items = [];
  export let disabled = false;
  
  // Eventos: on:select
</script>

<!-- Uso -->
<ContextMenuTrigger 
  items={messageMenuItems} 
  on:select={handleAction}
>
  <div class="message">
    Conteúdo da mensagem...
  </div>
</ContextMenuTrigger>
```

**Funcionalidades:**
- [ ] Wrapper transparente (não adiciona DOM extra)
- [ ] Handler automático de contextmenu
- [ ] Handler de Shift+F10 / tecla Menu
- [ ] Prop `disabled` para desativar

---

### DS3: ActionBar (Barra de Ações ao Hover)

Componente genérico para barra de ações que aparece ao hover.

```svelte
<!-- ActionBar.svelte -->
<script>
  export let actions = [];  // Array de ações
  export let position = 'top-right';  // top-left, top-right, bottom-left, bottom-right
  export let showOnHover = true;
  export let showOnFocus = true;
</script>
```

**Estrutura de ação:**

```typescript
interface Action {
  id: string;
  icon: string;
  label: string;        // Para aria-label e tooltip
  disabled?: boolean;
}
```

**Exemplo de uso:**

```svelte
<script>
  import ActionBar from './ActionBar.svelte';
  
  const actions = [
    { id: 'play', icon: '🔊', label: 'Ouvir mensagem' },
    { id: 'copy', icon: '📋', label: 'Copiar' },
    { id: 'more', icon: '⋮', label: 'Mais opções' }
  ];
</script>

<div class="message-wrapper">
  <ActionBar {actions} on:action={handleAction} />
  <div class="message-content">
    Conteúdo da mensagem...
  </div>
</div>
```

**Funcionalidades:**
- [ ] Aparece suavemente ao hover (transition)
- [ ] Delay configurável para evitar flickering
- [ ] Posição configurável
- [ ] Navegação por teclado entre botões
- [ ] Tooltips nos botões
- [ ] Acessibilidade (toolbar role)

---

### DS4: Composição de Componentes

Exemplo de como usar os componentes base juntos:

```svelte
<!-- MessageItem.svelte -->
<script>
  import ContextMenuTrigger from '$lib/components/ContextMenuTrigger.svelte';
  import ActionBar from '$lib/components/ActionBar.svelte';
  
  export let message;
  
  const contextMenuItems = [
    { id: 'copy', label: 'Copiar mensagem', icon: '📋', shortcut: 'Ctrl+C' },
    { id: 'copy-md', label: 'Copiar como Markdown', icon: '📋' },
    { id: 'resend', label: 'Reenviar mensagem', icon: '🔄' },
    { id: 'listen', label: 'Ouvir mensagem', icon: '🔊' },
    { separator: true },
    { id: 'delete', label: 'Excluir mensagem', icon: '🗑️', danger: true }
  ];
  
  const actionBarItems = [
    { id: 'listen', icon: '🔊', label: 'Ouvir' },
    { id: 'copy', icon: '📋', label: 'Copiar' },
    { id: 'more', icon: '⋮', label: 'Mais opções' }
  ];
</script>

<ContextMenuTrigger items={contextMenuItems} on:select={handleAction}>
  <li class="message" tabindex="0" on:keydown={handleKeydown}>
    <ActionBar actions={actionBarItems} on:action={handleAction} />
    
    <div class="message-header">
      <span class="role">{message.role}</span>
    </div>
    
    <div class="message-content">
      {message.content}
    </div>
  </li>
</ContextMenuTrigger>
```

---

### Onde usar cada componente

| Local | ContextMenu | ActionBar | Uso |
|-------|-------------|-----------|-----|
| **Mensagens do chat** | ✅ | ✅ | Copiar, reenviar, ouvir, excluir |
| **Grid de Memórias** | ✅ | ✅ | Editar, copiar, excluir |
| **Grid de FAQ** | ✅ | ✅ | Editar, copiar, testar, excluir |
| **Grid de Agentes** | ✅ | ✅ | Editar, duplicar, ativar/desativar |
| **Campo de texto** | ✅ | ❌ | Copiar, colar, limpar, inserir template |
| **Blocos de código** | ❌ | ✅ | Copiar, executar |
| **Tabelas** | ❌ | ✅ | Copiar, exportar CSV |

---

### Tarefas do Design System

| Componente | Prioridade | Status |
|------------|------------|--------|
| `ContextMenu.svelte` | Alta | ✅ Concluído |
| `ContextMenuTrigger.svelte` | Alta | ✅ Concluído |
| `ActionBar.svelte` | Média | ✅ Concluído |
| `InteractiveContainer.svelte` | - | ✅ Bônus (combina ActionBar + ContextMenu) |

**Status:** ✅ Concluído

---

## Fase UX: Interação com Mensagens

**Status:** ✅ Concluído

**Dependências:** Design System (DS1, DS2, DS3)

**Objetivo:** Melhorar a interação do usuário com mensagens do chat.

### Problemas resolvidos

1. ~~**Áudio não reproduzível:**~~ ✅ Espaço reproduz mensagem, botão 🔊 ao hover
2. ~~**Sem menu de contexto:**~~ ✅ ContextMenu com todas as opções
3. ~~**Sem ações visíveis:**~~ ✅ Barra de ações ao hover (🔊 📋 ⋮)

### UX1: Reproduzir Áudio de Mensagens

**Comportamento:**
- Pressionar `Espaço` em uma mensagem da lista reproduz o áudio via TTS
- Botão de play (🔊) aparece ao passar o mouse sobre a mensagem
- Acessível via teclado quando mensagem está focada

```
┌─────────────────────────────────────────────────────────────────┐
│  [Assistente]                                          [🔊] [⋮] │
│  A imagem mostra um gato dormindo no sofá...                    │
│                                                                 │
│  ← Espaço para ouvir novamente                                 │
│  ← 🔊 aparece ao hover                                          │
└─────────────────────────────────────────────────────────────────┘
```

**Tarefas:**
- [x] Adicionar handler de `keydown` (Espaço) nas mensagens
- [x] Botão de play visível ao hover
- [x] Manter referência ao texto limpo de cada mensagem
- [x] Integrar com `SpeechSynthesisManager` e SAPI5
- [x] Acessibilidade: `aria-label="Ouvir mensagem"`

**Status: ✅ Concluído**

---

### UX2: Menu de Contexto para Mensagens

**Ativação:**
- **Mouse:** Botão ⋮ aparece ao hover → abre menu
- **Mouse:** Click direito na mensagem → abre menu de contexto
- **Teclado:** `Shift+F10` ou tecla Menu quando mensagem focada

```
┌─────────────────────────────────────────────────────────────────┐
│  [Assistente]                                          [🔊] [⋮] │
│  A imagem mostra um gato dormindo...                     │      │
│                                                          ▼      │
│  ┌─────────────────────────────────────┐                        │
│  │ 📋 Copiar mensagem completa         │                        │
│  │ 📋 Copiar como Markdown             │                        │
│  │ 🔄 Reenviar esta mensagem           │                        │
│  │ 🔊 Ouvir mensagem                   │                        │
│  │ ───────────────────────────         │                        │
│  │ 📌 Fixar mensagem                   │                        │
│  │ 🗑️ Excluir mensagem                 │                        │
│  └─────────────────────────────────────┘                        │
└─────────────────────────────────────────────────────────────────┘
```

**Opções do menu:**

| Opção | Descrição | Atalho |
|-------|-----------|--------|
| Copiar mensagem | Copia texto sem formatação | `Ctrl+C` |
| Copiar como Markdown | Copia com formatação MD | `Ctrl+Shift+C` |
| Reenviar mensagem | Envia a mesma pergunta novamente | - |
| Ouvir mensagem | Reproduz via TTS | `Espaço` |
| Fixar mensagem | Marca como importante | - |
| Excluir mensagem | Remove do histórico | `Delete` |

**Tarefas:**
- [x] Menu de contexto integrado (usa `ContextMenu.svelte` genérico)
- [x] Handler de click direito nas mensagens
- [x] Botão ⋮ visível ao hover
- [x] Navegação por teclado no menu (setas, Enter, Esc)
- [x] Acessibilidade completa (role="menu", aria-*, anúncios)
- [x] Suporte a Shift+F10 e tecla Menu

**Status: ✅ Concluído**

---

### UX3: Ações em Blocos de Código

Blocos de código devem ter botões de ação específicos:

```
┌─────────────────────────────────────────────────────────────────┐
│  [Assistente]                                                   │
│  Aqui está o código:                                            │
│                                                                 │
│  ┌────────────────────────────────────────────────────┐         │
│  │ ```python                          [📋] [▶️] [📄]  │         │
│  │ def hello():                                       │         │
│  │     print("Hello, World!")                         │         │
│  │ ```                                                │         │
│  └────────────────────────────────────────────────────┘         │
│                                                                 │
│  📋 = Copiar código                                             │
│  ▶️ = Executar (se terminal disponível)                         │
│  📄 = Copiar como arquivo                                       │
└─────────────────────────────────────────────────────────────────┘
```

**Tarefas:**
- [x] Botões integrados em `Markdown.svelte` (não componente separado)
- [x] Botão copiar código
- [x] Detectar linguagem do bloco (exibe no wrapper)
- [x] Editor Monaco inline para edição/visualização
- [ ] (Opcional) Executar em terminal integrado
- [ ] Copiar como arquivo com nome sugerido

**Status: ✅ Concluído (funcionalidades principais)**

---

### UX4: Ações em Tabelas Markdown

```
┌─────────────────────────────────────────────────────────────────┐
│  [Assistente]                                                   │
│  Aqui está a comparação:                                        │
│                                                                 │
│  ┌────────────────────────────────────────────────────┐         │
│  │                                    [📋] [📊] [CSV] │         │
│  │  | Nome  | Idade | Cidade   |                      │         │
│  │  |-------|-------|----------|                      │         │
│  │  | João  | 25    | SP       |                      │         │
│  │  | Maria | 30    | RJ       |                      │         │
│  └────────────────────────────────────────────────────┘         │
│                                                                 │
│  📋 = Copiar como texto                                         │
│  📊 = Copiar como HTML (para colar em Excel/Sheets)             │
│  CSV = Baixar como CSV                                          │
└─────────────────────────────────────────────────────────────────┘
```

**Tarefas:**
- [x] Botões integrados em `Markdown.svelte` (não componente separado)
- [x] Copiar tabela como texto (TSV)
- [x] Editor Monaco inline para visualização
- [x] Copiar como HTML (compatível com Excel/Sheets)
- [x] Exportar como CSV (download de arquivo)
- [ ] Exportar como JSON (baixa prioridade)

**Status: ✅ Concluído**

---

### UX5: Barra de Ações ao Hover

Quando o mouse passa sobre qualquer mensagem, uma barra de ações aparece:

```
┌─────────────────────────────────────────────────────────────────┐
│                                                                 │
│  ┌───────────────────────────────────────────────────────────┐  │
│  │  [Você]                                     [🔊] [📋] [⋮] │  │
│  │  O que tem nesta imagem?                                  │  │
│  │  [imagem.jpg]                                             │  │
│  └───────────────────────────────────────────────────────────┘  │
│       ↑                                        ↑               │
│       │                                        └─ Barra de ações│
│       └─ Mensagem com hover                       (aparece hover)│
│                                                                 │
└─────────────────────────────────────────────────────────────────┘
```

**Botões da barra:**
- 🔊 Ouvir (se TTS ativo)
- 📋 Copiar
- ⋮ Mais opções (abre menu completo)

**Tarefas:**
- [x] Barra `message-actions` inline em `Chat.svelte` (não componente separado)
- [x] Mostrar ao hover via `hoveredMessageIndex`
- [x] Esconder quando mouse sai
- [x] Manter visível enquanto menu está aberto
- [x] Botões: 🔊 Ouvir, 📋 Copiar, ⋮ Menu
- [x] `aria-hidden="true"` para não interferir no leitor de tela

**Status: ✅ Concluído**

---

### Resumo de Tarefas UX

| Funcionalidade | Status | Implementação |
|----------------|--------|---------------|
| Barra de ações ao hover | ✅ | `Chat.svelte` (inline) |
| Menu de contexto | ✅ | `ContextMenu.svelte` (genérico) |
| Ações em blocos de código | ✅ | `Markdown.svelte` (inline) |
| Ações em tabelas | ✅ | `Markdown.svelte` (Copiar, CSV, Excel) |

### Tempo estimado: ~~2-3 dias~~ ✅ Concluído

---

## Estimativa de Esforço

| Fase | Status | O que falta |
|------|--------|-------------|
| **DS** | ✅ Concluído | - |
| **M0** | ✅ Concluído | - |
| **M1** | ✅ Concluído | - |
| M2 | 🔮 Futuro | Geração de imagens (DALL-E) |
| M3 | 🔮 Futuro | Áudio no prompt |
| M4 | 🔮 Futuro | WebSocket Realtime |
| **UX** | ✅ Concluído | - |

---

## Considerações de Acessibilidade

| Modalidade | Considerações |
|------------|---------------|
| Imagens | Alt text, descrição automática via Vision |
| Áudio | Transcrição sempre disponível |
| Realtime | Transcrição em tempo real para aria-live |
| **Mensagens** | Espaço para ouvir, menu via Shift+F10, navegação por teclado |
| **Código** | Botões focáveis, anúncio "Código copiado" |
| **Tabelas** | Navegação por células, anúncio de ações |

---

## Referências

- [OpenAI Vision Guide](https://platform.openai.com/docs/guides/vision)
- [OpenAI Realtime API](https://platform.openai.com/docs/guides/realtime)
- [OpenAI DALL-E API](https://platform.openai.com/docs/guides/images)
- [Google Gemini Multimodal](https://ai.google.dev/gemini-api/docs/vision)
- [Anthropic Vision](https://docs.anthropic.com/en/docs/build-with-claude/vision)

