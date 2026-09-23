import { describe, expect, it, vi } from 'vitest';
import {
  pruneWorkspacePanelFocus,
  cancelWorkspacePanelFocus,
  queueWorkspacePanelFocus,
  registerWorkspacePanelFocus,
  routeWorkspacePanelFocus,
  canFocusWorkspacePanelImmediately,
  getWorkspacePanelImmediateFocusHandler,
} from './workspacePanelFocusRegistry';

function flushRaf(): Promise<void> {
  return new Promise((resolve) => requestAnimationFrame(() => resolve()));
}

describe('workspacePanelFocusRegistry', () => {
  it('registro legado não anuncia capacidade imediata', () => {
    const deferred = vi.fn(() => true);
    const unregister = registerWorkspacePanelFocus('legacy-only', deferred);
    expect(canFocusWorkspacePanelImmediately('legacy-only')).toBe(false);
    expect(getWorkspacePanelImmediateFocusHandler('legacy-only')).toBeUndefined();
    expect(deferred).not.toHaveBeenCalled();
    unregister();
  });

  it('consulta prontidão sem executar nem agendar foco', () => {
    const deferred = vi.fn(() => true);
    const immediate = vi.fn(() => true);
    const ready = vi.fn(() => false);
    const unregister = registerWorkspacePanelFocus('readiness', deferred, immediate, ready);
    expect(canFocusWorkspacePanelImmediately('readiness')).toBe(false);
    ready.mockReturnValue(true);
    expect(canFocusWorkspacePanelImmediately('readiness')).toBe(true);
    expect(deferred).not.toHaveBeenCalled();
    expect(immediate).not.toHaveBeenCalled();
    getWorkspacePanelImmediateFocusHandler('readiness')?.();
    expect(immediate).toHaveBeenCalledTimes(1);
    expect(deferred).not.toHaveBeenCalled();
    unregister();
  });

  it('registro substituto possui nova identidade e sobrevive ao cleanup anterior', () => {
    const deferred = vi.fn(() => true);
    const immediate = vi.fn(() => true);
    const unregisterOld = registerWorkspacePanelFocus('replacement', deferred, immediate);
    const first = getWorkspacePanelImmediateFocusHandler('replacement');
    const unregisterNew = registerWorkspacePanelFocus('replacement', deferred, immediate);
    const second = getWorkspacePanelImmediateFocusHandler('replacement');
    expect(first).not.toBe(second);
    unregisterOld();
    expect(getWorkspacePanelImmediateFocusHandler('replacement')).toBe(second);
    unregisterNew();
    expect(canFocusWorkspacePanelImmediately('replacement')).toBe(false);
  });

  it('descarta pedidos pendentes de abas removidas', () => {
    const handler = vi.fn(() => true);

    queueWorkspacePanelFocus('removed-tab');
    pruneWorkspacePanelFocus(new Set(['active-tab']));
    const unregister = registerWorkspacePanelFocus('removed-tab', handler);

    expect(handler).not.toHaveBeenCalled();
    unregister();
  });

  it('substitui pedido por identidade e não entrega o RAF antigo', async () => {
    const handler = vi.fn(() => true);
    const cancelledOld = vi.fn();
    const appliedNew = vi.fn();
    queueWorkspacePanelFocus('replacement-request', () => true, undefined, undefined, cancelledOld);
    queueWorkspacePanelFocus('replacement-request', () => true, appliedNew);
    const unregister = registerWorkspacePanelFocus('replacement-request', handler);
    await flushRaf();
    expect(handler).toHaveBeenCalledTimes(1);
    expect(cancelledOld).toHaveBeenCalledOnce();
    expect(appliedNew).toHaveBeenCalledOnce();
    unregister();
  });

  it('cancela pedido já agendado sem executar handler nem callback antigo', async () => {
    const handler = vi.fn(() => true);
    const cancelled = vi.fn();
    const unregister = registerWorkspacePanelFocus('cancel-request', handler);
    queueWorkspacePanelFocus('cancel-request', () => true, undefined, undefined, cancelled);
    // A requisição foi agendada, mas ainda não cruzou o frame de entrega.
    const current = getWorkspacePanelImmediateFocusHandler('cancel-request');
    expect(current).toBeUndefined();
    // O handler regular continua registrado; cancelar remove somente o pedido.
    // (A capability imediata ausente também confirma que não houve bypass.)
    cancelWorkspacePanelFocus('cancel-request');
    await flushRaf();
    expect(handler).not.toHaveBeenCalled();
    expect(cancelled).toHaveBeenCalledOnce();
    unregister();
  });

  it('não usa handler deferred quando a entrega exige capability imediata ausente', async () => {
    const regular = vi.fn(() => true);
    const rejected = vi.fn();
    const unregister = registerWorkspacePanelFocus('immediate-required', regular);
    queueWorkspacePanelFocus('immediate-required', () => true, undefined, rejected, undefined, true);
    await flushRaf();
    expect(regular).not.toHaveBeenCalled();
    expect(rejected).toHaveBeenCalledOnce();
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
