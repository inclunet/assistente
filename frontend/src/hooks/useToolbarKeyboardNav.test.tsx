import { describe, expect, it, vi } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/react';
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
