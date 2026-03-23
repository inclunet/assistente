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
import { useWorkspaceStore, type TabType } from '../store/workspaceStore';
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
  const { workspace, addTab, removeTab, setActiveTab, createWorkspace } = useWorkspaceStore();
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
      const isDataGrid = target.closest('.datagrid-container') !== null;
      if (isDataGrid) return;

      // Chord mode: aguardando segunda tecla após Ctrl+N
      if (chordPendingRef.current && !event.ctrlKey && !event.altKey && !event.metaKey) {
        const key = event.key.toLowerCase();
        const match = CHORD_MAP[key];
        if (match) {
          event.preventDefault();
          event.stopPropagation();
          void addTab(match.type, '', match.title);
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
        createWorkspace(`Workspace ${Date.now().toString(36)}`);
        return;
      }

      // Ctrl+N: Abre chord para criar aba por tipo + abre menu visual
      if (event.ctrlKey && event.key === 'n' && !event.shiftKey && !event.altKey) {
        event.preventDefault();
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
        addTab('chat', '', 'Nova conversa');
        announce('Nova aba criada');
        return;
      }

      // Ctrl+W: Fechar aba ativa
      if (event.ctrlKey && event.key === 'w' && !event.shiftKey && !event.altKey && activeTabId) {
        event.preventDefault();
        void removeTab(activeTabId).then(() => requestAnimationFrame(() => restoreDefaultFocus()));
        return;
      }

      // Ctrl+F4: Fechar aba ativa (alternativo)
      if (event.ctrlKey && event.key === 'F4' && activeTabId) {
        event.preventDefault();
        void removeTab(activeTabId).then(() => requestAnimationFrame(() => restoreDefaultFocus()));
        return;
      }

      // Ctrl+Tab: Próxima aba
      if (event.ctrlKey && event.key === 'Tab' && !event.shiftKey) {
        event.preventDefault();
        navigateTab(1);
        return;
      }

      // Ctrl+Shift+Tab: Aba anterior
      if (event.ctrlKey && event.key === 'Tab' && event.shiftKey) {
        event.preventDefault();
        navigateTab(-1);
        return;
      }

      // Ctrl+PageDown: Próxima aba
      if (event.ctrlKey && event.key === 'PageDown') {
        event.preventDefault();
        navigateTab(1);
        return;
      }

      // Ctrl+PageUp: Aba anterior
      if (event.ctrlKey && event.key === 'PageUp') {
        event.preventDefault();
        navigateTab(-1);
        return;
      }

      // Ctrl+1-9: Vai direto para aba N
      if (event.ctrlKey && !event.shiftKey && !event.altKey) {
        const num = parseInt(event.key, 10);
        if (num >= 1 && num <= 9) {
          event.preventDefault();
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
