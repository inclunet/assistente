import { create } from 'zustand';
import { 
  SendMessage, 
  AddMessageWithMedia,
  GetAllTabs,
  CreateTab,
  CloseTab,
  SetActiveTab,
  UpdateTabTitle as BackendUpdateTabTitle,
  GetMessages,
  LoadConversationInTab,
} from '../../wailsjs/go/main/App';
import { EventsOn } from '../../wailsjs/runtime/runtime';
import { MediaFile } from '../services/mediaService';
import { database, main } from '../../wailsjs/go/models';
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
  // Nenhuma conversão necessária - backend manda tudo pronto
};

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
}

export interface ChatTab {
  id: string;
  title: string;
  threadedMessages: MessageNode[];  // Fonte única de verdade - estrutura hierárquica
  conversationId?: number;
  createdAt: number;
  updatedAt: number;
  backendId?: number; // ID do backend (database.ChatTab)
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
  handleDatabaseReset: () => void;
  handleTabClosed: (tabId: number) => void;
  consolidateEmptyTabs: () => void;
}

const generateId = () => `${Date.now()}-${Math.random().toString(36).substr(2, 9)}`;

// Mapa para rastrear cleanup functions de cada tab (previne acúmulo de listeners)
const activeListeners = new Map<string, () => void>();

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
  console.log('[backendTabToFrontend] Backend tab recebida:', {
    id: backendTab.id,
    conversation_id: backendTab.conversation_id,
    title: backendTab.title,
    is_active: backendTab.is_active
  });

  return {
    id: backendTab.id?.toString() || generateId(),
    backendId: backendTab.id,
    title: backendTab.title || 'Nova Conversa',
    threadedMessages: [], // Mensagens serão carregadas depois se necessário
    conversationId: backendTab.conversation_id || undefined,
    createdAt: backendTab.created_at ? Date.parse(backendTab.created_at as any) : Date.now(),
    updatedAt: backendTab.updated_at ? Date.parse(backendTab.updated_at as any) : Date.now(),
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
      console.log('[Chat] Initializing tabs from backend...');
      const backendTabs = await GetAllTabs();
      console.log('[Chat] Loaded tabs from backend:', backendTabs);

      let tabs = backendTabs.map(backendTabToFrontend);
      
      // Se não houver tabs, cria uma aba em branco (ativa)
      if (tabs.length === 0) {
        console.log('[Chat] No tabs found, creating default tab...');
        const newBackendTab = await CreateTab('Nova Conversa', '💬', true);
        tabs = [backendTabToFrontend(newBackendTab)];
      }
      
      const activeTab = tabs.find((t: ChatTab) => t.backendId && backendTabs.find((bt: database.ChatTab) => bt.id === t.backendId)?.is_active);

      set({
        tabs,
        activeTabId: activeTab?.id || tabs[0]?.id || null,
        isInitialized: true,
      });

      console.log('[Chat] Tabs initialized:', { 
        count: tabs.length, 
        active: activeTab?.id,
        titles: tabs.map((t: ChatTab) => ({ id: t.id, backendId: t.backendId, title: t.title }))
      });
      
      // Carrega mensagens da tab ativa se houver conversationId
      const initialActiveTab = activeTab || tabs[0];
      console.log('[Chat] ====================================');
      console.log('[Chat] initialActiveTab:', initialActiveTab);
      console.log('[Chat] conversationId:', initialActiveTab?.conversationId);
      console.log('[Chat] ====================================');
      
      if (initialActiveTab?.conversationId) {
        console.log('[Chat] Carregando mensagens da tab ativa:', initialActiveTab.conversationId);
        try {
          console.log('[Chat] ANTES de chamar GetMessages');
          const messageNodes = await GetMessages(initialActiveTab.conversationId, null);
          console.log('[Chat] DEPOIS de chamar GetMessages');
          console.log('[Chat] MessageNodes recebidos:', messageNodes);
          console.log('[Chat] MessageNodes length:', messageNodes?.length);
          
          // Backend já manda MessageNode[] pronto - adiciona apenas originalIndex
          const nodes: MessageNode[] = (messageNodes || []).map((node, index) => {
            (node as any).originalIndex = index;
            return node;
          });
          
          console.log('[Chat] ✅ Nodes recebidos:', nodes.length);
          if (nodes.length > 0) {
            console.log('[Chat] 📊 Primeiro node:', {
              id: nodes[0].message.id,
              childCount: nodes[0].childCount,
              children: nodes[0].children?.length || 0
            });
          }
          
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
          
          console.log('[Chat] ✅ Mensagens carregadas na tab ativa');
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
    try {
      console.log('[Chat] Creating new tab in backend...', { activate });
      const backendTab = await CreateTab('Nova Conversa', '💬', activate);
      const newTab = backendTabToFrontend(backendTab);
      
      set((state) => ({
        tabs: [...state.tabs, newTab],
        activeTabId: activate ? newTab.id : state.activeTabId,
      }));

      console.log('[Chat] Tab created:', { id: newTab.id, backendId: newTab.backendId, title: newTab.title, activate });
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
    if (!tab) return;

    // Não permite fechar se for a única aba
    if (state.tabs.length <= 1) {
      console.log('[Chat] Cannot close last tab');
      return;
    }

    // Limpa listeners desta tab se existirem
    const existingCleanup = activeListeners.get(tabId);
    if (existingCleanup) {
      console.log('[Chat] Limpando listeners ao deletar tab:', tabId);
      existingCleanup();
      activeListeners.delete(tabId);
    }

    try {
      // Note: OnTabClosed was removed from backend - embedding generation happens automatically
      
      if (tab.backendId) {
        console.log('[Chat] Closing tab in backend:', tab.backendId);
        await CloseTab(tab.backendId);
      }
    } catch (error) {
      console.error('[Chat] Error closing tab in backend:', error);
    }

    // Remove localmente (mesmo que backend falhe)
    set((state) => {
      const tabIndex = state.tabs.findIndex((t) => t.id === tabId);
      const newTabs = state.tabs.filter((t) => t.id !== tabId);
      
      // Se a guia deletada era a ativa, escolhe a próxima guia baseado na posição
      let newActiveTabId = state.activeTabId;
      if (state.activeTabId === tabId) {
        // Tenta pegar a guia seguinte, se não houver pega a anterior
        const nextIndex = Math.min(tabIndex, newTabs.length - 1);
        newActiveTabId = newTabs[nextIndex]?.id || null;
      }

      return {
        tabs: newTabs,
        activeTabId: newActiveTabId,
      };
    });

    // Foca na nova guia ativa após a remoção
    setTimeout(() => {
      const newActiveTabId = get().activeTabId;
      if (newActiveTabId) {
        const tabButton = document.querySelector(
          `[data-tab-id="${newActiveTabId}"]`
        ) as HTMLButtonElement;
        tabButton?.focus();
      }
    }, 50);
  },

  setActiveTab: async (tabId) => {
    console.log('[Chat] 🔵 setActiveTab CHAMADO com tabId:', tabId);
    const state = get();
    const tab = state.tabs.find(t => t.id === tabId);
    // previousTab removed - no longer needed after tool-calling removal
    console.log('[Chat] 🔵 Tab encontrada:', tab ? `id=${tab.id}, backendId=${tab.backendId}` : 'NÃO ENCONTRADA');
    
    // Note: OnTabInactive was removed from backend - embedding generation happens automatically
    
    try {
      if (tab?.backendId) {
        console.log('[Chat] Setting active tab in backend:', tab.backendId);
        await SetActiveTab(tab.backendId);
        
        // Carrega mensagens da conversa se houver conversationId e não houver mensagens carregadas
        if (tab.conversationId && tab.threadedMessages.length === 0) {
          console.log('[Chat]  Carregando mensagens da conversa:', tab.conversationId);
          try {
            const backendNodes = await GetMessages(tab.conversationId, null);
            console.log('[Chat]  Mensagens carregadas:', backendNodes.length);
            
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
            
            console.log('[Chat] ✅ Mensagens carregadas na tab:', messageNodes.length);
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
    
    console.log('[Chat] Updating tab title:', { tabId, backendId: tab?.backendId, title });
    
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

    console.log('[Chat] Tab titles after update:', get().tabs.map(t => ({ id: t.id, title: t.title })));
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

    console.log('[Chat] Message added:', { tabId, messageId, role: message.role, contentLength: message.content.length });
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

    // Log apenas a cada 100 caracteres para não poluir o console
    if (content.length % 100 === 0 || content.length < 10) {
      console.log('[Chat] Message updated:', { tabId, messageId, contentLength: content.length });
    }
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
    console.log('[Chat] 📨 Adding internal message:', { 
      tabId, 
      messageId: message.id,
      parentId,
      role: message.role,
    });
    
    set((state) => {
      const tab = state.tabs.find(t => t.id === tabId);
      if (!tab) return state;
      
      // Se não tem parentId, é uma mensagem raiz - adiciona normalmente
      if (!parentId) {
        console.log('[Chat] No parentId, adding as root message');
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
              console.log('[Chat] Message already exists in children, skipping:', message.id);
              return node;
            }
            
            // SEMPRE adiciona como filho E incrementa contador
            // A renderização (MessageNode) é que decide se mostra ou não baseado no isExpanded
            console.log('[Chat] ✅ Adding message as child of:', targetParentId);
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
        console.log('[Chat] ✅ Message added to tree under parent:', parentId);
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
      console.log('[Chat] ⚠️ Parent not found by ID:', parentId);
      
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
        console.log('[Chat] 🔄 Fallback: adding under last user message:', lastUserMessage.message.id);
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
      console.log('[Chat] ⚠️ All fallbacks failed, adding as root');
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
        console.log('[Chat] Criando nova conversa na aba ativa');
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
      console.log('[Chat] Carregando conversa', conversationId, 'na aba', activeTab.backendId);
      await LoadConversationInTab(activeTab.backendId, conversationId);

      // Carrega mensagens da conversa
      const backendNodes = await GetMessages(conversationId, null);
      console.log('[Chat] Mensagens carregadas:', backendNodes.length);

      // Adiciona originalIndex aos nodes do backend
      const messageNodes: MessageNode[] = backendNodes.map((node, index) => {
        (node as any).originalIndex = index;
        return node;
      });

      // Atualiza a aba com a nova conversa
      set((state) => ({
        tabs: state.tabs.map((t) =>
          t.id === activeTabId
            ? {
                ...t,
                conversationId,
                threadedMessages: messageNodes,
                title: conversationTitle || 'Conversa carregada',
                updatedAt: Date.now(),
              }
            : t
        ),
      }));

      // Se era uma aba em branco, cria uma nova aba em branco para futuras conversas
      if (isBlankTab) {
        console.log('[Chat] Creating new blank tab after loading conversation...');
        setTimeout(() => {
          createTab(false).then(newTabId => {
            console.log('[Chat] New blank tab created (not activated):', newTabId);
          }).catch((err: any) => {
            console.error('[Chat] Error creating new blank tab:', err);
          });
        }, 100);
      }

      console.log('[Chat] ✅ Conversa carregada com sucesso na aba ativa');
      
      // Anuncia para acessibilidade
      announce(`Conversa aberta: ${conversationTitle || 'Conversa carregada'}`);
    } catch (error) {
      console.error('[Chat] ❌ Erro ao carregar conversa na aba ativa:', error);
      throw error;
    }
  },

  sendMessage: async (content, mediaFiles) => {
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
          console.log('[Chat] Creating new blank tab for future conversations...');
          setTimeout(() => {
            createTab(false).then(newTabId => {
              console.log('[Chat] New blank tab created (not activated):', newTabId);
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
          console.log('[Chat] Conversation created:', data);
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
          console.log('[Chat] Conversation loaded in tab:', data);
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
          console.log('[Chat] 🔄 Messages ready event received:', data);
          
          if (data.userMessageId) {
            const backendUserId = data.userMessageId.toString();
            console.log('[Chat] 🔄 Updating user message ID from', userMessageId, 'to', backendUserId);
            
            // Atualiza o ID da mensagem do usuário para o ID real do banco
            set((state) => {
              const tab = state.tabs.find(t => t.id === currentTabId);
              if (!tab) return state;
              
              // Encontra e atualiza a mensagem do usuário
              const updateMessageId = (nodes: MessageNode[]): MessageNode[] => {
                return nodes.map(node => {
                  if (node.message.id === userMessageId) {
                    console.log('[Chat] ✅ Found user message, updating ID to:', backendUserId);
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

          console.log('[Chat] Sending message to backend...', {
            conversationId,
            contentLength: content.length,
            hasMedia: !!mediaFiles && mediaFiles.length > 0,
          });

          // Setup completion handler that will clean up everything
          // CRÍTICO: Registrar listeners ANTES de chamar SendMessage/AddMessageWithMedia
          // pois o backend inicia streaming em goroutine e pode emitir eventos antes do await retornar
          let unsubscribeStream: (() => void) | null = null;
          let unsubscribeComplete: (() => void) | null = null;
          let cleanupExecuted = false; // Flag para evitar cleanup duplicado
          let streamingAnnounced = false; // Flag para anunciar início do streaming apenas uma vez

          const cleanup = () => {
            // Previne execução múltipla do cleanup (backend pode emitir chat:done várias vezes)
            if (cleanupExecuted) {
              console.log('[Chat] ⚠️  Cleanup já foi executado para tab:', currentTabId, '- ignorando chamada duplicada');
              return;
            }
            cleanupExecuted = true;

            activeListenerCount--;
            console.log('[Chat] Cleaning up listeners for tab:', currentTabId, '| Active listeners:', activeListenerCount);
            if (unsubscribeStream) {
              unsubscribeStream();
              unsubscribeStream = null;
            }
            if (unsubscribeComplete) {
              unsubscribeComplete();
              unsubscribeComplete = null;
            }
            activeListeners.delete(currentTabId!);
            set({ isLoading: false, streamingMessageId: null, streamingReasoning: null, isThinking: false, activeToolCalls: [] });
          };

          // Limpa listeners antigos desta tab se existirem
          const existingCleanup = activeListeners.get(currentTabId!);
          if (existingCleanup) {
            console.log('[Chat] ⚠️  LIMPANDO listeners antigos da tab:', currentTabId, '(isso previne duplicação)');
            existingCleanup();
            // IMPORTANTE: Aguarda um tick para garantir que o cleanup foi processado
            await new Promise(resolve => setTimeout(resolve, 0));
          }

          // Listen for streaming chunks from backend
          activeListenerCount++;
          console.log('[Chat] Registrando NOVOS listeners para tab:', currentTabId, '| Total de listeners ativos:', activeListenerCount);
          
          unsubscribeStream = EventsOn('chat:stream', (event: any) => {
            // IMPORTANTE: Verifica se este listener ainda é válido (não foi limpo)
            if (!activeListeners.has(currentTabId!)) {
              console.log('[Chat] ⚠️  Ignorando evento de listener órfão para tab:', currentTabId);
              return;
            }

            // Log apenas eventos importantes para não poluir console
            if (event.done || event.error) {
              console.log('[Chat] Stream event:', event);
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
              console.log('[Chat] Stream completed');
              
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
              console.log('[Chat] Ignorando chat:thinking de listener órfão para tab:', currentTabId);
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
              console.log('[Chat] Thinking completed:', event.content?.length, 'chars');
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
            console.log('[Chat] 🔧 Tool start:', data.name, data.callId);
            
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
            announce(`Executando ferramenta: ${data.name}`, 'polite');
          });

          // Listen for tool execution end
          let unsubscribeToolEnd: (() => void) | null = null;
          unsubscribeToolEnd = EventsOn('chat:tool_end', (data: any) => {
            if (!activeListeners.has(currentTabId!)) return;
            console.log('[Chat] ✅ Tool end:', data.name, data.status);
            
            set((state) => ({
              activeToolCalls: state.activeToolCalls.map((tc) =>
                tc.callId === data.callId
                  ? { ...tc, status: (data.status === 'error' ? 'error' : 'done') as 'done' | 'error', summary: data.summary }
                  : tc
              ),
            }));
          });

          // Listen for segment done (assistant text before tool calls — for verbalization)
          let unsubscribeSegmentDone: (() => void) | null = null;
          unsubscribeSegmentDone = EventsOn('chat:segment_done', (data: any) => {
            if (!activeListeners.has(currentTabId!)) return;
            console.log('[Chat] 📝 Segment done:', data.iteration, 'hasMore:', data.hasMore);
            
            // Verbaliza o segmento intermediário se TTS estiver ativo
            if (data.content && ttsService.isAutoReadEnabled()) {
              ttsService.speak(data.content).catch((err: any) => {
                console.error('[Chat] TTS segment error:', err);
              });
            }
          });

          // Listen for completion event - this signals end of entire chat process
          unsubscribeComplete = EventsOn('chat:done', (data: any) => {
            // IMPORTANTE: Verifica se este listener ainda é válido
            if (!activeListeners.has(currentTabId!)) {
              console.log('[Chat] ⚠️  Ignorando chat:done de listener órfão para tab:', currentTabId);
              return;
            }
            
            console.log('[Chat] Chat complete:', data);
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
                console.log('[Chat] 🔄 Recarregando mensagens após tool calling...');
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
                  }));
                  console.log('[Chat] ✅ Mensagens recarregadas:', backendNodes.length);
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
          console.log('[Chat] Listeners registrados para tab:', currentTabId);

          // Store cleanup function for use in error handler
          unsubscribe = () => {
            cleanup();
            if (unsubscribeConvCreated) unsubscribeConvCreated();
            if (unsubscribeConvLoaded) unsubscribeConvLoaded();
            if (unsubscribeMessagesReady) unsubscribeMessagesReady();
          };

          // AGORA envia a mensagem — listeners já estão ativos para capturar streaming
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

            const mediaJson = JSON.stringify(mediaDataArray);
            console.log('[Chat] Sending message with media:', mediaDataArray.length, 'files');

            const newMessage = await AddMessageWithMedia(
              conversationId,
              'user',
              content,
              mediaJson
            );

            // Update tab with conversation ID if we didn't have one
            if (!tab?.conversationId && newMessage.conversationId) {
              set((state) => ({
                tabs: state.tabs.map((t) =>
                  t.id === currentTabId
                    ? { ...t, conversationId: newMessage.conversationId }
                    : t
                ),
              }));
            }

            const activeConvId = newMessage.conversationId || conversationId;
            if (!tab?.conversationId && activeConvId) {
              set((state) => ({
                tabs: state.tabs.map((t) =>
                  t.id === currentTabId
                    ? { ...t, conversationId: activeConvId }
                    : t
                ),
              }));
            }
          } else {
            // Normal message without media
            console.log('[Chat] Calling SendMessage...', { conversationId, contentLength: content.length });
            await SendMessage(conversationId, content, '', {
              model: '',
              temperature: 0,
              maxTokens: 0,
            });
            console.log('[Chat] SendMessage returned successfully');
          }
          
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
        } finally {
          set({ isLoading: false, streamingMessageId: null });
        }
      },

      stopStreaming: () => {
        set({ isLoading: false, streamingMessageId: null });
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
        console.log('[Chat]  Loading children for message:', messageId);
        
        try {
          const messageIdNum = parseInt(messageId, 10);
          if (isNaN(messageIdNum)) {
            console.error('[Chat] ❌ Invalid message ID:', messageId);
            return [];
          }
          
          // Chama backend para carregar filhos
          const { GetMessageChildren } = await import('../../wailsjs/go/main/App');
          const backendNodes = await GetMessageChildren(messageIdNum);
          
          console.log('[Chat] ✅ Loaded children from backend:', backendNodes.length);
          
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
      console.log('[Chat] 🗑️ handleConversationDeleted CHAMADO para conversationId:', conversationId);
      
      // Anuncia para acessibilidade
      announce('Conversa apagada permanentemente');
      
      const state = get();
      console.log('[Chat] 🗑️ Tabs atuais:', state.tabs.map(t => ({ id: t.id, conversationId: t.conversationId })));
      
      const tabToClose = state.tabs.find(tab => tab.conversationId === conversationId);
      
      if (!tabToClose) {
        console.log('[Chat] 🗑️ Tab com conversa deletada NÃO ENCONTRADA');
        return;
      }
      
      console.log('[Chat] 🗑️ Tab encontrada para fechar:', { id: tabToClose.id, backendId: tabToClose.backendId });

      // Se houver mais de uma aba, fecha a aba da conversa deletada
      if (state.tabs.length > 1) {
        console.log('[Chat] 🗑️ Fechando tab (mais de uma aba existe):', tabToClose.id);
        // Usa deleteTab para fechar corretamente
        get().deleteTab(tabToClose.id);
      } else {
        // Se for a única aba, apenas limpa
        console.log('[Chat] 🗑️ Única aba - limpando em vez de fechar');
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

      // Garante que só há uma aba vazia
      setTimeout(() => {
        get().consolidateEmptyTabs();
      }, 100);
      
      // Foca no input de mensagem após um pequeno delay
      setTimeout(() => {
        const input = document.querySelector('textarea[placeholder*="mensagem"], textarea[aria-label*="mensagem"]') as HTMLTextAreaElement;
        if (input) {
          input.focus();
          console.log('[Chat] 🗑️ Foco movido para input');
        }
      }, 200);
    },

    // Chamado quando uma conversa é limpa (mensagens removidas)
    handleConversationCleared: (conversationId: number) => {
      console.log('[Chat] 🧹 handleConversationCleared CHAMADO para conversationId:', conversationId);
      
      const state = get();
      const tabToUpdate = state.tabs.find(tab => tab.conversationId === conversationId);
      console.log('[Chat] 🧹 Tab encontrada:', tabToUpdate ? `id=${tabToUpdate.id}, backendId=${tabToUpdate.backendId}` : 'NENHUMA');
      
      // Anuncia para acessibilidade
      announce('Mensagens da conversa removidas');
      
      // Limpa as mensagens da aba que tinha essa conversa
      set((state) => {
        const updatedTabs: ChatTab[] = state.tabs.map(tab => {
          if (tab.conversationId === conversationId) {
            console.log('[Chat] 🧹 Limpando mensagens da tab:', tab.id);
            return {
              ...tab,
              threadedMessages: [] as MessageNode[],
              title: 'Conversa limpa',
              updatedAt: Date.now(),
            };
          }
          return tab;
        });
        console.log('[Chat] 🧹 Tabs atualizadas:', updatedTabs.length);
        return { tabs: updatedTabs };
      });
      
      // Foca no input de mensagem após um pequeno delay
      setTimeout(() => {
        const input = document.querySelector('textarea[placeholder*="mensagem"], textarea[aria-label*="mensagem"]') as HTMLTextAreaElement;
        if (input) {
          input.focus();
          console.log('[Chat] 🧹 Foco movido para input');
        }
      }, 200);
    },

    // Chamado quando uma conversa é renomeada
    handleConversationRenamed: (conversationId: number, newTitle: string) => {
      console.log('[Chat] Handling conversation renamed:', conversationId, '->', newTitle);
      
      set((state) => {
        const updatedTabs: ChatTab[] = state.tabs.map(tab => {
          if (tab.conversationId === conversationId) {
            console.log('[Chat] Renaming tab:', tab.id, 'to:', newTitle);
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
      
      // Anuncia para acessibilidade
      announce(`Conversa renomeada para ${newTitle}`);
    },

    // Chamado quando o banco de dados é resetado
    handleDatabaseReset: () => {
      console.log('[Chat] Handling database reset - reinitializing...');
      
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
      });
      
      // Reinicializa tabs do backend
      get().initializeTabs();
      
      // Anuncia para acessibilidade
      announce('Banco de dados resetado. Conversas reinicializadas.');
    },

    // Chamado quando uma tab é fechada externamente (via ChatManager ou outro)
    handleTabClosed: (backendTabId: number) => {
      console.log('[Chat] Handling tab closed externally:', backendTabId);
      
      const state = get();
      const tabToClose = state.tabs.find(t => t.backendId === backendTabId);
      
      if (!tabToClose) {
        console.log('[Chat] Tab not found, may have been closed locally already');
        return;
      }
      
      // Não permite fechar se for a última aba
      if (state.tabs.length <= 1) {
        console.log('[Chat] Cannot close last tab, clearing instead');
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
      
      // Limpa listeners se existirem
      const existingCleanup = activeListeners.get(tabToClose.id);
      if (existingCleanup) {
        existingCleanup();
        activeListeners.delete(tabToClose.id);
      }
      
      // Define próxima aba ativa se a tab fechada era a ativa
      let newActiveTabId = state.activeTabId;
      if (state.activeTabId === tabToClose.id) {
        const remainingTabs = state.tabs.filter(t => t.id !== tabToClose.id);
        const currentIndex = state.tabs.findIndex(t => t.id === tabToClose.id);
        const nextTab = remainingTabs[Math.min(currentIndex, remainingTabs.length - 1)];
        newActiveTabId = nextTab?.id || null;
      }
      
      // Remove a tab
      set((s) => ({
        tabs: s.tabs.filter(t => t.id !== tabToClose.id),
        activeTabId: newActiveTabId,
      }));
      
      // Consolida abas vazias
      setTimeout(() => get().consolidateEmptyTabs(), 100);
      
      // Anuncia para acessibilidade
      announce('Aba fechada');
    },

    // Consolida abas vazias - mantém apenas uma (a última)
    consolidateEmptyTabs: () => {
      const state = get();
      
      // Encontra todas as abas vazias (sem conversationId)
      const emptyTabs = state.tabs.filter(tab => !tab.conversationId);
      
      console.log('[Chat] Empty tabs found:', emptyTabs.length);
      
      // Se houver mais de uma aba vazia, fecha as extras (mantém a última)
      if (emptyTabs.length > 1) {
        // Mantém a última aba vazia (mais recente por updatedAt)
        const sortedEmpty = [...emptyTabs].sort((a, b) => b.updatedAt - a.updatedAt);
        const tabsToClose = sortedEmpty.slice(1); // Fecha todas exceto a mais recente
        
        console.log('[Chat] Closing extra empty tabs:', tabsToClose.map(t => t.id));
        
        // Fecha cada aba extra (precisa fazer sequencialmente para evitar race condition)
        for (const tab of tabsToClose) {
          const currentState = get();
          // Só fecha se não for a última aba total
          if (currentState.tabs.length > 1) {
            get().deleteTab(tab.id);
          }
        }
      }
    },
  };
});

// NOTA: Listeners globais de backend foram REMOVIDOS pois causavam loop infinito
// O backend já sincroniza as tabs via GetAllTabs() e não precisamos de listeners reativos
// Se precisar re-habilitar, mover para um hook React com cleanup apropriado

