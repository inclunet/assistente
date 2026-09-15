import { describe, expect, it, vi } from 'vitest';
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
  it('descarta pedidos pendentes de abas removidas', () => {
    const handler = vi.fn(() => true);

    queueWorkspacePanelFocus('removed-tab');
    pruneWorkspacePanelFocus(new Set(['active-tab']));
    const unregister = registerWorkspacePanelFocus('removed-tab', handler);

    expect(handler).not.toHaveBeenCalled();
    unregister();
  });

  describe('routeWorkspacePanelFocus (agnóstico de tipo)', () => {
    it('invoca o handler do painel quando já registrado', () => {
      const handler = vi.fn(() => true);
      const unregister = registerWorkspacePanelFocus('tab-a', handler);

      routeWorkspacePanelFocus('tab-a');

      expect(handler).toHaveBeenCalledTimes(1);
      unregister();
    });

    it('enfileira o pedido quando o painel ainda não montou, refazendo ao registrar', async () => {
      const handler = vi.fn(() => true);

      // Sem handler ainda: o pedido é enfileirado, não perdido.
      routeWorkspacePanelFocus('tab-lazy');
      expect(handler).not.toHaveBeenCalled();

      const unregister = registerWorkspacePanelFocus('tab-lazy', handler);
      await flushRaf();

      expect(handler).toHaveBeenCalledTimes(1);
      unregister();
    });

    it('enfileira igualmente para qualquer tipo de aba (sem ramo por tipo)', async () => {
      const handler = vi.fn(() => true);

      // Antes, chat/terminal caíam em restoreDefaultFocus; agora todo painel
      // registra handler e o roteamento é único: enfileira até montar.
      routeWorkspacePanelFocus('tab-chat');
      const unregister = registerWorkspacePanelFocus('tab-chat', handler);
      await flushRaf();

      expect(handler).toHaveBeenCalledTimes(1);
      unregister();
    });
  });
});
