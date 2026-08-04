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

/**
 * anuncioDe acha um anúncio pela chave entre os que o seletor faz — ele também
 * anuncia a navegação pela lista, e procurar no texto todo confundiria o rótulo
 * do item percorrido com o do valor que valeu.
 */
const anuncioDe = (key: string): string => {
  const call = announce.mock.calls.find(([msg]) => String(msg).includes(key));
  expect(call, `nada foi anunciado com ${key}`).toBeTruthy();
  return String(call?.[0]);
};

/** anuncioDaTroca é o anúncio da troca de modelo pedida pela pessoa. */
const anuncioDaTroca = (): string => anuncioDe('chat.agentOptions.modelChanged|');

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
    // O que a pessoa ouve é o rótulo que está escrito na lista, e não o
    // identificador do modelo: falado, ele é ilegível.
    const anunciado = anuncioDaTroca();
    expect(anunciado).toContain('Modelo B');
    expect(anunciado).not.toContain('modelo-b');
  });

  it('anuncia o modelo que o agente aplicou quando ele acomoda o pedido em outro', async () => {
    getOptions.mockResolvedValue(opcoesDoAgente());
    // O agente aceita a troca, mas em outro valor — fallback de limite de uso,
    // por exemplo. Quem decidiu foi ele, e é o valor dele que a pessoa precisa
    // ouvir: anunciar o pedido seria dizer que ela está num modelo em que não
    // está.
    setOption.mockResolvedValue(opcoesDoAgente('modelo-a'));

    render(<AgentOptionsPickers conversationId="conversa-1" />);
    await userEvent.click(await screen.findByRole('button', { name: /Modelo/ }));
    await userEvent.click(await screen.findByRole('option', { name: 'Modelo B' }));

    await waitFor(() => expect(setOption).toHaveBeenCalled());
    const anunciado = await waitFor(anuncioDaTroca);
    expect(anunciado).toContain('Modelo A');
    expect(anunciado).not.toContain('Modelo B');
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
    // Esta é a troca que a pessoa não viu acontecer, e é por isso que ela mais
    // depende do anúncio. Falada, tem de sair pelo rótulo da lista: o
    // identificador do protocolo é ilegível.
    const anunciado = anuncioDe('chat.agentOptions.modelChangedByAgent');
    expect(anunciado).toContain('Modelo B');
    expect(anunciado).not.toContain('modelo-b');
  });

  it('anuncia o modo que o agente trocou pelo rótulo que o agente deu', async () => {
    const comRotuloDeModo = () => {
      const estado = opcoesDoAgente('modelo-a', 'plan');
      // O agente pode mandar rótulo próprio para o modo. Quando manda, é ele que
      // aparece na lista — e é ele que a pessoa precisa ouvir, e não a tradução
      // que este app usa quando o agente não diz nada.
      estado.options[1].values = [
        { value: 'agent', name: 'Agente autônomo' },
        { value: 'plan', name: 'Planejamento' },
      ];
      return estado;
    };
    getOptions.mockResolvedValue(comRotuloDeModo());

    render(<AgentOptionsPickers conversationId="conversa-1" />);
    await screen.findByText('Modelo A');

    act(() => {
      emitAgentOptions?.({
        conversationId: 'conversa-1',
        options: comRotuloDeModo().options,
        mode: 'plan',
        modelChanged: false,
        modeChanged: true,
        announce: true,
      });
    });

    const anunciado = await waitFor(() => anuncioDe('chat.agentOptions.modeChangedByAgent'));
    expect(anunciado).toContain('Planejamento');
    expect(anunciado).not.toContain('chat.agentOptions.mode.plan');
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

  it('anuncia o modo trocado pelo agente com o rótulo traduzido', async () => {
    getOptions.mockResolvedValue(opcoesDoAgente());

    render(<AgentOptionsPickers conversationId="conversa-1" />);
    await screen.findByText('Modelo A');

    act(() => {
      emitAgentOptions?.({
        conversationId: 'conversa-1',
        options: opcoesDoAgente('modelo-a', 'plan').options,
        mode: 'plan',
        modelChanged: false,
        modeChanged: true,
        announce: true,
      });
    });

    const anunciado = await waitFor(() => {
      const texto = announce.mock.calls.map(([msg]) => String(msg)).join(' ');
      expect(texto).toContain('chat.agentOptions.modeChangedByAgent');
      return texto;
    });
    // O valor do protocolo é `plan`, em inglês. Falado cru, o leitor de telas
    // leria inglês no meio do português.
    expect(anunciado).toContain('chat.agentOptions.mode.plan');
    expect(anunciado).not.toContain('"mode":"plan"');
  });

  // Um agente que responde a troca com opções sem valores para escolher chega
  // aqui como lista vazia — o backend descarta o que não desenha seletor. Lista
  // vazia é truthy em JavaScript, e tratada como sucesso ela anunciaria a troca
  // enquanto os controles somem da barra: estado que ninguém consegue explicar
  // depois.
  it('resposta sem opções não anuncia troca nem faz os seletores sumirem', async () => {
    getOptions.mockResolvedValue(opcoesDoAgente());
    setOption.mockResolvedValue({ conversationId: 'conversa-1', available: false, options: [] });

    render(<AgentOptionsPickers conversationId="conversa-1" />);
    await userEvent.click(await screen.findByRole('button', { name: /Modelo/ }));
    await userEvent.click(await screen.findByRole('option', { name: 'Modelo B' }));

    await waitFor(() => expect(setOption).toHaveBeenCalled());
    await waitFor(() => {
      const anunciado = announce.mock.calls.map(([msg]) => String(msg)).join(' ');
      expect(anunciado).toContain('chat.agentOptions.changeUnknownState');
    });
    const anunciado = announce.mock.calls.map(([msg]) => String(msg)).join(' ');
    expect(anunciado).not.toContain('chat.agentOptions.modelChanged|');
    // Os seletores continuam mostrando o que a sessão conhecia.
    expect(screen.getByRole('button', { name: 'Modelo, Modelo A' })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: /Modo/ })).toBeInTheDocument();
  });

  it('não leva o resultado da troca para a conversa que entrou no lugar', async () => {
    getOptions.mockImplementation((id: string) => Promise.resolve(
      id === 'conversa-1' ? opcoesDoAgente('modelo-a') : opcoesDoAgente('modelo-b'),
    ));
    let concluirTroca: ((valor: unknown) => void) | null = null;
    setOption.mockImplementation(() => new Promise((resolve) => { concluirTroca = resolve; }));

    const { rerender } = render(<AgentOptionsPickers conversationId="conversa-1" />);
    await userEvent.click(await screen.findByRole('button', { name: /Modelo/ }));
    await userEvent.click(await screen.findByRole('option', { name: 'Modelo B' }));
    await waitFor(() => expect(setOption).toHaveBeenCalled());

    // A pessoa muda de conversa enquanto o agente ainda responde à troca.
    rerender(<AgentOptionsPickers conversationId="conversa-2" />);
    await waitFor(() => expect(screen.getByText('Modelo B')).toBeInTheDocument());

    // A resposta é da conversa que saiu da tela: escrevê-la aqui poria o estado
    // de uma conversa no seletor de outra.
    await act(async () => {
      concluirTroca?.(opcoesDoAgente('modelo-a'));
    });

    expect(screen.getByText('Modelo B')).toBeInTheDocument();
    // E o seletor da conversa nova não pode ficar travado esperando uma troca
    // que nunca foi dela.
    expect(screen.getByRole('button', { name: /Modelo/ })).not.toBeDisabled();
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
