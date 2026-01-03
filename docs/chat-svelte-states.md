# Chat.svelte - Documentação de Estados

## Objetivo
Documentar todos os estados do Chat.svelte para facilitar a migração para ChatContainer.

## Estados por Categoria

### 1. Conversa
| Estado | Tipo | Descrição | Vai para |
|--------|------|-----------|----------|
| `currentConversationId` | `string\|null` | ID da conversa atual | Chat.svelte (orquestrador) |
| `conversationTitle` | `string` | Título da conversa | Chat.svelte |
| `conversationData` | `object\|null` | Dados completos da conversa | Chat.svelte |
| `messages` | `array` | Array flat de mensagens | ChatContainer (prop) |

### 2. Input/Envio
| Estado | Tipo | Descrição | Vai para |
|--------|------|-----------|----------|
| `inputMessage` | `string` | Texto do input | ChatContainer (bind) |
| `isLoading` | `boolean` | Enviando mensagem | ChatContainer (prop) |
| `error` | `string` | Mensagem de erro | ChatContainer (prop) |
| `pendingMedia` | `array` | Mídia anexada | ChatContainer (prop) |
| `mediaError` | `string` | Erro de mídia | ChatContainer (prop) |
| `mediaMode` | `string` | 'normal' ou 'record_audio' | ChatInput (prop) |

### 3. Streaming
| Estado | Tipo | Descrição | Vai para |
|--------|------|-----------|----------|
| `streamingMessageId` | `string\|null` | ID da mensagem streamando | Chat.svelte |
| `streamingContent` | `string` | Conteúdo acumulado | ChatContainer (prop) |
| `currentStreamedMessage` | `string` | DEPRECATED | Remover |

### 4. Modelo/Parâmetros
| Estado | Tipo | Descrição | Vai para |
|--------|------|-----------|----------|
| `selectedModel` | `string` | Modelo selecionado | Chat.svelte (Toolbar) |
| `maxTokens` | `number` | Max tokens | Chat.svelte |
| `temperature` | `number` | Temperatura | Chat.svelte |
| `useTools` | `boolean` | Usar ferramentas | Chat.svelte |
| `showSettings` | `boolean` | Modal de config aberto | Chat.svelte |
| `executingTools` | `boolean` | Ferramentas executando | Chat.svelte |
| `toolsMessage` | `string` | Mensagem de ferramentas | Chat.svelte |
| `showInternalMessages` | `boolean` | Mostrar msgs internas | ChatContainer (config) |

### 5. TTS (Text-to-Speech)
| Estado | Tipo | Descrição | Vai para |
|--------|------|-----------|----------|
| `voiceEnabled` | `boolean` | TTS habilitado | Chat.svelte |
| `autoSpeak` | `boolean` | Falar automaticamente | Chat.svelte |
| `ttsManager` | `object` | Manager de TTS | Chat.svelte |
| `selectedVoice` | `string` | Voz selecionada | Chat.svelte |
| `selectedVoiceSource` | `string` | 'disabled', 'webspeech', 'sapi5', 'openai' | Chat.svelte |
| `openaiVoiceId` | `string\|null` | ID da voz OpenAI | Chat.svelte |
| `isTTSDisabled` | `boolean` | TTS desativado (computed) | ChatContainer (config) |
| `voiceVolume` | `number` | Volume (0-100) | Chat.svelte |
| `voiceRate` | `number` | Velocidade | Chat.svelte |
| `showVoiceSettings` | `boolean` | Modal de voz aberto | Chat.svelte |

### 6. STT (Speech-to-Text)
| Estado | Tipo | Descrição | Vai para |
|--------|------|-----------|----------|
| `selectedSTTProvider` | `string` | 'webspeech' ou 'whisper' | Chat.svelte |
| `isVoiceInput` | `boolean` | Input veio da voz | Chat.svelte |
| `recordingMode` | `string` | PTT, Toggle, VAD_* | Chat.svelte |

### 7. Mídia/Captura
| Estado | Tipo | Descrição | Vai para |
|--------|------|-----------|----------|
| `fileInputRef` | `element` | Input de arquivo | Chat.svelte |
| `mediaMenuComponent` | `component` | Menu de mídia | Chat.svelte |
| `isGeneratingAltText` | `boolean` | Gerando alt text (computed) | ChatInput (prop) |

### 8. Threads/Expansão
| Estado | Tipo | Descrição | Vai para |
|--------|------|-----------|----------|
| `expandedPaths` | `object` | Paths expandidos | ChatContainer (prop) |
| `loadingPaths` | `object` | Paths carregando | ChatContainer (prop) |
| `expandedThreads` | `object` | DEPRECATED | Remover |
| `expandedAgentThreads` | `object` | DEPRECATED | Remover |

### 9. Navegação/Foco
| Estado | Tipo | Descrição | Vai para |
|--------|------|-----------|----------|
| `focusedMessageIndex` | `number` | Índice focado | ChatContainer (interno) |
| `focusedThreadLevel` | `number` | Nível focado | ChatContainer (interno) |
| `focusedAgentIndex` | `number` | Índice agente focado | ChatContainer (interno) |
| `focusedToolIndex` | `number` | Índice tool focado | ChatContainer (interno) |
| `focusedParentIndex` | `number` | Índice pai focado | ChatContainer (interno) |

### 10. Acessibilidade
| Estado | Tipo | Descrição | Vai para |
|--------|------|-----------|----------|
| `messagesContainer` | `element` | Container de mensagens | ChatContainer (interno) |
| `inputElement` | `element` | Elemento input | ChatInput (interno) |
| `liveMessage` | `string` | Mensagem aria-live | ChatContainer (interno) |
| `navigationAnnouncement` | `string` | Anúncio de navegação | ChatContainer (interno) |

### 11. Referências de Componentes
| Estado | Tipo | Descrição | Vai para |
|--------|------|-----------|----------|
| `voiceButtonComponent` | `component` | VoiceButton | Chat.svelte |
| `voicePickerComponent` | `component` | VoicePicker | Chat.svelte |
| `modelPickerComponent` | `component` | ModelPicker | Chat.svelte |
| `sttPickerComponent` | `component` | STTProviderPicker | Chat.svelte |
| `maxTokensInput` | `element` | Input max tokens | Chat.svelte |
| `volumeInput` | `element` | Input volume | Chat.svelte |

### 12. Computed/Derivados
| Estado | Expressão | Descrição |
|--------|-----------|-----------|
| `hasContent` | `inputMessage.trim() \|\| pendingMedia.length > 0` | Tem conteúdo para enviar |
| `showVoiceButton` | `(!hasContent \|\| isVoiceInput) && voiceEnabled && !isLoading && mediaMode === 'normal'` | Mostrar botão de voz |
| `canSendMessage` | `hasContent && !isLoading && selectedModel && !isGeneratingAltText` | Pode enviar |

---

## Dependências entre Estados

```
conversation (prop)
    └── loadConversation()
        ├── currentConversationId
        ├── conversationTitle
        ├── conversationData
        └── messages

inputMessage + pendingMedia
    └── hasContent (computed)
        └── showVoiceButton (computed)
        └── canSendMessage (computed)

selectedVoice
    └── isTTSDisabled (computed)
        └── ChatContainer.config.enableTTS

isLoading + streamingMessageId
    └── UI de loading/streaming
    
expandedPaths + loadingPaths
    └── ChatHistory (visualização de threads)
```

---

## Eventos Wails

| Evento | Handler | Descrição |
|--------|---------|-----------|
| `chat:stream` | `handleStreamChunk` | Chunk de streaming |
| `chat:done` | `handleStreamDone` | Streaming concluído |
| `chat:error` | `handleStreamError` | Erro no streaming |
| `chat:tool_start` | `handleToolStart` | Ferramenta iniciou |
| `chat:tool_end` | `handleToolEnd` | Ferramenta terminou |

---

## Resumo da Migração

### Vai para ChatContainer (props):
- `messages`
- `inputMessage` (bind)
- `pendingMedia`
- `isLoading`
- `error`
- `expandedPaths`
- `loadingPaths`
- `showInternalMessages` (config)
- `isTTSDisabled` → `enableTTS` (config)

### Fica no Chat.svelte (orquestrador):
- Tudo de conversa (`currentConversationId`, `conversationData`, etc.)
- Tudo de modelo (`selectedModel`, `temperature`, etc.)
- Tudo de TTS (`selectedVoice`, `voiceVolume`, etc.)
- Tudo de STT (`selectedSTTProvider`, `recordingMode`, etc.)
- Tudo de streaming (`streamingMessageId`, handlers Wails)
- Toolbar e seus modais

### É interno ao ChatContainer:
- Estados de foco/navegação
- Estados de acessibilidade
- Referências a elementos DOM




