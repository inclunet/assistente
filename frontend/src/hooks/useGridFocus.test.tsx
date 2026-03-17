import { describe, expect, it } from 'vitest';
import { render, screen, act } from '@testing-library/react';
import { useGridFocus } from './useGridFocus';

function Fixture() {
  const { handleGridReady } = useGridFocus();
  return (
    <div>
      <button onClick={() => handleGridReady(() => {})}>ready</button>
      <span data-testid="status">ok</span>
    </div>
  );
}

describe('useGridFocus', () => {
  it('aceita handleGridReady sem erro', () => {
    render(<Fixture />);

    act(() => {
      screen.getByRole('button', { name: 'ready' }).click();
    });
    expect(screen.getByTestId('status')).toHaveTextContent('ok');
  });
});
