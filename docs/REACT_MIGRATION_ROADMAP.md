# Roadmap de Migração: Svelte → React

**Plano de Implementação Fase a Fase**

> 🎯 Este documento detalha o plano de migração do frontend Svelte para React, com estimativas realistas e checkpoints de validação.

---

## 📊 Visão Geral

### Estatísticas do Projeto
- **Componentes Svelte:** 73
- **Linhas de código:** ~30.000
- **Páginas principais:** 7
- **Features principais:** ~200+
- **Tempo estimado:** 6-8 semanas (1 dev fulltime)

### Princípios da Migração
1. **Não manter compatibilidade** - Reescrita limpa
2. **Backend inalterado** - Go não muda
3. **Fase por fase** - Validar antes de prosseguir
4. **Qualidade > Velocidade** - Fazer bem feito
5. **Documentar decisões** - Para futuras manutenções

---

## 🏗️ FASE 1: SETUP E FUNDAÇÃO (3-4 dias)

### Objetivos
- Criar projeto React funcional
- Configurar tooling completo
- Integração básica com Wails
- "Hello World" funcionando

### 1.1 Criar Novo Projeto React

**Opção A: Template Wails (Recomendado)**
```bash
cd c:\Users\leona\dev\assistente

# Criar diretório temporário para template
mkdir temp-react-setup
cd temp-react-setup

# Inicializar com template React+TS do Wails
wails init -n assistente-react -t react-ts

# Copiar estrutura gerada para nossa pasta frontend
cd ..
rm -rf frontend
cp -r temp-react-setup/frontend frontend

# Atualizar wails.json
# (manualmente ou via script)

# Limpar temporários
rm -rf temp-react-setup
```

**Opção B: Vite Manual**
```bash
cd frontend
npm create vite@latest . -- --template react-ts
```

### 1.2 Instalar Dependências Principais

```bash
cd frontend

# Core
npm install react react-dom react-router-dom

# State Management
npm install zustand immer

# UI Components (escolher um)
npx shadcn-ui@latest init
# OU
npm install @radix-ui/react-dialog @radix-ui/react-dropdown-menu ...
# OU
npm install @mantine/core @mantine/hooks

# Markdown & Syntax Highlighting
npm install react-markdown remark-gfm rehype-highlight
npm install react-syntax-highlighter @types/react-syntax-highlighter

# Utilities
npm install clsx tailwind-merge
npm install date-fns
npm install dompurify @types/dompurify

# i18n
npm install react-i18next i18next

# Dev Dependencies
npm install -D @types/node
npm install -D @types/react @types/react-dom
npm install -D eslint @typescript-eslint/parser @typescript-eslint/eslint-plugin
npm install -D prettier eslint-config-prettier
```

### 1.3 Configurar TypeScript

**tsconfig.json**
```json
{
  "compilerOptions": {
    "target": "ES2020",
    "useDefineForClassFields": true,
    "lib": ["ES2020", "DOM", "DOM.Iterable"],
    "module": "ESNext",
    "skipLibCheck": true,
    "moduleResolution": "bundler",
    "allowImportingTsExtensions": true,
    "resolveJsonModule": true,
    "isolatedModules": true,
    "noEmit": true,
    "jsx": "react-jsx",
    "strict": true,
    "noUnusedLocals": true,
    "noUnusedParameters": true,
    "noFallthroughCasesInSwitch": true,
    "baseUrl": ".",
    "paths": {
      "@/*": ["./src/*"],
      "@wailsjs/*": ["./wailsjs/*"]
    }
  },
  "include": ["src"],
  "references": [{ "path": "./tsconfig.node.json" }]
}
```

### 1.4 Configurar Vite

**vite.config.ts**
```typescript
import { defineConfig } from 'vite';
import react from '@vitejs/plugin-react';
import path from 'path';

export default defineConfig({
  plugins: [react()],
  resolve: {
    alias: {
      '@': path.resolve(__dirname, './src'),
      '@wailsjs': path.resolve(__dirname, './wailsjs'),
    },
  },
  server: {
    port: 5173,
  },
  build: {
    outDir: 'dist',
    emptyOutDir: true,
  },
});
```

### 1.5 Configurar ESLint e Prettier

**.eslintrc.json**
```json
{
  "extends": [
    "eslint:recommended",
    "plugin:@typescript-eslint/recommended",
    "plugin:react-hooks/recommended",
    "prettier"
  ],
  "parser": "@typescript-eslint/parser",
  "plugins": ["@typescript-eslint", "react-refresh"],
  "rules": {
    "react-refresh/only-export-components": "warn",
    "@typescript-eslint/no-explicit-any": "warn"
  }
}
```

**.prettierrc**
```json
{
  "semi": true,
  "singleQuote": true,
  "tabWidth": 2,
  "trailingComma": "es5",
  "printWidth": 100
}
```

### 1.6 Estrutura de Pastas

```
frontend/src/
├── assets/                 # Imagens, fontes, ícones
├── components/             # Componentes reutilizáveis
│   ├── ui/                # Componentes base (shadcn ou custom)
│   │   ├── Button.tsx
│   │   ├── Modal.tsx
│   │   ├── Input.tsx
│   │   └── ...
│   ├── layout/            # Layout components
│   │   ├── Layout.tsx
│   │   ├── Topbar.tsx
│   │   └── Sidebar.tsx
│   ├── markdown/          # Markdown renderer
│   │   └── Markdown.tsx
│   └── ...
├── features/              # Feature-based organization
│   ├── chat/             # Chat feature
│   │   ├── components/
│   │   ├── hooks/
│   │   ├── store/
│   │   ├── types/
│   │   └── utils/
│   ├── settings/
│   ├── faq/
│   ├── memory/
│   └── ...
├── hooks/                 # Global hooks
│   ├── useWails.ts       # Wails integration hooks
│   ├── useTheme.ts
│   └── ...
├── lib/                   # Utilities e serviços
│   ├── wails/            # Wails wrappers
│   ├── i18n/             # Translations
│   ├── utils/            # Helper functions
│   └── ...
├── store/                 # Global Zustand stores
│   ├── settingsStore.ts
│   ├── uiStore.ts
│   └── ...
├── types/                 # TypeScript types globais
│   ├── chat.ts
│   ├── settings.ts
│   └── ...
├── App.tsx               # Root component
├── main.tsx              # Entry point
└── index.css             # Global styles
```

### 1.7 Criar Hooks Wails Básicos

**hooks/useWails.ts**
```typescript
import { useEffect, useCallback } from 'react';
import { EventsOn, EventsOff } from '@wailsjs/runtime';

export function useWailsEvent<T = any>(
  eventName: string,
  handler: (data: T) => void,
  deps: React.DependencyList = []
) {
  useEffect(() => {
    const unsubscribe = EventsOn(eventName, handler);
    return () => {
      if (unsubscribe) EventsOff(unsubscribe);
    };
  }, [eventName, ...deps]);
}

export function useWailsAPI<T extends (...args: any[]) => Promise<any>>(
  apiFunction: T
) {
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<Error | null>(null);

  const execute = useCallback(
    async (...args: Parameters<T>): Promise<ReturnType<T>> => {
      setLoading(true);
      setError(null);
      try {
        const result = await apiFunction(...args);
        return result;
      } catch (err) {
        setError(err as Error);
        throw err;
      } finally {
        setLoading(false);
      }
    },
    [apiFunction]
  );

  return { execute, loading, error };
}
```

### 1.8 Migrar Bindings Wails

```bash
# Copiar wailsjs gerado
cp -r frontend-svelte-backup/wailsjs frontend/
```

### 1.9 Criar "Hello World" Funcional

**App.tsx**
```typescript
import { useState, useEffect } from 'react';
import { GetConfig } from '@wailsjs/go/main/App';

function App() {
  const [config, setConfig] = useState<any>(null);

  useEffect(() => {
    GetConfig().then(setConfig);
  }, []);

  return (
    <div className="app">
      <h1>Assistente IA - React</h1>
      <p>Config loaded: {config ? 'Yes' : 'No'}</p>
      {config && <pre>{JSON.stringify(config, null, 2)}</pre>}
    </div>
  );
}

export default App;
```

### 1.10 Testar Build

```bash
# Dev
wails dev

# Build
wails build
```

### ✅ Checkpoint Fase 1
- [ ] Projeto React inicializa sem erros
- [ ] TypeScript funcionando
- [ ] Wails dev funciona
- [ ] Consegue chamar GetConfig() e receber dados
- [ ] Build de produção funciona
- [ ] ESLint e Prettier configurados

---

## 🧱 FASE 2: INFRAESTRUTURA CORE (4-5 dias)

### Objetivos
- Sistema de roteamento
- State management setup
- Serviços JS migrados
- Theme system
- i18n setup

### 2.1 React Router Setup

**src/App.tsx**
```typescript
import { BrowserRouter, Routes, Route, Navigate } from 'react-router-dom';
import { Layout } from '@/components/layout/Layout';
import { ChatPage } from '@/features/chat/ChatPage';
import { SettingsPage } from '@/features/settings/SettingsPage';
// ... outras páginas

function App() {
  return (
    <BrowserRouter>
      <Routes>
        <Route path="/" element={<Layout />}>
          <Route index element={<Navigate to="/chat" replace />} />
          <Route path="chat" element={<ChatPage />} />
          <Route path="settings" element={<SettingsPage />} />
          <Route path="history" element={<HistoryPage />} />
          <Route path="faq" element={<FAQPage />} />
          <Route path="memory" element={<MemoryPage />} />
          <Route path="agents" element={<AgentsPage />} />
          <Route path="oauth" element={<OAuthPage />} />
        </Route>
      </Routes>
    </BrowserRouter>
  );
}
```

### 2.2 Zustand Stores

**store/settingsStore.ts**
```typescript
import { create } from 'zustand';
import { GetConfig, SaveSettings } from '@wailsjs/go/main/App';

interface SettingsState {
  apiKey: string;
  apiBaseURL: string;
  defaultModel: string;
  chatParams: {
    temperature: number;
    maxTokens: number;
    topP: number;
  };
  hasApiKey: boolean;
  loaded: boolean;
  
  // Actions
  loadConfig: () => Promise<void>;
  saveConfig: (settings: any) => Promise<void>;
  setApiKey: (key: string) => void;
  // ... outros setters
}

export const useSettingsStore = create<SettingsState>((set, get) => ({
  apiKey: '',
  apiBaseURL: 'https://api.openai.com/v1',
  defaultModel: '',
  chatParams: {
    temperature: 0.7,
    maxTokens: 4096,
    topP: 1.0,
  },
  hasApiKey: false,
  loaded: false,
  
  loadConfig: async () => {
    const config = await GetConfig();
    set({
      apiKey: config.api_key || '',
      apiBaseURL: config.api_base_url || 'https://api.openai.com/v1',
      defaultModel: config.chat_params?.model || '',
      chatParams: {
        temperature: config.chat_params?.temperature || 0.7,
        maxTokens: config.chat_params?.max_tokens || 4096,
        topP: config.chat_params?.top_p || 1.0,
      },
      hasApiKey: !!(config.api_key && config.api_key.trim()),
      loaded: true,
    });
  },
  
  saveConfig: async (settings) => {
    await SaveSettings(settings);
    await get().loadConfig();
  },
  
  setApiKey: (key) => set({ apiKey: key }),
  // ... outros setters
}));
```

**store/uiStore.ts**
```typescript
import { create } from 'zustand';
import { persist } from 'zustand/middleware';

interface UIState {
  theme: 'light' | 'dark' | 'auto';
  sidebarCollapsed: boolean;
  currentPage: string;
  
  setTheme: (theme: 'light' | 'dark' | 'auto') => void;
  toggleSidebar: () => void;
  setCurrentPage: (page: string) => void;
}

export const useUIStore = create<UIState>()(
  persist(
    (set) => ({
      theme: 'auto',
      sidebarCollapsed: false,
      currentPage: 'chat',
      
      setTheme: (theme) => set({ theme }),
      toggleSidebar: () => set((state) => ({ 
        sidebarCollapsed: !state.sidebarCollapsed 
      })),
      setCurrentPage: (page) => set({ currentPage: page }),
    }),
    {
      name: 'ui-storage',
    }
  )
);
```

### 2.3 Migrar Serviços JS

**lib/speech/tts-service.ts** (copiar e adaptar)
**lib/speech/stt-service.ts** (copiar e adaptar)
**lib/audio-feedback.ts** (copiar direto)
**lib/utils/media-service.ts** (copiar e adaptar)

### 2.4 i18n Setup

**lib/i18n/index.ts**
```typescript
import i18n from 'i18next';
import { initReactI18next } from 'react-i18next';
import ptBR from './locales/pt-BR.json';

i18n.use(initReactI18next).init({
  resources: {
    'pt-BR': { translation: ptBR },
  },
  lng: 'pt-BR',
  fallbackLng: 'pt-BR',
  interpolation: {
    escapeValue: false,
  },
});

export default i18n;
```

**lib/i18n/locales/pt-BR.json** (copiar do Svelte e adaptar)

### 2.5 Theme System

**hooks/useTheme.ts**
```typescript
import { useEffect } from 'react';
import { useUIStore } from '@/store/uiStore';

export function useTheme() {
  const theme = useUIStore((state) => state.theme);
  
  useEffect(() => {
    const root = document.documentElement;
    
    if (theme === 'auto') {
      const prefersDark = window.matchMedia('(prefers-color-scheme: dark)').matches;
      root.classList.toggle('dark', prefersDark);
    } else {
      root.classList.toggle('dark', theme === 'dark');
    }
  }, [theme]);
  
  return { theme, setTheme: useUIStore((state) => state.setTheme) };
}
```

### 2.6 Global CSS e Tailwind

**src/index.css** (configurar variáveis CSS, Tailwind, etc)

### ✅ Checkpoint Fase 2
- [ ] Roteamento funcionando entre páginas
- [ ] Zustand stores criadas e funcionais
- [ ] Serviços JS migrados e testados
- [ ] i18n configurado e traduzindo strings
- [ ] Theme system funcional (light/dark)
- [ ] CSS global aplicado

---

## 🎨 FASE 3: COMPONENTES BASE (5-7 dias)

### Objetivos
- Componentes UI reutilizáveis
- Layout principal
- Componentes de navegação
- Markdown renderer
- Pickers

### 3.1 Layout Components

**components/layout/Layout.tsx**
```typescript
import { Outlet } from 'react-router-dom';
import { Topbar } from './Topbar';
import { Sidebar } from './Sidebar';
import { useUIStore } from '@/store/uiStore';

export function Layout() {
  const sidebarCollapsed = useUIStore((state) => state.sidebarCollapsed);
  
  return (
    <div className="app-layout">
      <Topbar />
      <div className="app-content">
        <Sidebar collapsed={sidebarCollapsed} />
        <main className="main-content">
          <Outlet />
        </main>
      </div>
    </div>
  );
}
```

**components/layout/Topbar.tsx** (migrar do Svelte)
**components/layout/Sidebar.tsx** (migrar do Svelte)

### 3.2 UI Components Base

Se usando **shadcn/ui**:
```bash
npx shadcn-ui@latest add button
npx shadcn-ui@latest add dialog
npx shadcn-ui@latest add input
npx shadcn-ui@latest add select
npx shadcn-ui@latest add textarea
npx shadcn-ui@latest add toast
# ... etc
```

Ou criar componentes customizados:
- Button
- Modal
- Input
- Select
- Textarea
- Checkbox
- Radio
- etc.

### 3.3 Markdown Renderer

**components/markdown/Markdown.tsx**
```typescript
import ReactMarkdown from 'react-markdown';
import remarkGfm from 'remark-gfm';
import rehypeHighlight from 'rehype-highlight';
import { Prism as SyntaxHighlighter } from 'react-syntax-highlighter';
import { vscDarkPlus } from 'react-syntax-highlighter/dist/esm/styles/prism';

interface MarkdownProps {
  content: string;
}

export function Markdown({ content }: MarkdownProps) {
  return (
    <ReactMarkdown
      remarkPlugins={[remarkGfm]}
      rehypePlugins={[rehypeHighlight]}
      components={{
        code({ node, inline, className, children, ...props }) {
          const match = /language-(\w+)/.exec(className || '');
          return !inline && match ? (
            <SyntaxHighlighter
              style={vscDarkPlus}
              language={match[1]}
              PreTag="div"
              {...props}
            >
              {String(children).replace(/\n$/, '')}
            </SyntaxHighlighter>
          ) : (
            <code className={className} {...props}>
              {children}
            </code>
          );
        },
      }}
    >
      {content}
    </ReactMarkdown>
  );
}
```

### 3.4 DataGrid Component

**components/DataGrid.tsx** (migrar lógica do Svelte)

### 3.5 Pickers

**components/pickers/ModelPicker.tsx**
**components/pickers/VoicePicker.tsx**
**components/pickers/STTProviderPicker.tsx**
**components/pickers/ImageModelPicker.tsx**

### ✅ Checkpoint Fase 3
- [ ] Layout renderizando corretamente
- [ ] Navegação entre páginas funcional
- [ ] Componentes UI base criados
- [ ] Markdown renderizando com syntax highlight
- [ ] DataGrid funcionando
- [ ] Todos os pickers funcionais

---

## 📄 FASE 4: PÁGINAS SIMPLES (4-5 dias)

### Objetivos
- Implementar páginas menos complexas
- Ganhar confiança na arquitetura
- Validar padrões escolhidos

### 4.1 Settings Page (1-2 dias)

**features/settings/SettingsPage.tsx**
- Migrar toda lógica do Settings.svelte
- Tabs (Connection, Chat, Embeddings, Defaults)
- Formulários com validação
- Test connection
- Save/Reset

### 4.2 FAQ Manager (1 dia)

**features/faq/FAQPage.tsx**
- CRUD completo
- DataGrid
- Modal de formulário
- Busca
- Embeddings status

### 4.3 Memory Manager (1 dia)

**features/memory/MemoryPage.tsx**
- CRUD completo
- Categorias
- DataGrid
- Modal de formulário

### 4.4 Agent Manager (1-2 dias)

**features/agents/AgentsPage.tsx**
- Lista de agentes
- Editores específicos por tipo
- HTTP Agent Editor
- File Agent Config
- MCP Agent Editor
- Import/Export

### 4.5 OAuth Manager (0.5 dia)

**features/oauth/OAuthPage.tsx**
- Cards de serviços
- Connect/Disconnect
- Status visual

### 4.6 History Page (0.5 dia)

**features/history/HistoryPage.tsx**
- Lista de conversas
- Busca
- Abrir conversa no chat
- Delete

### ✅ Checkpoint Fase 4
- [ ] Settings completamente funcional
- [ ] FAQ Manager CRUD funcionando
- [ ] Memory Manager CRUD funcionando
- [ ] Agent Manager com todos os tipos
- [ ] OAuth Manager funcional
- [ ] History listando e abrindo conversas

---

## 💬 FASE 5: CHAT BÁSICO (3-4 dias)

### Objetivos
- Chat funcionando SEM tabs
- Enviar/receber mensagens
- Streaming básico
- Input e histórico

### 5.1 Chat Store

**features/chat/store/chatStore.ts**
```typescript
import { create } from 'zustand';
import { Message } from '../types';

interface ChatState {
  messages: Message[];
  isStreaming: boolean;
  conversationId: string | null;
  conversationTitle: string;
  
  addMessage: (message: Message) => void;
  updateMessage: (id: string, content: string) => void;
  setStreaming: (streaming: boolean) => void;
  clear: () => void;
  loadConversation: (id: string) => Promise<void>;
}

export const useChatStore = create<ChatState>((set, get) => ({
  messages: [],
  isStreaming: false,
  conversationId: null,
  conversationTitle: 'Nova conversa',
  
  addMessage: (message) => set((state) => ({
    messages: [...state.messages, message],
  })),
  
  updateMessage: (id, content) => set((state) => ({
    messages: state.messages.map((msg) =>
      msg.id === id ? { ...msg, content } : msg
    ),
  })),
  
  setStreaming: (streaming) => set({ isStreaming: streaming }),
  
  clear: () => set({
    messages: [],
    conversationId: null,
    conversationTitle: 'Nova conversa',
  }),
  
  loadConversation: async (id) => {
    // Implementar
  },
}));
```

### 5.2 useChat Hook

**features/chat/hooks/useChat.ts**
```typescript
import { useCallback } from 'react';
import { useChatStore } from '../store/chatStore';
import { useWailsEvent } from '@/hooks/useWails';
import { SendMessage } from '@wailsjs/go/main/App';

export function useChat() {
  const { messages, isStreaming, addMessage, updateMessage, setStreaming } = useChatStore();
  
  // Stream event
  useWailsEvent('chat:stream', (chunk: any) => {
    if (chunk.message_id) {
      updateMessage(chunk.message_id, chunk.content);
    }
  });
  
  // Done event
  useWailsEvent('chat:done', () => {
    setStreaming(false);
  });
  
  // Error event
  useWailsEvent('chat:error', (error: any) => {
    setStreaming(false);
    // Handle error
  });
  
  const sendMessage = useCallback(async (content: string) => {
    const userMessage = {
      id: generateId(),
      role: 'user',
      content,
      timestamp: Date.now(),
    };
    
    addMessage(userMessage);
    setStreaming(true);
    
    try {
      await SendMessage(content, {/* options */});
    } catch (error) {
      setStreaming(false);
      // Handle error
    }
  }, [addMessage, setStreaming]);
  
  return {
    messages,
    isStreaming,
    sendMessage,
  };
}
```

### 5.3 Chat Components

**features/chat/components/ChatHistory.tsx** (lista de mensagens)
**features/chat/components/MessageNode.tsx** (mensagem individual)
**features/chat/components/ChatInput.tsx** (input de texto)
**features/chat/components/SendButton.tsx** (botão enviar)

### 5.4 Chat Page (Simples)

**features/chat/ChatPage.tsx**
```typescript
import { ChatHistory } from './components/ChatHistory';
import { ChatInput } from './components/ChatInput';
import { useChat } from './hooks/useChat';

export function ChatPage() {
  const { messages, isStreaming, sendMessage } = useChat();
  
  return (
    <div className="chat-page">
      <ChatHistory messages={messages} />
      <ChatInput 
        onSend={sendMessage} 
        disabled={isStreaming}
      />
    </div>
  );
}
```

### ✅ Checkpoint Fase 5
- [ ] Consegue enviar mensagem
- [ ] Recebe resposta com streaming
- [ ] Histórico de mensagens renderiza
- [ ] Markdown funcionando nas mensagens
- [ ] Stop button funciona durante streaming
- [ ] Errors tratados adequadamente

---

## 🚀 FASE 6: CHAT AVANÇADO - TABS (2-3 dias)

### Objetivos
- Sistema de múltiplas tabs
- Estado isolado por tab
- Sincronização com backend

### 6.1 Chat Tabs Store

**features/chat/store/chatTabsStore.ts**
```typescript
import { create } from 'zustand';

interface Tab {
  id: string;
  title: string;
  conversationId: string | null;
}

interface ChatTabsState {
  tabs: Tab[];
  activeTabId: string;
  
  createTab: () => void;
  closeTab: (tabId: string) => void;
  setActiveTab: (tabId: string) => void;
  updateTabTitle: (tabId: string, title: string) => void;
}

export const useChatTabsStore = create<ChatTabsState>((set, get) => ({
  tabs: [{ id: 'tab-1', title: 'Nova conversa', conversationId: null }],
  activeTabId: 'tab-1',
  
  createTab: () => {
    const newTab = {
      id: `tab-${Date.now()}`,
      title: 'Nova conversa',
      conversationId: null,
    };
    set((state) => ({
      tabs: [...state.tabs, newTab],
      activeTabId: newTab.id,
    }));
  },
  
  closeTab: (tabId) => {
    set((state) => {
      const newTabs = state.tabs.filter((t) => t.id !== tabId);
      let newActiveId = state.activeTabId;
      
      if (tabId === state.activeTabId && newTabs.length > 0) {
        newActiveId = newTabs[0].id;
      }
      
      return {
        tabs: newTabs,
        activeTabId: newActiveId,
      };
    });
  },
  
  setActiveTab: (tabId) => set({ activeTabId: tabId }),
  
  updateTabTitle: (tabId, title) => set((state) => ({
    tabs: state.tabs.map((t) =>
      t.id === tabId ? { ...t, title } : t
    ),
  })),
}));
```

### 6.2 Chat Tabs Components

**features/chat/components/ChatTabsContainer.tsx**
**features/chat/components/ChatTab.tsx**
**features/chat/components/TabBar.tsx**

### 6.3 Isolamento de Estado

Usar Zustand com múltiplas stores (uma por tab) OU Context API

### 6.4 Sincronização Backend

```typescript
useWailsEvent('tabs:updated', (tabs) => {
  // Sync tabs
});

useWailsEvent('tabs:activated', (tabId) => {
  // Set active tab
});
```

### ✅ Checkpoint Fase 6
- [ ] Múltiplas tabs abertas
- [ ] Criar/fechar tabs
- [ ] Alternar entre tabs
- [ ] Estado isolado por tab
- [ ] Sincronização com backend funciona

---

## 🎤 FASE 7: VOZ E MÍDIA (3-4 dias)

### Objetivos
- TTS/STT integrado
- Upload de mídia
- Capture screen/webcam

### 7.1 TTS Integration

**features/chat/components/VoiceButton.tsx**
**features/chat/hooks/useTTS.ts**

### 7.2 STT Integration

**features/chat/components/VoiceRecordButton.tsx**
**features/chat/hooks/useSTT.ts**

### 7.3 Media Upload

**features/chat/components/MediaPicker.tsx**
**features/chat/components/MediaPreview.tsx**
**features/chat/hooks/useMediaUpload.ts**

### ✅ Checkpoint Fase 7
- [ ] TTS funcionando (play mensagens)
- [ ] STT funcionando (gravar e transcrever)
- [ ] Upload de imagens funciona
- [ ] Drag & drop funciona
- [ ] Screen capture funciona
- [ ] Preview de mídia anexada

---

## 🧵 FASE 8: THREADING E FEATURES AVANÇADAS (2-3 dias)

### Objetivos
- Threading de mensagens
- Tool calls visualization
- Preferências por conversa

### 8.1 Threading

**features/chat/components/ThreadIndicator.tsx**
**features/chat/utils/threading.ts** (lógica de árvore)

### 8.2 Tool Calls

**features/chat/components/ToolCallMessage.tsx**

### 8.3 Preferências do Chat

**features/chat/components/ChatPreferences.tsx** (modal)

### ✅ Checkpoint Fase 8
- [ ] Threading funcionando
- [ ] Tool calls visíveis
- [ ] Preferências por conversa
- [ ] Todas features do Chat completas

---

## ✨ FASE 9: REFINAMENTOS (3-4 dias)

### Objetivos
- Acessibilidade completa
- Performance
- Polish UI/UX

### 9.1 Acessibilidade

- [ ] ARIA labels everywhere
- [ ] Keyboard navigation completa
- [ ] Skip links
- [ ] Live regions
- [ ] Screen reader testing

### 9.2 Performance

- [ ] React.memo em componentes pesados
- [ ] useCallback/useMemo adequados
- [ ] Lazy loading de páginas
- [ ] Virtualização de listas (se necessário)

### 9.3 Polish

- [ ] Loading states everywhere
- [ ] Error boundaries
- [ ] Animações suaves (framer-motion?)
- [ ] Feedback sonoro
- [ ] Toast notifications

### ✅ Checkpoint Fase 9
- [ ] App totalmente acessível
- [ ] Performance otimizada
- [ ] UI polida e consistente

---

## 🧪 FASE 10: TESTES E VALIDAÇÃO (3-5 dias)

### Objetivos
- Testar todas as funcionalidades
- Comparar com versão Svelte
- Corrigir bugs encontrados

### 10.1 Teste Manual

Checklist completo de todas as features (usar REACT_MIGRATION_FEATURES.md)

### 10.2 Testes Automatizados (Opcional)

- Unit tests para hooks
- Integration tests para fluxos principais

### 10.3 Bug Fixes

- Corrigir tudo que for encontrado

### ✅ Checkpoint Fase 10
- [ ] Todas features testadas e funcionais
- [ ] Paridade com versão Svelte
- [ ] Bugs críticos corrigidos
- [ ] App pronto para produção

---

## 📦 FASE 11: BUILD E DEPLOY (1 dia)

### Objetivos
- Build de produção funcional
- Testar executável
- Documentação atualizada

### 11.1 Build Final

```bash
wails build
```

### 11.2 Testes do Executável

- Testar em Windows
- Verificar performance
- Verificar tamanho do executável

### 11.3 Atualizar Documentação

- README.md
- CHANGELOG.md
- Docs técnicos

### ✅ Checkpoint Fase 11
- [ ] Build funciona perfeitamente
- [ ] Executável testado e aprovado
- [ ] Documentação atualizada

---

## 🎯 CRONOGRAMA RESUMIDO

| Fase | Duração | Objetivo Principal |
|------|---------|-------------------|
| 1. Setup | 3-4 dias | Projeto React funcional |
| 2. Infraestrutura | 4-5 dias | Stores, routing, services |
| 3. Componentes Base | 5-7 dias | UI reutilizável |
| 4. Páginas Simples | 4-5 dias | Settings, FAQ, Memory, etc |
| 5. Chat Básico | 3-4 dias | Enviar/receber mensagens |
| 6. Chat Tabs | 2-3 dias | Múltiplas conversas |
| 7. Voz e Mídia | 3-4 dias | TTS, STT, upload |
| 8. Features Avançadas | 2-3 dias | Threading, tool calls |
| 9. Refinamentos | 3-4 dias | Acessibilidade, performance |
| 10. Testes | 3-5 dias | Validação completa |
| 11. Deploy | 1 dia | Build final |
| **TOTAL** | **33-47 dias** | **~7-9 semanas** |

---

## 📋 CHECKLIST DIÁRIO

### Todo Dia Deve:
- [ ] Commit de progresso no Git
- [ ] Testar o que foi implementado
- [ ] Atualizar este documento com progresso
- [ ] Anotar problemas/decisões

### A Cada Feature Completa:
- [ ] Testar manualmente
- [ ] Verificar acessibilidade básica
- [ ] Code review (se possível)
- [ ] Documentar mudanças importantes

---

## 🚨 PONTOS DE ATENÇÃO

### Riscos Identificados
1. **Streaming de chat** - Implementação complexa
2. **Estado de tabs** - Isolamento pode ser tricky
3. **Media upload** - Drag & drop cross-browser
4. **TTS/STT** - APIs diferentes, compatibilidade
5. **Performance** - Lista de mensagens pode crescer muito

### Mitigações
- Começar simples, adicionar complexidade gradualmente
- Testar em cada checkpoint
- Consultar docs do React/Wails
- Pedir ajuda se travar

---

## 💡 DICAS DE IMPLEMENTAÇÃO

### Estado
- **Zustand** para estado global (settings, UI)
- **Local state** (useState) para UI temporária
- **Context** para features isoladas (se necessário)

### Performance
- **Não otimizar prematuramente**
- Usar React DevTools Profiler para identificar gargalos
- Memoizar apenas quando necessário

### Código
- **TypeScript estrito** desde o início
- **Componentes pequenos** e focados
- **Extrair lógica** para hooks customizados
- **Reutilizar** ao máximo

### Commits
- **Commits pequenos** e frequentes
- **Mensagens descritivas**
- **Branch por fase** (opcional)

---

## 🎉 PRÓXIMOS PASSOS

### Começar Agora:
1. **Fazer backup** (✅ feito)
2. **Criar branch:** `git checkout -b feat/react-migration`
3. **Começar Fase 1** - Setup

### Primeira Tarefa:
```bash
cd c:\Users\leona\dev\assistente
git checkout -b feat/react-migration
# Seguir passos da Fase 1
```

---

**Última atualização:** 19 de janeiro de 2026
**Status:** Roadmap completo ✅
**Próximo passo:** Iniciar Fase 1 - Setup

---

## 📚 RECURSOS ÚTEIS

- [React Docs](https://react.dev/)
- [React Router Docs](https://reactrouter.com/)
- [Zustand Docs](https://docs.pmnd.rs/zustand)
- [Wails Docs](https://wails.io/docs/introduction)
- [shadcn/ui](https://ui.shadcn.com/)
- [React TypeScript Cheatsheet](https://react-typescript-cheatsheet.netlify.app/)

**Boa sorte! 🚀**
