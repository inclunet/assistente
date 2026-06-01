import { describe, expect, it, vi } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/react';
import { KeyboardShortcutsHelp } from './KeyboardShortcutsHelp';
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
 */
const CANONICAL_SHORTCUT_COMBOS = [
  'Ctrl+N',     // nova aba / criar
  'Ctrl+L',     // limpar conversa
  'Ctrl+P',     // ação no contexto da aba (seletor de perfil)
  'Ctrl+H',     // histórico
  'Ctrl+M',     // modelos
  'Ctrl+I',     // perfis de interação
  'Space',      // falar mensagem
  'Enter',      // detalhes da mensagem
  'Shift+F10',  // menu de contexto
  '↑',          // mensagem anterior
  '↓',          // próxima mensagem
  'Ctrl+Enter', // enviar mensagem
  '?',          // ajuda
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
