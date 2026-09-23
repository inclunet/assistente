import { render, screen } from '@testing-library/react';
import { useState } from 'react';
import userEvent from '@testing-library/user-event';
import { describe, expect, it, vi } from 'vitest';
import { axe } from '../../test/a11yAxe';
import {
  CommandPresentationEditor,
  isCommandPresentationValid,
  normalizeCommandPresentation,
} from './CommandPresentationEditor';

describe('CommandPresentationEditor', () => {
  it('oferece três campos nomeados e atualizáveis por teclado', async () => {
    const onChange = vi.fn();
    const user = userEvent.setup();
    function Harness() {
      const [value, setValue] = useState<Record<string, unknown>>({ version: 1 });
      return <CommandPresentationEditor value={value} onChange={next => { setValue(next); onChange(next); }} />;
    }
    const { container } = render(
      <Harness />
    );

    const portuguese = screen.getByRole('textbox', { name: 'commandSettings.presentation.locales.ptBR' });
    await user.tab();
    expect(portuguese).toHaveFocus();
    await user.keyboard('  Configurações  ');

    expect(portuguese).toHaveFocus();
    expect(onChange).toHaveBeenCalledWith({
      version: 1,
      title_by_locale: { 'pt-BR': '  Configurações  ' },
    });
    await user.tab();
    expect(screen.getByRole('textbox', { name: 'commandSettings.presentation.locales.en' })).toHaveFocus();
    await user.keyboard('Settings');
    await user.tab();
    expect(screen.getByRole('textbox', { name: 'commandSettings.presentation.locales.es' })).toHaveFocus();
    await user.tab({ shift: true });
    expect(screen.getByRole('textbox', { name: 'commandSettings.presentation.locales.en' })).toHaveFocus();
    expect(await axe(container)).toHaveNoViolations();
  });

  it('bloqueia NUL e títulos acima de 256 codepoints', () => {
    const { rerender } = render(
      <CommandPresentationEditor
        value={{ version: 1, title_by_locale: { en: 'ok' } }}
        onChange={vi.fn()}
      />
    );

    rerender(
      <CommandPresentationEditor
        value={{ version: 1, title_by_locale: { en: 'bad\0title' } }}
        onChange={vi.fn()}
      />
    );
    expect(isCommandPresentationValid({ title_by_locale: { en: 'bad\0title' } })).toBe(false);
    expect(screen.getByText('commandSettings.presentation.invalidNul')).toBeInTheDocument();

    rerender(
      <CommandPresentationEditor
        value={{ version: 1, title_by_locale: { en: '😀'.repeat(257) } }}
        onChange={vi.fn()}
      />
    );
    expect(isCommandPresentationValid({ title_by_locale: { en: '😀'.repeat(257) } })).toBe(false);
    expect(screen.getByText('commandSettings.presentation.invalidLength')).toBeInTheDocument();
    expect(screen.getByRole('textbox', { name: 'commandSettings.presentation.locales.en' })).toHaveAttribute('aria-invalid', 'true');
    expect(isCommandPresentationValid({ title_by_locale: { en: ' 😀'.trim().repeat(256) } })).toBe(true);
  });

  it('remove locale vazio, recorta títulos e preserva outros metadados', () => {
    const presentation = {
      version: 1,
      icon: 'settings',
      title_by_locale: { 'pt-BR': '  Configurações  ', en: '' },
    };
    expect(normalizeCommandPresentation(presentation)).toEqual({
      version: 1,
      icon: 'settings',
      title_by_locale: { 'pt-BR': 'Configurações' },
    });
  });
});
