import { describe, expect, it, vi } from 'vitest';
import {
  registerDefaultFocus,
  unregisterDefaultFocus,
  restoreDefaultFocus,
} from './useDefaultFocus';

/**
 * Cobertura da regressão da issue #205: o registro de "default focus area" é
 * uma PILHA. Quando um painel especializado (ex.: TaskListView) fica ativo e
 * empilha seu próprio foco, ao desativá-lo o slot deve VOLTAR para o
 * registrante anterior (WorkspaceLayout / Content Area inteligente) em vez de
 * virar nulo — garantindo que a troca de painel leve o foco ao input do chat.
 *
 * Cada teste desregistra o que registra, mantendo a pilha (estado de módulo)
 * vazia entre os casos.
 */
describe('useDefaultFocus (pilha)', () => {
  it('restoreDefaultFocus retorna false com a pilha vazia', () => {
    expect(restoreDefaultFocus()).toBe(false);
  });

  it('usa o registrante mais recente (topo da pilha)', () => {
    const workspaceDefault = vi.fn(() => true);
    const tasklistDefault = vi.fn(() => true);

    registerDefaultFocus(workspaceDefault);
    registerDefaultFocus(tasklistDefault);

    restoreDefaultFocus();

    expect(tasklistDefault).toHaveBeenCalledTimes(1);
    expect(workspaceDefault).not.toHaveBeenCalled();

    unregisterDefaultFocus(tasklistDefault);
    unregisterDefaultFocus(workspaceDefault);
  });

  it('#205: ao desativar o painel da tasklist, o default volta para o WorkspaceLayout', () => {
    const workspaceDefault = vi.fn(() => true); // foco inteligente da Content Area (input do chat)
    const tasklistDefault = vi.fn(() => true); // foco próprio da tasklist (kanban/grid)

    // 1. WorkspaceLayout registra o default ao montar.
    registerDefaultFocus(workspaceDefault);
    // 2. Uma aba de tasklist fica ativa e empilha seu próprio foco.
    registerDefaultFocus(tasklistDefault);

    // 3. Usuário navega para o chat (Ctrl+Tab): a tasklist é desativada e
    //    desregistra seu foco.
    unregisterDefaultFocus(tasklistDefault);

    // 4. A troca de aba dispara restoreDefaultFocus(): deve cair no foco
    //    inteligente do WorkspaceLayout (input do chat), NÃO em null.
    const restored = restoreDefaultFocus();

    expect(restored).toBe(true);
    expect(workspaceDefault).toHaveBeenCalledTimes(1);

    unregisterDefaultFocus(workspaceDefault);
  });

  it('re-registrar a mesma função não cria entradas duplicadas', () => {
    const a = vi.fn(() => true);
    const b = vi.fn(() => true);

    registerDefaultFocus(a);
    registerDefaultFocus(b);
    // Re-registra "a": deve ir para o topo (sem duplicar).
    registerDefaultFocus(a);

    restoreDefaultFocus();
    expect(a).toHaveBeenCalledTimes(1);
    expect(b).not.toHaveBeenCalled();

    // Desregistrar "a" uma única vez basta (sem duplicatas) → cai em "b".
    unregisterDefaultFocus(a);
    a.mockClear();
    restoreDefaultFocus();
    expect(b).toHaveBeenCalledTimes(1);
    expect(a).not.toHaveBeenCalled();

    unregisterDefaultFocus(b);
  });
});
