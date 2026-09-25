import { describe, expect, it, vi } from 'vitest';
import { act, render, screen, fireEvent } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { useToolbarKeyboardNav } from './useToolbarKeyboardNav';

function Fixture({ onFocusContent }: { onFocusContent?: () => void }) {
  const ref = useToolbarKeyboardNav(onFocusContent);
  return (
    <div ref={ref}>
      <button>Primeiro</button>
      <button>Segundo</button>
    </div>
  );
}

describe('useToolbarKeyboardNav', () => {
  it('move foco com setas', () => {
    render(<Fixture />);

    const first = screen.getByRole('button', { name: 'Primeiro' });
    const second = screen.getByRole('button', { name: 'Segundo' });

    first.focus();
    fireEvent.keyDown(first, { key: 'ArrowRight' });

    expect(second).toHaveFocus();
  });

  it('inclui checkbox no roving tabindex, navega com setas/Home/End e não o marca ao focar', async () => {
    const user = userEvent.setup();

    function CheckboxFixture() {
      const ref = useToolbarKeyboardNav();
      return (
        <div ref={ref} role="toolbar" aria-label="Ações">
          <button>Antes</button>
          <input type="checkbox" aria-label="Consentimento" />
          <button disabled>Indisponível</button>
          <button>Depois</button>
        </div>
      );
    }

    const { container } = render(<CheckboxFixture />);
    const first = screen.getByRole('button', { name: 'Antes' });
    const checkbox = screen.getByRole('checkbox', { name: 'Consentimento' });
    const last = screen.getByRole('button', { name: 'Depois' });

    expect(container.querySelectorAll('[tabindex="0"]')).toHaveLength(1);
    first.focus();
    fireEvent.keyDown(first, { key: 'ArrowRight' });
    expect(checkbox).toHaveFocus();
    expect(checkbox).not.toBeChecked();
    expect(checkbox).toHaveAttribute('tabindex', '0');
    expect(container.querySelectorAll('[tabindex="0"]')).toHaveLength(1);

    await user.keyboard(' ');
    expect(checkbox).toBeChecked();

    fireEvent.keyDown(checkbox, { key: 'ArrowRight' });
    expect(last).toHaveFocus();
    fireEvent.keyDown(last, { key: 'Home' });
    expect(first).toHaveFocus();
    fireEvent.keyDown(first, { key: 'End' });
    expect(last).toHaveFocus();
    expect(screen.getByRole('button', { name: 'Indisponível' })).not.toHaveAttribute('tabindex', '0');
  });

  it('não intercepta setas em campo de texto ou picker combobox', () => {
    function InputFixture() {
      const ref = useToolbarKeyboardNav();
      return (
        <div ref={ref} role="toolbar" aria-label="Ações">
          <button>Antes</button>
          <input aria-label="Busca" />
          <input role="combobox" aria-label="Picker" />
          <button>Depois</button>
        </div>
      );
    }

    render(<InputFixture />);
    const text = screen.getByRole('textbox', { name: 'Busca' });
    act(() => text.focus());
    const textEvent = fireEvent.keyDown(text, { key: 'ArrowRight' });
    expect(textEvent).toBe(true);
    expect(text).toHaveFocus();

    const picker = screen.getByRole('combobox', { name: 'Picker' });
    act(() => picker.focus());
    const pickerEvent = fireEvent.keyDown(picker, { key: 'ArrowRight' });
    expect(pickerEvent).toBe(true);
    expect(picker).toHaveFocus();
  });

  it('chama onFocusContent no Enter do campo de busca', () => {
    const onFocusContent = vi.fn();

    function SearchFixture() {
      const ref = useToolbarKeyboardNav(onFocusContent);
      return (
        <div ref={ref}>
          <input className="toolbar__search" />
          <button>Botao</button>
        </div>
      );
    }

    render(<SearchFixture />);

    const input = screen.getByRole('textbox');
    input.focus();
    fireEvent.keyDown(input, { key: 'Enter' });

    expect(onFocusContent).toHaveBeenCalled();
  });
});
