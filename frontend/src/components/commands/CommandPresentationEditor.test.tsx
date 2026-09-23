import { fireEvent, render, screen } from '@testing-library/react';
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
  it('oferece seletor de ícone e três campos nomeados atualizáveis por teclado', async () => {
    const onChange = vi.fn();
    const user = userEvent.setup();
    function Harness() {
      const [value, setValue] = useState<Record<string, unknown>>({ version: 1 });
      return <CommandPresentationEditor value={value} onChange={next => { setValue(next); onChange(next); }} />;
    }
    const { container } = render(
      <Harness />
    );

    const icon = screen.getByRole('combobox', { name: 'commandSettings.presentation.icon.label' });
    const portuguese = screen.getByRole('textbox', { name: 'commandSettings.presentation.locales.ptBR' });
    await user.tab();
    expect(icon).toHaveFocus();
    await user.selectOptions(icon, 'folder');
    await user.tab();
    expect(icon).toHaveValue('folder');
    expect(onChange).toHaveBeenLastCalledWith({ version: 1, icon: 'folder' });
    expect(portuguese).toHaveFocus();
    await user.keyboard('  Configurações  ');

    expect(portuguese).toHaveFocus();
    expect(onChange).toHaveBeenCalledWith({
      version: 1,
      icon: 'folder',
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

  it('preserva um ícone legado desconhecido até o usuário substituí-lo ou removê-lo', () => {
    const onChange = vi.fn();
    const { rerender } = render(
      <CommandPresentationEditor
        value={{ version: 1, icon: 'legacy-deck-token', status_label_keys: { active: 'status.active' } }}
        onChange={onChange}
      />
    );

    const icon = screen.getByRole('combobox', { name: 'commandSettings.presentation.icon.label' });
    expect(icon).toHaveValue('legacy-deck-token');
    expect(screen.getByRole('option', { name: 'commandSettings.presentation.icon.unavailable' })).toHaveValue('legacy-deck-token');
    fireEvent.change(icon, { target: { value: 'folder' } });
    expect(onChange).toHaveBeenLastCalledWith({ version: 1, icon: 'folder', status_label_keys: { active: 'status.active' } });
    rerender(
      <CommandPresentationEditor
        value={{ version: 1, icon: 'folder', status_label_keys: { active: 'status.active' } }}
        onChange={onChange}
      />
    );
    fireEvent.change(screen.getByRole('combobox', { name: 'commandSettings.presentation.icon.label' }), { target: { value: '' } });
    expect(onChange).toHaveBeenLastCalledWith({ version: 1, status_label_keys: { active: 'status.active' } });
  });
});
