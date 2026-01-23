console.log('🟡🟡🟡 [chatStore.ts] MÓDULO SENDO CARREGADO 🟡🟡🟡');
console.log('🟡 Timestamp:', new Date().toISOString());

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
} from '../../wailsjs/go/main/App';
import { EventsOn } from '../../wailsjs/runtime/runtime';
import { MediaFile } from '../services/mediaService';
import { database, main } from '../../wailsjs/go/models';
import { announce } from '../hooks/useAnnouncer';
import { playSendSound, playReceiveSound } from '../services/audioFeedback';
import { ttsService } from '../services/tts';
import { messageAudioService } from '../services/messageAudio';
import { VOICE_DISABLED } from '../components/pickers/VoicePicker';
import { useSettingsStore } from './settingsStore';
import { stripMarkdown } from '../lib/stripMarkdown';

interface MediaData {
  name: string;
  type: string;
  data: string; // base64
  size: number;
}

export interface ToolCall {
  id: string;
  type: string;
  function: {
    name: string;
    arguments: string;
  };
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
  toolCallId?: string;
  agentName?: string;
  toolName?: string;
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

  // Initialization
  initializeTabs: () => Promise<void>;

  // Tab management
  createTab: (activate?: boolean) => Promise<string>;
  deleteTab: (tabId: string) => Promise<void>;
  setActiveTab: (tabId: string) => Promise<void>;
  updateTabTitle: (tabId: string, title: string) => void;

  // Message management
  addMessage: (tabId: string, message: NewMessageData) => string;
  updateMessage: (tabId: string, messageId: string, content: string) => void;
  addInternalMessage: (tabId: string, message: Message) => void; // Adiciona mensagem interna (tool call)
  clearMessages: (tabId: string) => void;
  clearActiveTab: () => void;
  
  // Thread management
  toggleThreadExpanded: (messageId: string) => void;
  isThreadExpanded: (messageId: string) => boolean;

  // Chat actions
  sendMessage: (content: string, mediaFiles?: MediaFile[]) => Promise<void>;
  stopStreaming: () => void;

  // Utility
  getActiveTab: () => ChatTab | undefined;
  getTabMessages: (tabId: string) => Message[];
  
  // Thread management
  getThreadedMessages: () => MessageNode[] | undefined;
  loadMessageChildren: (messageId: string) => Promise<MessageNode[]>;
}

const generateId = () => `${Date.now()}-${Math.random().toString(36).substr(2, 9)}`;

// Mapa para rastrear cleanup functions de cada tab (previne acúmulo de listeners)
const activeListeners = new Map<string, () => void>();

// Contador global de listeners para debug
let activeListenerCount = 0;

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

export const useChatStore = create<ChatStore>()((set, get) => {
  console.log('🟢🟢🟢 [chatStore] CRIANDO STORE 🟢🟢🟢');
  console.log('🟢 Estado inicial: tabs=[], isInitialized=false');
  
  // FORCE RESET - Remove qualquer estado persistido
  if (typeof window !== 'undefined') {
    const keysToRemove: string[] = [];
    for (let i = 0; i < localStorage.length; i++) {
      const key = localStorage.key(i);
      if (key && (key.includes('chat') || key.includes('tabs') || key.includes('zustand'))) {
        keysToRemove.push(key);
      }
    }
    keysToRemove.forEach(key => {
      console.log('🔥 Removendo localStorage:', key);
      localStorage.removeItem(key);
    });
  }
  
  return {
    tabs: [],
    activeTabId: null,
    isLoading: false,
    streamingMessageId: null,
    isInitialized: false,
    expandedThreads: new Set<string>(),

    initializeTabs: async () => {
      console.log('========================================');
      console.log('===== [initializeTabs] INICIANDO =====');
      console.log('========================================');
    try {
      console.log('[Chat] Initializing tabs from backend...');
      const backendTabs = await GetAllTabs();
      console.log('[Chat] Loaded tabs from backend:', backendTabs);

      let tabs = backendTabs.map(backendTabToFrontend);
      
      // Se não houver tabs, cria uma aba em branco
      if (tabs.length === 0) {
        console.log('[Chat] No tabs found, creating default tab...');
        const newBackendTab = await CreateTab('Nova Conversa', '💬');
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
      console.log('[Chat] Creating new tab in backend...');
      const backendTab = await CreateTab('Nova Conversa', '💬');
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
    const tab = get().tabs.find(t => t.id === tabId);
    console.log('[Chat] 🔵 Tab encontrada:', tab ? `id=${tab.id}, backendId=${tab.backendId}` : 'NÃO ENCONTRADA');
    
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
    const newMessage = new main.EnrichedMessage({
      ...message,
      id: messageId,
      timestamp: Date.now(),
      conversationId: 0, // Será atualizado pelo backend
      isStreaming: message.isStreaming ?? false,
      internal: false,
      createdAt: new Date(),
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

    // Anuncia mensagem para leitores de tela
    if (message.role === 'user') {
      // Mensagem do usuário
      const cleanContent = stripMarkdown(message.content);
      const preview = cleanContent.slice(0, 150);
      announce(`Você: ${preview}${cleanContent.length > 150 ? '...' : ''}`);
      
      // Toca som de envio
      playSendSound();
    } else if (message.role === 'assistant' && !message.isStreaming) {
      // Mensagem do assistente completa (não streaming)
      
      // Verifica se esta é a aba ativa
      const isActiveTab = get().activeTabId === tabId;
      
      // Toca som de recebimento (apenas na aba ativa)
      if (isActiveTab) {
        playReceiveSound();
      }
      
      // Verifica se TTS vai ler a mensagem
      const settings = useSettingsStore.getState();
      const voiceEnabled = settings.config?.voice && settings.config.voice !== VOICE_DISABLED;
      const willUseTTS = voiceEnabled && ttsService.isAutoReadEnabled();
      
      // Só anuncia via aria-live se TTS NÃO estiver ativo
      // (evita conflito entre TTS e leitor de tela)
      if (!willUseTTS && isActiveTab) {
        const cleanContent = stripMarkdown(message.content);
        const preview = cleanContent.slice(0, 150);
        announce(`Assistente: ${preview}${cleanContent.length > 150 ? '...' : ''}`);
      }
      
      // REMOVIDO: Lógica antiga de TTS que causava duplicação
      // Agora o TTS é gerenciado exclusivamente no streamComplete via synthesizeForMessage()
      // e messageAudioService para reprodução isolada por mensagem
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
                // Função recursiva para atualizar em qualquer nível
                const updateNodeContent = (n: MessageNode): MessageNode => {
                  if (n.message.id === messageId) {
                    n.message.content = content;
                  }
                  if (n.children && n.children.length > 0) {
                    n.children = n.children.map(updateNodeContent);
                  }
                  return n;
                };
                return updateNodeContent(node);
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
  
  // Adiciona mensagem interna (filho de uma mensagem raiz, ex: tool calls)
  addInternalMessage: (tabId, message) => {
    console.log('[Chat] Adding internal message:', { tabId, message });
    
    set((state) => {
      const tab = state.tabs.find(t => t.id === tabId);
      if (!tab) return state;
      
      // Verifica se a mensagem já existe na árvore
      const exists = flattenThreadedMessages(tab.threadedMessages).some(m => m.id === message.id);
      if (exists) {
        console.log('[Chat] Message already exists, skipping:', message.id);
        return state;
      }
      
      // TODO: Implementar inserção correta na árvore baseado no parentId
      // Por enquanto, apenas adiciona como nó raiz
      const newNode = new main.MessageNode({
        message,
        children: [],
        level: message.parentId ? 1 : 0,
        childCount: 0,
      });
      
      return {
        tabs: state.tabs.map((t) =>
          t.id === tabId
            ? {
                ...t,
                threadedMessages: [...t.threadedMessages, newNode],
              }
            : t
        ),
        // Auto-expande a thread da mensagem pai quando uma mensagem interna chega
        expandedThreads: message.parentId 
          ? new Set([...state.expandedThreads, message.parentId.toString()])
          : state.expandedThreads,
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

  sendMessage: async (content, mediaFiles) => {
        const { activeTabId, addMessage, createTab, updateTabTitle, tabs } = get();
        
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
        
        addMessage(currentTabId, {
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

        try {
          const tab = get().tabs.find((t) => t.id === currentTabId);
          const conversationId = tab?.conversationId || 0;

          console.log('[Chat] Sending message to backend...', {
            conversationId,
            contentLength: content.length,
            hasMedia: !!mediaFiles && mediaFiles.length > 0,
          });

          // If we have media files, use AddMessageWithMedia
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

            // AddMessageWithMedia returns the new message (ChatMessage with id and conversation_id)
            const newMessage = await AddMessageWithMedia(
              conversationId,
              'user',
              content,
              mediaJson,
              '', // toolCalls
              ''  // toolResults
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

            // Update tab with conversation ID
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
            await SendMessage(conversationId, content, 'user', {
              model: '',
              temperature: 0.7,
              maxTokens: 4096,
              useTools: true,
            });
          }

          // Setup completion handler that will clean up everything
          let unsubscribeStream: (() => void) | null = null;
          let unsubscribeComplete: (() => void) | null = null;
          let cleanupExecuted = false; // Flag para evitar cleanup duplicado

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
            set({ isLoading: false, streamingMessageId: null });
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
              // O backend já envia o conteúdo completo acumulado, não apenas o delta
              get().updateMessage(currentTabId!, assistantMessageId, event.content);
            }
            
            if (event.error) {
              console.error('[Chat] Stream error:', event.error);
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
                
                // Verifica se TTS deve ler a mensagem
                const settings = useSettingsStore.getState();
                const voiceEnabled = settings.config?.voice && settings.config.voice !== VOICE_DISABLED;
                const willUseTTS = voiceEnabled && ttsService.isAutoReadEnabled();
                
                // Sintetiza áudio para esta mensagem (não toca ainda)
                // IMPORTANTE: Verifica se este listener ainda é válido ANTES de sintetizar
                if (willUseTTS && isActiveTab && cleanupExecuted === false) {
                  // Sintetiza de forma assíncrona (não bloqueia)
                  ttsService.synthesizeForMessage(finalMessage.content).then((audioBlob) => {
                    if (audioBlob) {
                      // Cria player para esta mensagem
                      messageAudioService.createAudioForMessage(
                        assistantMessageId,
                        audioBlob
                      );
                      
                      // Toca automaticamente se ainda for a aba ativa
                      const currentActiveTabId = get().activeTabId;
                      const isStillActive = currentTabId === currentActiveTabId;
                      
                      if (isStillActive) {
                        const volume = ttsService.getVolume();
                        ttsService.stop();
                        
                        messageAudioService.playMessage(assistantMessageId, volume).catch((error) => {
                          console.error('[Chat] ❌ Erro ao reproduzir áudio:', error);
                        });
                      }
                    }
                  }).catch((error) => {
                    console.error('[Chat] ❌ Erro ao sintetizar TTS:', error);
                  });
                }
                
                // Só anuncia via aria-live se TTS NÃO estiver ativo
                // (evita conflito entre TTS e leitor de tela)
                if (!willUseTTS && isActiveTab) {
                  const cleanContent = stripMarkdown(finalMessage.content);
                  const preview = cleanContent.slice(0, 150);
                  announce(`Assistente: ${preview}${cleanContent.length > 150 ? '...' : ''}`);
                }
              }
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
            
            // Cleanup listeners
            cleanup();
          });

          // Listen for tool execution events
          const unsubscribeTools = EventsOn('chat:tools', (event: any) => {
            if (!activeListeners.has(currentTabId!)) return;
            
            console.log('[Chat] Tools executing:', event);
            
            // Adiciona mensagem visual de tools sendo executadas
            const toolNames = event.tools?.join(', ') || 'ferramentas';
            announce(`Executando ${toolNames}`, 'polite');
          });

          // Listen for tool results
          const unsubscribeToolResults = EventsOn('chat:tool_results', (event: any) => {
            if (!activeListeners.has(currentTabId!)) return;
            
            console.log('[Chat] Tool results:', event);
          });

          // Listen for agent messages
          const unsubscribeAgentMessage = EventsOn('chat:agent_message', (event: any) => {
            if (!activeListeners.has(currentTabId!)) return;
            
            console.log('[Chat] Agent message:', event);
            
            // Anuncia atividade do agente
            if (event.agentName) {
              const agentDisplay = event.agentName.split('_').map((w: string) => 
                w.charAt(0).toUpperCase() + w.slice(1)
              ).join(' ');
              announce(`${agentDisplay} está processando`, 'polite');
            }
          });
          
          // Listen for internal messages (tool calls, agent responses)
          const unsubscribeInternalMessage = EventsOn('chat:internal_message', (event: any) => {
            if (!activeListeners.has(currentTabId!)) return;
            
            console.log('[Chat] Internal message received:', event);
            
            // Cria instância de EnrichedMessage para mensagem interna
            const internalMessage = new main.EnrichedMessage({
              id: event.id?.toString() || generateId(),
              conversationId: 0,
              parentId: event.parentId || event.parent_id,
              role: event.role || 'tool',
              content: event.content || '',
              toolCallId: event.toolCallId || event.tool_call_id,
              agentName: event.agentName || event.agent_name,
              toolName: event.toolName || event.tool_name,
              internal: true,
              timestamp: Date.now(),
              isStreaming: false,
              createdAt: new Date(),
            });
            
            // Adiciona mensagem interna à tab
            get().addInternalMessage(currentTabId!, internalMessage);
            
            // Anuncia execução de ferramenta/agente
            if (internalMessage.agentName) {
              const agentDisplay = internalMessage.agentName.split('_').map((w: string) => 
                w.charAt(0).toUpperCase() + w.slice(1)
              ).join(' ');
              announce(`${agentDisplay} respondeu`, 'polite');
            }
          });

          // Atualiza cleanup para incluir novos listeners
          const originalCleanup = cleanup;
          const enhancedCleanup = () => {
            originalCleanup();
            if (unsubscribeTools) unsubscribeTools();
            if (unsubscribeToolResults) unsubscribeToolResults();
            if (unsubscribeAgentMessage) unsubscribeAgentMessage();
            if (unsubscribeInternalMessage) unsubscribeInternalMessage();
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
          };
          
          // Note: DO NOT cleanup here - listeners need to stay active for streaming events
        } catch (error) {
          console.error('[Chat] Error sending message:', error);
          
          // Cleanup listener if it exists
          if (unsubscribe) {
            unsubscribe();
          }

          get().updateMessage(
            currentTabId!,
            assistantMessageId,
            `Erro ao enviar mensagem: ${error instanceof Error ? error.message : 'Erro desconhecido'}`
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
        console.log('[chatStore] 🔄 toggleThreadExpanded called for:', messageId);
        set((state) => {
          const expanded = new Set(state.expandedThreads);
          const wasExpanded = expanded.has(messageId);
          if (wasExpanded) {
            expanded.delete(messageId);
            console.log('[chatStore] ➖ Collapsed thread:', messageId);
          } else {
            expanded.add(messageId);
            console.log('[chatStore] ➕ Expanded thread:', messageId);
          }
          console.log('[chatStore] 📊 Current expanded threads:', Array.from(expanded));
          return { expandedThreads: expanded };
        });
      },
      
      // Verifica se uma thread está expandida
      isThreadExpanded: (messageId) => {
        return get().expandedThreads.has(messageId);
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
  };
});

// NOTA: Listeners globais de backend foram REMOVIDOS pois causavam loop infinito
// O backend já sincroniza as tabs via GetAllTabs() e não precisamos de listeners reativos
// Se precisar re-habilitar, mover para um hook React com cleanup apropriado

