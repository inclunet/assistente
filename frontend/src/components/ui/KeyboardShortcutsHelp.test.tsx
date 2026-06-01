import { describe, expect, it, vi } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/react';
import { KeyboardShortcutsHelp } from './KeyboardShortcutsHelp';
import { isModalOpen } from './Modal';
import {
  expectDisplayedShortcutsAreCanonical,
  getDisplayedShortcutCombos,
} from '../../test/a11yHelpers';

vi.mock('react-i18next', () => ({
  useTranslation: () => ({
    t: (key: string) => key,
  }),
}));

/*
 * Lista canônica de combinações de atalho que o app DE FATO trata. É a fonte
 * única de verdade: quando um atalho é adicionado/renomeado no código, esta
 * lista deve ser atualizada. O teste abaixo afirma que toda combinação
 * exibida no painel pertence a esta lista — pegando regressões em que o
 * painel passa a mostrar um atalho sem handler real correspondente.
 *
 * NOTA (Issue #37): o painel deixou de exibir `Ctrl+M` (sem handler real) e
 * `Ctrl+I` (perfis usam `Ctrl+P` no ChatToolbar). A navegação entre abas usa
 * `Ctrl+PageDown / Ctrl+PageUp`; `Ctrl+P` aparece apenas para "Perfis de
 * interação". Mantenha esta lista em sincronia com `KeyboardShortcutsHelp`.
 */
const CANONICAL_SHORTCUT_COMBOS = [
  // Navegação
  'Ctrl+T',                       // nova aba de chat
  'Ctrl+N',                       // menu de criação de aba
  'Ctrl+W',                       // fechar aba
  'Ctrl+Tab',                     // próxima aba
  'Ctrl+Shift+Tab',               // aba anterior
  'Ctrl+1…9',                     // ir para aba N
  'Ctrl+PageDown / Ctrl+PageUp',  // navegação entre abas
  // Chat
  'Ctrl+Enter',                   // enviar mensagem
  'Ctrl+L',                       // limpar conversa
  'Ctrl+H',                       // histórico
  'Ctrl+P',                       // perfis de interação (ChatToolbar)
  'Space',                        // falar mensagem
  'Enter',                        // detalhes da mensagem
  'Shift+F10',                    // menu de contexto
  '↑',                            // mensagem anterior
  '↓',                            // próxima mensagem
  // Geral
  'Ctrl+?',                       // ajuda
  'F1',                           // página de ajuda
  'Alt+M',                        // abrir menu
  'Esc',                          // fechar diálogos
];

describe('KeyboardShortcutsHelp', () => {
  it('renderiza quando aberto e fecha no Escape', () => {
    const onClose = vi.fn();

    render(<KeyboardShortcutsHelp isOpen={true} onClose={onClose} />);

    expect(screen.getByRole('dialog')).toBeInTheDocument();
    fireEvent.keyDown(document, { key: 'Escape' });

    expect(onClose).toHaveBeenCalled();
  });

  it('nao renderiza quando fechado', () => {
    render(<KeyboardShortcutsHelp isOpen={false} onClose={() => {}} />);

    expect(screen.queryByRole('dialog')).toBeNull();
  });

  it('agrupa os atalhos por categorias', () => {
    render(<KeyboardShortcutsHelp isOpen={true} onClose={() => {}} />);

    expect(screen.getByText('ui.shortcuts.categories.navigation')).toBeInTheDocument();
    expect(screen.getByText('ui.shortcuts.categories.chat')).toBeInTheDocument();
    expect(screen.getByText('ui.shortcuts.categories.general')).toBeInTheDocument();
    expect(screen.getByText('Ctrl+?')).toBeInTheDocument();
  });

  it('exibe a tecla real (Ctrl+P) para perfis e usa a chave i18n renomeada; sem atalhos fantasmas', () => {
    render(<KeyboardShortcutsHelp isOpen={true} onClose={() => {}} />);

    // "Perfis de interação" reflete o handler real (ChatToolbar: Ctrl+P), não Ctrl+I.
    expect(screen.getByText('Ctrl+P')).toBeInTheDocument();
    expect(screen.queryByText('Ctrl+I')).toBeNull();
    expect(screen.getByText('ui.shortcuts.interactionProfiles')).toBeInTheDocument();

    // Chave i18n renomeada para refletir o comportamento de Ctrl+N (abre o menu de criação de aba).
    expect(screen.getByText('ui.shortcuts.openNewTabMenu')).toBeInTheDocument();
    expect(screen.queryByText('ui.shortcuts.newConversation')).toBeNull();

    // Ctrl+M não possui handler real — item removido do painel.
    expect(screen.queryByText('Ctrl+M')).toBeNull();
    expect(screen.queryByText('ui.shortcuts.selectModel')).toBeNull();
  });

  it('mostra a navegação de abas real (PageDown/PageUp), não como Ctrl+P', () => {
    render(<KeyboardShortcutsHelp isOpen={true} onClose={() => {}} />);

    // A navegação entre abas usa Ctrl+PageDown/PageUp; Ctrl+P é o seletor de
    // perfil (exibido na categoria de chat), não navegação.
    expect(screen.getByText('Ctrl+PageDown / Ctrl+PageUp')).toBeInTheDocument();
    expect(screen.getByText('ui.shortcuts.navigateTabs')).toBeInTheDocument();
  });

  it('registra-se no stack de modal compartilhado enquanto aberto (isModalOpen)', () => {
    expect(isModalOpen()).toBe(false);

    const { rerender } = render(<KeyboardShortcutsHelp isOpen={true} onClose={() => {}} />);
    expect(isModalOpen()).toBe(true);

    rerender(<KeyboardShortcutsHelp isOpen={false} onClose={() => {}} />);
    expect(isModalOpen()).toBe(false);
  });

  it('toda combinação exibida corresponde a um atalho canônico', () => {
    const { container } = render(<KeyboardShortcutsHelp isOpen={true} onClose={() => {}} />);

    const displayed = getDisplayedShortcutCombos(container);
    expect(displayed.length).toBeGreaterThan(0);

    expectDisplayedShortcutsAreCanonical({
      displayed,
      canonical: CANONICAL_SHORTCUT_COMBOS,
    });
  });

  it('o helper detecta um atalho exibido sem handler canônico (regressão)', () => {
    // Simula o painel passando a exibir um atalho que não existe no código.
    const displayedComRegressao = [...CANONICAL_SHORTCUT_COMBOS, 'Ctrl+Shift+Z'];

    expect(() =>
      expectDisplayedShortcutsAreCanonical({
        displayed: displayedComRegressao,
        canonical: CANONICAL_SHORTCUT_COMBOS,
      }),
    ).toThrow();
  });
});
