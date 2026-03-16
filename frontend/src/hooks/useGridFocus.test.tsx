import { describe, expect, it } from 'vitest';
import { render, screen, act } from '@testing-library/react';
import { useGridFocus } from './useGridFocus';

function Fixture() {
  const { focusFirstCell, handleGridReady } = useGridFocus();
  return (
    <div>
      <button onClick={() => handleGridReady(() => {})}>ready</button>
      <span data-testid="focus">{focusFirstCell ? 'set' : 'unset'}</span>
    </div>
  );
}

describe('useGridFocus', () => {
  it('armazena funcao de foco', () => {
    render(<Fixture />);

    expect(screen.getByTestId('focus')).toHaveTextContent('unset');
    act(() => {
      screen.getByRole('button', { name: 'ready' }).click();
    });
    expect(screen.getByTestId('focus')).toHaveTextContent('set');
  });
});
