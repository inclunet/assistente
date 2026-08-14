import { beforeEach, describe, expect, it, vi } from 'vitest';
import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { AgentWorkDirControl } from './AgentWorkDirControl';

const getWorkDir = vi.fn();
const setWorkDir = vi.fn();
const announce = vi.fn();

vi.mock('react-i18next', () => ({
  useTranslation: () => ({
    t: (key: string, options?: Record<string, unknown>) => (
      options && Object.keys(options).length > 0 ? `${key}|${JSON.stringify(options)}` : key
    ),
  }),
}));

vi.mock('@wailsjs/go/wailsapi/ACPWorkDir', () => ({
  GetAgentConversationWorkDir: (id: string) => getWorkDir(id),
  SetAgentConversationWorkDir: (id: string, dir: string) => setWorkDir(id, dir),
}));

vi.mock('../../hooks/useAnnouncer', () => ({
  useAnnouncer: () => ({ announce, announceRequest: vi.fn() }),
}));

/** estado é o que o backend devolve para uma conversa de agente de código. */
const estado = (over: Record<string, unknown> = {}) => ({
  conversationId: 'conversa-1',
  available: true,
  dir: '/casa/ana/projeto',
  workspaceDir: '/casa/ana/projeto',
  pinned: false,
  sessionDir: '',
  ...over,
});

const abrirDialogo = async (user: ReturnType<typeof userEvent.setup>) => {
  const botao = await screen.findByRole('button', { name: /chat\.agentWorkDir\.button/ });
  await user.click(botao);
  const campo = await screen.findByRole('textbox', { name: 'chat.agentWorkDir.fieldLabel' });
  // O modal aplica o foco inicial em dois tempos, e o segundo é uma conferência
  // 150ms depois da abertura. Digitar por cima dela mediria a corrida entre as
  // teclas e o foco — que foi o que quebrou este teste numa máquina mais lenta —
  // em vez de medir o campo.
  await new Promise((resolve) => setTimeout(resolve, 250));
  return campo;
};

beforeEach(() => {
  getWorkDir.mockReset();
  setWorkDir.mockReset();
  announce.mockReset();
  getWorkDir.mockResolvedValue(estado());
});

describe('AgentWorkDirControl', () => {
  // O alcance do agente precisa estar à vista, e o nome curto que cabe no botão
  // não distingue dois projetos com o mesmo nome de pasta: quem usa leitor de
  // telas ouve o caminho inteiro.
  it('mostra a pasta na barra e o caminho inteiro para o leitor de telas', async () => {
    render(<AgentWorkDirControl conversationId="conversa-1" />);

    const botao = await screen.findByRole('button', { name: /chat\.agentWorkDir\.buttonWorkspace/ });
    expect(botao).toHaveTextContent('projeto');
    expect(botao.getAttribute('aria-label')).toContain('/casa/ana/projeto');
  });

  // Conversa que não fala com agente de código não tem diretório nenhum, e um
  // botão com o caminho do workspace ali diria que um agente age sobre ele.
  // Este é o caso que a produção monta: o backend responde, e responde que não
  // há o que mostrar.
  it('não aparece quando a conversa não tem agente de código', async () => {
    getWorkDir.mockResolvedValue(estado({ available: false, dir: '', pinned: false }));
    render(<AgentWorkDirControl conversationId="conversa-1" />);

    await waitFor(() => expect(getWorkDir).toHaveBeenCalled());
    expect(screen.queryByRole('button', { name: /chat\.agentWorkDir/ })).toBeNull();
  });

  // Falha ao perguntar também esconde o controle: sem saber onde o agente age,
  // mostrar um caminho seria inventar o alcance dele.
  it('não aparece quando a pergunta ao backend falha', async () => {
    getWorkDir.mockRejectedValue(new Error('banco indisponível'));
    render(<AgentWorkDirControl conversationId="conversa-1" />);

    await waitFor(() => expect(getWorkDir).toHaveBeenCalled());
    expect(screen.queryByRole('button', { name: /chat\.agentWorkDir/ })).toBeNull();
  });

  // O caminho é digitado letra a letra, e não colado de uma vez: um campo que
  // perde o foco a cada tecla — ou que só aceita um caractere — passa por
  // qualquer teste que escreva o valor final direto no estado.
  it('deixa digitar o caminho inteiro e o entrega ao backend', async () => {
    const user = userEvent.setup();
    setWorkDir.mockResolvedValue(estado({ dir: '/casa/ana/outro', pinned: true }));
    render(<AgentWorkDirControl conversationId="conversa-1" />);

    const campo = await abrirDialogo(user);
    await user.clear(campo);
    await user.type(campo, '/casa/ana/outro');
    expect(campo).toHaveValue('/casa/ana/outro');
    expect(campo).toHaveFocus();

    await user.click(screen.getByRole('button', { name: 'chat.agentWorkDir.confirm' }));

    await waitFor(() => expect(setWorkDir).toHaveBeenCalledWith('conversa-1', '/casa/ana/outro'));
  });

  // O diálogo abre com o diretório atual escrito: trocar de diretório costuma
  // ser corrigir um caminho, e um campo vazio obrigaria a redigitar tudo.
  it('abre o campo com o diretório em que o agente está', async () => {
    const user = userEvent.setup();
    render(<AgentWorkDirControl conversationId="conversa-1" />);

    const campo = await abrirDialogo(user);
    expect(campo).toHaveValue('/casa/ana/projeto');
  });

  // O aviso de que o agente perde a memória fica no diálogo, escrito, antes da
  // confirmação: é a informação que decide se vale trocar agora ou terminar o
  // assunto primeiro (AEP-0084 D5).
  it('avisa que trocar o diretório faz o agente recomeçar sem memória', async () => {
    const user = userEvent.setup();
    render(<AgentWorkDirControl conversationId="conversa-1" />);

    await abrirDialogo(user);
    expect(screen.getByText('chat.agentWorkDir.warning')).toBeInTheDocument();
  });

  // O que é anunciado é o caminho que valeu, e não o que foi digitado: um "."
  // vira o caminho inteiro no backend, e é ele o alcance do agente.
  it('anuncia o caminho que valeu, não o que foi digitado', async () => {
    const user = userEvent.setup();
    setWorkDir.mockResolvedValue(estado({ dir: '/casa/ana/outro', pinned: true }));
    render(<AgentWorkDirControl conversationId="conversa-1" />);

    const campo = await abrirDialogo(user);
    await user.clear(campo);
    await user.type(campo, '.');
    await user.click(screen.getByRole('button', { name: 'chat.agentWorkDir.confirm' }));

    await waitFor(() => {
      const anuncio = announce.mock.calls.map(([msg]) => String(msg)).join('\n');
      expect(anuncio).toContain('chat.agentWorkDir.announceChanged');
      expect(anuncio).toContain('/casa/ana/outro');
    });
  });

  // Voltar ao workspace ativo é escolha legítima, e é o caminho vazio: sem esse
  // botão a conversa ficaria presa para sempre ao primeiro diretório escolhido.
  it('devolve a conversa ao workspace ativo', async () => {
    const user = userEvent.setup();
    getWorkDir.mockResolvedValue(estado({ dir: '/casa/ana/outro', pinned: true }));
    setWorkDir.mockResolvedValue(estado());
    render(<AgentWorkDirControl conversationId="conversa-1" />);

    await abrirDialogo(user);
    await user.click(screen.getByRole('button', { name: 'chat.agentWorkDir.useWorkspace' }));

    await waitFor(() => expect(setWorkDir).toHaveBeenCalledWith('conversa-1', ''));
    await waitFor(() => {
      const anuncio = announce.mock.calls.map(([msg]) => String(msg)).join('\n');
      expect(anuncio).toContain('chat.agentWorkDir.announceWorkspace');
    });
  });

  // O erro do backend explica o que houve — "o diretório X não existe" — e
  // precisa ficar na tela, junto do campo, além de anunciado: sumir com ele
  // deixaria a pessoa achando que a troca valeu.
  it('mostra e anuncia o erro de um caminho que não existe', async () => {
    const user = userEvent.setup();
    setWorkDir.mockRejectedValue(new Error('o diretório /nao/existe não existe'));
    render(<AgentWorkDirControl conversationId="conversa-1" />);

    const campo = await abrirDialogo(user);
    await user.clear(campo);
    await user.type(campo, '/nao/existe');
    await user.click(screen.getByRole('button', { name: 'chat.agentWorkDir.confirm' }));

    // O texto do erro descreve o campo, e é assim que o leitor de telas o lê ao
    // voltar para ele. Quem fala na hora é o anunciador global (AEP-0058).
    const erro = await screen.findByText('o diretório /nao/existe não existe');
    expect(campo).toHaveAttribute('aria-invalid', 'true');
    expect(campo.getAttribute('aria-describedby')).toBe(erro.id);
    const anuncio = announce.mock.calls.map(([msg]) => String(msg)).join('\n');
    expect(anuncio).toContain('chat.agentWorkDir.announceError');
    // O diálogo continua aberto: fechá-lo com o caminho recusado esconderia o
    // que precisa ser corrigido.
    expect(screen.getByRole('textbox', { name: 'chat.agentWorkDir.fieldLabel' })).toBeInTheDocument();
  });

  // Corrigir o caminho apaga o erro da tentativa anterior: mantê-lo acusaria um
  // problema que a pessoa já está resolvendo.
  it('some com o erro quando a pessoa corrige o caminho', async () => {
    const user = userEvent.setup();
    setWorkDir.mockRejectedValue(new Error('não existe'));
    render(<AgentWorkDirControl conversationId="conversa-1" />);

    const campo = await abrirDialogo(user);
    await user.clear(campo);
    await user.type(campo, '/nao/existe');
    await user.click(screen.getByRole('button', { name: 'chat.agentWorkDir.confirm' }));
    await screen.findByText('não existe');

    await user.type(campo, 'm');
    expect(screen.queryByText('não existe')).toBeNull();
  });
});
