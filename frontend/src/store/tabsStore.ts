/**
 * Store Zustand para gerenciamento de estado das abas de chat
 */

import { create } from 'zustand';
import type { TabsState, ChatTab } from '../types/tabs';
import * as tabsService from '../services/tabsService';

export const useTabsStore = create<TabsState>((set, get) => ({
  tabs: [],
  activeTabId: null,
  isLoading: false,
  error: null,

  /**
   * Carrega todas as abas do backend
   */
  loadTabs: async () => {
    set({ isLoading: true, error: null });
    try {
      const tabs = await tabsService.getTabs();
      const activeTab = tabs.find(tab => tab.isActive);
      set({
        tabs,
        activeTabId: activeTab?.id || null,
        isLoading: false,
      });
    } catch (error) {
      set({
        error: error instanceof Error ? error.message : 'Erro ao carregar abas',
        isLoading: false,
      });
    }
  },

  /**
   * Cria uma nova aba
   */
  createTab: async (title?: string, icon?: string) => {
    try {
      const newTab = await tabsService.createTab({ title, icon });
      set(state => ({
        tabs: state.tabs.map(tab => ({ ...tab, isActive: false })).concat(newTab),
        activeTabId: newTab.id,
      }));
      return newTab;
    } catch (error) {
      set({
        error: error instanceof Error ? error.message : 'Erro ao criar aba',
      });
      return null;
    }
  },

  /**
   * Fecha uma aba
   */
  closeTab: async (tabId: number) => {
    try {
      await tabsService.closeTab(tabId);
      
      const state = get();
      const tabIndex = state.tabs.findIndex(t => t.id === tabId);
      if (tabIndex === -1) return;

      const wasActive = state.tabs[tabIndex].isActive;
      const newTabs = state.tabs.filter(t => t.id !== tabId);

      // Se era a única aba, criar uma nova
      if (newTabs.length === 0) {
        await get().createTab();
        return;
      }

      // Se era a aba ativa, ativar outra
      let newActiveTabId = state.activeTabId;
      if (wasActive) {
        const newActiveIndex = Math.max(0, tabIndex - 1);
        newActiveTabId = newTabs[newActiveIndex].id;
        newTabs[newActiveIndex] = { ...newTabs[newActiveIndex], isActive: true };
      }

      set({
        tabs: newTabs,
        activeTabId: newActiveTabId,
      });
    } catch (error) {
      set({
        error: error instanceof Error ? error.message : 'Erro ao fechar aba',
      });
    }
  },

  /**
   * Define uma aba como ativa
   */
  setActiveTab: async (tabId: number) => {
    try {
      await tabsService.setActiveTab(tabId);
      set(state => ({
        tabs: state.tabs.map(tab => ({
          ...tab,
          isActive: tab.id === tabId,
        })),
        activeTabId: tabId,
      }));
    } catch (error) {
      set({
        error: error instanceof Error ? error.message : 'Erro ao ativar aba',
      });
    }
  },

  /**
   * Atualiza o título de uma aba
   */
  updateTabTitle: async (tabId: number, title: string) => {
    try {
      await tabsService.updateTabTitle(tabId, title);
      set(state => ({
        tabs: state.tabs.map(tab =>
          tab.id === tabId
            ? { ...tab, title, updatedAt: new Date().toISOString() }
            : tab
        ),
      }));
    } catch (error) {
      set({
        error: error instanceof Error ? error.message : 'Erro ao atualizar título',
      });
    }
  },

  /**
   * Carrega uma conversa em uma aba
   */
  loadConversationInTab: async (tabId: number, conversationId: number) => {
    try {
      await tabsService.loadConversationInTab(tabId, conversationId);
      set(state => ({
        tabs: state.tabs.map(tab =>
          tab.id === tabId
            ? {
                ...tab,
                conversationId,
                title: `Conversa ${conversationId}`, // TODO: buscar título real
                updatedAt: new Date().toISOString(),
              }
            : tab
        ),
      }));
    } catch (error) {
      set({
        error: error instanceof Error ? error.message : 'Erro ao carregar conversa',
      });
    }
  },

  /**
   * Limpa uma aba (remove a conversa)
   */
  clearTab: async (tabId: number) => {
    try {
      await tabsService.clearTab(tabId);
      set(state => ({
        tabs: state.tabs.map(tab =>
          tab.id === tabId
            ? {
                ...tab,
                conversationId: null,
                title: 'Nova Conversa',
                icon: '💬',
                updatedAt: new Date().toISOString(),
              }
            : tab
        ),
      }));
    } catch (error) {
      set({
        error: error instanceof Error ? error.message : 'Erro ao limpar aba',
      });
    }
  },

  /**
   * Reordena as abas
   */
  reorderTabs: async (tabIds: number[]) => {
    try {
      await tabsService.reorderTabs(tabIds);
      set(state => {
        const reordered = tabIds
          .map(id => state.tabs.find(t => t.id === id))
          .filter((tab): tab is ChatTab => tab !== undefined)
          .map((tab, index) => ({ ...tab, position: index }));
        return { tabs: reordered };
      });
    } catch (error) {
      set({
        error: error instanceof Error ? error.message : 'Erro ao reordenar abas',
      });
    }
  },
}));
