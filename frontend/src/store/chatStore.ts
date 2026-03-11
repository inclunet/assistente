import { create } from 'zustand';
import { 
  SendMessage, 
  GetAllTabs,
  CreateTab,
  CloseTab,
  SetActiveTab,
  UpdateTabTitle as BackendUpdateTabTitle,
  GetMessages,
  LoadConversationInTab,
  AssignConversationToChannel,
  UnassignConversationFromChannel,
	GetMessageChildren,
} from '@wailsjs/go/main/App';
import { EventsOn } from '@wailsjs/runtime/runtime';
import { MediaFile } from '../services/mediaService';
import { database, llm, main } from '../../wailsjs/go/models';
import { announce } from '../hooks/useAnnouncer';
import { playSendSound, playReceiveSound } from '../services/audioFeedback';
import { ttsService } from '../services/tts';
import { messageAudioService } from '../services/messageAudio';
import { stripMarkdown } from '../lib/stripMarkdown';
import type { ToolCallStatus } from '../components/chat/ToolCallsSection';

// Constantes de validação de input (devem corresponder ao backend)
const MAX_MESSAGE_CONTENT_SIZE = 500 * 1024; // 500KB
const MAX_MEDIA_SIZE = 10 * 1024 * 1024; // 10MB

// Constantes de performance para streaming
const STREAM_UPDATE_DEBOUNCE_MS = 16; // ~60fps - debounce para atualizações de streaming

interface MediaData {
  name: string;
  type: string;
  data: string; // base64
  size: number;
}

// Usa os tipos gerados pelo Wails diretamente
export type MessageNode = main.MessageNode & {
  originalIndex?: number; // Adicionado pelo frontend
  isExpanded?: boolean;    // Estado de expansão controlado pela store
};

export type Message = main.EnrichedMessage & {
  _turnSegments?: TurnSegment[];
};

/**
 * Represents a segment within an agentic turn.
 * Segments alternate between text (assistant reasoning) and tool calls,
 * showing the progression: think → use tools → think → use tools → final answer.
 */
export interface TurnSegment {
  type: 'text' | 'tool_calls';
  content?: string;
  toolCalls?: Array<{
    id: string;
    type: string;
    function: { name: string; arguments: string };
    result?: string;
  }>;
}

/**
 * Deriva um array flat de mensagens da estrutura hierárquica threadedMessages.
 * Percorre a árvore em profundidade e coleta todas as mensagens.
 */
function flattenThreadedMessages(nodes: MessageNode[] | undefined): Message[] {
  if (!nodes || nodes.length === 0) return [];
  
  const flat: Message[] = [];
  
  function traverse(node: MessageNode) {
    flat.push(node.message);
    if (node.children && node.children.length > 0) {
      node.children.forEach(traverse);
    }
  }
  
  nodes.forEach(traverse);
  return flat;
}

// Tipo para criar mensagens novas (campos mínimos necessários)
export interface NewMessageData {
  role: string;
  content: string;
  isStreaming?: boolean;
  parentId?: string;
  source?: string;
}

export interface ChatTab {
  id: string;
  title: string;
  threadedMessages: MessageNode[];  // Fonte única de verdade - estrutura hierárquica
  conversationId?: number;
  createdAt: number;
  updatedAt: number;
  backendId?: number; // ID do backend (database.ChatTab)
  channel?: string;    // Canal vinculado: "signal", "telegram", etc.
  contactId?: string;  // ID do contato no canal (phone, UUID, chatID)
}

interface ChatStore {
  tabs: ChatTab[];
  activeTabId: string | null;
  isLoading: boolean;
  streamingMessageId: string | null;
  isInitialized: boolean;
  expandedThreads: Set<string>; // IDs de mensagens com threads expandidas
  editingMessageId: string | null; // ID da mensagem sendo editada (acionado por F2 ou menu)
  readingMessageId: string | null; // ID da mensagem para ativar modo leitura (acionado pelo menu de contexto)
  skipFocusRestore: boolean; // Flag para pular restauração de foco após fechar menu
  
  // Reasoning/Thinking - cadeia de pensamento do modelo durante streaming
  streamingReasoning: string | null; // Reasoning acumulado durante streaming
  isThinking: boolean; // Se está recebendo reasoning do modelo
  expandedReasonings: Set<string>; // IDs de mensagens com reasoning expandido

  // Tool calling - estado durante streaming do agentic loop
  activeToolCalls: ToolCallStatus[]; // Tool calls em execução/concluídos durante streaming
  hadToolCalls: boolean; // Se houve tool calls neste turno (para saber se precisa reload)
  completedSegments: TurnSegment[]; // Segments from previous iterations (text + tools interleaved)

  // Initialization
  initializeTabs: () => Promise<void>;
  
  // Editing
  setEditingMessageId: (id: string | null) => void;
  startEditing: (id: string) => void; // Inicia edição e marca para pular restauração de foco
  consumeSkipFocusRestore: () => boolean; // Consome o flag e retorna se deve pular
  
  // Reading mode (virtual modal)
  setReadingMessageId: (id: string | null) => void;
  startReading: (id: string) => void; // Inicia modo leitura e marca para pular restauração de foco

  // Tab management
  createTab: (activate?: boolean) => Promise<string>;
  deleteTab: (tabId: string) => Promise<void>;
  setActiveTab: (tabId: string) => Promise<void>;
  updateTabTitle: (tabId: string, title: string) => void;

  // Message management
  addMessage: (tabId: string, message: NewMessageData) => string;
  updateMessage: (tabId: string, messageId: string, content: string) => void;
  updateMessageReasoning: (tabId: string, messageId: string, reasoning: string) => void; // Atualiza reasoning
  addInternalMessage: (tabId: string, message: Message) => void; // Adiciona mensagem interna (tool call)
  clearMessages: (tabId: string) => void;
  clearActiveTab: () => void;
  loadConversationInActiveTab: (conversationId: number, conversationTitle: string) => Promise<void>;
  
  // Thread management
  toggleThreadExpanded: (messageId: string) => void;
  isThreadExpanded: (messageId: string) => boolean;
  
  // Reasoning management
  toggleReasoningExpanded: (messageId: string) => void;
  isReasoningExpanded: (messageId: string) => boolean;

  // Chat actions
  sendMessage: (content: string, mediaFiles?: MediaFile[]) => Promise<void>;
  sendMessageWithParams: (content: string, mediaFiles?: MediaFile[], paramsOverride?: Partial<llm.ChatParams>) => Promise<void>;
  stopStreaming: () => void;

  // Utility
  getActiveTab: () => ChatTab | undefined;
  getTabMessages: (tabId: string) => Message[];
  
  // Thread management
  getThreadedMessages: () => MessageNode[] | undefined;
  loadMessageChildren: (messageId: string) => Promise<MessageNode[]>;
  
  // External events
  handleConversationDeleted: (conversationId: number) => void;
  handleConversationCleared: (conversationId: number) => void;
  handleConversationRenamed: (conversationId: number, newTitle: string) => void;
  handleTabTitleUpdated: (backendTabId: number, newTitle: string) => void;
  handleDatabaseReset: () => void;
  handleTabClosed: (tabId: number) => void;
  consolidateEmptyTabs: () => void;

  // Channel assignment (bridge bidirecional)
  assignChannelToTab: (tabId: string, channel: string, contactId: string) => Promise<void>;
  unassignChannelFromTab: (tabId: string) => Promise<void>;

  // Messaging (external channels)
  reloadActiveTabMessages: () => Promise<void>;
  reloadConversationMessages: (conversationId: number) => Promise<void>;
  handleExternalIncoming: (data: {
    channel: string; from: string; fromId?: string; text: string; conversationId: number;
    newConversation?: boolean; tabId?: number; tabCreated?: boolean;
    tabTitle?: string; tabIcon?: string;
  }) => void;
}

const generateId = () => `${Date.now()}-${Math.random().toString(36).substr(2, 9)}`;

// Mapa para rastrear cleanup functions de cada tab (previne acúmulo de listeners)
const activeListeners = new Map<string, () => void>();

// Flag de reentrância para consolidateEmptyTabs
let consolidatingEmptyTabs = false;

// Contador global de listeners para debug
let activeListenerCount = 0;

// Debouncing para atualizações de streaming (reduz re-renders)
const streamUpdateTimers = new Map<string, NodeJS.Timeout>();
const pendingStreamUpdates = new Map<string, { tabId: string; messageId: string; content: string }>();

const fileToBase64 = (file: File): Promise<string> => {
  return new Promise((resolve, reject) => {
    const reader = new FileReader();
    reader.onload = () => {
      const result = reader.result as string;
      // Remove data URL prefix to get pure base64
      const base64 = result.split(',')[1];
      resolve(base64);
    };
    reader.onerror = reject;
    reader.readAsDataURL(file);
  });
};

// Converte database.ChatTab (backend) para ChatTab (frontend)
const backendTabToFrontend = (backendTab: database.ChatTab): ChatTab => {
  return {
    id: backendTab.id?.toString() || generateId(),
    backendId: backendTab.id,
    title: backendTab.title || 'Nova Conversa',
    threadedMessages: [], // Mensagens serão carregadas depois se necessário
    conversationId: backendTab.conversation_id || undefined,
    createdAt: backendTab.created_at ? Date.parse(backendTab.created_at as any) : Date.now(),
    updatedAt: backendTab.updated_at ? Date.parse(backendTab.updated_at as any) : Date.now(),
    channel: backendTab.conversation?.channel || undefined,
    contactId: backendTab.conversation?.contact_id || undefined,
  };
};

// Helper para debounced stream updates (reduz re-renders durante streaming)
const debouncedUpdateMessage = (
  tabId: string,
  messageId: string,
  content: string,
  updateFn: (tabId: string, messageId: string, content: string) => void
) => {
  const key = `${tabId}-${messageId}`;

  // Armazena o update pendente
  pendingStreamUpdates.set(key, { tabId, messageId, content });

  // Limpa timer anterior se existir
  const existingTimer = streamUpdateTimers.get(key);
  if (existingTimer) {
    clearTimeout(existingTimer);
  }

  // Agenda novo update
  const timer = setTimeout(() => {
    const pending = pendingStreamUpdates.get(key);
    if (pending) {
      updateFn(pending.tabId, pending.messageId, pending.content);
      pendingStreamUpdates.delete(key);
      streamUpdateTimers.delete(key);
    }
  }, STREAM_UPDATE_DEBOUNCE_MS);

  streamUpdateTimers.set(key, timer);
};

// Helper para forçar update imediato (usado em done/error)
const flushPendingUpdate = (
  tabId: string,
  messageId: string,
  updateFn: (tabId: string, messageId: string, content: string) => void
) => {
  const key = `${tabId}-${messageId}`;

  // Cancela timer se existir
  const existingTimer = streamUpdateTimers.get(key);
  if (existingTimer) {
    clearTimeout(existingTimer);
    streamUpdateTimers.delete(key);
  }

  // Executa update pendente imediatamente
  const pending = pendingStreamUpdates.get(key);
  if (pending) {
    updateFn(pending.tabId, pending.messageId, pending.content);
    pendingStreamUpdates.delete(key);
  }
};

// Helper extraído: remove uma tab do state e limpa seus listeners.
// Usado por handleTabClosed (caminho principal) e deleteTab (fallback para tabs locais).
function removeTabFromState(
  tabId: string,
  get: () => ChatStore,
  set: (fn: (state: ChatStore) => Partial<ChatStore>) => void,
  listeners: Map<string, () => void>,
) {
  const cleanup = listeners.get(tabId);
  if (cleanup) {
    cleanup();
    listeners.delete(tabId);
  }

  set((s) => {
    const tabIndex = s.tabs.findIndex(t => t.id === tabId);
    const newTabs = s.tabs.filter(t => t.id !== tabId);
    let newActiveTabId = s.activeTabId;
    if (s.activeTabId === tabId) {
      const nextIndex = Math.min(tabIndex, newTabs.length - 1);
      newActiveTabId = newTabs[nextIndex]?.id || null;
    }
    return { tabs: newTabs, activeTabId: newActiveTabId };
  });

  // Foca na nova guia ativa
  setTimeout(() => {
    const newActiveTabId = get().activeTabId;
    if (newActiveTabId) {
      const escaped =
        (globalThis as any).CSS?.escape?.(newActiveTabId) ??
        newActiveTabId.replace(/"/g, '\\"');
      const tabButton =
        (document.querySelector(
          `button[role="tab"][data-tab-value="${escaped}"]`
        ) as HTMLButtonElement | null) ??
        (document.querySelector(
          `[data-tab-id="${escaped}"]`
        ) as HTMLButtonElement | null);
      tabButton?.focus();
    }
  }, 50);
}

export const useChatStore = create<ChatStore>()((set, get) => {
  // Limpa localStorage relacionado ao chat na inicialização
  if (typeof window !== 'undefined') {
    const keysToRemove: string[] = [];
    for (let i = 0; i < localStorage.length; i++) {
      const key = localStorage.key(i);
      if (key && (key.includes('chat') || key.includes('tabs') || key.includes('zustand'))) {
        keysToRemove.push(key);
      }
    }
    keysToRemove.forEach(key => localStorage.removeItem(key));
  }
  
  return {
    tabs: [],
    activeTabId: null,
    isLoading: false,
    streamingMessageId: null,
    isInitialized: false,
    expandedThreads: new Set<string>(),
    editingMessageId: null,
    readingMessageId: null,
    skipFocusRestore: false,
    streamingReasoning: null, // Reasoning durante streaming
    isThinking: false, // Se está recebendo reasoning
    expandedReasonings: new Set<string>(), // IDs de mensagens com reasoning expandido
    activeToolCalls: [], // Tool calls durante streaming
    hadToolCalls: false, // Se houve tool calls neste turno
    completedSegments: [], // Segments from previous agentic iterations
    
    setEditingMessageId: (id: string | null) => {
      set({ editingMessageId: id });
    },
    
    startEditing: (id: string) => {
      // Marca para pular restauração de foco E inicia edição
      set({ editingMessageId: id, skipFocusRestore: true });
    },
    
    setReadingMessageId: (id: string | null) => {
      set({ readingMessageId: id });
    },
    
    startReading: (id: string) => {
      // Marca para pular restauração de foco E inicia modo leitura
      set({ readingMessageId: id, skipFocusRestore: true });
    },
    
    consumeSkipFocusRestore: () => {
      const shouldSkip = get().skipFocusRestore;
      if (shouldSkip) {
        set({ skipFocusRestore: false });
      }
      return shouldSkip;
    },

    initializeTabs: async () => {
    try {
      const backendTabs = await GetAllTabs();

      let tabs = backendTabs.map(backendTabToFrontend);
      
      // Se não houver tabs, cria uma aba em branco (ativa)
      if (tabs.length === 0) {
        const newBackendTab = await CreateTab('Nova Conversa', '💬', true);
        tabs = [backendTabToFrontend(newBackendTab)];
      }
      
      const activeTab = tabs.find((t: ChatTab) => t.backendId && backendTabs.find((bt: database.ChatTab) => bt.id === t.backendId)?.is_active);

      set({
        tabs,
        activeTabId: activeTab?.id || tabs[0]?.id || null,
        isInitialized: true,
      });
      
      // Carrega mensagens da tab ativa se houver conversationId
      const initialActiveTab = activeTab || tabs[0];
      
      if (initialActiveTab?.conversationId) {
        try {
          const messageNodes = await GetMessages(initialActiveTab.conversationId, null);
          
          // Backend já manda MessageNode[] pronto - adiciona apenas originalIndex
          const nodes: MessageNode[] = (messageNodes || []).map((node, index) => {
            (node as any).originalIndex = index;
            return node;
          });
          
          // Atualiza a tab com os nodes hierárquicos
          set((state) => ({
            tabs: state.tabs.map((t) =>
              t.id === initialActiveTab.id
                ? { 
                    ...t, 
                    threadedMessages: nodes, // Guarda MessageNode[] do backend com childCount
                    updatedAt: Date.now() 
                  }
                : t
            ),
          }));
        } catch (error) {
          console.error('[Chat] ❌ Erro ao carregar mensagens da tab ativa:', error);
        }
      }
    } catch (error) {
      console.error('[Chat] Error initializing tabs:', error);
      // Se falhar, cria uma aba padrão localmente
      const defaultTab: ChatTab = {
        id: generateId(),
        title: 'Nova Conversa',
        threadedMessages: [],
        createdAt: Date.now(),
        updatedAt: Date.now(),
      };
      set({
        tabs: [defaultTab],
        activeTabId: defaultTab.id,
        isInitialized: true,
      });
    }
  },

  createTab: async (activate = true) => {
    // Evita proliferação de tabs vazias:
    // - activate=false (background): retorna tab vazia existente se houver
    // - activate=true (foreground): se a tab ativa já estiver vazia, apenas retorna ela
    if (!activate) {
      const existingEmpty = get().tabs.find(tab => !tab.conversationId && tab.threadedMessages.length === 0);
      if (existingEmpty) {
        return existingEmpty.id;
      }
    } else {
      const { activeTabId, tabs } = get();
      const activeTab = tabs.find(t => t.id === activeTabId);
      if (activeTab && !activeTab.conversationId && activeTab.threadedMessages.length === 0) {
        return activeTab.id;
      }
    }

    try {
      const backendTab = await CreateTab('Nova Conversa', '💬', activate);
      const newTab = backendTabToFrontend(backendTab);
      
      set((state) => ({
        tabs: [...state.tabs, newTab],
        activeTabId: activate ? newTab.id : state.activeTabId,
      }));
      return newTab.id;
    } catch (error) {
      console.error('[Chat] Error creating tab:', error);
      // Fallback: cria tab local
      const localTab: ChatTab = {
        id: generateId(),
        title: 'Nova Conversa',
        threadedMessages: [],
        createdAt: Date.now(),
        updatedAt: Date.now(),
      };
      set((state) => ({
        tabs: [...state.tabs, localTab],
        activeTabId: activate ? localTab.id : state.activeTabId,
      }));
      return localTab.id;
    }
  },

  deleteTab: async (tabId) => {
    const state = get();
    const tab = state.tabs.find(t => t.id === tabId);
    if (!tab || state.tabs.length <= 1) return;

    if (tab.backendId) {
      // Delega ao backend — o evento tab_closed será tratado por handleTabClosed,
      // que é o ÚNICO ponto de remoção de tabs do state.
      // Round-trip Wails é < 1ms (mesmo processo): sem necessidade de optimistic update.
      try {
        await CloseTab(tab.backendId);
      } catch (error) {
        console.error('[Chat] Error closing tab in backend:', error);
      }
    } else {
      // Tab local sem backendId (fallback raro quando CreateTab falha).
      // Não há evento do backend, então remove direto do state.
      removeTabFromState(tabId, get, set, activeListeners);
    }
  },

  setActiveTab: async (tabId) => {
    const state = get();
    const tab = state.tabs.find(t => t.id === tabId);
    // previousTab removed - no longer needed after tool-calling removal
    
    // Note: OnTabInactive was removed from backend - embedding generation happens automatically
    
    try {
      if (tab?.backendId) {
        await SetActiveTab(tab.backendId);
        
        // Carrega mensagens da conversa se houver conversationId e não houver mensagens carregadas
        if (tab.conversationId && tab.threadedMessages.length === 0) {
          try {
            const backendNodes = await GetMessages(tab.conversationId, null);
            
            // Adiciona originalIndex aos nodes do backend
            const messageNodes: MessageNode[] = backendNodes.map((node, index) => {
              (node as any).originalIndex = index;
              return node;
            });
            
            // Atualiza as mensagens na tab
            set((state) => ({
              tabs: state.tabs.map((t) =>
                t.id === tabId
                  ? { 
                      ...t, 
                      threadedMessages: messageNodes, // Hierárquica com childCount do backend
                      updatedAt: Date.now() 
                    }
                  : t
              ),
            }));
          } catch (error) {
            console.error('[Chat] ❌ Erro ao carregar mensagens:', error);
          }
        }
      }
    } catch (error) {
      console.error('[Chat] Error setting active tab in backend:', error);
    }

    // Atualiza localmente (mesmo que backend falhe)
    set({ activeTabId: tabId });
    
    // Anuncia para acessibilidade
    if (tab && tab.id !== state.activeTabId) {
      const tabTitle = tab.title || 'Nova Conversa';
      const tabIndex = state.tabs.findIndex(t => t.id === tabId) + 1;
      announce(`${tabTitle}, conversa ${tabIndex} de ${state.tabs.length}`);
    }
  },

  updateTabTitle: (tabId, title) => {
    const tab = get().tabs.find(t => t.id === tabId);
    
    // Atualiza localmente imediatamente (otimista)
    set((state) => ({
      tabs: state.tabs.map((t) =>
        t.id === tabId
          ? { ...t, title, updatedAt: Date.now() }
          : t
      ),
    }));

    // Sincroniza com backend em background
    if (tab?.backendId) {
      BackendUpdateTabTitle(tab.backendId, title).catch(error => {
        console.error('[Chat] Error updating tab title in backend:', error);
      });
    }
  },

  addMessage: (tabId, message) => {
    const messageId = generateId();
    // Cria instância da classe EnrichedMessage para ter método convertValues
    // IMPORTANTE: createdAt DEVE ser string ISO, não Date object.
    // convertValues(source["createdAt"], null) tenta "new null(obj)" quando recebe um objeto,
    // causando "classs is not a constructor".
    const newMessage = new main.EnrichedMessage({
      ...message,
      id: messageId,
      timestamp: Date.now(),
      conversationId: 0, // Será atualizado pelo backend
      isStreaming: message.isStreaming ?? false,
      internal: false,
      createdAt: new Date().toISOString(),
    });

    // Cria MessageNode para visualização hierárquica
    const newNode = new main.MessageNode({
      message: newMessage,
      children: [],
      level: 0,
      childCount: 0,
    });

    set((state) => ({
      tabs: state.tabs.map((tab) =>
        tab.id === tabId
          ? {
              ...tab,
              threadedMessages: [...tab.threadedMessages, newNode],
              updatedAt: Date.now(),
            }
          : tab
      ),
    }));

    // Anuncia mensagem para leitores de tela e TTS
    // TTS é configurado pelo perfil global via ttsService (fonte de verdade)
    const isActiveTab = get().activeTabId === tabId;
    
    if (message.role === 'user') {
      // Mensagem do usuário
      playSendSound();
      
      if (ttsService.isEnabledForUser()) {
        // TTS para mensagem do usuário
        const cleanContent = stripMarkdown(message.content);
        ttsService.speak(cleanContent).catch((err: any) => {
          console.error('[Chat] TTS speak error (user):', err);
        });
      } else if (ttsService.shouldUseAriaLiveForUser()) {
        // Anuncia via aria-live se TTS não estiver ativo
        const cleanContent = stripMarkdown(message.content);
        announce(`Você: ${cleanContent}`);
      }
    } else if (message.role === 'assistant' && !message.isStreaming) {
      // Mensagem do assistente completa (não streaming)
      
      // Toca som de recebimento (apenas na aba ativa)
      if (isActiveTab) {
        playReceiveSound();
      }
      
      // Só anuncia via aria-live se TTS NÃO estiver ativo
      // (evita conflito entre TTS e leitor de tela)
      if (ttsService.shouldUseAriaLiveForAgent() && isActiveTab) {
        const cleanContent = stripMarkdown(message.content);
        announce(`Assistente: ${cleanContent}`);
      }
      
      // TTS para assistente é gerenciado no streamComplete via ttsService.speak()
    }

    return messageId;
  },

  updateMessage: (tabId, messageId, content) => {
    set((state) => ({
      tabs: state.tabs.map((tab) =>
        tab.id === tabId
          ? {
              ...tab,
              // Navega a árvore para encontrar e atualizar a mensagem
              threadedMessages: tab.threadedMessages.map((node) => {
                // Função recursiva otimizada: atualiza in-place e retorna flag
                const updateNodeContent = (n: MessageNode): boolean => {
                  // Encontrou a mensagem? Atualiza e retorna true
                  if (n.message.id === messageId) {
                    n.message.content = content;
                    return true;
                  }

                  // Busca nos filhos se existirem
                  if (n.children && n.children.length > 0) {
                    for (const child of n.children) {
                      if (updateNodeContent(child)) {
                        return true; // Early exit: já encontrou
                      }
                    }
                  }

                  return false;
                };

                updateNodeContent(node);
                return node;
              }),
              updatedAt: Date.now(),
            }
          : tab
      ),
    }));

  },

  // Atualiza reasoning de uma mensagem específica
  updateMessageReasoning: (tabId, messageId, reasoning) => {
    set((state) => ({
      tabs: state.tabs.map((tab) =>
        tab.id === tabId
          ? {
              ...tab,
              threadedMessages: tab.threadedMessages.map((node) => {
                const updateNodeReasoning = (n: MessageNode): boolean => {
                  if (n.message.id === messageId) {
                    n.message.reasoning = reasoning;
                    return true;
                  }
                  if (n.children && n.children.length > 0) {
                    for (const child of n.children) {
                      if (updateNodeReasoning(child)) {
                        return true;
                      }
                    }
                  }
                  return false;
                };
                updateNodeReasoning(node);
                return node;
              }),
              updatedAt: Date.now(),
            }
          : tab
      ),
    }));
  },
  
  // Adiciona mensagem interna (filho de uma mensagem raiz, ex: tool calls)
  // 
  // ARQUITETURA DE THREADS EM TEMPO REAL:
  // - SEMPRE adiciona mensagem à árvore no lugar correto (para manter hierarquia)
  // - SEMPRE incrementa childCount do pai (para contador de interações)
  // - A RENDERIZAÇÃO (MessageNode) controla o que mostra baseado no estado de expansão
  // 
  // Isso garante que:
  // 1. Mensagens de nível 2+ encontrem seus pais na árvore
  // 2. Contador de interações esteja sempre correto
  // 3. UI só mostra threads expandidas (comportamento existente)
  addInternalMessage: (tabId, message) => {
    const parentId = message.parentId?.toString();
    
    set((state) => {
      const tab = state.tabs.find(t => t.id === tabId);
      if (!tab) return state;
      
      // Se não tem parentId, é uma mensagem raiz - adiciona normalmente
      if (!parentId) {
        const newNode = new main.MessageNode({
          message,
          children: [],
          level: 0,
          childCount: 0,
        });
        return {
          tabs: state.tabs.map((t) =>
            t.id === tabId
              ? { ...t, threadedMessages: [...t.threadedMessages, newNode] }
              : t
          ),
        };
      }
      
      // Função recursiva para encontrar o nó pai e adicionar como filho
      // SEMPRE adiciona, independente do estado de expansão da thread
      const addToTree = (nodes: MessageNode[], targetParentId: string, level: number): { nodes: MessageNode[], found: boolean } => {
        let found = false;
        const updatedNodes = nodes.map(node => {
          // Encontrou o nó pai
          if (node.message.id === targetParentId) {
            found = true;
            
            // Verifica se a mensagem já existe como filho
            const existsInChildren = (node.children || []).some(child => child.message.id === message.id);
            if (existsInChildren) {
              return node;
            }
            
            // SEMPRE adiciona como filho E incrementa contador
            // A renderização (MessageNode) é que decide se mostra ou não baseado no isExpanded
            const newChildNode = new main.MessageNode({
              message,
              children: [],
              level: level + 1,
              childCount: 0,
            });
            
            return new main.MessageNode({
              ...node,
              children: [...(node.children || []), newChildNode],
              childCount: (node.childCount || 0) + 1,
            });
          }
          
          // Procura recursivamente nos filhos
          if (node.children && node.children.length > 0) {
            const result = addToTree(node.children, targetParentId, level + 1);
            if (result.found) {
              found = true;
              return new main.MessageNode({
                ...node,
                children: result.nodes,
              });
            }
          }
          
          return node;
        });
        
        return { nodes: updatedNodes, found };
      };
      
      // Tenta encontrar o pai e adicionar à árvore
      const result = addToTree(tab.threadedMessages, parentId, 0);
      
      if (result.found) {
        return {
          tabs: state.tabs.map((t) =>
            t.id === tabId
              ? { ...t, threadedMessages: result.nodes }
              : t
          ),
        };
      }
      
      // Pai não encontrado na árvore
      // Razões possíveis:
      // 1. Evento chegou antes do chat:messages_ready (ID da mensagem do usuário ainda é local)
      // 2. É uma mensagem de nível 2+ e o pai de nível 1 ainda não chegou
      
      // Fallback: procura a última mensagem do usuário (ancestral de todas as mensagens)
      const findLastUserMessage = (nodes: MessageNode[]): MessageNode | null => {
        for (let i = nodes.length - 1; i >= 0; i--) {
          if (nodes[i].message.role === 'user') {
            return nodes[i];
          }
        }
        return null;
      };
      
      const lastUserMessage = findLastUserMessage(tab.threadedMessages);
      
      if (lastUserMessage) {
        const fallbackResult = addToTree(tab.threadedMessages, lastUserMessage.message.id, 0);
        
        if (fallbackResult.found) {
          return {
            tabs: state.tabs.map((t) =>
              t.id === tabId
                ? { ...t, threadedMessages: fallbackResult.nodes }
                : t
            ),
          };
        }
      }
      
      // Último recurso: adiciona como nó raiz
      const newNode = new main.MessageNode({
        message,
        children: [],
        level: message.parentId ? 1 : 0,
        childCount: 0,
      });
      
      return {
        tabs: state.tabs.map((t) =>
          t.id === tabId
            ? { ...t, threadedMessages: [...t.threadedMessages, newNode] }
            : t
        ),
      };
    });
  },

  clearMessages: (tabId) => {
    set((state) => ({
      tabs: state.tabs.map((tab) =>
        tab.id === tabId
          ? { ...tab, threadedMessages: [], updatedAt: Date.now() }
          : tab
      ),
    }));
  },

  clearActiveTab: () => {
    const { activeTabId, clearMessages } = get();
    if (activeTabId) {
      clearMessages(activeTabId);
    }
  },

  loadConversationInActiveTab: async (conversationId, conversationTitle) => {
    const { activeTabId, tabs, createTab } = get();
    
    if (!activeTabId) {
      console.error('[Chat] Nenhuma aba ativa para carregar conversa');
      return;
    }

    const activeTab = tabs.find(t => t.id === activeTabId);
    if (!activeTab || !activeTab.backendId) {
      console.error('[Chat] Aba ativa não encontrada ou sem backendId');
      return;
    }

    // Detecta se estamos carregando uma conversa em uma aba vazia
    const isBlankTab = !activeTab.conversationId && activeTab.threadedMessages.length === 0;

    try {
      // Se conversationId é 0, limpa a aba (nova conversa)
      if (conversationId === 0) {
        set((state) => ({
          tabs: state.tabs.map((t) =>
            t.id === activeTabId
              ? {
                  ...t,
                  conversationId: undefined,
                  threadedMessages: [],
                  title: 'Nova Conversa',
                  updatedAt: Date.now(),
                }
              : t
          ),
        }));
        return;
      }

      // Carrega conversa no backend
      await LoadConversationInTab(activeTab.backendId, conversationId);

      // Carrega mensagens da conversa
      const backendNodes = await GetMessages(conversationId, null);

      // Adiciona originalIndex aos nodes do backend
      const messageNodes: MessageNode[] = backendNodes.map((node, index) => {
        (node as any).originalIndex = index;
        return node;
      });

      // Atualiza a aba com a nova conversa
      set((state) => {
        const newTabs = state.tabs.map((t) =>
          t.id === activeTabId
            ? {
                ...t,
                conversationId,
                threadedMessages: messageNodes,
                title: conversationTitle || 'Conversa carregada',
                updatedAt: Date.now(),
              }
            : t
        );
        return { tabs: newTabs };
      });

      // Se era uma aba em branco, cria uma nova aba em branco para futuras conversas
      if (isBlankTab) {
        setTimeout(() => {
          createTab(false).then(() => {
          }).catch((err: any) => {
            console.error('[Chat] Error creating new blank tab:', err);
          });
        }, 100);
      }
      
      // Anuncia para acessibilidade
      announce(`Conversa aberta: ${conversationTitle || 'Conversa carregada'}`);
    } catch (error) {
      console.error('[Chat] ❌ Erro ao carregar conversa na aba ativa:', error);
      throw error;
    }
  },

  sendMessage: async (content, mediaFiles) => {
        return get().sendMessageWithParams(content, mediaFiles);
      },

  sendMessageWithParams: async (content, mediaFiles, paramsOverride) => {
        const { activeTabId, addMessage, createTab, updateTabTitle, tabs } = get();

        // Validação de tamanho do conteúdo
        if (content.length > MAX_MESSAGE_CONTENT_SIZE) {
          const errorMsg = `Mensagem muito grande (${content.length} bytes). Máximo permitido: ${MAX_MESSAGE_CONTENT_SIZE} bytes (500KB)`;
          console.error('[Chat]', errorMsg);
          announce(errorMsg);
          return;
        }

        // Validação de tamanho total dos arquivos de mídia
        if (mediaFiles && mediaFiles.length > 0) {
          const totalSize = mediaFiles.reduce((acc, f) => acc + f.file.size, 0);
          // Base64 aumenta o tamanho em ~33%
          const estimatedBase64Size = Math.ceil(totalSize * 1.37);
          if (estimatedBase64Size > MAX_MEDIA_SIZE) {
            const errorMsg = `Arquivos de mídia muito grandes (~${Math.round(estimatedBase64Size / 1024 / 1024)}MB). Máximo permitido: 10MB`;
            console.error('[Chat]', errorMsg);
            announce(errorMsg);
            return;
          }
        }
        
        // Ensure we have an active tab
        let currentTabId = activeTabId;
        if (!currentTabId) {
          currentTabId = await createTab();
        }

        // Add user message
        if (!currentTabId) {
          console.error('[Chat] No active tab');
          return;
        }
        
        const userMessageId = addMessage(currentTabId, {
          role: 'user',
          content,
        });
        
        // Detecta se esta é uma aba em branco (sem conversationId) recebendo sua primeira mensagem
        const currentTab = tabs.find(t => t.id === currentTabId);
        const isBlankTab = currentTab && !currentTab.conversationId && currentTab.threadedMessages.length <= 1;
        
        // Auto-generate title from first message if it's still "Nova Conversa"
        if (currentTab && currentTab.threadedMessages.length === 0 && currentTab.title === 'Nova Conversa') {
          const title = content.slice(0, 50) + (content.length > 50 ? '...' : '');
          updateTabTitle(currentTabId, title);
        }
        
        // Se for uma aba em branco recebendo sua primeira mensagem,
        // cria uma nova aba em branco para futuras conversas (sem ativar)
        if (isBlankTab) {
          setTimeout(() => {
            createTab(false).then(() => {
            }).catch(err => {
              console.error('[Chat] Error creating new blank tab:', err);
            });
          }, 500); // Pequeno delay para não interferir com o envio da mensagem
        }

        // Add empty assistant message for streaming
        const assistantMessageId = addMessage(currentTabId, {
          role: 'assistant',
          content: '',
          isStreaming: true,
        });

        set({ isLoading: true, streamingMessageId: assistantMessageId });

        let unsubscribe: (() => void) | null = null;
        
        // IMPORTANTE: Registrar listeners ANTES de enviar mensagem
        // para garantir que capturamos todos os eventos
        
        // Listen for conversation created event
        const unsubscribeConvCreated = EventsOn('chat:conversation_created', (data: any) => {
          if (data.id && data.title) {
            const tabIdToUpdate = currentTabId;
            set((state) => ({
              tabs: state.tabs.map((t) =>
                t.id === tabIdToUpdate
                  ? { ...t, conversationId: data.id, title: data.title, updatedAt: Date.now() }
                  : t
              ),
            }));
          }
        });

        // Listen for conversation loaded in tab (backend vincula conversa à tab)
        const unsubscribeConvLoaded = EventsOn('conversation_loaded_in_tab', (data: any) => {
          if (data.tabId && data.conversationId) {
            // Encontra a tab pelo backendId e atualiza conversationId
            set((state) => ({
              tabs: state.tabs.map((t) =>
                t.backendId === data.tabId
                  ? { ...t, conversationId: data.conversationId, title: data.title || t.title, updatedAt: Date.now() }
                  : t
              ),
            }));
          }
        });

        // Listen for messages_ready - atualiza IDs das mensagens para IDs reais do banco
        // CRÍTICO: Isso permite que mensagens internas (com parentId do banco) encontrem o pai correto
        const unsubscribeMessagesReady = EventsOn('chat:messages_ready', (data: any) => {
          if (data.userMessageId) {
            const backendUserId = data.userMessageId.toString();
            
            // Atualiza o ID da mensagem do usuário para o ID real do banco
            set((state) => {
              const tab = state.tabs.find(t => t.id === currentTabId);
              if (!tab) return state;
              
              // Encontra e atualiza a mensagem do usuário
              const updateMessageId = (nodes: MessageNode[]): MessageNode[] => {
                return nodes.map(node => {
                  if (node.message.id === userMessageId) {
                    // Cria novo nó com ID atualizado
                    const updatedMessage = new main.EnrichedMessage({
                      ...node.message,
                      id: backendUserId,
                    });
                    return new main.MessageNode({
                      ...node,
                      message: updatedMessage,
                    });
                  }
                  return node;
                });
              };
              
              return {
                tabs: state.tabs.map((t) =>
                  t.id === currentTabId
                    ? { ...t, threadedMessages: updateMessageId(t.threadedMessages) }
                    : t
                ),
              };
            });
          }
        });

        try {
          const tab = get().tabs.find((t) => t.id === currentTabId);
          const conversationId = tab?.conversationId || 0;

          // Setup completion handler that will clean up everything
          // CRÍTICO: Registrar listeners ANTES de chamar SendMessage
          // pois o backend inicia streaming em goroutine e pode emitir eventos antes do await retornar
          let unsubscribeStream: (() => void) | null = null;
          let unsubscribeComplete: (() => void) | null = null;
          let cleanupExecuted = false; // Flag para evitar cleanup duplicado
          let streamingAnnounced = false; // Flag para anunciar início do streaming apenas uma vez

          const cleanup = () => {
            // Previne execução múltipla do cleanup (backend pode emitir chat:done várias vezes)
            if (cleanupExecuted) {
              return;
            }
            cleanupExecuted = true;

            activeListenerCount--;
            if (unsubscribeStream) {
              unsubscribeStream();
              unsubscribeStream = null;
            }
            if (unsubscribeComplete) {
              unsubscribeComplete();
              unsubscribeComplete = null;
            }
            activeListeners.delete(currentTabId!);
            set({ isLoading: false, streamingMessageId: null, streamingReasoning: null, isThinking: false, activeToolCalls: [], completedSegments: [] });
          };

          // Limpa listeners antigos desta tab se existirem
          const existingCleanup = activeListeners.get(currentTabId!);
          if (existingCleanup) {
            existingCleanup();
            // IMPORTANTE: Aguarda um tick para garantir que o cleanup foi processado
            await new Promise(resolve => setTimeout(resolve, 0));
          }

          // Listen for streaming chunks from backend
          activeListenerCount++;
          
          unsubscribeStream = EventsOn('chat:stream', (event: any) => {
            // IMPORTANTE: Verifica se este listener ainda é válido (não foi limpo)
            if (!activeListeners.has(currentTabId!)) {
              return;
            }

            if (event.content) {
              // Anuncia início do streaming apenas uma vez
              if (!streamingAnnounced && !event.done && !event.error) {
                streamingAnnounced = true;
                announce('Assistente está respondendo', 'polite');
              }

              // Durante streaming: usa debouncing para reduzir re-renders
              if (!event.done && !event.error) {
                debouncedUpdateMessage(
                  currentTabId!,
                  assistantMessageId,
                  event.content,
                  get().updateMessage
                );
              } else {
                // No final (done/error): flush imediatamente para garantir conteúdo final
                flushPendingUpdate(currentTabId!, assistantMessageId, get().updateMessage);
                get().updateMessage(currentTabId!, assistantMessageId, event.content);
              }
            }

            if (event.error) {
              console.error('[Chat] Stream error:', event.error);
              // Flush pendente antes do erro
              flushPendingUpdate(currentTabId!, assistantMessageId, get().updateMessage);
              get().updateMessage(currentTabId!, assistantMessageId, `Erro: ${event.error}`);
              cleanup();
            }

            // If done, mark as not streaming
            if (event.done) {
              // Obtém o conteúdo final da mensagem para anunciar
              const currentState = get();
              const currentTab = currentState.tabs.find(t => t.id === currentTabId);
              const flatMessages = flattenThreadedMessages(currentTab?.threadedMessages);
              const finalMessage = flatMessages.find(m => m.id === assistantMessageId);
              
              set((state) => ({
                tabs: state.tabs.map((tab) =>
                  tab.id === currentTabId
                    ? {
                        ...tab,
                        threadedMessages: tab.threadedMessages.map((node) => {
                          const updateStreamingStatus = (n: MessageNode): MessageNode => {
                            if (n.message.id === assistantMessageId) {
                              n.message.isStreaming = false;
                            }
                            if (n.children && n.children.length > 0) {
                              n.children = n.children.map(updateStreamingStatus);
                            }
                            return n;
                          };
                          return updateStreamingStatus(node);
                        }),
                      }
                    : tab
                ),
              }));

              // Anuncia a resposta do assistente
              if (finalMessage?.content) {
                // Verifica se esta é a aba ativa
                const isActiveTab = currentState.activeTabId === currentTabId;
                
                // Toca som de recebimento (apenas na aba ativa)
                if (isActiveTab) {
                  playReceiveSound();
                }
                
                // TTS é configurado pelo perfil global via ttsService (fonte de verdade)
                const willUseTTS = ttsService.isAutoReadEnabled();
                
                // Sintetiza e reproduz áudio para esta mensagem
                if (willUseTTS && isActiveTab && !cleanupExecuted) {
                  // Para qualquer áudio anterior
                  messageAudioService.stopAll();
                  ttsService.stop();
                  
                  // Usa speak() para reprodução
                  ttsService.speak(finalMessage.content).catch((err: any) => {
                    console.error('[Chat] TTS speak error:', err);
                  });
                }
                
                // Só anuncia via aria-live se TTS NÃO estiver ativo
                // (evita conflito entre TTS e leitor de tela)
                if (ttsService.shouldUseAriaLiveForAgent() && isActiveTab) {
                  const cleanContent = stripMarkdown(finalMessage.content);
                  announce(`Assistente: ${cleanContent}`);
                }
              }
            }
          });

          // Listen for thinking/reasoning events from model (DeepSeek, Claude, o1, etc)
          let unsubscribeThinking: (() => void) | null = null;
          unsubscribeThinking = EventsOn('chat:thinking', (event: any) => {
            if (!activeListeners.has(currentTabId!)) {
              return;
            }

            // Atualiza estado de thinking
            if (event.started) {
              // Início do thinking
              set({ isThinking: true, streamingReasoning: event.content || '' });
              announce('O modelo está pensando...', 'polite');
            } else if (event.done) {
              // Fim do thinking - salva o reasoning na mensagem
              set({ isThinking: false });
              if (event.content) {
                get().updateMessageReasoning(currentTabId!, assistantMessageId, event.content);
              }
            } else {
              // Atualização durante thinking
              set({ streamingReasoning: event.content || '' });
            }
          });

          // ==================== Tool Calling Events ====================

          // Listen for tool execution start
          let unsubscribeToolStart: (() => void) | null = null;
          unsubscribeToolStart = EventsOn('chat:tool_start', (data: any) => {
            if (!activeListeners.has(currentTabId!)) return;
            
            set((state) => ({
              hadToolCalls: true,
              activeToolCalls: [
                ...state.activeToolCalls,
                {
                  name: data.name,
                  callId: data.callId,
                  args: data.args,
                  status: 'running' as const,
                },
              ],
            }));
          });

          // Listen for tool execution end
          let unsubscribeToolEnd: (() => void) | null = null;
          unsubscribeToolEnd = EventsOn('chat:tool_end', (data: any) => {
            if (!activeListeners.has(currentTabId!)) return;
            
            set((state) => ({
              activeToolCalls: state.activeToolCalls.map((tc) =>
                tc.callId === data.callId
                  ? { ...tc, status: (data.status === 'error' ? 'error' : 'done') as 'done' | 'error', summary: data.summary }
                  : tc
              ),
            }));

            if (data.status === 'error') {
              announce(`Ferramenta ${data.name} falhou`, 'assertive');
            }
          });

          // Listen for segment done (assistant text before tool calls — for verbalization + segment accumulation)
          let unsubscribeSegmentDone: (() => void) | null = null;
          unsubscribeSegmentDone = EventsOn('chat:segment_done', (data: any) => {
            if (!activeListeners.has(currentTabId!)) return;

            if (data.hasMore) {
              const state = get();
              const newSegments: TurnSegment[] = [...state.completedSegments];

              // Snapshot completed tool calls from previous iteration
              if (state.activeToolCalls.length > 0) {
                const toolCount = state.activeToolCalls.length;
                newSegments.push({
                  type: 'tool_calls',
                  toolCalls: state.activeToolCalls.map(tc => ({
                    id: tc.callId,
                    type: 'function',
                    function: { name: tc.name, arguments: tc.args || '' },
                    result: tc.summary,
                  })),
                });
                announce(toolCount === 1 ? state.activeToolCalls[0].name : `${toolCount} ferramentas`, 'polite');
              }

              if (data.content) {
                newSegments.push({ type: 'text', content: data.content });

                // Verbalize segment text for screen reader users
                if (ttsService.isAutoReadEnabled()) {
                  ttsService.speak(data.content).catch((err: any) => {
                    console.error('[Chat] TTS segment error:', err);
                  });
                } else {
                  const cleanContent = stripMarkdown(data.content);
                  announce(cleanContent, 'assertive');
                }
              }

              set({ completedSegments: newSegments, activeToolCalls: [] });

              // Clear message content since it's now captured in completedSegments.
              // Prevents brief visual duplication before the next iteration starts streaming.
              flushPendingUpdate(currentTabId!, assistantMessageId, get().updateMessage);
              get().updateMessage(currentTabId!, assistantMessageId, '');
            }
          });

          // Listen for completion event - this signals end of entire chat process
          unsubscribeComplete = EventsOn('chat:done', () => {
            // IMPORTANTE: Verifica se este listener ainda é válido
            if (!activeListeners.has(currentTabId!)) {
              return;
            }
            const didUseTools = get().hadToolCalls;
            
            // Mark message as no longer streaming
            set((state) => {
              const tabIndex = state.tabs.findIndex((t) => t.id === currentTabId);
              if (tabIndex >= 0) {
                state.tabs[tabIndex].threadedMessages = state.tabs[tabIndex].threadedMessages.map((node) => {
                  const updateStreamingStatus = (n: MessageNode): MessageNode => {
                    if (n.message.id === assistantMessageId) {
                      n.message.isStreaming = false;
                    }
                    if (n.children && n.children.length > 0) {
                      n.children = n.children.map(updateStreamingStatus);
                    }
                    return n;
                  };
                  return updateStreamingStatus(node);
                });
              }
              return state;
            });

            // Se houve tool calls, recarrega mensagens do backend para obter a estrutura completa
            if (didUseTools) {
              const tab = get().tabs.find(t => t.id === currentTabId);
              if (tab?.conversationId) {
                GetMessages(tab.conversationId, null).then((backendNodes) => {
                  const messageNodes: MessageNode[] = backendNodes.map((node, index) => {
                    (node as any).originalIndex = index;
                    return node;
                  });
                  set((state) => ({
                    tabs: state.tabs.map((t) =>
                      t.id === currentTabId
                        ? { ...t, threadedMessages: messageNodes, updatedAt: Date.now() }
                        : t
                    ),
                    hadToolCalls: false,
                    completedSegments: [],
                  }));
                }).catch((err) => {
                  console.error('[Chat] ❌ Erro ao recarregar mensagens:', err);
                });
              }
            }
            
            // Cleanup listeners
            cleanup();
          });

          // Atualiza cleanup para incluir novos listeners
          const originalCleanup = cleanup;
          const enhancedCleanup = () => {
            originalCleanup();
            if (unsubscribeThinking) unsubscribeThinking();
            if (unsubscribeToolStart) unsubscribeToolStart();
            if (unsubscribeToolEnd) unsubscribeToolEnd();
            if (unsubscribeSegmentDone) unsubscribeSegmentDone();
          };

          // CRÍTICO: Armazena cleanup no Map IMEDIATAMENTE após criar os listeners
          // Isso garante que se uma nova mensagem for enviada antes do término,
          // os listeners antigos serão limpos corretamente
          activeListeners.set(currentTabId!, enhancedCleanup);

          // Store cleanup function for use in error handler
          unsubscribe = () => {
            cleanup();
            if (unsubscribeConvCreated) unsubscribeConvCreated();
            if (unsubscribeConvLoaded) unsubscribeConvLoaded();
            if (unsubscribeMessagesReady) unsubscribeMessagesReady();
          };

          // AGORA envia a mensagem — listeners já estão ativos para capturar streaming
          let mediaJson = '';
          if (mediaFiles && mediaFiles.length > 0) {
            const mediaDataArray: MediaData[] = [];
            for (const mediaFile of mediaFiles) {
              const base64Data = await fileToBase64(mediaFile.file);
              mediaDataArray.push({
                name: mediaFile.file.name,
                type: mediaFile.file.type,
                data: base64Data,
                size: mediaFile.file.size,
              });
            }
            mediaJson = JSON.stringify(mediaDataArray);
          }

          const mergedParams: any = {
            model: '',
            temperature: 0,
            maxTokens: 0,
            ...(paramsOverride || {}),
          };
          await SendMessage(conversationId, content, mediaJson, mergedParams);
          
          // Note: DO NOT cleanup here - listeners need to stay active for streaming events
        } catch (error: any) {
          console.error('[Chat] Error sending message:', error);
          console.error('[Chat] Error type:', typeof error, '| message:', error?.message || String(error));
          
          // Cleanup listener if it exists
          if (unsubscribe) {
            unsubscribe();
          }

          const errorMsg = error?.message || (typeof error === 'string' ? error : JSON.stringify(error));
          get().updateMessage(
            currentTabId!,
            assistantMessageId,
            `Erro ao enviar mensagem: ${errorMsg}`
          );

          // Mark as not streaming
          set((state) => {
            const tabIndex = state.tabs.findIndex((t) => t.id === currentTabId);
            if (tabIndex >= 0) {
              state.tabs[tabIndex].threadedMessages = state.tabs[tabIndex].threadedMessages.map((node) => {
                const updateStreamingStatus = (n: MessageNode): MessageNode => {
                  if (n.message.id === assistantMessageId) {
                    n.message.isStreaming = false;
                  }
                  if (n.children && n.children.length > 0) {
                    n.children = n.children.map(updateStreamingStatus);
                  }
                  return n;
                };
                return updateStreamingStatus(node);
              });
            }
            return state;
          });

          set({ isLoading: false, streamingMessageId: null });
        }
        // NOTE: No `finally` block here — streamingMessageId must stay set while
        // streaming is active. The cleanup() (triggered by chat:done) resets it.
      },

      stopStreaming: () => {
        set({ isLoading: false, streamingMessageId: null, completedSegments: [] });
        // TODO: Cancel backend request
      },

      getActiveTab: () => {
        const { tabs, activeTabId } = get();
        return tabs.find((tab) => tab.id === activeTabId);
      },

      getTabMessages: (tabId) => {
        const tab = get().tabs.find((t) => t.id === tabId);
        return flattenThreadedMessages(tab?.threadedMessages);
      },
      
      // Alterna expansão de uma thread
      toggleThreadExpanded: (messageId) => {
        set((state) => {
          const expanded = new Set(state.expandedThreads);
          if (expanded.has(messageId)) {
            expanded.delete(messageId);
          } else {
            expanded.add(messageId);
          }
          return { expandedThreads: expanded };
        });
      },
      
      // Verifica se uma thread está expandida
      isThreadExpanded: (messageId) => {
        return get().expandedThreads.has(messageId);
      },
      
      // Alterna expansão de reasoning de uma mensagem
      toggleReasoningExpanded: (messageId) => {
        set((state) => {
          const expanded = new Set(state.expandedReasonings);
          if (expanded.has(messageId)) {
            expanded.delete(messageId);
          } else {
            expanded.add(messageId);
          }
          return { expandedReasonings: expanded };
        });
      },
      
      // Verifica se o reasoning de uma mensagem está expandido
      isReasoningExpanded: (messageId) => {
        return get().expandedReasonings.has(messageId);
      },
      
      // Retorna os MessageNode[] que o backend já enviou prontos
      // Backend usa lazy loading e envia childCount correto
      getThreadedMessages: () => {
        const state = get();
        const activeTab = state.tabs.find(t => t.id === state.activeTabId);
        return activeTab?.threadedMessages;
      },
      
      // Carrega filhos de uma mensagem específica
      loadMessageChildren: async (messageId) => {
        try {
          const messageIdNum = parseInt(messageId, 10);
          if (isNaN(messageIdNum)) {
            console.error('[Chat] ❌ Invalid message ID:', messageId);
            return [];
          }
          
          // Chama backend para carregar filhos
          const backendNodes = await GetMessageChildren(messageIdNum);
          
          // Adiciona originalIndex aos nodes do backend
          const frontendNodes: MessageNode[] = (backendNodes || []).map((node, index) => {
            (node as any).originalIndex = index;
            return node;
          });
          
          // Atualiza a árvore de mensagens com os filhos carregados
          set((state) => {
            const activeTab = state.tabs.find(t => t.id === state.activeTabId);
            if (!activeTab) return state;
            
            // Atualiza a árvore com os novos filhos
            const updateTreeWithChildren = (nodes: MessageNode[]): MessageNode[] => {
              return nodes.map(node => {
                if (node.message.id === messageId) {
                  // Encontrou o nó pai - cria novo nó com os filhos
                  return new main.MessageNode({
                    ...node,
                    children: frontendNodes,
                  });
                }
                // Recursivamente procura nos filhos
                if (node.children && node.children.length > 0) {
                  return new main.MessageNode({
                    ...node,
                    children: updateTreeWithChildren(node.children),
                  });
                }
                return node;
              });
            };
            
            const updatedTabs = state.tabs.map(tab => 
              tab.id === state.activeTabId 
                ? { ...tab, threadedMessages: updateTreeWithChildren(tab.threadedMessages) }
                : tab
            );
            
            return { tabs: updatedTabs };
          });
          
          return frontendNodes;
        } catch (error) {
          console.error('[Chat] ❌ Error loading children:', error);
          return [];
        }
      },

    // Chamado quando uma conversa é deletada pelo backend
    handleConversationDeleted: (conversationId: number) => {
      announce('Conversa apagada permanentemente');
      
      const state = get();
      const tabToClose = state.tabs.find(tab => tab.conversationId === conversationId);
      
      if (!tabToClose) return;

      if (state.tabs.length > 1 && tabToClose.backendId) {
        // Delega ao backend — handleTabClosed cuidará da remoção do state
        CloseTab(tabToClose.backendId).catch(err => {
          console.error('[Chat] Error closing tab for deleted conversation:', err);
        });
      } else if (state.tabs.length > 1) {
        // Tab sem backendId: remove direto
        removeTabFromState(tabToClose.id, get, set, activeListeners);
      } else {
        // Única aba: apenas limpa
        set((s) => ({
          tabs: s.tabs.map(tab => 
            tab.id === tabToClose.id
              ? {
                  ...tab,
                  conversationId: undefined,
                  title: 'Nova Conversa',
                  threadedMessages: [] as MessageNode[],
                  updatedAt: Date.now(),
                }
              : tab
          ),
        }));
      }
      
      setTimeout(() => {
        const input = document.querySelector('textarea[placeholder*="mensagem"], textarea[aria-label*="mensagem"]') as HTMLTextAreaElement;
        if (input) input.focus();
      }, 200);
    },

    // Chamado quando uma conversa é limpa (mensagens removidas)
    handleConversationCleared: (conversationId: number) => {
      // Anuncia para acessibilidade
      announce('Mensagens da conversa removidas');
      
      // Limpa as mensagens da aba que tinha essa conversa
      set((state) => {
        const updatedTabs: ChatTab[] = state.tabs.map(tab => {
          if (tab.conversationId === conversationId) {
            return {
              ...tab,
              threadedMessages: [] as MessageNode[],
              title: 'Conversa limpa',
              updatedAt: Date.now(),
            };
          }
          return tab;
        });
        return { tabs: updatedTabs };
      });
      
      // Foca no input de mensagem após um pequeno delay
      setTimeout(() => {
        const input = document.querySelector('textarea[placeholder*="mensagem"], textarea[aria-label*="mensagem"]') as HTMLTextAreaElement;
        if (input) {
          input.focus();
        }
      }, 200);
    },

    // Chamado quando uma conversa é renomeada (match por conversationId)
    handleConversationRenamed: (conversationId: number, newTitle: string) => {
      set((state) => {
        const updatedTabs: ChatTab[] = state.tabs.map(tab => {
          if (tab.conversationId === conversationId) {
            return {
              ...tab,
              title: newTitle,
              updatedAt: Date.now(),
            };
          }
          return tab;
        });
        return { tabs: updatedTabs };
      });
    },

    // Chamado quando o título de uma aba é atualizado (match por backendId — mais confiável)
    handleTabTitleUpdated: (backendTabId: number, newTitle: string) => {
      set((state) => {
        const updatedTabs: ChatTab[] = state.tabs.map(tab => {
          if (tab.backendId === backendTabId) {
            return {
              ...tab,
              title: newTitle,
              updatedAt: Date.now(),
            };
          }
          return tab;
        });
        return { tabs: updatedTabs };
      });

      // Anuncia com assertive e delay para não ser sobrescrito por eventos de streaming
      setTimeout(() => {
        announce(`Conversa renomeada para ${newTitle}`, 'assertive');
      }, 150);
    },

    // Chamado quando o banco de dados é resetado
    handleDatabaseReset: () => {
      // Limpa todos os listeners ativos
      activeListeners.forEach((cleanup) => cleanup());
      activeListeners.clear();
      
      // Reseta o estado e reinicializa
      set({
        tabs: [],
        activeTabId: null,
        isLoading: false,
        streamingMessageId: null,
        isInitialized: false,
        expandedThreads: new Set<string>(),
        expandedReasonings: new Set<string>(),
        streamingReasoning: null,
        isThinking: false,
        activeToolCalls: [],
        hadToolCalls: false,
        completedSegments: [],
      });
      
      // Reinicializa tabs do backend
      get().initializeTabs();
      
      // Anuncia para acessibilidade
      announce('Banco de dados resetado. Conversas reinicializadas.');
    },

    // ÚNICO ponto de remoção de tabs do state (exceto fallback local em deleteTab).
    // Disparado pelo evento tab_closed do backend — seja via deleteTab, tool close_tab, ou qualquer outro caminho.
    handleTabClosed: (backendTabId: number) => {
      const state = get();
      const tabToClose = state.tabs.find(t => t.backendId === backendTabId);
      
      if (!tabToClose) return;
      
      // Última aba: apenas limpa o conteúdo sem removê-la
      if (state.tabs.length <= 1) {
        set((s) => ({
          tabs: s.tabs.map(tab => 
            tab.backendId === backendTabId
              ? {
                  ...tab,
                  conversationId: undefined,
                  title: 'Nova Conversa',
                  threadedMessages: [] as MessageNode[],
                  updatedAt: Date.now(),
                }
              : tab
          ),
        }));
        return;
      }
      
      removeTabFromState(tabToClose.id, get, set, activeListeners);
      
      // Safety net: consolida tabs vazias se houver mais de uma
      setTimeout(() => get().consolidateEmptyTabs(), 100);
      
      announce('Aba fechada');
    },

    // Safety net: se houver mais de uma tab vazia, fecha as extras via backend.
    // Com a guarda em createTab, isso raramente deveria disparar.
    // Cada CloseTab gera um evento tab_closed → handleTabClosed remove do state.
    // Para evitar cascata (handleTabClosed → consolidateEmptyTabs → handleTabClosed → ...),
    // usa um flag de reentrância.
    consolidateEmptyTabs: () => {
      if (consolidatingEmptyTabs) return;

      const state = get();
      const emptyTabs = state.tabs.filter(tab => !tab.conversationId && tab.threadedMessages.length === 0);
      if (emptyTabs.length <= 1) return;

      consolidatingEmptyTabs = true;

      const sortedEmpty = [...emptyTabs].sort((a, b) => b.updatedAt - a.updatedAt);
      const tabsToRemove = sortedEmpty.slice(1);

      // Tabs com backendId: delega ao backend (handleTabClosed faz o state update)
      // Tabs sem backendId: remove direto do state
      for (const tab of tabsToRemove) {
        if (tab.backendId) {
          CloseTab(tab.backendId).catch(err => {
            console.error('[Chat] consolidateEmptyTabs: error closing tab:', err);
          });
        } else {
          removeTabFromState(tab.id, get, set, activeListeners);
        }
      }

      // Libera o flag depois que os eventos tiverem tempo de ser processados
      setTimeout(() => { consolidatingEmptyTabs = false; }, 500);
    },

    // Recarrega mensagens da aba ativa a partir do banco de dados.
    // Usado quando mensagens chegam de canais externos (Signal, Telegram).
    reloadActiveTabMessages: async () => {
      const { activeTabId, tabs } = get();
      const tab = tabs.find((t) => t.id === activeTabId);
      if (!tab?.conversationId) return;

      try {
        const backendNodes = await GetMessages(tab.conversationId, null);
        const messageNodes: MessageNode[] = backendNodes.map((node, index) => {
          (node as any).originalIndex = index;
          return node;
        });

        set((state) => ({
          tabs: state.tabs.map((t) =>
            t.id === activeTabId
              ? { ...t, threadedMessages: messageNodes, updatedAt: Date.now() }
              : t
          ),
        }));
      } catch (err) {
        console.error('[Chat] Erro ao recarregar mensagens (external):', err);
      }
    },

    // Recarrega mensagens de uma conversa específica, independente de qual aba está ativa.
    // Encontra a aba que contém essa conversa e atualiza suas mensagens.
    // Se a conversa não está em nenhuma aba, não faz nada (as mensagens estão salvas no banco).
    reloadConversationMessages: async (conversationId: number) => {
      const { tabs } = get();
      const tab = tabs.find((t) => t.conversationId === conversationId);
      if (!tab) {
        return;
      }

      try {
        const backendNodes = await GetMessages(conversationId, null);
        const messageNodes: MessageNode[] = backendNodes.map((node, index) => {
          (node as any).originalIndex = index;
          return node;
        });

        set((state) => ({
          tabs: state.tabs.map((t) =>
            t.id === tab.id
              ? { ...t, threadedMessages: messageNodes, updatedAt: Date.now() }
              : t
          ),
        }));
      } catch (err) {
        console.error('[Chat] Erro ao recarregar conversa', conversationId, ':', err);
      }
    },

    // Atribui um canal externo a uma conversa (bridge bidirecional).
    assignChannelToTab: async (tabId, channel, contactId) => {
      const tab = get().tabs.find((t) => t.id === tabId);
      if (!tab?.conversationId) {
        console.warn('[Chat] assignChannelToTab: aba sem conversationId', tabId);
        return;
      }
      await AssignConversationToChannel(tab.conversationId, channel, contactId);
      set((state) => ({
        tabs: state.tabs.map((t) =>
          t.id === tabId ? { ...t, channel, contactId } : t
        ),
      }));
    },

    // Remove a vinculação de canal de uma conversa.
    unassignChannelFromTab: async (tabId) => {
      const tab = get().tabs.find((t) => t.id === tabId);
      if (!tab?.conversationId) return;
      await UnassignConversationFromChannel(tab.conversationId);
      set((state) => ({
        tabs: state.tabs.map((t) =>
          t.id === tabId ? { ...t, channel: undefined, contactId: undefined } : t
        ),
      }));
    },

    // Trata mensagem recebida de canal externo (Signal, Telegram) com streaming completo.
    // Replica o fluxo de sendMessage: placeholder de usuário + assistente streaming + listeners.
    // Cada contato externo tem sua conversa e aba dedicada (criada pelo backend).
    handleExternalIncoming: (data) => {
      const { addMessage, tabs } = get();
      const { channel, from, text, conversationId, tabId, tabTitle } = data;

      // 1. Encontra a aba dedicada desta conversa no store.
      //    O backend já garante que a aba e a conversa existem.
      let targetTabId: string | null = null;

      // Primeiro tenta por conversationId (mais confiável)
      if (conversationId > 0) {
        const existingTab = tabs.find((t) => t.conversationId === conversationId);
        if (existingTab) {
          targetTabId = existingTab.id;
        }
      }

      // Se a aba foi criada pelo backend mas o frontend ainda não conhece, adiciona
      if (!targetTabId && tabId && tabId > 0) {
        const frontendTabId = tabId.toString();
        // Verifica se já existe pelo backendId
        const existingByBackendId = tabs.find((t) => t.backendId === tabId);
        if (existingByBackendId) {
          targetTabId = existingByBackendId.id;
        } else {
          // Cria a aba localmente no store (sincronizado com o backend)
          const newTab: ChatTab = {
            id: frontendTabId,
            backendId: tabId,
            title: tabTitle || `[${channel}] ${from}`,
            threadedMessages: [],
            conversationId: conversationId || undefined,
            createdAt: Date.now(),
            updatedAt: Date.now(),
            channel: channel || undefined,
            contactId: data.fromId || undefined,
          };
          set((state) => ({
            tabs: [...state.tabs, newTab],
          }));
          targetTabId = frontendTabId;
        }
      }

      if (!targetTabId) {
        return;
      }

      // 2. Adiciona a mensagem do usuário (origin badge via source)
      //    Para áudio, text pode ser vazio — será atualizado por chat:messages_ready com a transcrição.
      const userMessageId = addMessage(targetTabId, {
        role: 'user',
        content: text || 'Transcrevendo áudio...',
        source: channel,
      });

      // 3. Adiciona placeholder de streaming para a resposta do assistente
      const assistantMessageId = addMessage(targetTabId, {
        role: 'assistant',
        content: '',
        isStreaming: true,
      });

      set({ isLoading: true, streamingMessageId: assistantMessageId });

      // 4. Registra listeners de streaming (mesmo padrão do sendMessage)
      let cleanupExecuted = false;
      let streamingAnnounced = false;

      const cleanup = () => {
        if (cleanupExecuted) return;
        cleanupExecuted = true;

        activeListenerCount--;
        unsubStream();
        unsubThinking();
        unsubToolStart();
        unsubToolEnd();
        unsubSegmentDone();
        unsubDone();
        unsubReady();
        activeListeners.delete(targetTabId!);
        set({ isLoading: false, streamingMessageId: null, streamingReasoning: null, isThinking: false, activeToolCalls: [], completedSegments: [] });
      };

      // Limpa listeners antigos desta tab se existirem
      const existingCleanup = activeListeners.get(targetTabId);
      if (existingCleanup) {
        existingCleanup();
      }

      activeListenerCount++;

      // chat:messages_ready — atualiza ID, conteúdo e conversationId do user message
      const unsubReady = EventsOn('chat:messages_ready', (event: any) => {
        if (!activeListeners.has(targetTabId!)) return;
        if (event.userMessageId) {
          const backendUserId = event.userMessageId.toString();
          
          // Atualiza ID e conteúdo da mensagem do usuário (ID local → ID do banco, + transcrição de áudio)
          set((state) => {
            const tab = state.tabs.find(t => t.id === targetTabId);
            if (!tab) return state;

            const updateNodes = (nodes: MessageNode[]): MessageNode[] => {
              return nodes.map(node => {
                if (node.message.id === userMessageId) {
                  const updatedMessage = new main.EnrichedMessage({
                    ...node.message,
                    id: backendUserId,
                    content: event.userContent || node.message.content,
                  });
                  return new main.MessageNode({
                    ...node,
                    message: updatedMessage,
                  });
                }
                return node;
              });
            };

            return {
              tabs: state.tabs.map((t) =>
                t.id === targetTabId
                  ? {
                      ...t,
                      threadedMessages: updateNodes(t.threadedMessages),
                      conversationId: event.conversationId || t.conversationId,
                      updatedAt: Date.now(),
                    }
                  : t
              ),
            };
          });

          // Anuncia a mensagem transcrita para o leitor de telas
          if (event.userContent) {
            const cleanContent = stripMarkdown(event.userContent);
            announce(`${from} via ${channel}: ${cleanContent}`);
          }
        }
      });

      // chat:stream — atualiza conteúdo da resposta em tempo real
      const unsubStream = EventsOn('chat:stream', (event: any) => {
        if (!activeListeners.has(targetTabId!)) return;

        if (event.content) {
          if (!streamingAnnounced && !event.done && !event.error) {
            streamingAnnounced = true;
            announce('Assistente está respondendo', 'polite');
          }

          if (!event.done && !event.error) {
            debouncedUpdateMessage(targetTabId!, assistantMessageId, event.content, get().updateMessage);
          } else {
            flushPendingUpdate(targetTabId!, assistantMessageId, get().updateMessage);
            get().updateMessage(targetTabId!, assistantMessageId, event.content);
          }
        }

        if (event.error) {
          flushPendingUpdate(targetTabId!, assistantMessageId, get().updateMessage);
          get().updateMessage(targetTabId!, assistantMessageId, `Erro: ${event.error}`);
          cleanup();
        }

        if (event.done) {
          const currentState = get();
          const currentTab = currentState.tabs.find(t => t.id === targetTabId);
          const flatMessages = flattenThreadedMessages(currentTab?.threadedMessages);
          const finalMessage = flatMessages.find(m => m.id === assistantMessageId);

          set((state) => ({
            tabs: state.tabs.map((tab) =>
              tab.id === targetTabId
                ? {
                    ...tab,
                    threadedMessages: tab.threadedMessages.map((node) => {
                      const markDone = (n: MessageNode): MessageNode => {
                        if (n.message.id === assistantMessageId) n.message.isStreaming = false;
                        if (n.children?.length) n.children = n.children.map(markDone);
                        return n;
                      };
                      return markDone(node);
                    }),
                  }
                : tab
            ),
          }));

          if (finalMessage?.content) {
            const isActiveTab = currentState.activeTabId === targetTabId;
            if (isActiveTab) playReceiveSound();

            if (ttsService.isAutoReadEnabled() && isActiveTab && !cleanupExecuted) {
              messageAudioService.stopAll();
              ttsService.stop();
              ttsService.speak(finalMessage.content).catch((err: any) => {
                console.error('[Chat] TTS error (external):', err);
              });
            }

            if (ttsService.shouldUseAriaLiveForAgent() && isActiveTab) {
              announce(`Assistente: ${stripMarkdown(finalMessage.content)}`);
            }
          }
        }
      });

      // chat:thinking
      const unsubThinking = EventsOn('chat:thinking', (event: any) => {
        if (!activeListeners.has(targetTabId!)) return;
        if (event.started) {
          set({ isThinking: true, streamingReasoning: event.content || '' });
          announce('O modelo está pensando...', 'polite');
        } else if (event.done) {
          set({ isThinking: false });
          if (event.content) get().updateMessageReasoning(targetTabId!, assistantMessageId, event.content);
        } else {
          set({ streamingReasoning: event.content || '' });
        }
      });

      // chat:tool_start
      const unsubToolStart = EventsOn('chat:tool_start', (event: any) => {
        if (!activeListeners.has(targetTabId!)) return;
        set((state) => ({
          hadToolCalls: true,
          activeToolCalls: [...state.activeToolCalls, {
            name: event.name, callId: event.callId, args: event.args, status: 'running' as const,
          }],
        }));
        announce(`Executando ferramenta: ${event.name}`, 'polite');
      });

      // chat:tool_end
      const unsubToolEnd = EventsOn('chat:tool_end', (event: any) => {
        if (!activeListeners.has(targetTabId!)) return;
        set((state) => ({
          activeToolCalls: state.activeToolCalls.map((tc) =>
            tc.callId === event.callId
              ? { ...tc, status: (event.status === 'error' ? 'error' : 'done') as 'done' | 'error', summary: event.summary }
              : tc
          ),
        }));
        const statusLabel = event.status === 'error' ? 'falhou' : 'concluída';
        announce(`Ferramenta ${event.name} ${statusLabel}`, 'polite');
      });

      // chat:segment_done
      const unsubSegmentDone = EventsOn('chat:segment_done', (event: any) => {
        if (!activeListeners.has(targetTabId!)) return;
        if (event.hasMore && event.content && ttsService.isAutoReadEnabled()) {
          ttsService.speak(event.content).catch((err: any) => {
            console.error('[Chat] TTS segment error (external):', err);
          });
        }
        if (event.hasMore) {
          const state = get();
          const newSegments: TurnSegment[] = [...state.completedSegments];
          if (state.activeToolCalls.length > 0) {
            newSegments.push({
              type: 'tool_calls',
              toolCalls: state.activeToolCalls.map(tc => ({
                id: tc.callId,
                type: 'function',
                function: { name: tc.name, arguments: tc.args || '' },
                result: tc.summary,
              })),
            });
          }
          if (event.content) {
            newSegments.push({ type: 'text', content: event.content });
          }
          set({ completedSegments: newSegments, activeToolCalls: [] });
          flushPendingUpdate(targetTabId!, assistantMessageId, get().updateMessage);
          get().updateMessage(targetTabId!, assistantMessageId, '');
        }
      });

      // chat:done — finaliza e faz reload se houve tool calls
      const unsubDone = EventsOn('chat:done', () => {
        if (!activeListeners.has(targetTabId!)) return;

        const didUseTools = get().hadToolCalls;

        set((state) => {
          const tabIndex = state.tabs.findIndex((t) => t.id === targetTabId);
          if (tabIndex >= 0) {
            state.tabs[tabIndex].threadedMessages = state.tabs[tabIndex].threadedMessages.map((node) => {
              const markDone = (n: MessageNode): MessageNode => {
                if (n.message.id === assistantMessageId) n.message.isStreaming = false;
                if (n.children?.length) n.children = n.children.map(markDone);
                return n;
              };
              return markDone(node);
            });
          }
          return state;
        });

        if (didUseTools) {
          const tab = get().tabs.find(t => t.id === targetTabId);
          if (tab?.conversationId) {
            GetMessages(tab.conversationId, null).then((backendNodes) => {
              const messageNodes: MessageNode[] = backendNodes.map((node, index) => {
                (node as any).originalIndex = index;
                return node;
              });
              set((state) => ({
                tabs: state.tabs.map((t) =>
                  t.id === targetTabId
                    ? { ...t, threadedMessages: messageNodes, updatedAt: Date.now() }
                    : t
                ),
                hadToolCalls: false,
                completedSegments: [],
              }));
            });
          }
        }

        cleanup();
      });

      // Armazena cleanup no Map para evitar duplicação
      activeListeners.set(targetTabId, cleanup);
    },
  };
});

// NOTA: Listeners globais de backend foram REMOVIDOS pois causavam loop infinito
// O backend já sincroniza as tabs via GetAllTabs() e não precisamos de listeners reativos
// Se precisar re-habilitar, mover para um hook React com cleanup apropriado

