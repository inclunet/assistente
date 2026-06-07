import { describe, expect, it, vi, beforeEach } from 'vitest';

/**
 * Cobertura da regressão da issue #205: o registro de "default focus area" é
 * uma PILHA. Quando um painel especializado (ex.: TaskListView) fica ativo e
 * empilha seu próprio foco, ao desativá-lo o slot deve VOLTAR para o
 * registrante anterior (WorkspaceLayout / Content Area inteligente) em vez de
 * virar nulo — garantindo que a troca de painel leve o foco ao input do chat.
 *
 * A pilha é estado de módulo. Para isolar cada caso, recarregamos o módulo em
 * um `beforeEach` com `vi.resetModules()` + `await import(...)` — mesmo padrão
 * de `frontend/src/services/audioFeedback.test.ts` — evitando que uma falha no
 * meio de um teste suje a pilha e cause falhas em cascata nos seguintes.
 */
let registerDefaultFocus: typeof import('./useDefaultFocus').registerDefaultFocus;
let unregisterDefaultFocus: typeof import('./useDefaultFocus').unregisterDefaultFocus;
let restoreDefaultFocus: typeof import('./useDefaultFocus').restoreDefaultFocus;

describe('useDefaultFocus (pilha)', () => {
  beforeEach(async () => {
    vi.resetModules();
    const mod = await import('./useDefaultFocus');
    registerDefaultFocus = mod.registerDefaultFocus;
    unregisterDefaultFocus = mod.unregisterDefaultFocus;
    restoreDefaultFocus = mod.restoreDefaultFocus;
  });

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
  });
});
