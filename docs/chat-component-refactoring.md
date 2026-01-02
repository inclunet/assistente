# Refatoração do Sistema de Chat - Componentes Modulares

## Objetivo

Transformar o sistema de chat monolítico em componentes independentes e reutilizáveis que podem ser usados em diferentes contextos (LLM, P2P, suporte, etc.).

## Princípios

1. **Event-Driven**: Todos os componentes emitem eventos para TODAS as ações. Nenhuma lógica de execução dentro dos componentes de exibição - apenas disparam eventos que podem ser interceptados.

2. **Independência Total**: Qualquer componente pode ser usado sozinho, sem dependências de outros componentes do chat.

3. **Zero Lógica de Negócio**: Componentes são puramente visuais. Toda lógica (TTS, API, salvamento) fica em callbacks externos.

4. **Documentação Interna**: README.md dentro da pasta com documentação completa de cada componente.

## Arquitetura Proposta

```
┌─────────────────────────────────────────────────────────┐
│                      ChatContainer                       │
│  (Orquestra tudo, mas componentes podem ser usados      │
│   independentemente)                                     │
├─────────────────────────────────────────────────────────┤
│                                                         │
│  ┌─────────────────────────────────────────────────┐   │
│  │              ChatHistory                         │   │
│  │  ┌─────────────────────────────────────────┐    │   │
│  │  │         MessageNode (recursivo)          │    │   │
│  │  │  ┌─────────────────────────────────┐    │    │   │
│  │  │  │      MessageContent             │    │    │   │
│  │  │  └─────────────────────────────────┘    │    │   │
│  │  │  ┌─────────────────────────────────┐    │    │   │
│  │  │  │      MessageActions             │    │    │   │
│  │  │  └─────────────────────────────────┘    │    │   │
│  │  └─────────────────────────────────────────┘    │   │
│  └─────────────────────────────────────────────────┘   │
│                                                         │
│  ┌─────────────────────────────────────────────────┐   │
│  │              ChatInput                           │   │
│  │  ┌──────────────┐  ┌──────────────────────┐     │   │
│  │  │ MediaPicker  │  │   InputArea          │     │   │
│  │  └──────────────┘  └──────────────────────┘     │   │
│  └─────────────────────────────────────────────────┘   │
│                                                         │
└─────────────────────────────────────────────────────────┘
```

## Componentes

### 1. MessageNode (já existe, precisa ajustes)

**Props genéricos:**
```typescript
interface MessageNodeProps {
  // Dados da mensagem
  message: {
    id: string | number;
    content: string;
    timestamp?: Date;
    media?: MediaItem[];
    metadata?: Record<string, any>;
  };
  
  // Autor (genérico, não assume LLM)
  author: {
    id: string;
    name: string;
    avatar?: string;      // URL ou base64
    color?: string;       // Cor do autor
    role?: string;        // 'user' | 'assistant' | 'agent' | custom
  };
  
  // Contexto
  isMe: boolean;          // É minha mensagem?
  level: number;          // Nível de aninhamento
  
  // Threading
  children?: MessageNode[];
  childCount?: number;
  
  // Configuração
  config?: {
    showAvatar?: boolean;
    showTimestamp?: boolean;
    showActions?: boolean;
    editable?: boolean;
    deletable?: boolean;
    pinnable?: boolean;
    speakable?: boolean;
  };
}
```

**Eventos emitidos:**
```typescript
// Eventos de ação
dispatch('copy', { message, format: 'text' | 'markdown' });
dispatch('speak', { message });
dispatch('delete', { message });
dispatch('pin', { message, pinned: boolean });
dispatch('reply', { message });
dispatch('resend', { message });           // Reenviar mensagem
dispatch('detail', { message });           // Abrir modal de detalhes

// Eventos de edição (inline)
dispatch('editStart', { message });        // Iniciar edição
dispatch('editSave', { message, newContent }); // Salvar edição
dispatch('editCancel', { message });       // Cancelar edição

// Eventos de navegação
dispatch('toggle', { path, expand: boolean });
dispatch('focus', { path });
dispatch('boundary', { edge, level, path });

// Eventos de mídia
dispatch('mediaClick', { media });
dispatch('imageZoom', { src, alt });       // Abrir modal de zoom
dispatch('imageDownload', { imageData });
dispatch('imageCopy', { imageData });

// Eventos de conteúdo especial
dispatch('copyTable', { table, format });  // Copiar tabela
dispatch('copyCode', { code, language });  // Copiar código
dispatch('openLink', { url });             // Abrir link

// Acessibilidade
dispatch('announce', { message, priority }); // Anúncio para aria-live
```

### 2. MessageContent (já existe)

Componente puro para renderizar conteúdo. Sem mudanças significativas.

### 3. MessageActions (NOVO)

**Props:**
```typescript
interface MessageActionsProps {
  message: Message;
  actions: Action[];  // Ações disponíveis
  layout: 'hover' | 'inline' | 'menu';
}

interface Action {
  id: string;
  label: string;
  icon: string;
  shortcut?: string;
  disabled?: boolean;
  danger?: boolean;
  handler: () => void;
}
```

### 4. ChatHistory (NOVO)

Lista de mensagens com navegação por teclado.

**Props:**
```typescript
interface ChatHistoryProps {
  messages: MessageNode[];
  
  // Configuração
  config?: {
    virtualScroll?: boolean;     // Para listas grandes
    autoScroll?: boolean;        // Scroll automático
    groupByDate?: boolean;       // Agrupar por data
    showThreads?: boolean;       // Mostrar threads
    lazyLoadChildren?: boolean;  // Lazy load de filhos
  };
  
  // Callbacks
  onLoadChildren?: (messageId) => Promise<MessageNode[]>;
  
  // Theming
  theme?: 'default' | 'compact' | 'bubble';
}
```

**Eventos:**
```typescript
dispatch('messageAction', { action, message });
dispatch('loadMore', { direction: 'up' | 'down' });
dispatch('selectionChange', { messages: Message[] });
```

### 5. ChatInput (NOVO)

Campo de entrada com suporte a mídia.

**Props:**
```typescript
interface ChatInputProps {
  value: string;
  placeholder?: string;
  disabled?: boolean;
  
  // Mídia
  mediaConfig?: {
    allowImages?: boolean;
    allowAudio?: boolean;
    allowDocuments?: boolean;
    allowScreenshot?: boolean;
    allowWebcam?: boolean;
    maxFiles?: number;
    maxFileSize?: number;
  };
  
  pendingMedia?: MediaItem[];
  
  // Ações extras
  actions?: InputAction[];  // Botões adicionais
}
```

**Eventos:**
```typescript
dispatch('submit', { content, media });
dispatch('mediaAdd', { media });
dispatch('mediaRemove', { index });
dispatch('typing', { isTyping: boolean });
dispatch('cancel', {});
```

### 6. ChatContainer (NOVO)

Wrapper que orquestra tudo com sistema de registro de callbacks.

**Props:**
```typescript
interface ChatContainerProps {
  // Dados
  messages: MessageNode[];
  currentUser: Author;
  
  // Sistema de Callbacks (registro de handlers)
  handlers: ChatHandlers;
  
  // Configuração
  config?: ChatConfig;
  
  // Theming
  theme?: ChatTheme;
}
```

**Sistema de Registro de Callbacks:**
```typescript
interface ChatHandlers {
  // === Mensagens (backend) ===
  onSend?: (content: string, media: MediaItem[]) => Promise<Message>;
  onEdit?: (messageId: string, newContent: string) => Promise<void>;
  onDelete?: (messageId: string) => Promise<void>;
  onPin?: (messageId: string, pinned: boolean) => Promise<void>;
  onResend?: (message: Message) => Promise<void>;
  
  // === Carregamento (backend) ===
  onLoadHistory?: () => Promise<Message[]>;
  onLoadChildren?: (messageId: string) => Promise<Message[]>;
  onLoadMore?: (direction: 'up' | 'down') => Promise<Message[]>;
  
  // === TTS (serviço externo) ===
  onSpeak?: (text: string) => void;
  onStopSpeaking?: () => void;
  
  // === Mídia (pode ser local ou backend) ===
  onCaptureScreen?: () => Promise<MediaItem>;
  onCaptureWebcam?: () => Promise<MediaItem>;
  onRecordAudio?: () => Promise<MediaItem>;
  onGenerateAltText?: (imageBase64: string) => Promise<string>;
  
  // === Notificações (opcional) ===
  onPlaySound?: (type: 'send' | 'receive' | 'error') => void;
  onAnnounce?: (message: string, priority: 'polite' | 'assertive') => void;
  onError?: (error: Error) => void;
  
  // === Streaming LLM (opcional) ===
  onStreamStart?: () => void;
  onStreamChunk?: (chunk: string, messageId: string) => void;
  onStreamEnd?: (messageId: string) => void;
}
```

**Uso simplificado:**
```svelte
<script>
  import { ChatContainer } from './components/chat';
  
  // Registra todos os handlers em um objeto
  const handlers = {
    // Backend
    onSend: async (content, media) => {
      return await api.sendMessage(conversationId, content, media);
    },
    onEdit: async (id, content) => {
      await api.updateMessage(id, content);
    },
    onDelete: async (id) => {
      await api.deleteMessage(id);
    },
    onLoadChildren: async (id) => {
      return await api.getMessageChildren(id);
    },
    
    // TTS
    onSpeak: (text) => {
      speechSynthesis.speak(new SpeechSynthesisUtterance(text));
    },
    
    // Feedback
    onPlaySound: (type) => {
      audioService.play(type);
    },
    onAnnounce: (message) => {
      ariaLive.textContent = message;
    }
  };
</script>

<!-- Uma única prop para todos os handlers! -->
<ChatContainer 
  {messages} 
  {handlers}
  config={{ enableTTS: true, enableThreads: true }}
/>
```

**Configuração:**
```typescript
interface ChatConfig {
  // Features (habilita/desabilita)
  enableTTS?: boolean;          // Mostrar botão de falar
  enableThreads?: boolean;      // Mostrar threads
  enablePinning?: boolean;      // Permitir fixar mensagens
  enableEditing?: boolean;      // Permitir editar mensagens
  enableMedia?: boolean;        // Permitir anexar mídia
  
  // História
  lazyLoadChildren?: boolean;   // Carregar filhos sob demanda
  autoScroll?: boolean;         // Scroll automático
  groupByDate?: boolean;        // Agrupar por data
  
  // Input
  placeholder?: string;
  maxMediaFiles?: number;
  allowedMediaTypes?: string[];
  
  // Header
  showHeader?: boolean;
  title?: string;
}
```

**Handlers com fallback padrão:**

O ChatContainer pode ter implementações padrão para ações locais:

```javascript
// Dentro do ChatContainer
const defaultHandlers = {
  // Ações locais têm implementação padrão
  onCopy: (text) => navigator.clipboard.writeText(text),
  onDownloadImage: (data) => downloadBlob(data),
  onOpenLink: (url) => window.open(url, '_blank'),
  
  // Ações externas não têm padrão (obrigatórias)
  onSend: null,  // Usuário DEVE fornecer
};

// Merge com handlers do usuário
const finalHandlers = { ...defaultHandlers, ...handlers };
```
```

## Filosofia Event-Driven

### Dois tipos de ações:

#### 1. Ações Locais (executadas internamente + emitem evento)

Ações que **não precisam de backend** são executadas dentro do componente E emitem evento para quem quiser saber:

```svelte
<!-- Componente executa E notifica -->
<button on:click={() => {
  // Executa localmente
  navigator.clipboard.writeText(content);
  
  // Notifica quem quiser saber (opcional de escutar)
  dispatch('copied', { message, format: 'text' });
}}>
  Copiar
</button>
```

**Exemplos de ações locais:**
- ✅ Copiar texto/markdown para clipboard
- ✅ Download de imagem
- ✅ Copiar imagem para clipboard
- ✅ Abrir link em nova aba
- ✅ Renderizar markdown/código
- ✅ Expandir/recolher conteúdo
- ✅ Zoom de imagem (abrir modal interno)

#### 2. Ações Externas (apenas emitem evento)

Ações que **precisam de backend ou lógica externa** apenas emitem evento:

```svelte
<!-- Componente só notifica, quem usa implementa -->
<button on:click={() => dispatch('delete', { message })}>
  Excluir
</button>

<!-- Orquestrador implementa -->
<MessageNode on:delete={(e) => {
  await api.deleteMessage(e.detail.message.id);
  messages = messages.filter(m => m.id !== e.detail.message.id);
}} />
```

**Exemplos de ações externas:**
- 🔄 Editar mensagem (salvar no backend)
- 🔄 Excluir mensagem (remover do backend)
- 🔄 Fixar mensagem (salvar no backend)
- 🔄 Reenviar mensagem (chamar API)
- 🔄 TTS/Falar (depende do serviço configurado)
- 🔄 Carregar filhos (lazy load do backend)
- 🔄 Enviar nova mensagem (chamar API)

### Eventos por Componente

Cada componente documenta seus eventos no README. Exemplos:

**MessageNode:**
- `copy` - Usuário quer copiar mensagem
- `speak` - Usuário quer ouvir mensagem
- `edit` - Usuário quer editar
- `delete` - Usuário quer excluir
- `pin` - Usuário quer fixar/desafixar
- `reply` - Usuário quer responder
- `select` - Mensagem foi selecionada
- `contextMenu` - Menu de contexto solicitado
- `toggle` - Expandir/recolher thread
- `focus` - Mensagem recebeu foco
- `blur` - Mensagem perdeu foco
- `boundary` - Navegação atingiu limite

**ChatInput:**
- `submit` - Usuário quer enviar
- `change` - Conteúdo mudou
- `focus` - Campo recebeu foco
- `blur` - Campo perdeu foco
- `mediaAdd` - Mídia anexada
- `mediaRemove` - Mídia removida
- `cancel` - Usuário cancelou
- `typing` - Estado de digitação mudou

**ChatHistory:**
- `messageSelect` - Mensagem selecionada
- `messageAction` - Ação em mensagem (propaga eventos filhos)
- `loadMore` - Carregar mais mensagens
- `scrollEnd` - Scroll chegou ao fim
- `empty` - Lista está vazia

**Eventos de Acessibilidade (todos os componentes podem emitir):**
- `announce` - Anúncio para aria-live region (ex: "Mensagem copiada", "3 de 5")
- `focus` - Foco mudou para elemento
- `blur` - Elemento perdeu foco

```svelte
<!-- Componente emite evento de anúncio -->
<button on:click={() => {
  dispatch('copy', { message });
  dispatch('announce', { message: 'Mensagem copiada para área de transferência', priority: 'polite' });
}}>
  Copiar
</button>

<!-- Quem usa decide como anunciar -->
<MessageNode 
  on:announce={(e) => {
    // Pode usar aria-live region, TTS, ou ambos
    ariaLiveRegion.textContent = e.detail.message;
  }}
/>
```

## Garantia de Compatibilidade

### Princípio: Nada quebra

A refatoração será feita de forma incremental, garantindo que o sistema atual continue funcionando em cada etapa.

### Estratégia de Migração

1. **Criar novos componentes em paralelo**: Os novos componentes são criados na pasta `chat/` sem modificar os existentes.

2. **Adapter EXTERNO ao componente**: O adapter LLM fica FORA da pasta `chat/` - é específico do nosso projeto.

3. **Migração gradual do Chat.svelte**: O Chat.svelte vai gradualmente importar e usar os novos componentes.

4. **Testes a cada passo**: Cada fase termina com o sistema funcionando igual ao anterior.

### Separação Clara: Componente vs Projeto

```
frontend/src/
├── components/
│   └── chat/                    # COMPONENTE GENÉRICO (pode ser copiado para outro projeto)
│       ├── README.md
│       ├── index.js
│       ├── ChatContainer.svelte
│       ├── message/
│       ├── input/
│       └── ...
│
├── lib/                         # CÓDIGO ESPECÍFICO DO PROJETO (não faz parte do componente)
│   └── llmChatAdapter.js        # Adapter para nosso backend LLM
│
└── routes/
    └── Chat.svelte              # Usa ChatContainer + llmChatAdapter
```

### O Componente é 100% Agnóstico

O `ChatContainer` não sabe:
- Que existe um LLM
- Que existe Wails
- Que existe streaming
- Como funciona nossa API

Ele apenas:
- Recebe mensagens
- Renderiza UI
- Chama handlers quando ações acontecem

### Chat.svelte usa o Componente + Adapter

```svelte
<!-- Chat.svelte (específico do nosso projeto) -->
<script>
  // Componente genérico
  import { ChatContainer } from './components/chat';
  
  // Adapter específico do nosso projeto (FORA do componente)
  import { createLLMHandlers, convertMessages } from '$lib/llmChatAdapter';
  
  // Cria handlers específicos para nosso backend
  const handlers = createLLMHandlers({
    sendMessage: SendMessage,
    getConversation: GetConversationWithThreads,
    getChildren: GetMessageChildren,
    updateMessage: UpdateMessage,
    deleteMessage: DeleteMessage,
    runtime: runtime,
    ttsService: { voice: selectedVoice, rate: voiceRate }
  });
  
  // Converte mensagens do formato do backend para formato genérico
  $: displayMessages = convertMessages(rawMessages);
</script>

<ChatContainer 
  messages={displayMessages}
  {handlers}
  currentUser={{ id: 'user', name: 'Você' }}
  config={{
    enableTTS: !isTTSDisabled,
    enableThreads: showInternalMessages,
    title: conversationTitle
  }}
/>
```

### llmChatAdapter.js (fora do componente)

```javascript
// frontend/src/lib/llmChatAdapter.js
// Este arquivo NÃO faz parte do componente chat/
// É específico do nosso projeto

import { EventsOn, EventsEmit } from '../../wailsjs/runtime';

/**
 * Converte mensagens do formato do backend LLM para formato genérico
 */
export function convertMessages(llmMessages) {
  return llmMessages.map(msg => ({
    message: {
      id: msg.id,
      content: msg.content,
      timestamp: msg.created_at,
      media: msg.media
    },
    author: getAuthor(msg),
    isMe: msg.role === 'user',
    children: msg.children?.map(c => convertMessages([c])[0]) || [],
    childCount: msg.child_count || 0
  }));
}

function getAuthor(msg) {
  if (msg.role === 'user') return { id: 'user', name: 'Você', avatar: '👤' };
  if (msg.role === 'assistant') return { id: 'assistant', name: 'Assistente', avatar: '🤖' };
  if (msg.role === 'agent') return { id: msg.agent_name, name: formatAgentName(msg.agent_name), avatar: '🔧' };
  if (msg.role === 'tool') return { id: msg.tool_name, name: msg.tool_name, avatar: '📥' };
  return { id: 'system', name: 'Sistema', avatar: '⚙️' };
}

/**
 * Cria handlers para o ChatContainer baseado no nosso backend
 */
export function createLLMHandlers(config) {
  const { sendMessage, getConversation, getChildren, updateMessage, deleteMessage, runtime, ttsService } = config;
  
  return {
    // === Backend ===
    onSend: async (content, media) => {
      return await sendMessage(content, media);
    },
    
    onEdit: async (messageId, newContent) => {
      await updateMessage(messageId, newContent);
    },
    
    onDelete: async (messageId) => {
      await deleteMessage(messageId);
    },
    
    onLoadChildren: async (messageId) => {
      const children = await getChildren(messageId);
      return convertMessages(children);
    },
    
    // === TTS ===
    onSpeak: ttsService ? (text) => {
      const utterance = new SpeechSynthesisUtterance(text);
      utterance.voice = ttsService.voice;
      utterance.rate = ttsService.rate;
      speechSynthesis.speak(utterance);
    } : null,
    
    onStopSpeaking: () => {
      speechSynthesis.cancel();
    },
    
    // === Streaming (Wails events) ===
    setupStreaming: (callbacks) => {
      EventsOn('chat:stream', callbacks.onChunk);
      EventsOn('chat:done', callbacks.onDone);
      EventsOn('chat:error', callbacks.onError);
    }
  };
}
```

```javascript
// llmChatAdapter.js
export function convertLLMMessage(llmMessage) {
  return {
    message: {
      id: llmMessage.id,
      content: llmMessage.content,
      media: llmMessage.media,
    },
    author: {
      id: llmMessage.role,
      name: getAuthorName(llmMessage),
      role: llmMessage.role,
      avatar: getAuthorAvatar(llmMessage.role),
    },
    isMe: llmMessage.role === 'user',
    level: llmMessage.level || 0,
  };
}

function getAuthorName(msg) {
  if (msg.role === 'user') return 'Você';
  if (msg.role === 'assistant') return 'Assistente';
  if (msg.role === 'agent') return formatAgentName(msg.agent_name);
  if (msg.role === 'tool') return formatToolName(msg.tool_name);
  return 'Sistema';
}
```

## Estrutura de Arquivos

```
frontend/src/components/chat/
├── README.md                    # Documentação completa
├── index.js                     # Exports públicos
├── theme.css                    # Variáveis CSS customizáveis
│
├── core/                        # Componentes de visualização (PUROS)
│   ├── message/
│   │   ├── MessageNode.svelte
│   │   ├── MessageContent.svelte
│   │   ├── MessageActions.svelte
│   │   ├── MessageAvatar.svelte
│   │   ├── MessageHeader.svelte
│   │   └── ThreadIndicator.svelte
│   │
│   ├── input/
│   │   ├── ChatInput.svelte
│   │   ├── MediaPreview.svelte
│   │   └── MediaPicker.svelte
│   │
│   ├── feedback/
│   │   ├── TypingIndicator.svelte
│   │   ├── EmptyState.svelte
│   │   ├── LoadingIndicator.svelte
│   │   └── StreamingIndicator.svelte
│   │
│   ├── modals/
│   │   ├── ImageModal.svelte
│   │   └── MessageDetailModal.svelte
│   │
│   ├── ChatHistory.svelte
│   └── ChatToolbar.svelte
│
├── wrappers/                    # Wrappers opcionais (podem ser ignorados)
│   └── ChatContainer.svelte     # Wrapper com sistema de handlers
│
└── index.js                     # Exports (core + wrappers separados)
```

### Exports separados

```javascript
// index.js
// Componentes core (visualização pura)
export { default as MessageNode } from './core/message/MessageNode.svelte';
export { default as MessageContent } from './core/message/MessageContent.svelte';
export { default as ChatHistory } from './core/ChatHistory.svelte';
export { default as ChatInput } from './core/input/ChatInput.svelte';
// ... todos os componentes core

// Wrapper opcional
export { default as ChatContainer } from './wrappers/ChatContainer.svelte';

// Tema
export { applyTheme, defaultTheme } from './theme.js';
```

### Uso sem wrapper (componentes puros)

```svelte
<script>
  // Usa apenas componentes de visualização
  import { ChatHistory, ChatInput } from './components/chat';
  
  // Implementa seus próprios handlers
  function handleDelete(e) {
    myApi.delete(e.detail.message.id);
  }
</script>

<ChatHistory {messages} on:delete={handleDelete} />
<ChatInput on:submit={handleSubmit} />
```

### Uso com wrapper (handlers pré-configurados)

```svelte
<script>
  // Usa wrapper que gerencia handlers
  import { ChatContainer } from './components/chat';
</script>

<ChatContainer {messages} {handlers} {config} />
```
```

**Nota**: Subpastas agrupam componentes correlacionados. Componentes de alto nível (ChatHistory, ChatToolbar) ficam na raiz.

## Funcionalidades a Cobrir

### ✅ Funcionalidades Core do Chat (devem estar nos componentes)

| Funcionalidade | Componente/Evento |
|----------------|-------------------|
| Renderização de mensagens | `MessageNode`, `MessageContent` |
| **Edição de mensagem** | `MessageNode` com prop `editable` + evento `on:edit`, `on:editSave`, `on:editCancel` |
| **Reenviar mensagem** | Evento `on:resend` no `MessageNode` |
| **Modal de imagem (zoom)** | Componente `ImageModal.svelte` + evento `on:imageZoom` |
| **Modal de detalhes** | Componente `MessageDetailModal.svelte` + evento `on:detail` |
| Ações em mensagens | `on:copy`, `on:speak`, `on:delete`, `on:pin` |
| Threads e navegação | `on:toggle`, `on:focus`, `on:boundary` |
| Campo de entrada | `ChatInput` |
| Mídia | `MediaPreview`, `MediaPicker` |
| Anúncios de acessibilidade | Evento `on:announce` |
| Menu de contexto | Evento `on:contextMenu` |
| Lazy loading | Evento `on:loadChildren` |
| Indicadores visuais | `TypingIndicator`, `EmptyState`, `LoadingIndicator` |

### ⚠️ Funcionalidades Opcionais (orquestrador decide se usa)

| Funcionalidade | Solução |
|----------------|---------|
| Streaming LLM | Eventos `on:streamStart`, `on:streamChunk`, `on:streamEnd` |
| Configurações de voz | Orquestrador (não é visual) |
| Captura de tela/webcam | Eventos no MediaPicker |
| Drag & Drop | Eventos no ChatInput |
| Paste de imagens | Evento `on:paste` no ChatInput |
| Sons de feedback | Evento `on:playSound` |
| Hotkeys globais | Orquestrador |
| Gerenciamento de conversas | Orquestrador |
| Conteúdo especial | Eventos `on:copyTable`, `on:copyCode`, `on:openLink` |

### Componentes Adicionais Necessários

```
frontend/src/components/chat/
├── modals/                      # Modais do chat
│   ├── ImageModal.svelte        # Visualização de imagem em zoom
│   └── MessageDetailModal.svelte # Detalhes/navegação da mensagem
```

### Lista Completa de Eventos por Componente

Legenda:
- ✅ = Ação local (componente executa + emite evento informativo)
- 🔄 = Ação externa (componente só emite, orquestrador implementa)

**MessageNode:**
```
Ações locais (✅):
  copied           - Texto/markdown copiado para clipboard
  linkOpened       - Link aberto em nova aba
  tableCopied      - Tabela copiada (com formato)
  codeCopied       - Código copiado
  imageDownloaded  - Imagem baixada
  imageCopied      - Imagem copiada para clipboard

Ações externas (🔄):
  speak            - Solicita TTS (depende do serviço)
  delete           - Solicita exclusão (backend)
  pin              - Solicita fixar/desafixar (backend)
  resend           - Solicita reenvio (backend)
  editStart        - Inicia modo edição
  editSave         - Solicita salvar edição (backend)
  editCancel       - Cancela edição

Navegação:
  toggle           - Expandir/recolher thread
  focus            - Mensagem recebeu foco
  boundary         - Navegação atingiu limite
  detail           - Abrir modal de detalhes (✅ se modal interno, 🔄 se externo)

Mídia:
  imageZoom        - Abrir zoom de imagem (✅ modal interno)
  mediaClick       - Clique em mídia anexada

Acessibilidade:
  announce         - Anúncio para aria-live
```

**ChatInput:**
```
Ações externas (🔄):
  submit           - Solicita envio de mensagem
  captureScreen    - Solicita captura de tela
  captureWebcam    - Solicita captura de webcam
  recordStart      - Inicia gravação de áudio
  recordStop       - Para gravação de áudio

Eventos informativos:
  change           - Conteúdo mudou
  mediaAdd         - Mídia anexada
  mediaRemove      - Mídia removida
  paste            - Conteúdo colado
  focus            - Campo recebeu foco
  blur             - Campo perdeu foco
  typing           - Estado de digitação mudou
```

**ChatHistory:**
```
Ações externas (🔄):
  loadMore         - Solicita carregar mais mensagens
  loadChildren     - Solicita carregar filhos (lazy load)

Eventos informativos:
  messageSelect    - Mensagem selecionada
  scrollEnd        - Scroll chegou ao fim
  empty            - Lista está vazia

Streaming (🔄):
  streamStart      - Streaming iniciou
  streamChunk      - Chunk recebido
  streamEnd        - Streaming terminou

Propaga todos os eventos dos filhos (MessageNode)
```

**ImageModal (componente interno):**
```
Ações locais (✅):
  downloaded       - Imagem baixada
  copied           - Imagem copiada

Navegação:
  close            - Modal fechado
  next             - Próxima imagem
  previous         - Imagem anterior
```

**MessageDetailModal (componente interno):**
```
Navegação:
  close            - Modal fechado
  navigate         - Ir para outra mensagem
```

## Plano de Implementação

### Fase 1: Estrutura Base
1. [ ] Criar pasta `components/chat/` com subpastas (message/, input/, feedback/)
2. [ ] Criar `README.md` com documentação inicial
3. [ ] Criar `index.js` com exports
4. [ ] ✅ **Testar**: Imports funcionam sem erros

### Fase 2: Migrar MessageContent e MessageNode
5. [ ] Copiar `MessageContent.svelte` → `chat/message/MessageContent.svelte`
6. [ ] Copiar `MessageNode.svelte` → `chat/message/MessageNode.svelte`
7. [ ] Refatorar para event-driven (substituir funções por dispatch)
8. [ ] Adicionar props genéricos (`author`, `isMe`, `config`)
9. [ ] Adicionar evento `announce` para acessibilidade
10. [ ] Atualizar imports no Chat.svelte original
11. [ ] ✅ **Testar**: Chat funciona igual ao antes

### Fase 3: Extrair Componentes de Mensagem
12. [ ] Extrair `MessageActions.svelte`
13. [ ] Extrair `MessageAvatar.svelte`
14. [ ] Extrair `MessageHeader.svelte`
15. [ ] Extrair `ThreadIndicator.svelte`
16. [ ] ✅ **Testar**: Mensagens renderizam corretamente

### Fase 4: Extrair Componentes de Input
17. [ ] Extrair `ChatInput.svelte` do Chat.svelte
18. [ ] Extrair `MediaPreview.svelte`
19. [ ] Extrair `MediaPicker.svelte`
20. [ ] ✅ **Testar**: Entrada de texto e mídia funcionam

### Fase 5: Extrair Componentes de Feedback
21. [ ] Criar `TypingIndicator.svelte`
22. [ ] Criar `EmptyState.svelte`
23. [ ] Criar `LoadingIndicator.svelte`
24. [ ] Criar `StreamingIndicator.svelte`
25. [ ] ✅ **Testar**: Indicadores aparecem corretamente

### Fase 6: Extrair Modais
26. [ ] Extrair `ImageModal.svelte` do Chat.svelte
27. [ ] Extrair `MessageDetailModal.svelte` do Chat.svelte
28. [ ] ✅ **Testar**: Modais abrem e fecham corretamente

### Fase 7: ChatHistory e ChatContainer
29. [ ] Extrair `ChatHistory.svelte` do Chat.svelte
30. [ ] Criar `ChatContainer.svelte` com sistema de handlers
31. [ ] ✅ **Testar**: Navegação, threads, lazy loading funcionam

### Fase 8: Adapter LLM (fora do componente)
32. [ ] Criar `frontend/src/lib/llmChatAdapter.js`
33. [ ] Implementar `convertMessages()` 
34. [ ] Implementar `createLLMHandlers()`
35. [ ] ✅ **Testar**: Adapter converte dados corretamente

### Fase 9: Integração Final
36. [ ] Atualizar `Chat.svelte` para usar ChatContainer + adapter
37. [ ] Remover código duplicado do Chat.svelte original
38. [ ] ✅ **Testar**: Todas as funcionalidades OK

### Fase 10: Sistema de Theming
39. [ ] Criar `theme/theme.css` com variáveis CSS
40. [ ] Criar `theme/theme.js` com `applyTheme()`
41. [ ] Criar `theme/presets/dark.js` e `light.js`
42. [ ] Atualizar todos os componentes para usar variáveis CSS
43. [ ] ✅ **Testar**: Tema customizado funciona

### Fase 11: Internacionalização (i18n)
44. [ ] Criar `i18n/labels.js` com `defaultLabels`
45. [ ] Criar `i18n/presets/pt-BR.js`, `en.js`
46. [ ] Atualizar componentes para usar labels via props
47. [ ] ✅ **Testar**: Labels em inglês funcionam

### Fase 12: Tipos e Slots
48. [ ] Criar `types.ts` com interfaces
49. [ ] Adicionar slots nos componentes principais
50. [ ] ✅ **Testar**: Slots permitem customização

### Fase 13: Documentação ✅ CONCLUÍDA
51. [x] Documentar cada componente no README.md
52. [x] Documentar sistema de theming
53. [x] Documentar sistema de i18n
54. [x] Documentar slots disponíveis
55. [x] Adicionar exemplos de uso genéricos
56. [x] ✅ **Testar Final**: Componente pode ser copiado para outro projeto

---

## Fases de Migração Final (Chat.svelte → ChatContainer)

**Status**: O componente `chat/` está pronto. Resta migrar o `Chat.svelte` para usar `ChatContainer`.

**Riscos**: Alto - muita lógica acoplada. Fazer em etapas pequenas com testes.

### Fase 14: Preparação
57. [ ] Criar backup do Chat.svelte atual
58. [ ] Documentar todos os estados existentes no Chat.svelte
59. [ ] Mapear dependências entre estados
60. [ ] ✅ **Testar**: Aplicação funciona normalmente

### Fase 15: ChatContainer como Base
61. [ ] Substituir imports de `ChatHistory` + `ChatInput` por `ChatContainer`
62. [ ] Passar handlers básicos (onSend, onCopy)
63. [ ] Manter lógica de streaming no Chat.svelte
64. [ ] ✅ **Testar**: Envio de mensagem funciona, histórico renderiza

### Fase 16: Modais Externos
65. [ ] Mover `ImageModal` para fora do ChatContainer (já está)
66. [ ] Mover `MessageDetailModal` para fora do ChatContainer
67. [ ] Conectar eventos `on:imageZoom` e `on:detail`
68. [ ] ✅ **Testar**: Modais abrem corretamente via eventos

### Fase 17: Menu de Contexto
69. [ ] Usar evento `on:contextMenu` do ChatContainer
70. [ ] Gerar itens com `getMessageMenuItems()` + extraItems do projeto
71. [ ] Manter `ContextMenu.svelte` existente
72. [ ] Adicionar opções específicas (TTS, STT, etc.)
73. [ ] ✅ **Testar**: Menu funciona com todas as opções

### Fase 18: Ações de Teclado
74. [ ] Implementar handler para `on:keyAction`
75. [ ] Mapear teclas para ações (Enter→detail, Space→TTS, etc.)
76. [ ] ✅ **Testar**: Navegação e ações por teclado funcionam

### Fase 19: TTS Integration
77. [ ] Conectar evento `on:keyAction` (Space) ao TTS
78. [ ] Manter lógica de TTS no Chat.svelte (SAPI5, OpenAI, WebSpeech)
79. [ ] Adicionar item TTS no menu de contexto via extraItems
80. [ ] ✅ **Testar**: TTS funciona via teclado e menu

### Fase 20: STT Integration
81. [ ] Conectar eventos `on:recordStart` e `on:recordStop`
82. [ ] Manter lógica de gravação no Chat.svelte (WebSpeech, Whisper)
83. [ ] Passar estado `isRecording` para ChatContainer
84. [ ] ✅ **Testar**: Gravação de voz funciona

### Fase 21: Streaming LLM
85. [ ] Manter lógica de streaming no Chat.svelte
86. [ ] Passar estado `isStreaming` e `streamingContent` para ChatContainer
87. [ ] Usar StreamingIndicator via slot ou prop
88. [ ] ✅ **Testar**: Streaming funciona com feedback visual

### Fase 22: Mídia (Captura, Paste, Drag)
89. [ ] Manter lógica de captura de tela no Chat.svelte
90. [ ] Manter lógica de captura de webcam no Chat.svelte
91. [ ] Conectar eventos de paste e drag do ChatInput
92. [ ] Passar `pendingMedia` para ChatContainer
93. [ ] ✅ **Testar**: Todas as formas de anexar mídia funcionam

### Fase 23: Toolbar
94. [ ] Manter Toolbar fora do ChatContainer (já está)
95. [ ] Conectar seleção de modelo
96. [ ] Conectar configurações de TTS/STT
97. [ ] Conectar toggles (showInternalMessages, useTools, etc.)
98. [ ] ✅ **Testar**: Toolbar funciona normalmente

### Fase 24: Limpeza Final
99. [ ] Remover código duplicado do Chat.svelte
100. [ ] Remover imports não utilizados
101. [ ] Verificar que Chat.svelte usa apenas ChatContainer + handlers
102. [ ] Contar linhas finais do Chat.svelte
103. [ ] ✅ **Testar Final**: Todas as funcionalidades OK

### Fase 25: Validação
104. [ ] Testar acessibilidade (navegação por teclado, leitor de tela)
105. [ ] Testar responsividade
106. [ ] Testar performance com muitas mensagens
107. [ ] Documentar qualquer breaking change
108. [ ] ✅ **Migração Completa**

---

## Resumo do Progresso

| Fase | Descrição | Status |
|------|-----------|--------|
| 1-7 | Estrutura e Componentes Base | ✅ Concluída |
| 8 | Adapter LLM | ✅ Concluída |
| 9 | Integração Inicial | ✅ Concluída |
| 10-13 | Theming, i18n, Tipos, Docs | ✅ Concluída |
| 14 | Preparação | ✅ Concluída |
| 15 | ChatContainer como Base | ✅ Concluída |
| 16 | Modais Externos | ✅ Concluída |
| 17 | Menu de Contexto | ✅ Concluída |
| 18 | Ações de Teclado | ✅ Concluída |
| 19-20 | TTS/STT Integration | ✅ Concluída |
| 21-22 | Streaming e Mídia | ✅ Concluída |
| 23 | Toolbar | ✅ Concluída |
| 24 | Limpeza Final | ✅ Concluída |
| 25 | Validação | ✅ Concluída |

**🎉 MIGRAÇÃO COMPLETA!**

### Melhorias Adicionais Implementadas

- **Svelte Context API** para navegação interna entre componentes
- **Slots modulares** (`input-area`, `messages-area`) para customização total
- **Botões reutilizáveis** (`SendButton`, `VoiceRecordButton`, `MediaButton`)
- **Arquitetura 100% event-driven** com `keyAction` genérico

---

## Exemplos de Uso

### MessageNode Isolado
```svelte
<script>
  import { MessageNode } from './components/chat';
  
  const message = {
    id: 1,
    content: 'Olá mundo!',
    timestamp: new Date()
  };
  
  const author = {
    id: 'user-1',
    name: 'João',
    avatar: '/avatars/joao.png'
  };
</script>

<MessageNode
  {message}
  {author}
  isMe={true}
  level={0}
  on:copy={(e) => navigator.clipboard.writeText(e.detail.message.content)}
  on:speak={(e) => speechSynthesis.speak(new SpeechSynthesisUtterance(e.detail.message.content))}
  on:delete={(e) => api.deleteMessage(e.detail.message.id)}
  on:edit={(e) => openEditModal(e.detail.message)}
/>
```

### Chat P2P entre Usuários
```svelte
<script>
  import { ChatHistory, ChatInput } from './components/chat';
  
  let messages = [];
  
  function handleSend(e) {
    const { content, media } = e.detail;
    websocket.send({ type: 'message', content, media });
  }
  
  function handleSpeak(e) {
    myTTSService.speak(e.detail.message.content);
  }
</script>

<ChatHistory
  {messages}
  on:copy={(e) => clipboard.write(e.detail.message.content)}
  on:speak={handleSpeak}
  on:select={(e) => selectedMessage = e.detail.message}
/>

<ChatInput
  placeholder="Mensagem..."
  on:submit={handleSend}
  on:typing={(e) => websocket.send({ type: 'typing', isTyping: e.detail.isTyping })}
/>
```

### Histórico Read-Only (sem ações)
```svelte
<script>
  import { ChatHistory } from './components/chat';
</script>

<ChatHistory
  messages={archivedMessages}
  config={{ 
    actionsEnabled: false,
    showTimestamps: true 
  }}
/>
```

### Chat LLM Completo (nosso projeto)
```svelte
<script>
  // Componente genérico (pode ser copiado para qualquer projeto)
  import { ChatContainer } from './components/chat';
  
  // Adapter específico do NOSSO projeto (fora do componente)
  import { createLLMHandlers, convertMessages } from '$lib/llmChatAdapter';
  
  // Cria handlers usando nosso adapter
  const handlers = createLLMHandlers({
    sendMessage: SendMessage,
    getConversation: GetConversationWithThreads,
    getChildren: GetMessageChildren,
    updateMessage: UpdateMessage,
    deleteMessage: DeleteMessage,
    ttsService: { voice: selectedVoice, rate: voiceRate },
    runtime: runtime
  });
  
  // Converte do formato do backend para formato genérico
  $: messages = convertMessages(rawMessages);
</script>

<!-- Componente genérico recebe dados no formato padrão -->
<ChatContainer 
  {messages} 
  {handlers}
  currentUser={{ id: 'user', name: 'Você' }}
  config={{
    enableTTS: !isTTSDisabled,
    enableThreads: showInternalMessages,
    enableEditing: true,
    enablePinning: true,
    lazyLoadChildren: true,
    title: conversationTitle
  }}
/>
```

### Chat P2P com WebSocket
```svelte
<script>
  import { ChatContainer } from './components/chat';
  
  const ws = new WebSocket('wss://chat.example.com');
  let messages = [];
  
  // Handlers para chat P2P
  const handlers = {
    onSend: async (content, media) => {
      const msg = { id: Date.now(), content, media, author: currentUser };
      ws.send(JSON.stringify({ type: 'message', data: msg }));
      return msg;
    },
    
    onDelete: async (id) => {
      ws.send(JSON.stringify({ type: 'delete', id }));
    },
    
    // Sem TTS neste chat
    onSpeak: null,
    
    // Notificações
    onPlaySound: (type) => new Audio(`/sounds/${type}.mp3`).play()
  };
  
  // Recebe mensagens
  ws.onmessage = (e) => {
    const { type, data } = JSON.parse(e.data);
    if (type === 'message') messages = [...messages, data];
  };
</script>

<ChatContainer 
  {messages} 
  {handlers}
  currentUser={currentUser}
  config={{
    enableTTS: false,
    enableThreads: false,
    enableEditing: false
  }}
/>
```

### Apenas Componentes Individuais
```svelte
<script>
  import { ChatHistory, ChatInput } from './components/chat';
</script>

<!-- Usar componentes separadamente ainda é possível -->
<ChatHistory
  {messages}
  on:copied={(e) => console.log('Copiou:', e.detail)}
  on:delete={(e) => handleDelete(e.detail.message)}
/>

<ChatInput
  on:submit={(e) => handleSend(e.detail)}
/>
```

---

## Sistema de Theming

### Variáveis CSS Customizáveis

```css
/* theme.css - Variáveis padrão */
:root {
  /* Cores principais */
  --chat-bg: #ffffff;
  --chat-text: #1a1a1a;
  --chat-text-secondary: #666666;
  --chat-border: #e0e0e0;
  
  /* Mensagens do usuário */
  --chat-user-bg: #0066cc;
  --chat-user-text: #ffffff;
  --chat-user-border: #0055aa;
  
  /* Mensagens do sistema/assistente */
  --chat-assistant-bg: #f5f5f5;
  --chat-assistant-text: #1a1a1a;
  --chat-assistant-border: #e0e0e0;
  
  /* Mensagens internas (threads) */
  --chat-internal-bg: #fafafa;
  --chat-internal-border: #eeeeee;
  
  /* Input */
  --chat-input-bg: #ffffff;
  --chat-input-border: #cccccc;
  --chat-input-focus: #0066cc;
  
  /* Ações/Botões */
  --chat-action-bg: transparent;
  --chat-action-hover: #f0f0f0;
  --chat-action-active: #e0e0e0;
  --chat-action-danger: #cc0000;
  
  /* Feedback */
  --chat-loading: #0066cc;
  --chat-error: #cc0000;
  --chat-success: #00aa00;
  
  /* Tipografia */
  --chat-font-family: system-ui, -apple-system, sans-serif;
  --chat-font-size: 14px;
  --chat-line-height: 1.5;
  
  /* Espaçamento */
  --chat-spacing-xs: 4px;
  --chat-spacing-sm: 8px;
  --chat-spacing-md: 16px;
  --chat-spacing-lg: 24px;
  
  /* Bordas */
  --chat-radius-sm: 4px;
  --chat-radius-md: 8px;
  --chat-radius-lg: 16px;
  
  /* Sombras */
  --chat-shadow: 0 2px 8px rgba(0, 0, 0, 0.1);
}
```

### Aplicar Tema Customizado

```javascript
// theme.js
export const defaultTheme = { /* variáveis padrão */ };

export function applyTheme(theme) {
  const root = document.documentElement;
  Object.entries(theme).forEach(([key, value]) => {
    root.style.setProperty(`--chat-${key}`, value);
  });
}

// Temas prontos (opcionais)
export const darkTheme = {
  'bg': '#1a1a1a',
  'text': '#ffffff',
  'text-secondary': '#aaaaaa',
  'user-bg': '#0066cc',
  'assistant-bg': '#2a2a2a',
  // ...
};
```

### Uso

```svelte
<script>
  import { ChatContainer, applyTheme, darkTheme } from './components/chat';
  
  // Aplicar tema escuro
  applyTheme(darkTheme);
  
  // Ou tema customizado
  applyTheme({
    'user-bg': '#7c3aed',      // Roxo
    'user-text': '#ffffff',
    'assistant-bg': '#fef3c7', // Amarelo claro
    'radius-md': '20px'        // Bordas mais arredondadas
  });
</script>

<ChatContainer ... />
```

### Tema via Props (alternativa)

```svelte
<ChatContainer 
  {messages}
  {handlers}
  theme={{
    userBg: '#7c3aed',
    userText: '#ffffff',
    assistantBg: '#fef3c7'
  }}
/>
```

---

## Resumo das Decisões

1. ✅ **Ações Locais vs Externas**: Ações sem backend executam internamente + emitem evento.
2. ✅ **Sistema de Handlers**: Objeto `handlers` registra callbacks (wrapper opcional).
3. ✅ **Core vs Wrapper**: Componentes core puros + wrapper opcional.
4. ✅ **Adapter EXTERNO**: `llmChatAdapter.js` fica FORA do componente.
5. ✅ **Theming**: Variáveis CSS + `applyTheme()` + presets (dark/light).
6. ✅ **i18n**: Labels configuráveis + presets (pt-BR, en, es).
7. ✅ **Slots**: Customização de partes específicas dos componentes.
8. ✅ **TypeScript**: Tipos exportados para projetos TS.
9. ✅ **Subpastas**: `core/`, `wrappers/`, `theme/`, `i18n/`, `utils/`.
10. ✅ **Documentação**: README.md com API completa.

## Checklist de Reusabilidade

### ✅ Já Coberto
- [x] Componentes puros (event-driven)
- [x] Sistema de handlers (callbacks)
- [x] Wrapper opcional
- [x] Theming via CSS variables
- [x] Adapter externo ao componente
- [x] Acessibilidade (aria-live, navegação por teclado)
- [x] Modais internos (imagem, detalhes)
- [x] Lazy loading de threads

### ⚠️ A Considerar

| Item | Descrição | Prioridade |
|------|-----------|------------|
| **i18n** | Textos hardcoded ("Copiar", "Editar") devem ser configuráveis | Alta |
| **Slots** | Permitir customização de partes via slots Svelte | Média |
| **TypeScript** | Exportar tipos/interfaces para quem usa TS | Média |
| **RTL** | Suporte a idiomas da direita para esquerda | Baixa |
| **Virtual Scroll** | Para listas com milhares de mensagens | Baixa |
| **Animações** | Transições suaves (entrada/saída de mensagens) | Baixa |

---

## Internacionalização (i18n)

Os componentes não devem ter textos hardcoded. Usar sistema de labels configuráveis:

```typescript
interface ChatLabels {
  // Ações
  copy: string;           // "Copiar" / "Copy"
  copyMarkdown: string;   // "Copiar como Markdown"
  edit: string;           // "Editar" / "Edit"
  delete: string;         // "Excluir" / "Delete"
  pin: string;            // "Fixar" / "Pin"
  unpin: string;          // "Desafixar" / "Unpin"
  speak: string;          // "Ouvir" / "Listen"
  resend: string;         // "Reenviar" / "Resend"
  
  // Navegação
  expand: string;         // "Expandir" / "Expand"
  collapse: string;       // "Recolher" / "Collapse"
  interactions: string;   // "{count} interação(ões)" / "{count} interaction(s)"
  
  // Input
  placeholder: string;    // "Digite sua mensagem..." / "Type a message..."
  send: string;           // "Enviar" / "Send"
  
  // Feedback
  copied: string;         // "Copiado!" / "Copied!"
  deleted: string;        // "Mensagem excluída" / "Message deleted"
  
  // Acessibilidade
  you: string;            // "Você" / "You"
  assistant: string;      // "Assistente" / "Assistant"
  messageOf: string;      // "{n} de {total}" / "{n} of {total}"
  startOfMessages: string; // "Início das mensagens"
  endOfMessages: string;   // "Fim das mensagens"
}
```

### Uso

```svelte
<script>
  import { ChatContainer, defaultLabels } from './components/chat';
  
  // Labels em inglês
  const englishLabels = {
    ...defaultLabels,
    copy: 'Copy',
    edit: 'Edit',
    delete: 'Delete',
    you: 'You',
    assistant: 'Assistant'
  };
</script>

<ChatContainer {messages} {handlers} labels={englishLabels} />
```

### Labels Padrão (Português)

```javascript
// labels.js
export const defaultLabels = {
  copy: 'Copiar',
  copyMarkdown: 'Copiar como Markdown',
  edit: 'Editar',
  delete: 'Excluir',
  pin: 'Fixar',
  unpin: 'Desafixar',
  speak: 'Ouvir',
  resend: 'Reenviar',
  expand: 'Expandir',
  collapse: 'Recolher',
  interactions: '{count} interação(ões)',
  placeholder: 'Digite sua mensagem...',
  send: 'Enviar',
  copied: 'Copiado!',
  deleted: 'Mensagem excluída',
  you: 'Você',
  assistant: 'Assistente',
  messageOf: '{n} de {total}',
  startOfMessages: 'Início das mensagens',
  endOfMessages: 'Fim das mensagens'
};
```

---

## Slots para Customização

Permitir substituir partes específicas do componente:

```svelte
<!-- MessageNode com slots -->
<MessageNode {message} {author}>
  <!-- Slot para avatar customizado -->
  <svelte:fragment slot="avatar">
    <MyCustomAvatar user={author} />
  </svelte:fragment>
  
  <!-- Slot para ações extras -->
  <svelte:fragment slot="actions">
    <button on:click={() => handleReact(message)}>👍</button>
  </svelte:fragment>
  
  <!-- Slot para footer customizado -->
  <svelte:fragment slot="footer">
    <span class="reactions">{message.reactions}</span>
  </svelte:fragment>
</MessageNode>
```

### Slots Disponíveis

| Componente | Slots |
|------------|-------|
| MessageNode | `avatar`, `header`, `content`, `actions`, `footer` |
| ChatHistory | `empty`, `loading`, `header` |
| ChatInput | `prefix`, `suffix`, `mediaPreview` |
| ChatContainer | `toolbar`, `history`, `input` |

---

## Tipos TypeScript

Exportar tipos para projetos TypeScript:

```typescript
// types.ts
export interface Message {
  id: string | number;
  content: string;
  timestamp?: Date;
  media?: MediaItem[];
  metadata?: Record<string, unknown>;
}

export interface Author {
  id: string;
  name: string;
  avatar?: string;
  color?: string;
}

export interface MessageNode {
  message: Message;
  author: Author;
  isMe: boolean;
  level: number;
  children?: MessageNode[];
  childCount?: number;
}

export interface ChatHandlers {
  onSend?: (content: string, media: MediaItem[]) => Promise<Message>;
  onEdit?: (messageId: string, newContent: string) => Promise<void>;
  onDelete?: (messageId: string) => Promise<void>;
  // ... todos os handlers
}

export interface ChatConfig {
  enableTTS?: boolean;
  enableThreads?: boolean;
  enableEditing?: boolean;
  // ... todas as configs
}

export interface ChatLabels {
  copy: string;
  edit: string;
  // ... todos os labels
}

export type ChatTheme = Record<string, string>;
```

---

## Estrutura Final Atualizada

```
frontend/src/components/chat/
├── README.md
├── index.js                     # Exports
├── types.ts                     # Tipos TypeScript
│
├── core/                        # Componentes de visualização
│   ├── message/
│   ├── input/
│   ├── feedback/
│   ├── modals/
│   ├── ChatHistory.svelte
│   └── ChatToolbar.svelte
│
├── wrappers/
│   └── ChatContainer.svelte
│
├── theme/
│   ├── theme.css                # Variáveis CSS
│   ├── theme.js                 # applyTheme()
│   └── presets/                 # Temas prontos
│       ├── dark.js
│       └── light.js
│
├── i18n/
│   ├── labels.js                # defaultLabels
│   └── presets/                 # Traduções prontas
│       ├── en.js
│       ├── pt-BR.js
│       └── es.js
│
└── utils/
    └── helpers.js               # Funções utilitárias
```

---

## Próximos Passos

Confirme se este plano está alinhado com sua visão. Posso começar pela Fase 1 imediatamente.

