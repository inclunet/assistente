# Chat Components

Sistema de componentes modulares para chat, independente de backend.

## Filosofia

1. **Event-Driven**: Todos os componentes emitem eventos para TODAS as ações. Nenhuma lógica de execução dentro dos componentes de exibição.

2. **Independência Total**: Qualquer componente pode ser usado sozinho, sem dependências de outros componentes do chat.

3. **Zero Lógica de Negócio**: Componentes são puramente visuais. Toda lógica (TTS, API, salvamento) fica em callbacks externos.

4. **100% Externo**: Menu de contexto, modais (imagem/detalhes), toolbar - tudo fica FORA do componente de chat. O componente apenas dispara eventos.

## Estrutura de Mensagem

O componente espera mensagens no seguinte formato:

```typescript
interface Message {
  id: string | number;           // Identificador único
  content: string;               // Conteúdo da mensagem (texto ou markdown)
  role: 'user' | 'assistant' | 'agent' | 'tool' | 'system';
  timestamp?: Date | string;     // Data/hora da mensagem
  media?: MediaItem[];           // Mídia anexada (imagens, áudio, etc.)
  
  // Campos opcionais para threads/agentes
  agent_name?: string;           // Nome do agente (para role='agent')
  toolName?: string;             // Nome da ferramenta (para role='tool')
  toolCalls?: ToolCall[];        // Chamadas de ferramenta
  internal?: boolean;            // Mensagem interna (debug)
  pinned?: boolean;              // Mensagem fixada
}

interface MessageNode {
  message: Message;
  children?: MessageNode[];      // Mensagens filhas (threads)
  childCount?: number;           // Contagem de filhos (para lazy loading)
}

interface MediaItem {
  type: 'image' | 'audio' | 'document';
  preview?: string;              // URL ou base64 para preview
  url?: string;                  // URL do arquivo
  base64?: string;               // Conteúdo em base64
  altText?: string;              // Texto alternativo
  mimeType?: string;             // Tipo MIME
}
```

### Adaptando sua estrutura

Se sua estrutura de mensagem é diferente, você pode transformá-la antes de passar:

```javascript
// Sua estrutura
const myMessages = [
  { id: 1, text: 'Hello', sender: 'user', createdAt: '2024-01-01' }
];

// Transformar para o formato esperado
const chatMessages = myMessages.map(msg => ({
  message: {
    id: msg.id,
    content: msg.text,
    role: msg.sender === 'user' ? 'user' : 'assistant',
    timestamp: msg.createdAt,
  },
  children: [],
}));
```

## Dependências

### Peer Dependencies

O componente requer:

```json
{
  "peerDependencies": {
    "svelte": "^4.0.0 || ^5.0.0",
    "svelte-i18n": "^4.0.0"
  }
}
```

### Configurando svelte-i18n

```javascript
// main.js ou App.svelte
import { init, register, waitLocale } from 'svelte-i18n';

// Registrar seus locales
register('en', () => import('./locales/en.json'));
register('pt-BR', () => import('./locales/pt-BR.json'));

// Inicializar
init({
  fallbackLocale: 'en',
  initialLocale: navigator.language,
});

// Aguardar locale antes de montar o app
await waitLocale();
```

## Estrutura

```
chat/
├── index.js                     # Exports públicos
├── types.ts                     # Tipos TypeScript
├── README.md                    # Esta documentação
│
├── context/                     # Svelte Context API
│   └── navigation.js            # Store de navegação interna
│
├── core/                        # Componentes de visualização (puros)
│   ├── message/
│   │   ├── MessageNode.svelte       # Nó de mensagem recursivo
│   │   ├── MessageContent.svelte    # Renderização de conteúdo
│   │   ├── MessageHeader.svelte     # Cabeçalho com ícone/título
│   │   ├── MessageActions.svelte    # Ações no hover
│   │   ├── MessageAvatar.svelte     # Avatar do autor
│   │   ├── MessageNodeEdit.svelte   # Modo de edição
│   │   └── ThreadIndicator.svelte   # Indicador de thread
│   │
│   ├── input/
│   │   ├── ChatInput.svelte         # Campo de entrada
│   │   ├── MediaPreview.svelte      # Preview de mídia pendente
│   │   ├── MediaPicker.svelte       # Seletor de mídia
│   │   ├── SendButton.svelte        # Botão de enviar (slot-ready)
│   │   ├── VoiceRecordButton.svelte # Botão de gravar voz (slot-ready)
│   │   └── MediaButton.svelte       # Botão de anexar mídia (slot-ready)
│   │
│   ├── feedback/
│   │   ├── EmptyState.svelte        # Estado vazio
│   │   ├── LoadingIndicator.svelte  # Spinner de loading
│   │   ├── TypingIndicator.svelte   # Indicador de digitação
│   │   └── StreamingIndicator.svelte # Indicador de streaming
│   │
│   └── ChatHistory.svelte           # Container do histórico
│
├── wrappers/
│   └── ChatContainer.svelte         # Wrapper com sistema de handlers
│
├── styles/
│   ├── tokens.css                   # Design tokens (--chat-*)
│   └── themes/
│       ├── dark.css                 # Tema escuro
│       └── light.css                # Tema claro
│
├── adapters/
│   ├── assistente.css               # Adapter para este projeto
│   └── example.css                  # Adapter de exemplo/template
│
└── utils/
    ├── helpers.js                   # Funções utilitárias
    └── menuItems.js                 # Helper para gerar itens de menu de contexto
```

## Componentes Obrigatórios vs Opcionais

### Obrigatórios (core)
- `ChatContainer` - Wrapper principal
- `ChatHistory` - Lista de mensagens
- `ChatInput` - Campo de entrada
- `MessageNode` - Renderização de mensagem

### Opcionais (use nos slots)
- `SendButton`, `VoiceRecordButton`, `MediaButton` - Botões prontos para slots

### Externos (você fornece)
- **Toolbar** - Use sua própria toolbar (ex: `Toolbar.svelte`)
- **Modais** - Use seus próprios modais para imagem/detalhes
- **Menu de Contexto** - Use `ContextMenu` de `./contextmenu`

## Arquitetura

O componente de chat é **mínimo e agnóstico**. Ele NÃO contém:

- ❌ Menu de contexto
- ❌ Modal de detalhes da mensagem
- ❌ Modal de zoom de imagem
- ❌ Toolbar com configurações
- ❌ Lógica de TTS/STT
- ❌ Integração com backend

Ele apenas:
- ✅ Exibe mensagens
- ✅ Captura input do usuário
- ✅ Dispara eventos para TUDO

```
┌─────────────────────────────────────────────────────────────┐
│                     SUA APLICAÇÃO                           │
│                                                             │
│   ┌───────────────────────────────────────────────────┐    │
│   │ Sua Toolbar (TTS, STT, Modelo, Nova Conversa)     │    │
│   └───────────────────────────────────────────────────┘    │
│                                                             │
│   ┌───────────────────────────────────────────────────┐    │
│   │ ChatContainer (componente genérico)                │    │
│   │                                                    │    │
│   │  Dispara eventos:                                  │    │
│   │   • send, delete, edit, pin                        │    │
│   │   • detail, imageZoom, contextMenu                 │    │
│   │   • recordStart, recordStop                        │    │
│   │   • speak, copy, etc.                              │    │
│   └───────────────────────────────────────────────────┘    │
│                                                             │
│   ┌───────────────┐  ┌───────────────┐  ┌─────────────┐   │
│   │ Seu Modal     │  │ Seu Lightbox  │  │ ContextMenu │   │
│   │ (detalhes)    │  │ (imagem)      │  │ (seu)       │   │
│   └───────────────┘  └───────────────┘  └─────────────┘   │
└─────────────────────────────────────────────────────────────┘
```

## Uso Básico

### Importação

```javascript
import { 
  ChatContainer,      // Wrapper completo
  ChatHistory,        // Só o histórico
  ChatInput,          // Só o input
  MessageNode,        // Componente de mensagem
  ImageModal,         // Modal de imagem (use FORA do container)
  MessageDetailModal, // Modal de detalhes (use FORA do container)
  getMessageMenuItems // Helper para menu de contexto
} from './components/chat';
```

### ChatContainer

O `ChatContainer` é um wrapper mínimo que gerencia estado interno e dispara eventos:

```svelte
<script>
  import { ChatContainer, ImageModal, MessageDetailModal, getMessageMenuItems } from './components/chat';
  import ContextMenu from './components/ContextMenu.svelte';
  import MyToolbar from './components/MyToolbar.svelte';
  
  let messages = [];
  let contextMenu;
  let menuItems = [];
  
  // Modais gerenciados na sua aplicação
  let imageModal = { visible: false, src: '', alt: '' };
  let detailModal = { visible: false, content: '', media: [] };
</script>

<MyToolbar 
  on:newChat={...}
  on:toggleTTS={...}
  on:changeModel={...}
/>

<ChatContainer
  {messages}
  handlers={{
    onSend: async (content, media) => await api.sendMessage(content, media),
    onDelete: async (id) => await api.deleteMessage(id),
    onCopy: (text) => navigator.clipboard.writeText(text),
  }}
  on:detail={(e) => {
    detailModal = { visible: true, content: e.detail.message.content, media: e.detail.message.media };
  }}
  on:imageZoom={(e) => {
    imageModal = { visible: true, src: e.detail.src, alt: e.detail.alt };
  }}
  on:contextMenu={(e) => {
    menuItems = getMessageMenuItems(e.detail.message, { ... });
    contextMenu.open(e.detail.x, e.detail.y);
  }}
  on:recordStart={() => audioRecorder.start()}
  on:recordStop={() => audioRecorder.stop()}
  on:speak={(e) => tts.speak(e.detail.message.content)}
/>

<!-- Modais FORA do componente de chat -->
<ImageModal 
  visible={imageModal.visible} 
  src={imageModal.src} 
  alt={imageModal.alt}
  on:close={() => imageModal.visible = false} 
/>

<MessageDetailModal 
  open={detailModal.visible}
  content={detailModal.content}
  media={detailModal.media}
  on:close={() => detailModal.visible = false}
/>

<ContextMenu
  bind:this={contextMenu}
  items={menuItems}
  on:select={(e) => e.detail.item.action?.()}
/>
```

### Componentes Individuais

Para uso granular, use os componentes diretamente:

```svelte
<script>
  import { ChatHistory, ChatInput } from './components/chat';
  
  let messages = [];
  
  function handleSpeak(event) {
    const { message, text } = event.detail;
    tts.speak(text);
  }
  
  function handleCopy(event) {
    const { message, format } = event.detail;
    navigator.clipboard.writeText(message.content);
  }
</script>

<ChatHistory
  threadedMessages={messages}
  on:speak={handleSpeak}
  on:copy={handleCopy}
  on:delete={handleDelete}
  on:pin={handlePin}
  on:detail={handleDetail}
  on:editStart={handleEditStart}
  on:editSave={handleEditSave}
/>

<ChatInput
  showVoiceButton={true}
  isRecording={isRecording}
  on:submit={handleSubmit}
  on:typing={handleTyping}
  on:recordStart={() => {
    isRecording = true;
    audioRecorder.start();
  }}
  on:recordStop={() => {
    isRecording = false;
    const audioBlob = audioRecorder.stop();
    // Processa o áudio
  }}
/>
```

## Eventos de Gravação de Voz

O `ChatInput` suporta gravação de voz através de eventos:

```svelte
<ChatInput
  showVoiceButton={true}    <!-- Mostra botão de microfone quando input vazio -->
  isRecording={isRecording} <!-- Estado de gravação (você controla) -->
  on:recordStart={...}      <!-- Usuário clicou para gravar -->
  on:recordStop={...}       <!-- Usuário clicou para parar -->
  on:recordCancel={...}     <!-- Gravação cancelada -->
>
  <!-- Slot opcional para botão customizado -->
  <div slot="buttons" let:isRecording let:toggleRecording>
    <MyVoiceButton {isRecording} on:click={toggleRecording} />
  </div>
</ChatInput>
```

O componente NÃO implementa gravação de áudio - apenas dispara eventos e exibe estado visual.

## Sistema de Eventos

Todos os componentes são event-driven. Nenhuma lógica de negócio dentro dos componentes.

### Eventos de Teclado (keyAction)

**Eventos genéricos por tecla** - O app decide o que fazer com cada tecla:

```svelte
<ChatContainer
  on:keyAction={(e) => {
    const { key, message, originalEvent } = e.detail;
    
    // Você decide a semântica
    switch (key) {
      case 'Enter':
        openDetailModal(message);
        originalEvent.preventDefault();
        break;
      case 'Space':
        tts.speak(message.content);
        originalEvent.preventDefault();
        break;
      case 'E':
        startEditing(message);
        break;
      case 'D':
        confirmDelete(message);
        break;
      case 'C':
        copyToClipboard(message.content);
        break;
      case 'Ctrl+C':
        copyToClipboard(message.content);
        break;
      case 'R':
        resendMessage(message);
        break;
      // Qualquer outra tecla que você quiser...
    }
  }}
/>
```

**Payload do `keyAction`:**

| Campo | Tipo | Descrição |
|-------|------|-----------|
| `key` | `string` | Identificador da tecla (ex: "Enter", "Space", "Ctrl+C", "Shift+E") |
| `originalKey` | `string` | Tecla original (event.key) |
| `ctrlKey` | `boolean` | Ctrl pressionado |
| `shiftKey` | `boolean` | Shift pressionado |
| `altKey` | `boolean` | Alt pressionado |
| `metaKey` | `boolean` | Meta/Cmd pressionado |
| `message` | `object` | Objeto da mensagem |
| `index` | `number` | Índice da mensagem |
| `level` | `number` | Nível na árvore |
| `path` | `string` | Caminho na árvore |
| `originalEvent` | `KeyboardEvent` | Evento original (para preventDefault) |

**Teclas de navegação (tratadas internamente):**
- ↑/↓ - Navegar entre mensagens
- ←/→ - Expandir/recolher threads
- Home/End - Ir para primeira/última mensagem
- Escape - Recolher thread ou voltar ao pai

### Eventos do MessageNode

**100% Event-Driven** - Componentes NUNCA executam lógica de negócio (TTS, API, etc.)

| Evento | Payload | Tratamento |
|--------|---------|------------|
| `keyAction` | `{ key, message, ... }` | 📤 Externo (você decide) |
| `speak` | `{ message, text }` | 📤 Externo (TTS) |
| `copy` | `{ message, format }` | ✅ Interno (clipboard) |
| `delete` | `{ message, index }` | 📤 Externo (backend) |
| `pin` | `{ message, pinned }` | 📤 Externo (backend) |
| `resend` | `{ message }` | 📤 Externo (backend) |
| `detail` | `{ message, index, path }` | 📤 Externo (você abre modal) |
| `imageZoom` | `{ src, alt }` | 📤 Externo (você abre lightbox) |
| `contextMenu` | `{ event, message, index, x, y }` | 📤 Externo (você abre menu) |
| `editStart` | `{ message, index }` | ✅ Interno (UI) |
| `editSave` | `{ message, newContent }` | 📤 Externo (backend) |
| `editCancel` | `{ message }` | ✅ Interno (UI) |
| `toggle` | `{ path, expand }` | ✅ Interno (UI) |
| `loadChildren` | `{ messageId, path }` | 📤 Externo (backend) |
| `imageDownload` | `{ imageData }` | 📤 Externo (download) |
| `imageCopy` | `{ imageData }` | 📤 Externo (clipboard) |
| `mediaClick` | `{ media }` | 📤 Externo (você decide ação) |
| `announce` | `{ message, priority }` | ✅ Interno (aria-live) |
| `hover` | `{ index, hovered }` | ✅ Interno (UI) |
| `focus` | `{ index, path }` | ✅ Interno (UI) |
| `boundary` | `{ edge, level, path }` | ✅ Interno (navegação) |

**Legenda:**
- ✅ Interno = Tratado dentro dos componentes de chat
- 📤 Externo = Dispara evento para ser tratado pelo app

### Eventos do ChatInput

| Evento | Payload | Descrição |
|--------|---------|-----------|
| `submit` | - | Envio de mensagem |
| `typing` | `{ isTyping }` | Indicador de digitação |
| `keydown` | `{ event }` | Tecla pressionada |
| `paste` | `{ event }` | Colagem de conteúdo |
| `drop` | `{ event }` | Drag and drop |
| `removeMedia` | `{ index }` | Remove mídia pendente |
| `focus` | - | Input recebeu foco |
| `blur` | - | Input perdeu foco |
| `recordStart` | - | Iniciar gravação de voz |
| `recordStop` | - | Parar gravação de voz |
| `recordCancel` | - | Cancelar gravação |

## Menu de Contexto

O menu de contexto é **100% externo** ao componente de chat. O componente apenas:
1. Dispara evento `contextMenu` com dados da mensagem
2. Quem usa decide como montar e exibir o menu

### Arquitetura:

```
ChatContainer/ChatHistory          Seu Código               ContextMenu
        |                              |                        |
        |-- on:contextMenu ----------->|                        |
        |   { message, index,          |                        |
        |     level, x, y }            |                        |
        |                              |                        |
        |                    getMessageMenuItems()              |
        |                    + seus itens extras                |
        |                              |                        |
        |                              |-- open(x, y, items) -->|
        |                              |                        |
        |                              |<-- on:select ---------|
        |                              |                        |
        |                        item.action()                  |
```

### Exemplo completo:

```svelte
<script>
  import { ChatContainer, getMessageMenuItems } from './components/chat';
  import ContextMenu from './components/ContextMenu.svelte';
  
  let contextMenu;
  let menuItems = [];
  let isTTSEnabled = true;
  
  function handleContextMenu(e) {
    const { message, index, level, x, y } = e.detail;
    
    // Gera itens básicos + extras específicos do seu app
    const items = getMessageMenuItems(message, {
      index,
      level,
      config: { showCopy: true, showEdit: true, showDelete: true },
      extraItems: [
        // TTS - específico do seu app
        isTTSEnabled && {
          id: 'speak',
          label: 'Ouvir mensagem',
          icon: '🔊',
          position: 'afterCopy',
          action: () => synthesizeVoice(message.content)
        },
        // Outras ações customizadas
        {
          id: 'translate',
          label: 'Traduzir',
          icon: '🌐',
          position: 'afterCopy',
          action: () => translateMessage(message.content)
        }
      ].filter(Boolean),
      handlers: {
        onCopied: () => showToast('Copiado!'),
        onDelete: (d) => api.deleteMessage(d.message.id),
      },
      t: $_,
    });
    
    menuItems = items;
    contextMenu.open(x, y);
  }
</script>

<ChatContainer
  {messages}
  on:contextMenu={handleContextMenu}
/>

<ContextMenu
  bind:this={contextMenu}
  items={menuItems}
  on:select={(e) => e.detail.item.action?.()}
/>
```

### Gerando itens:

```javascript
import { getMessageMenuItems } from './components/chat';

const items = getMessageMenuItems(message, {
  index: 0,
  level: 0,
  config: {
    showCopy: true,
    showEdit: true,
    showDelete: true,
    showPin: false,
  },
  // Itens extras injetáveis (TTS, ações customizadas, etc.)
  extraItems: [
    {
      id: 'speak',
      label: 'Ouvir mensagem',
      icon: '🔊',
      shortcut: 'Space',
      position: 'afterCopy', // onde inserir no menu
      condition: (msg) => !!msg.content, // quando mostrar
      event: 'speak' // evento a disparar
    },
    {
      id: 'translate',
      label: 'Traduzir',
      icon: '🌐',
      position: 'afterCopy',
      event: 'translate'
    }
  ],
  handlers: {
    onCopied: (detail) => console.log('Copiado!'),
    onDelete: (detail) => api.delete(detail.message.id),
    // Handler para eventos de extraItems
    onEvent: (eventName, payload) => {
      if (eventName === 'speak') {
        tts.speak(payload.content);
      } else if (eventName === 'translate') {
        translator.translate(payload.content);
      }
    },
  },
  t: $_, // função de tradução
});
```

### Posições de inserção para extraItems:

| Posição | Descrição |
|---------|-----------|
| `start` | No início do menu |
| `afterCopy` | Após copiar/markdown/tela cheia (ideal para TTS) |
| `beforeManagement` | Antes de pin/resend |
| `beforeDelete` | Antes do delete |
| `end` | No final do menu (default) |

### Estrutura de um extraItem:

```typescript
interface ExtraMenuItem {
  id: string;           // Identificador único
  label: string;        // Texto exibido
  icon?: string;        // Emoji ou ícone
  shortcut?: string;    // Atalho de teclado
  event?: string;       // Nome do evento a disparar
  action?: Function;    // Ou função a executar diretamente
  danger?: boolean;     // Estilo vermelho
  position?: string;    // Onde inserir (default: 'end')
  condition?: Function; // (message, options) => boolean
  submenu?: Array;      // Submenu de itens
}
```

### Itens disponíveis:

| Helper | Descrição |
|--------|-----------|
| `getMessageMenuItems()` | Itens para mensagem (copiar, editar, falar, etc.) |
| `getCodeMenuItems()` | Itens para bloco de código |
| `getTableMenuItems()` | Itens para tabela (com submenu de formatos) |
| `getImageMenuItems()` | Itens para imagem (zoom, copiar, download) |

## Drag & Drop de Arquivos

O `ChatContainer` gerencia drag & drop internamente e dispara eventos para o host processar os arquivos.

### Eventos

| Evento | Payload | Descrição |
|--------|---------|-----------|
| `filesDropped` | `{ files: File[], source?: string }` | Arquivos foram soltos ou colados |
| `dragStateChange` | `{ isDragging: boolean }` | Estado de drag mudou |

### Exemplo

```svelte
<ChatContainer
  on:filesDropped={(e) => {
    for (const file of e.detail.files) {
      await processMediaFile(file, e.detail.source);
    }
  }}
  on:dragStateChange={(e) => {
    // Atualiza UI se necessário
    isDragging = e.detail.isDragging;
  }}
/>
```

### Com slot customizado

Se você usar o slot `input-area`, precisará lidar com drag & drop manualmente:

```svelte
<ChatContainer>
  <svelte:fragment slot="input-area">
    <div
      on:dragenter={handleDragEnter}
      on:dragover={handleDragOver}
      on:dragleave={handleDragLeave}
      on:drop={handleDrop}
    >
      <ChatInput ... />
    </div>
  </svelte:fragment>
</ChatContainer>
```

## Navegação de Threads

O `ChatContainer` gerencia expansão de threads internamente e expõe métodos públicos.

### Métodos Públicos

```javascript
let chatContainerRef;

// Expansão por índice (nível 0)
await chatContainerRef.expandThread(0);    // Expande primeira thread
chatContainerRef.collapseThread(0);         // Recolhe primeira thread
chatContainerRef.isThreadExpanded(0);       // Verifica se está expandida

// Expansão por path (qualquer nível)
await chatContainerRef.expandPath('0-1');   // Expande segundo filho da primeira thread
chatContainerRef.collapsePath('0-1');        // Recolhe

// Utilitários
chatContainerRef.getNodeByPath('0-1-2');    // Encontra node pelo path

// Lazy loading
chatContainerRef.completeChildrenLoad(path, success); // Notifica fim do carregamento
```

### Lazy Loading

Quando uma thread precisa carregar filhos do backend:

```svelte
<ChatContainer
  bind:this={chatContainerRef}
  on:loadChildren={async (e) => {
    const { messageId, path, node } = e.detail;
    
    try {
      // Carrega do seu backend
      const children = await api.getMessageChildren(messageId);
      
      // Atualiza o node
      node.children = children;
      threadedMessages = [...threadedMessages];
      
      // Notifica o container que terminou
      chatContainerRef.completeChildrenLoad(path, true);
    } catch (err) {
      chatContainerRef.completeChildrenLoad(path, false);
    }
  }}
/>
```

### Eventos de Toggle

| Evento | Payload | Descrição |
|--------|---------|-----------|
| `toggle` | `{ path, expand }` | Thread foi expandida/recolhida |
| `loadChildren` | `{ messageId, path, node }` | Precisa carregar filhos |
| `pathToggle` | `{ path, expand }` | Path foi alterado (quando controlado externamente) |

## Sistema de Handlers

O `ChatContainer` aceita um objeto `handlers` com callbacks:

```typescript
interface ChatHandlers {
  // Envio
  onSend?: (content: string, media: Media[]) => Promise<void>;
  
  // Mensagens
  onEdit?: (messageId: string, newContent: string) => Promise<void>;
  onDelete?: (messageId: string) => Promise<void>;
  onPin?: (messageId: string, pinned: boolean) => Promise<void>;
  onResend?: (message: Message) => Promise<void>;
  
  // Threads
  onLoadChildren?: (messageId: string) => Promise<MessageNode[]>;
  
  // TTS
  onSpeak?: (text: string) => void;
  
  // Ações locais (fallbacks disponíveis)
  onCopy?: (text: string) => Promise<boolean>;
  onOpenLink?: (url: string) => boolean;
  
  // Streaming
  onStreamCancel?: () => void;
  
  // Erros
  onError?: (error: Error) => void;
  
  // Acessibilidade
  onAnnounce?: (message: string, priority: 'polite' | 'assertive') => void;
}
```

## Slots - Arquitetura Modular

A arquitetura de slots permite substituição completa de qualquer área do chat.

### Slots Principais do ChatContainer

| Slot | Descrição | Props Disponíveis |
|------|-----------|-------------------|
| `messages-area` | Substitui TODA a área de mensagens | messages, threadedMessages, isLoading, isTyping, error |
| `input-area` | Substitui TODA a área de input | inputMessage, pendingMedia, disabled, isLoading, canSendMessage, placeholder, etc. |
| `header` | Área acima do histórico | - |
| `footer` | Área abaixo do input | - |

### Exemplo: Customização Completa com ContextMenuTrigger

```svelte
<ChatContainer {messages} {threadedMessages}>
  <!-- Área de input customizada com menu de contexto -->
  <svelte:fragment slot="input-area">
    <ContextMenuTrigger items={inputMenuItems} on:select={handleInputMenuSelect}>
      <div class="input-wrapper">
        <ChatInput bind:inputMessage on:submit={handleSubmit}>
          <!-- Botão de mídia -->
          <svelte:fragment slot="prefix">
            <MediaButton on:click={openFilePicker} hasMedia={pendingMedia.length > 0} />
          </svelte:fragment>
          
          <!-- Botões de ação -->
          <svelte:fragment slot="buttons">
            {#if showVoiceButton}
              <VoiceRecordButton 
                {isRecording} 
                mode="ptt"
                on:recordStart={handleRecordStart}
                on:recordStop={handleRecordStop}
              />
            {:else}
              <SendButton {isLoading} disabled={!canSendMessage} />
            {/if}
          </svelte:fragment>
        </ChatInput>
      </div>
    </ContextMenuTrigger>
  </svelte:fragment>
</ChatContainer>
```

### Componentes de Botão Prontos

O pacote inclui botões reutilizáveis para usar nos slots:

| Componente | Uso | Props |
|------------|-----|-------|
| `SendButton` | Botão de enviar com estados | `disabled`, `isLoading`, `isGeneratingAltText` |
| `VoiceRecordButton` | Botão de gravar voz | `isRecording`, `mode` (ptt/toggle/vad), `disabled` |
| `MediaButton` | Botão de anexar mídia | `disabled`, `hasMedia` |

```javascript
import { 
  SendButton, 
  VoiceRecordButton, 
  MediaButton 
} from './components/chat';
```

### Por que Slots ao invés de Props/Callbacks?

1. **Flexibilidade total**: Você pode envolver qualquer área com seus componentes (ContextMenu, etc.)
2. **Lógica customizada**: Decidir quando mostrar qual botão com sua própria lógica
3. **Menos props**: Não precisa passar dezenas de props de configuração
4. **Composição**: Componentes menores que você combina como quiser

## Slots Detalhados

Componentes suportam slots para customização:

### MessageNode

- `avatar` - Avatar customizado
- `header` - Header customizado
- `content` - Conteúdo customizado
- `actions` - Ações customizadas
- `footer` - Rodapé customizado

### ChatHistory

- `header` - Conteúdo antes das mensagens
- `empty` - Estado vazio customizado
- `loading` - Indicador de loading customizado
- `footer` - Conteúdo após as mensagens

### ChatInput

- `prefix` - Antes do input
- `mediaPreview` - Preview de mídia customizado
- `buttons` - Botões customizados (enviar, voz, etc.)
- `suffix` - Após o input

### ChatContainer

- `toolbar` - Toolbar customizada
- `toolbar-actions` - Ações na toolbar
- `before-messages` - Antes do histórico
- `after-messages` - Após o histórico
- `input-prefix` - Antes do input
- `input-buttons` - Botões do input
- `input-suffix` - Após o input
- `empty` - Estado vazio
- `loading` - Loading

## Theming

O sistema usa CSS custom properties com prefixo `--chat-*`.

### Tokens Disponíveis

```css
/* Cores principais */
--chat-color-bg
--chat-color-text
--chat-color-border
--chat-color-focus-ring

/* Mensagens */
--chat-color-user-bg
--chat-color-assistant-bg
--chat-color-agent-bg
--chat-color-tool-bg

/* Input */
--chat-color-input-bg
--chat-color-input-border

/* Botões */
--chat-btn-primary-bg
--chat-btn-primary-text

/* Espaçamentos */
--chat-space-1, --chat-space-2, --chat-space-3, --chat-space-4

/* Tipografia */
--chat-font-family
--chat-font-size-base
--chat-font-size-sm
```

### Uso

1. Importe os tokens base (obrigatório):
```javascript
import './components/chat/styles/tokens.css';
```

2. Opcionalmente, importe um tema:
```javascript
import './components/chat/styles/themes/dark.css';
```

3. Ou crie um adapter para seu design system:
```css
/* adapters/meu-projeto.css */
:root {
  --chat-color-bg: var(--meu-bg);
  --chat-color-text: var(--meu-texto);
  /* ... */
}
```

### Adapter de Exemplo

Veja `adapters/example.css` para um template completo que você pode copiar e customizar.
O arquivo documenta todos os tokens disponíveis e como mapeá-los para seu design system.

```javascript
// Copie e customize
import './components/chat/adapters/example.css';
```

## i18n

O sistema usa `svelte-i18n`. Traduções em `src/lib/locales/*.json`.

### Chaves Usadas

```json
{
  "chat": {
    "copy", "edit", "delete", "pin", "speak",
    "send", "cancel", "loading", "error",
    "you", "assistant", "agent", "tool",
    ...
  }
}
```

## Acessibilidade

- **Roving tabindex** para navegação com setas na lista de mensagens
- **Tab/Shift+Tab** para entrar/sair da lista (comportamento padrão do navegador)
- **aria-label** descritivo em cada mensagem
- **aria-live** para anúncios de ações
- **Navegação por teclado** (tratada internamente):
  - `↑/↓` - Navegar entre mensagens
  - `←/→` - Expandir/recolher threads
  - `Home/End` - Ir para início/fim da lista
  - `Escape` - Recolher thread ou voltar ao pai
  - `Tab/Shift+Tab` - Sair da lista (comportamento padrão)
- **Ações por teclado** (via evento `keyAction` - você decide):
  - `Enter` - Sugestão: ver detalhes
  - `Space` - Sugestão: TTS/play
  - `Ctrl+C` - Sugestão: copiar
  - Qualquer outra tecla que você quiser mapear

## Context API - Navegação Interna

O sistema usa **Svelte Context API** para navegação entre componentes, funcionando mesmo com slots customizados.

### Como Funciona

```
ChatContainer
  ├── setContext('chat-navigation', store)
  │
  ├── ChatHistory (getContext)
  │     └── Reage a focusTarget === 'lastMessage' | 'firstMessage'
  │
  └── [slot: input-area]
        └── ChatInput (getContext) ← funciona mesmo em slot!
              └── Reage a focusTarget === 'input'
              └── ↑ com input vazio → dispara focusLastMessage()
```

### Navegação Automática (out of the box)

| Ação | Resultado |
|------|-----------|
| `↓` na última mensagem | Foca no input |
| `↑` no input vazio | Foca na última mensagem |

Isso funciona automaticamente, mesmo quando você usa slots customizados!

### Usando o Context Diretamente

Se precisar controlar a navegação manualmente:

```javascript
import { getContext } from 'svelte';
import { CHAT_NAVIGATION_KEY } from './components/chat';

const navigation = getContext(CHAT_NAVIGATION_KEY);

// Métodos disponíveis
navigation.focusInput();         // Foca no input
navigation.focusLastMessage();   // Foca na última mensagem
navigation.focusFirstMessage();  // Foca na primeira mensagem
navigation.focusMessage(5);      // Foca na mensagem de índice 5
navigation.clearFocusTarget();   // Limpa alvo de foco
```

### Por que Context API?

1. **Funciona com slots** - Componentes em slots herdam o contexto
2. **SSR-safe** - Não usa `document` global
3. **Testável** - Pode mockar o contexto em testes
4. **Desacoplado** - Sem queries de DOM
5. **Idiomático Svelte** - Padrão do framework
