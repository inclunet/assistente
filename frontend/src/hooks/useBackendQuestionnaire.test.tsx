/** @vitest-environment jsdom */
import { describe, it, expect, beforeEach, vi } from 'vitest';
import { renderHook, act } from '@testing-library/react';

import { useBackendQuestionnaire } from './useBackendQuestionnaire';
import type { QuestionnairePayload } from '../components/ui/QuestionnaireDialog';

const handlers: Record<string, (data: unknown) => void> = {};
vi.mock('@wailsjs/runtime/runtime', () => ({
  EventsOn: (event: string, handler: (data: unknown) => void) => {
    handlers[event] = handler;
    return vi.fn();
  },
}));

const addToastSpy = vi.fn();
vi.mock('../store/uiStore', () => ({
  useUIStore: (selector: (s: { addToast: typeof addToastSpy }) => unknown) =>
    selector({ addToast: addToastSpy }),
}));

vi.mock('react-i18next', () => ({
  useTranslation: () => ({ t: (key: string) => key }),
}));

function abrir(id: string) {
  act(() => handlers['tool:questionnaire']?.({ id, title: 'Pergunta', questions: [] }));
}

function abrirDecisao(id: string) {
  act(() => handlers['tool:questionnaire']?.({
    id,
    kind: 'decision',
    title: 'Pergunta',
    questions: [],
    actions: [{ id: 'allow', label: 'Permitir' }],
  } as QuestionnairePayload));
}

function fechar(id: string, reason?: string) {
  act(() => handlers['tool:questionnaire:closed']?.({ id, reason }));
}

describe('useBackendQuestionnaire', () => {
  beforeEach(() => {
    addToastSpy.mockClear();
  });

  it('põe na tela a pergunta que o backend abriu', () => {
    const { result } = renderHook(() => useBackendQuestionnaire());
    abrir('pergunta-1');

    expect(result.current.data?.id).toBe('pergunta-1');
  });

  it('tira da tela a pergunta que perdeu o dono, dizendo por quê', () => {
    const { result } = renderHook(() => useBackendQuestionnaire());
    abrir('pergunta-1');
    fechar('pergunta-1', 'cancelled');

    expect(result.current.data).toBeNull();
    expect(addToastSpy).toHaveBeenCalledWith(
      'app.questionnaire.closedCancelled',
      'warning',
      8000,
    );
  });

  it('fechamento que chega antes do render ainda tira o diálogo da tela', () => {
    // Turno cancelado logo depois de abrir o diálogo: os dois eventos caem no
    // mesmo ciclo. Comparar com o estado do render anterior descartaria o
    // fechamento, e a pessoa ficaria diante de um pedido que já não existe.
    const { result } = renderHook(() => useBackendQuestionnaire());
    act(() => {
      handlers['tool:questionnaire']?.({ id: 'pergunta-1', title: 'P', questions: [] });
      handlers['tool:questionnaire:closed']?.({ id: 'pergunta-1', reason: 'cancelled' });
    });

    expect(result.current.data).toBeNull();
    expect(addToastSpy).toHaveBeenCalledTimes(1);
  });

  it('não derruba a pergunta seguinte por causa do fechamento da anterior', () => {
    const { result } = renderHook(() => useBackendQuestionnaire());
    abrir('pergunta-1');
    abrir('pergunta-2');
    fechar('pergunta-1', 'timeout');

    expect(result.current.data?.id).toBe('pergunta-2');
    expect(addToastSpy).not.toHaveBeenCalled();
  });

  it('publica o scope junto à decisão e não limpa o pedido novo por close/clear atrasados', () => {
    const { result } = renderHook(() => useBackendQuestionnaire());
    abrirDecisao('decision-old');
    const oldScope = result.current.scope;
    expect(oldScope?.dialogId).toBe('decision-old');

    abrirDecisao('decision-current');
    const currentScope = result.current.scope;
    expect(result.current.data?.id).toBe('decision-current');
    expect(currentScope?.dialogId).toBe('decision-current');
    fechar('decision-old', 'cancelled');
    let staleClearResult: boolean | undefined;
    act(() => { staleClearResult = result.current.clear('decision-old'); });

    expect(staleClearResult).toBe(false);
    expect(result.current.data?.id).toBe('decision-current');
    expect(result.current.scope).toBe(currentScope);
    expect(addToastSpy).not.toHaveBeenCalled();
  });

  it('mantém formulário sem scope e clear sem ID remove payload e scope (logout)', () => {
    const { result } = renderHook(() => useBackendQuestionnaire());
    abrir('form');
    expect(result.current.data?.id).toBe('form');
    expect(result.current.scope).toBeNull();

    abrirDecisao('decision');
    expect(result.current.scope?.dialogId).toBe('decision');
    act(() => result.current.clear());

    expect(result.current.data).toBeNull();
    expect(result.current.scope).toBeNull();
    fechar('decision', 'timeout');
    expect(addToastSpy).not.toHaveBeenCalled();
  });

  it('não avisa fechamento de pergunta já respondida', () => {
    const { result } = renderHook(() => useBackendQuestionnaire());
    abrir('pergunta-1');
    act(() => result.current.clear());
    fechar('pergunta-1', 'timeout');

    expect(result.current.data).toBeNull();
    expect(addToastSpy).not.toHaveBeenCalled();
  });

  it('avisa quem estava lendo quando o diálogo abriu', () => {
    const aoAbrir = vi.fn();
    renderHook(() => useBackendQuestionnaire(aoAbrir));
    abrir('pergunta-1');

    expect(aoAbrir).toHaveBeenCalledTimes(1);
  });
});
