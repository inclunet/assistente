import { describe, expect, it, vi } from 'vitest';
import { render, screen, act } from '@testing-library/react';
import { useDebouncedValue } from './useDebouncedValue';

function Fixture({ value, delay }: { value: string; delay: number }) {
  const debounced = useDebouncedValue(value, delay);
  return <div data-testid="value">{debounced}</div>;
}

describe('useDebouncedValue', () => {
  it('atualiza valor apos delay', () => {
    vi.useFakeTimers();

    const { rerender } = render(<Fixture value="a" delay={100} />);
    expect(screen.getByTestId('value')).toHaveTextContent('a');

    rerender(<Fixture value="b" delay={100} />);
    expect(screen.getByTestId('value')).toHaveTextContent('a');

    act(() => {
      vi.advanceTimersByTime(100);
    });
    expect(screen.getByTestId('value')).toHaveTextContent('b');

    vi.useRealTimers();
  });
});
