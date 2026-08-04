import { describe, expect, it, vi, beforeEach } from 'vitest';
import { act, render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { AgentOptionsPickers } from './AgentOptionsPickers';

const getOptions = vi.fn();
const setOption = vi.fn();
const announce = vi.fn();
let emitAgentOptions: ((data: unknown) => void) | null = null;

vi.mock('react-i18next', () => ({
  useTranslation: () => ({
    t: (key: string, options?: Record<string, unknown>) => (
      options && Object.keys(options).length > 0 ? `${key}|${JSON.stringify(options)}` : key
    ),
  }),
}));

vi.mock('@wailsjs/go/app/App', () => ({
  GetAgentSessionOptions: (id: string) => getOptions(id),
  SetAgentSessionOption: (id: string, optionId: string, value: string) => setOption(id, optionId, value),
}));

vi.mock('@wailsjs/runtime/runtime', () => ({
  EventsOn: (event: string, callback: (data: unknown) => void) => {
    if (event === 'chat:agent_options') emitAgentOptions = callback;
    return () => {
      emitAgentOptions = null;
    };
  },
}));

vi.mock('../../hooks/useAnnouncer', () => ({
  useAnnouncer: () => ({ announce, announceRequest: vi.fn() }),
}));

/** opcoesDoAgente é o que o backend devolve para uma conversa de agente. */
const opcoesDoAgente = (modelo = 'modelo-a', modo = 'agent') => ({
  conversationId: 'conversa-1',
  available: true,
  options: [
    {
      id: 'model',
      name: 'Modelo',
      category: 'model',
      currentValue: modelo,
      values: [
        { value: 'modelo-a', name: 'Modelo A' },
        { value: 'modelo-b', name: 'Modelo B' },
      ],
    },
    {
      id: 'mode',
      name: 'Modo',
      category: 'mode',
      currentValue: modo,
      values: [{ value: 'agent' }, { value: 'plan' }, { value: 'ask' }],
    },
  ],
});

beforeEach(() => {
  getOptions.mockReset();
  setOption.mockReset();
  announce.mockReset();
  emitAgentOptions = null;
});

describe('AgentOptionsPickers', () => {
  it('mostra o modelo e o modo em que o agente está', async () => {
    getOptions.mockResolvedValue(opcoesDoAgente());

    render(<AgentOptionsPickers conversationId="conversa-1" />);

    expect(await screen.findByText('Modelo A')).toBeInTheDocument();
    // O nome acessível é o que o leitor de telas fala, e é nele que o valor
    // corrente precisa aparecer — o rótulo visual é cortado pela largura.
    expect(screen.getByRole('button', { name: 'Modelo, Modelo A' })).toBeInTheDocument();
    // O modo não traz rótulo do agente: quem exibe traduz o valor.
    expect(
      screen.getByRole('button', { name: 'Modo, chat.agentOptions.mode.agent' }),
    ).toBeInTheDocument();
  });

  it('não desenha seletor para conversa sem agente', async () => {
    getOptions.mockResolvedValue({ conversationId: 'conversa-1', available: false, options: [] });

    const { container } = render(<AgentOptionsPickers conversationId="conversa-1" />);

    await waitFor(() => expect(getOptions).toHaveBeenCalled());
    expect(container.firstChild).toBeNull();
  });

  it('troca o modelo no agente e anuncia que vale do próximo turno', async () => {
    getOptions.mockResolvedValue(opcoesDoAgente());
    setOption.mockResolvedValue(opcoesDoAgente('modelo-b'));

    render(<AgentOptionsPickers conversationId="conversa-1" />);
    const botao = await screen.findByRole('button', { name: /Modelo/ });

    await userEvent.click(botao);
    await userEvent.click(await screen.findByRole('option', { name: 'Modelo B' }));

    await waitFor(() => expect(setOption).toHaveBeenCalledWith('conversa-1', 'model', 'modelo-b'));
    await waitFor(() => expect(screen.getByText('Modelo B')).toBeInTheDocument());
    expect(announce.mock.calls.some(([msg]) => String(msg).includes('chat.agentOptions.modelChanged'))).toBe(true);
  });

  it('troca o modo no agente', async () => {
    getOptions.mockResolvedValue(opcoesDoAgente());
    setOption.mockResolvedValue(opcoesDoAgente('modelo-a', 'plan'));

    render(<AgentOptionsPickers conversationId="conversa-1" />);
    const botao = await screen.findByRole('button', { name: /Modo/ });

    await userEvent.click(botao);
    await userEvent.click(await screen.findByRole('option', { name: 'chat.agentOptions.mode.plan' }));

    await waitFor(() => expect(setOption).toHaveBeenCalledWith('conversa-1', 'mode', 'plan'));
  });

  it('anuncia a troca que o próprio agente fez e mostra o modelo novo', async () => {
    getOptions.mockResolvedValue(opcoesDoAgente());

    render(<AgentOptionsPickers conversationId="conversa-1" />);
    await screen.findByText('Modelo A');

    act(() => {
      emitAgentOptions?.({
        conversationId: 'conversa-1',
        options: opcoesDoAgente('modelo-b').options,
        model: 'modelo-b',
        modelChanged: true,
        modeChanged: false,
        announce: true,
      });
    });

    await waitFor(() => expect(screen.getByText('Modelo B')).toBeInTheDocument());
    const anunciado = announce.mock.calls.map(([msg]) => String(msg)).join(' ');
    expect(anunciado).toContain('chat.agentOptions.modelChangedByAgent');
    expect(anunciado).toContain('modelo-b');
  });

  it('ignora o aviso de outra conversa', async () => {
    getOptions.mockResolvedValue(opcoesDoAgente());

    render(<AgentOptionsPickers conversationId="conversa-1" />);
    await screen.findByText('Modelo A');

    act(() => {
      emitAgentOptions?.({
        conversationId: 'conversa-2',
        options: opcoesDoAgente('modelo-b').options,
        model: 'modelo-b',
        modelChanged: true,
        modeChanged: false,
        announce: true,
      });
    });

    await waitFor(() => expect(screen.getByText('Modelo A')).toBeInTheDocument());
    expect(announce).not.toHaveBeenCalled();
  });

  it('não anuncia quando o agente só repete o estado', async () => {
    getOptions.mockResolvedValue(opcoesDoAgente());

    render(<AgentOptionsPickers conversationId="conversa-1" />);
    await screen.findByText('Modelo A');

    act(() => {
      emitAgentOptions?.({
        conversationId: 'conversa-1',
        options: opcoesDoAgente().options,
        model: 'modelo-a',
        modelChanged: false,
        modeChanged: false,
        announce: false,
      });
    });

    await waitFor(() => expect(screen.getByText('Modelo A')).toBeInTheDocument());
    expect(announce).not.toHaveBeenCalled();
  });

  it('avisa quando a troca não valeu, em vez de voltar o seletor calado', async () => {
    getOptions.mockResolvedValue(opcoesDoAgente());
    setOption.mockRejectedValue(new Error('o agente recusou'));

    render(<AgentOptionsPickers conversationId="conversa-1" />);
    const botao = await screen.findByRole('button', { name: /Modelo/ });

    await userEvent.click(botao);
    await userEvent.click(await screen.findByRole('option', { name: 'Modelo B' }));

    await waitFor(() => {
      expect(announce.mock.calls.some(([msg]) => String(msg).includes('chat.agentOptions.changeError'))).toBe(true);
    });
    // O modelo mostrado continua sendo o que o agente usa de verdade.
    expect(screen.getByText('Modelo A')).toBeInTheDocument();
  });
});
