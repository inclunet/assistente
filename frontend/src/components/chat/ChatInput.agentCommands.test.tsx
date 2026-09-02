import { useState } from 'react';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { render, screen, fireEvent, waitFor } from '@testing-library/react';
import { ChatInput } from './ChatInput';

// De propósito sem mock do SlashCommandMenu: o defeito que interessa aqui mora
// justamente entre o menu e o campo — a seta contando itens de uma lista e o
// Enter escolhendo na outra.
const getSkillsForProfileSpy = vi.fn();
const announceSpy = vi.hoisted(() => vi.fn());

vi.mock('react-i18next', () => ({
  useTranslation: () => ({
    t: (key: string, fallbackOrOptions?: string | Record<string, unknown>) =>
      typeof fallbackOrOptions === 'string' ? fallbackOrOptions : key,
  }),
}));

vi.mock('../../hooks/useAnnouncer', () => ({
  useAnnouncer: () => ({ announce: announceSpy }),
}));

vi.mock('@wailsjs/go/wailsapi/Skills', () => ({
  GetUserInvocableSkillsForProfile: (profileSlug: string) => getSkillsForProfileSpy(profileSlug),
}));

vi.mock('../../services/mediaService', () => ({
  processMediaFiles: vi.fn(),
}));

vi.mock('./MediaPreview', () => ({
  MediaPreview: () => <div data-testid="media-preview" />,
}));

vi.mock('./VoiceButton', () => ({
  VoiceButton: () => <button data-testid="voice-button" />,
}));

const comandos = [
  { name: 'plan', description: 'Monta um plano', acceptsInput: true },
  { name: 'revisar', description: 'Revisa o diff', acceptsInput: false },
];

// CampoControlado é o campo como o painel de chat o usa: com o texto vivendo
// fora dele. Escolher um item escreve nesse texto, e o teste que guardasse o
// valor por conta própria não provaria que a escrita chegou lá.
function CampoControlado({
  agentCommands = comandos,
  onSend = () => {},
}: {
  agentCommands?: typeof comandos;
  onSend?: (message: string) => void;
}) {
  const [message, setMessage] = useState('');
  return (
    <ChatInput
      onSend={onSend}
      message={message}
      onMessageChange={setMessage}
      agentCommands={agentCommands}
    />
  );
}

async function digitaNoCampo(texto: string) {
  const textarea = await screen.findByLabelText('chat.messageLabel');
  fireEvent.change(textarea, { target: { value: texto } });
  return textarea;
}

describe('ChatInput com comandos do agente', () => {
  beforeEach(() => {
    getSkillsForProfileSpy.mockReset();
    announceSpy.mockReset();
    getSkillsForProfileSpy.mockResolvedValue([]);
  });

  it('mostra os comandos do agente mesmo quando o perfil não tem skill nenhuma', async () => {
    render(<CampoControlado />);
    await waitFor(() => expect(getSkillsForProfileSpy).toHaveBeenCalled());

    await digitaNoCampo('/');

    const opcoes = await screen.findAllByRole('option');
    expect(opcoes.map((opcao) => opcao.textContent)).toEqual([
      expect.stringContaining('/plan'),
      expect.stringContaining('/revisar'),
    ]);
  });

  it('separa os comandos do agente das skills do app, com rótulo de grupo', async () => {
    getSkillsForProfileSpy.mockResolvedValue([
      { slug: 'resumo', name: 'Resumo', description: 'Resume um texto' },
    ]);
    render(<CampoControlado />);
    await waitFor(() => expect(getSkillsForProfileSpy).toHaveBeenCalled());

    await digitaNoCampo('/');

    expect(await screen.findByRole('group', { name: 'chat.availableSkills' })).toBeInTheDocument();
    expect(screen.getByRole('group', { name: 'Comandos do agente' })).toBeInTheDocument();
  });

  it('digitar depois da barra filtra as duas origens juntas', async () => {
    getSkillsForProfileSpy.mockResolvedValue([
      { slug: 'planilha', name: 'Planilha', description: 'Abre uma planilha' },
      { slug: 'resumo', name: 'Resumo', description: 'Resume um texto' },
    ]);
    render(<CampoControlado />);
    await waitFor(() => expect(getSkillsForProfileSpy).toHaveBeenCalled());

    // Uma letra por vez: filtrar só no valor final esconderia o campo que perde
    // a digitação no meio do caminho.
    await digitaNoCampo('/');
    await digitaNoCampo('/p');
    await digitaNoCampo('/pl');
    await digitaNoCampo('/pla');

    const opcoes = await screen.findAllByRole('option');
    expect(opcoes.map((opcao) => opcao.textContent)).toEqual([
      expect.stringContaining('/planilha'),
      expect.stringContaining('/plan'),
    ]);
  });

  it('a seta atravessa da última skill para o primeiro comando do agente', async () => {
    getSkillsForProfileSpy.mockResolvedValue([
      { slug: 'resumo', name: 'Resumo', description: 'Resume um texto' },
    ]);
    render(<CampoControlado />);
    await waitFor(() => expect(getSkillsForProfileSpy).toHaveBeenCalled());

    const textarea = await digitaNoCampo('/');
    await screen.findAllByRole('option');

    fireEvent.keyDown(textarea, { key: 'ArrowDown' });

    const selecionado = screen.getAllByRole('option').find((opcao) => opcao.getAttribute('aria-selected') === 'true');
    expect(selecionado?.textContent).toContain('/plan');
  });

  it('expõe combobox e relaciona a opção ativa ao listbox com IDs estáveis', async () => {
    render(<CampoControlado />);
    await waitFor(() => expect(getSkillsForProfileSpy).toHaveBeenCalled());

    const textarea = await digitaNoCampo('/');
    const listbox = await screen.findByRole('listbox');
    const activeOption = screen.getAllByRole('option')[0];

    expect(textarea).toHaveAttribute('role', 'combobox');
    expect(textarea).toHaveAttribute('aria-expanded', 'true');
    expect(textarea).toHaveAttribute('aria-controls', listbox.id);
    expect(textarea).toHaveAttribute('aria-activedescendant', activeOption.id);
    expect(activeOption).toHaveAttribute('aria-selected', 'true');

    fireEvent.keyDown(textarea, { key: 'Escape' });

    expect(textarea).toHaveFocus();
    expect(textarea).toHaveAttribute('aria-expanded', 'false');
    expect(textarea).not.toHaveAttribute('aria-controls');
    expect(textarea).not.toHaveAttribute('aria-activedescendant');
    expect(announceSpy).toHaveBeenCalledWith('chat.slashMenuClosed', 'polite');
  });

  it('anuncia abertura, opção ativa e seleção pelo announcer global', async () => {
    render(<CampoControlado />);
    await waitFor(() => expect(getSkillsForProfileSpy).toHaveBeenCalled());

    const textarea = await digitaNoCampo('/');
    await screen.findAllByRole('option');
    expect(announceSpy).toHaveBeenCalledWith('chat.slashMenuOpened', 'polite');

    fireEvent.keyDown(textarea, { key: 'ArrowDown' });
    expect(announceSpy).toHaveBeenCalledWith('chat.slashActiveOption', 'polite');

    fireEvent.keyDown(textarea, { key: 'Enter' });
    expect(announceSpy).toHaveBeenCalledWith('chat.slashItemSelected', 'polite');
    await waitFor(() => expect(textarea).toHaveFocus());
  });

  it('não repete o anúncio de abertura ao filtrar o menu', async () => {
    render(<CampoControlado />);
    await waitFor(() => expect(getSkillsForProfileSpy).toHaveBeenCalled());

    await digitaNoCampo('/');
    await screen.findAllByRole('option');
    announceSpy.mockClear();

    await digitaNoCampo('/rev');

    expect(await screen.findByRole('option')).toHaveTextContent('/revisar');
    expect(announceSpy).not.toHaveBeenCalledWith('chat.slashMenuOpened', 'polite');
  });

  it('anuncia o fechamento quando o campo perde o foco', async () => {
    render(<CampoControlado />);
    await waitFor(() => expect(getSkillsForProfileSpy).toHaveBeenCalled());

    const textarea = await digitaNoCampo('/');
    await screen.findAllByRole('option');
    announceSpy.mockClear();

    fireEvent.blur(textarea);

    expect(announceSpy).toHaveBeenCalledWith('chat.slashMenuClosed', 'polite');
    expect(announceSpy).toHaveBeenCalledTimes(1);
  });

  it('escolher um comando do agente escreve a barra e o nome no campo', async () => {
    render(<CampoControlado />);
    await waitFor(() => expect(getSkillsForProfileSpy).toHaveBeenCalled());

    const textarea = await digitaNoCampo('/');
    await screen.findAllByRole('option');

    fireEvent.keyDown(textarea, { key: 'Enter' });

    // O comando aceita argumento, então o campo fica pronto para continuar
    // escrevendo — sem isso a pessoa teria de apagar o cursor para dentro.
    await waitFor(() => expect(textarea).toHaveValue('/plan '));
  });

  it('comando sem argumento não deixa espaço solto no fim da mensagem', async () => {
    render(<CampoControlado />);
    await waitFor(() => expect(getSkillsForProfileSpy).toHaveBeenCalled());

    const textarea = await digitaNoCampo('/rev');
    await screen.findAllByRole('option');

    fireEvent.keyDown(textarea, { key: 'Enter' });

    await waitFor(() => expect(textarea).toHaveValue('/revisar'));
  });

  it('preserva seleção por Tab e restaura o foco no campo', async () => {
    render(<CampoControlado />);
    await waitFor(() => expect(getSkillsForProfileSpy).toHaveBeenCalled());

    const textarea = await digitaNoCampo('/');
    await screen.findAllByRole('option');
    fireEvent.keyDown(textarea, { key: 'Tab' });

    await waitFor(() => {
      expect(textarea).toHaveValue('/plan ');
      expect(textarea).toHaveFocus();
    });
  });

  it('não intercepta Shift+Tab quando o menu está aberto', async () => {
    render(<CampoControlado />);
    await waitFor(() => expect(getSkillsForProfileSpy).toHaveBeenCalled());

    const textarea = await digitaNoCampo('/');
    await screen.findAllByRole('option');

    expect(fireEvent.keyDown(textarea, { key: 'Tab', shiftKey: true })).toBe(true);
    expect(textarea).toHaveValue('/');
  });

  it('envia texto iniciado por barra quando o filtro não tem opções', async () => {
    const onSend = vi.fn();
    render(<CampoControlado onSend={onSend} />);
    await waitFor(() => expect(getSkillsForProfileSpy).toHaveBeenCalled());

    const textarea = await digitaNoCampo('/comando-inexistente');
    expect(await screen.findByText('chat.noSlashItemsFound')).toBeInTheDocument();

    fireEvent.keyDown(textarea, { key: 'Enter' });

    expect(onSend).toHaveBeenCalledWith('/comando-inexistente', undefined);
  });

  it('sem skills e sem comandos o menu não aparece', async () => {
    render(<CampoControlado agentCommands={[]} />);
    await waitFor(() => expect(getSkillsForProfileSpy).toHaveBeenCalled());

    await digitaNoCampo('/');

    expect(screen.queryByRole('listbox')).not.toBeInTheDocument();
  });
});
