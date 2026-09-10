import { describe, expect, it, vi } from 'vitest';
import { render, screen, act } from '@testing-library/react';
import { useGridFocus } from './useGridFocus';

const focusRequest = vi.fn(() => true);

function Fixture() {
  const { handleGridReady, requestGridFocus } = useGridFocus();
  return (
    <div>
      <button onClick={() => handleGridReady(focusRequest)}>ready</button>
      <button onClick={() => requestGridFocus({ itemId: 'b', rowIndex: 1, colIndex: 2 })}>restore</button>
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

  it('encaminha restauração por ID, índice e coluna', () => {
    focusRequest.mockClear();
    render(<Fixture />);
    act(() => {
      screen.getByRole('button', { name: 'ready' }).click();
      screen.getByRole('button', { name: 'restore' }).click();
    });
    expect(focusRequest).toHaveBeenCalledWith({ itemId: 'b', rowIndex: 1, colIndex: 2 });
  });
});
