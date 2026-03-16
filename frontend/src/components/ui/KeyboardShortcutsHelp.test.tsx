import { describe, expect, it, vi } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/react';
import { KeyboardShortcutsHelp } from './KeyboardShortcutsHelp';

vi.mock('react-i18next', () => ({
  useTranslation: () => ({
    t: (key: string) => key,
  }),
}));

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
});
