/**
 * Tipos para o sistema de guias (tabs) de chat
 */

/**
 * Representa uma aba de chat
 */
export interface ChatTab {
  /** ID único da aba (do banco de dados) */
  id: number;
  /** ID da conversa carregada nesta aba (null se aba vazia) */
  conversationId: number | null;
  /** Título da aba */
  title: string;
  /** Ícone da aba (emoji) */
  icon: string;
  /** Posição/ordem da aba */
  position: number;
  /** Se esta é a aba ativa */
  isActive: boolean;
  /** Data de criação */
  createdAt: string;
  /** Data de última atualização */
  updatedAt: string;
}

/**
 * Estado do store de guias
 */
export interface TabsState {
  /** Lista de todas as abas abertas */
  tabs: ChatTab[];
  /** ID da aba atualmente ativa */
  activeTabId: number | null;
  /** Se está carregando as abas */
  isLoading: boolean;
  /** Erro ao carregar/manipular abas */
  error: string | null;
  
  // Ações
  /** Carrega todas as abas do backend */
  loadTabs: () => Promise<void>;
  /** Cria uma nova aba */
  createTab: (title?: string, icon?: string) => Promise<ChatTab | null>;
  /** Fecha uma aba */
  closeTab: (tabId: number) => Promise<void>;
  /** Define uma aba como ativa */
  setActiveTab: (tabId: number) => Promise<void>;
  /** Atualiza o título de uma aba */
  updateTabTitle: (tabId: number, title: string) => Promise<void>;
  /** Carrega uma conversa em uma aba */
  loadConversationInTab: (tabId: number, conversationId: number) => Promise<void>;
  /** Limpa uma aba (remove a conversa) */
  clearTab: (tabId: number) => Promise<void>;
  /** Reordena as abas */
  reorderTabs: (tabIds: number[]) => Promise<void>;
}

/**
 * Dados para criação de uma nova aba
 */
export interface CreateTabRequest {
  title?: string;
  icon?: string;
}

/**
 * Dados para atualização de uma aba
 */
export interface UpdateTabRequest {
  title?: string;
  icon?: string;
  position?: number;
}
