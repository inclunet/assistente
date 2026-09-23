/**
 * Atalhos globais de teclado do workspace.
 *
 * Abas:
 * Workspace: atalhos de criação e mutação são resolvidos pelo mapa efetivo
 * no Topbar; este hook mantém apenas atalhos locais legados ainda não migrados.
 */

import { useEffect } from 'react';
import { useShortcutsHelpStore } from '../store/shortcutsHelpStore';

export function useWorkspaceKeyboardShortcuts() {
  useEffect(() => {
    const handleShortcutsHelpKeyDown = (event: KeyboardEvent) => {
      if (event.defaultPrevented || event.isComposing || event.keyCode === 229 || event.getModifierState('AltGraph')) return;
      const target = event.target;
      if (!(target instanceof Element)) return;

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
      }
    };

    const reserveDevToolsShortcut = (event: KeyboardEvent) => {
      if (event.defaultPrevented || event.isComposing || event.keyCode === 229 || event.getModifierState('AltGraph')) return;
      const target = event.target;
      if (!(target instanceof Element)) return;
      // Ctrl+Shift+I fica reservado contra o DevTools. A execução é feita
      // exclusivamente pelo mapa efetivo/contextual do Topbar; não há fallback
      // legado nem abertura direta do modal neste listener.
      if (
        event.ctrlKey &&
        event.shiftKey &&
        (event.code === 'KeyI' || event.key === 'i' || event.key === 'I') &&
        !event.altKey
      ) {
        event.preventDefault();
      }
    };

    // Preserve Ctrl+? in window capture. Keep the DevTools reservation on
    // document capture so editors cannot stop it from a child listener.
    window.addEventListener('keydown', handleShortcutsHelpKeyDown, true);
    document.addEventListener('keydown', reserveDevToolsShortcut, true);
    return () => {
      window.removeEventListener('keydown', handleShortcutsHelpKeyDown, true);
      document.removeEventListener('keydown', reserveDevToolsShortcut, true);
    };
  }, []);
}
