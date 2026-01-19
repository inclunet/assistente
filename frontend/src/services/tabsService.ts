/**
 * Serviço para comunicação com o backend Wails - Gerenciamento de Abas
 */

import type { ChatTab, CreateTabRequest, UpdateTabRequest } from '../types/tabs';

// Simulação temporária até o backend estar pronto
// TODO: Substituir por chamadas reais ao Wails quando backend estiver implementado
const MOCK_ENABLED = true;

let mockTabs: ChatTab[] = [
  {
    id: 1,
    conversationId: null,
    title: 'Nova Conversa',
    icon: '💬',
    position: 0,
    isActive: true,
    createdAt: new Date().toISOString(),
    updatedAt: new Date().toISOString(),
  },
];
let nextMockId = 2;

/**
 * Obtém todas as abas do usuário
 */
export async function getTabs(): Promise<ChatTab[]> {
  if (MOCK_ENABLED) {
    return Promise.resolve([...mockTabs]);
  }
  
  // TODO: Implementar chamada real
  // return window.go.main.App.GetTabs();
  throw new Error('Backend não implementado');
}

/**
 * Obtém a aba ativa
 */
export async function getActiveTab(): Promise<ChatTab | null> {
  if (MOCK_ENABLED) {
    return Promise.resolve(mockTabs.find(tab => tab.isActive) || null);
  }
  
  // TODO: Implementar chamada real
  // return window.go.main.App.GetActiveTab();
  throw new Error('Backend não implementado');
}

/**
 * Cria uma nova aba
 */
export async function createTab(data: CreateTabRequest = {}): Promise<ChatTab> {
  if (MOCK_ENABLED) {
    // Desativa aba atual
    mockTabs = mockTabs.map(tab => ({ ...tab, isActive: false }));
    
    const newTab: ChatTab = {
      id: nextMockId++,
      conversationId: null,
      title: data.title || 'Nova Conversa',
      icon: data.icon || '💬',
      position: mockTabs.length,
      isActive: true,
      createdAt: new Date().toISOString(),
      updatedAt: new Date().toISOString(),
    };
    mockTabs.push(newTab);
    return Promise.resolve(newTab);
  }
  
  // TODO: Implementar chamada real
  // return window.go.main.App.CreateTab(data.title || 'Nova Conversa', data.icon || '💬');
  throw new Error('Backend não implementado');
}

/**
 * Fecha uma aba
 * Se for a única aba, cria uma nova automaticamente
 * Se for a aba ativa, ativa a aba anterior (ou próxima)
 */
export async function closeTab(tabId: number): Promise<void> {
  if (MOCK_ENABLED) {
    const tabIndex = mockTabs.findIndex(t => t.id === tabId);
    if (tabIndex === -1) return;
    
    const wasActive = mockTabs[tabIndex].isActive;
    mockTabs.splice(tabIndex, 1);
    
    // Se era a última aba, cria uma nova
    if (mockTabs.length === 0) {
      await createTab();
      return;
    }
    
    // Se era a aba ativa, ativa outra
    if (wasActive) {
      const newActiveIndex = Math.max(0, tabIndex - 1);
      mockTabs[newActiveIndex].isActive = true;
    }
    
    // Reajusta positions
    mockTabs.forEach((tab, index) => {
      tab.position = index;
    });
    
    return Promise.resolve();
  }
  
  // TODO: Implementar chamada real
  // return window.go.main.App.CloseTab(tabId);
  throw new Error('Backend não implementado');
}

/**
 * Define uma aba como ativa
 */
export async function setActiveTab(tabId: number): Promise<void> {
  if (MOCK_ENABLED) {
    mockTabs = mockTabs.map(tab => ({
      ...tab,
      isActive: tab.id === tabId,
    }));
    return Promise.resolve();
  }
  
  // TODO: Implementar chamada real
  // return window.go.main.App.SetActiveTab(tabId);
  throw new Error('Backend não implementado');
}

/**
 * Atualiza o título de uma aba
 */
export async function updateTabTitle(tabId: number, title: string): Promise<void> {
  if (MOCK_ENABLED) {
    const tab = mockTabs.find(t => t.id === tabId);
    if (tab) {
      tab.title = title;
      tab.updatedAt = new Date().toISOString();
    }
    return Promise.resolve();
  }
  
  // TODO: Implementar chamada real
  // return window.go.main.App.UpdateTabTitle(tabId, title);
  throw new Error('Backend não implementado');
}

/**
 * Carrega uma conversa em uma aba
 * Atualiza o título da aba para o título da conversa
 */
export async function loadConversationInTab(tabId: number, conversationId: number): Promise<void> {
  if (MOCK_ENABLED) {
    const tab = mockTabs.find(t => t.id === tabId);
    if (tab) {
      tab.conversationId = conversationId;
      // TODO: Buscar título real da conversa
      tab.title = `Conversa ${conversationId}`;
      tab.updatedAt = new Date().toISOString();
    }
    return Promise.resolve();
  }
  
  // TODO: Implementar chamada real
  // return window.go.main.App.LoadConversationInTab(tabId, conversationId);
  throw new Error('Backend não implementado');
}

/**
 * Limpa uma aba (remove a conversa carregada)
 */
export async function clearTab(tabId: number): Promise<void> {
  if (MOCK_ENABLED) {
    const tab = mockTabs.find(t => t.id === tabId);
    if (tab) {
      tab.conversationId = null;
      tab.title = 'Nova Conversa';
      tab.icon = '💬';
      tab.updatedAt = new Date().toISOString();
    }
    return Promise.resolve();
  }
  
  // TODO: Implementar chamada real
  // return window.go.main.App.ClearTab(tabId);
  throw new Error('Backend não implementado');
}

/**
 * Reordena as abas
 * @param tabIds Array com os IDs das abas na nova ordem
 */
export async function reorderTabs(tabIds: number[]): Promise<void> {
  if (MOCK_ENABLED) {
    const reordered: ChatTab[] = [];
    tabIds.forEach((id, index) => {
      const tab = mockTabs.find(t => t.id === id);
      if (tab) {
        reordered.push({ ...tab, position: index });
      }
    });
    mockTabs = reordered;
    return Promise.resolve();
  }
  
  // TODO: Implementar chamada real
  // return window.go.main.App.ReorderTabs(tabIds);
  throw new Error('Backend não implementado');
}
