import { afterEach, describe, expect, it, vi } from 'vitest';

const restoreDefaultFocus = vi.fn(() => true);
vi.mock('../../hooks/useDefaultFocus', () => ({
  restoreDefaultFocus: () => restoreDefaultFocus(),
}));

import {
  pruneWorkspacePanelFocus,
  queueWorkspacePanelFocus,
  registerWorkspacePanelFocus,
  routeWorkspacePanelFocus,
} from './workspacePanelFocusRegistry';

function flushRaf(): Promise<void> {
  return new Promise((resolve) => requestAnimationFrame(() => resolve()));
}

describe('workspacePanelFocusRegistry', () => {
  afterEach(() => {
    restoreDefaultFocus.mockClear();
  });

  it('descarta pedidos pendentes de abas removidas', () => {
    const handler = vi.fn(() => true);

    queueWorkspacePanelFocus('removed-tab');
    pruneWorkspacePanelFocus(new Set(['active-tab']));
    const unregister = registerWorkspacePanelFocus('removed-tab', handler);

    expect(handler).not.toHaveBeenCalled();
    unregister();
  });

  describe('routeWorkspacePanelFocus', () => {
    it('invoca o handler do painel quando já registrado', () => {
      const handler = vi.fn(() => true);
      const unregister = registerWorkspacePanelFocus('tab-editor', handler);

      routeWorkspacePanelFocus('tab-editor', 'editor');

      expect(handler).toHaveBeenCalledTimes(1);
      expect(restoreDefaultFocus).not.toHaveBeenCalled();
      unregister();
    });

    it('enfileira o pedido para painel assíncrono (tasklist) ainda não montado', async () => {
      const handler = vi.fn(() => true);

      routeWorkspacePanelFocus('tab-tasklist', 'tasklist');
      expect(handler).not.toHaveBeenCalled();
      expect(restoreDefaultFocus).not.toHaveBeenCalled();

      // Ao montar e registrar, o pedido pendente é refeito no próximo frame.
      const unregister = registerWorkspacePanelFocus('tab-tasklist', handler);
      await flushRaf();

      expect(handler).toHaveBeenCalledTimes(1);
      unregister();
    });

    it('também enfileira o pedido para o editor ainda não montado', async () => {
      const handler = vi.fn(() => true);

      routeWorkspacePanelFocus('tab-editor-lazy', 'editor');
      const unregister = registerWorkspacePanelFocus('tab-editor-lazy', handler);
      await flushRaf();

      expect(handler).toHaveBeenCalledTimes(1);
      unregister();
    });

    it('cai no default focus para tipos síncronos (chat/terminal) sem handler', () => {
      routeWorkspacePanelFocus('tab-chat', 'chat');

      expect(restoreDefaultFocus).toHaveBeenCalledTimes(1);
    });

    it('cai no default focus quando o tipo é desconhecido', () => {
      routeWorkspacePanelFocus('tab-unknown', undefined);

      expect(restoreDefaultFocus).toHaveBeenCalledTimes(1);
    });
  });
});
