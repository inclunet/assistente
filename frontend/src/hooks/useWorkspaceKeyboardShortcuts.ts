/**
 * Atalhos globais de teclado do workspace.
 *
 * Abas:
 * - Ctrl+T: Nova aba de chat (ação rápida)
 * - Ctrl+N: Menu "Criar..." — seguido de C(hat), E(ditor), R(terminal), T(asklist)
 * - Ctrl+W / Ctrl+F4: Fechar aba ativa
 * - Ctrl+Tab / Ctrl+PageDown: Próxima aba
 * - Ctrl+Shift+Tab / Ctrl+PageUp: Aba anterior
 * - Ctrl+1..9: Vai direto para aba N
 *
 * Workspace:
 * - Ctrl+Shift+N: Novo workspace
 */

import { useEffect, useRef } from 'react';
import i18next from 'i18next';
import { useWorkspaceStore, type TabType } from '../store/workspaceStore';
import { useShallow } from 'zustand/shallow';
import { useWorkspaceChatModalStore } from '../store/workspaceChatModalStore';
import { useShortcutsHelpStore } from '../store/shortcutsHelpStore';
import { isModalOpen } from '../components/ui/Modal';
import { useAnnouncer } from './useAnnouncer';
import { restoreDefaultFocus } from './useDefaultFocus';

const CHORD_TIMEOUT_MS = 1500;

const CHORD_MAP: Record<string, { type: TabType; title: string }> = {
  c: { type: 'chat', title: 'Nova conversa' },
  e: { type: 'editor', title: 'Novo documento' },
  r: { type: 'terminal', title: 'Terminal' },
  t: { type: 'tasklist', title: 'Tarefas' },
};

export function useWorkspaceKeyboardShortcuts() {
  const { workspace, addTab, removeTab, setActiveTab, createWorkspace } = useWorkspaceStore(
    useShallow((s) => ({ workspace: s.workspace, addTab: s.addTab, removeTab: s.removeTab, setActiveTab: s.setActiveTab, createWorkspace: s.createWorkspace }))
  );
  const { announce } = useAnnouncer();

  const tabs = workspace?.tabs || [];
  const activeTabId = workspace?.activeTabId || null;

  const chordPendingRef = useRef(false);
  const chordTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);

  useEffect(() => {
    return () => {
      if (chordTimerRef.current) clearTimeout(chordTimerRef.current);
    };
  }, []);

  useEffect(() => {
    const handleKeyDown = (event: KeyboardEvent) => {
      const target = event.target as HTMLElement;

      // Ctrl+? (Ctrl+Shift+/): alterna o painel global de atalhos.
      // Trata variações de layout: alguns teclados emitem `?` direto (o caractere
      // já reflete o Shift), outros exigem Shift sobre `/` (`code === 'Slash'`
      // cobre a tecla física em layouts US). Quando a tecla base é `/`/`Slash`,
      // o Shift é obrigatório — assim `Ctrl+/` puro NÃO é interceptado.
      if (
        event.ctrlKey &&
        !event.altKey &&
        !event.metaKey &&
        (event.key === '?' ||
          (event.shiftKey && (event.key === '/' || event.code === 'Slash')))
      ) {
        event.preventDefault();
        useShortcutsHelpStore.getState().toggle();
        return;
      }

      // Ctrl+Shift+I: chat modal do painel (adaptador registado pela aba ativa)
      if (
        event.ctrlKey &&
        event.shiftKey &&
        (event.code === 'KeyI' || event.key === 'i' || event.key === 'I') &&
        !event.altKey
      ) {
        // Sempre previne o default (DevTools do navegador), mesmo com um modal
        // aberto; mas não aciona o chat modal enquanto isModalOpen() for true
        // (não agir na UI de fundo / não empilhar modais).
        event.preventDefault();
        if (isModalOpen()) return;
        if (!activeTabId) return;
        void useWorkspaceChatModalStore.getState().requestOpen(activeTabId);
        return;
      }

      const isDataGrid = target.closest('.datagrid-container') !== null;
      if (isDataGrid) return;

      // Chord mode: aguardando segunda tecla após Ctrl+N
      if (chordPendingRef.current && !event.ctrlKey && !event.altKey && !event.metaKey) {
        // Se um modal (ex.: o painel de atalhos) abriu durante o chord, cancela
        // sem agir na UI de fundo.
        if (isModalOpen()) {
          chordPendingRef.current = false;
          if (chordTimerRef.current) {
            clearTimeout(chordTimerRef.current);
            chordTimerRef.current = null;
          }
          return;
        }
        const key = event.key.toLowerCase();
        const match = CHORD_MAP[key];
        if (match) {
          event.preventDefault();
          event.stopPropagation();
          void addTab(match.type, match.title);
          announce(`Nova aba: ${match.title}`);
        }
        chordPendingRef.current = false;
        if (chordTimerRef.current) {
          clearTimeout(chordTimerRef.current);
          chordTimerRef.current = null;
        }
        if (match) return;
      }

      // Ctrl+Shift+N: Novo workspace
      if (event.ctrlKey && event.shiftKey && event.key === 'N') {
        event.preventDefault();
        if (isModalOpen()) return;
        createWorkspace(`Workspace ${Date.now().toString(36)}`);
        return;
      }

      // Ctrl+N: Abre chord para criar aba por tipo + abre menu visual
      if (event.ctrlKey && event.key === 'n' && !event.shiftKey && !event.altKey) {
        event.preventDefault();
        if (isModalOpen()) return;
        chordPendingRef.current = true;
        if (chordTimerRef.current) clearTimeout(chordTimerRef.current);
        chordTimerRef.current = setTimeout(() => {
          chordPendingRef.current = false;
          chordTimerRef.current = null;
        }, CHORD_TIMEOUT_MS);
        window.dispatchEvent(new CustomEvent('workspace:open-new-tab-menu'));
        announce('Criar aba: C chat, E editor, R terminal, T tarefas');
        return;
      }

      // Ctrl+T: Nova aba de chat (ação rápida)
      if (event.ctrlKey && event.key === 't' && !event.shiftKey && !event.altKey) {
        const isInput = target.tagName === 'INPUT' || target.tagName === 'TEXTAREA' || target.isContentEditable;
        if (isInput) return;
        event.preventDefault();
        if (isModalOpen()) return;
        addTab('chat', i18next.t('chat.newConversation'));
        announce(i18next.t('workspace.tabCreated'));
        return;
      }

      // Ctrl+W: Fechar aba ativa
      if (event.ctrlKey && event.key === 'w' && !event.shiftKey && !event.altKey && activeTabId) {
        event.preventDefault();
        if (isModalOpen()) return;
        void removeTab(activeTabId).then(() => requestAnimationFrame(() => restoreDefaultFocus()));
        return;
      }

      // Ctrl+F4: Fechar aba ativa (alternativo)
      if (event.ctrlKey && event.key === 'F4' && activeTabId) {
        event.preventDefault();
        if (isModalOpen()) return;
        void removeTab(activeTabId).then(() => requestAnimationFrame(() => restoreDefaultFocus()));
        return;
      }

      // Ctrl+Tab / Ctrl+PageDown/Up: delega para escopos de abas aninhados
      const insideTabScope = target.closest('[data-tab-scope]') !== null;

      // Ctrl+Tab: Próxima aba
      if (event.ctrlKey && event.key === 'Tab' && !event.shiftKey) {
        if (insideTabScope) return;
        event.preventDefault();
        navigateTab(1);
        return;
      }

      // Ctrl+Shift+Tab: Aba anterior
      if (event.ctrlKey && event.key === 'Tab' && event.shiftKey) {
        if (insideTabScope) return;
        event.preventDefault();
        navigateTab(-1);
        return;
      }

      // Ctrl+PageDown: Próxima aba
      if (event.ctrlKey && event.key === 'PageDown') {
        if (insideTabScope) return;
        event.preventDefault();
        navigateTab(1);
        return;
      }

      // Ctrl+PageUp: Aba anterior
      if (event.ctrlKey && event.key === 'PageUp') {
        if (insideTabScope) return;
        event.preventDefault();
        navigateTab(-1);
        return;
      }

      // Ctrl+1-9: Vai direto para aba N
      if (event.ctrlKey && !event.shiftKey && !event.altKey) {
        const num = parseInt(event.key, 10);
        if (num >= 1 && num <= 9) {
          event.preventDefault();
          if (isModalOpen()) {
            announce(i18next.t('workspace.closeDialogBeforeChangingTabs'));
            return;
          }
          const targetTab = tabs[num - 1];
          if (targetTab) {
            setActiveTab(targetTab.id);
            announce(`${targetTab.title}, ${num} de ${tabs.length}`);
          }
        }
      }
    };

    function navigateTab(direction: 1 | -1) {
      if (tabs.length <= 1) return;
      if (isModalOpen()) {
        announce(i18next.t('workspace.closeDialogBeforeChangingTabs'));
        return;
      }
      const currentIndex = tabs.findIndex(t => t.id === activeTabId);
      if (currentIndex === -1) return;

      let nextIndex = currentIndex + direction;
      if (nextIndex >= tabs.length) nextIndex = 0;
      if (nextIndex < 0) nextIndex = tabs.length - 1;

      const nextTab = tabs[nextIndex];
      if (nextTab) {
        setActiveTab(nextTab.id);
        announce(`${nextTab.title}, ${nextIndex + 1} de ${tabs.length}`);
      }
    }

    window.addEventListener('keydown', handleKeyDown, true);
    return () => window.removeEventListener('keydown', handleKeyDown, true);
  }, [tabs, activeTabId, addTab, removeTab, setActiveTab, createWorkspace, announce]);
}
