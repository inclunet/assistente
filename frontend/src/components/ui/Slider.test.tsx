import { describe, expect, it, vi } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/react';
import { Slider } from './Slider';

describe('Slider', () => {
  it('renderiza label e dispara onChange', () => {
    const onChange = vi.fn();

    render(<Slider value={5} onChange={onChange} label="Volume" showValue />);

    const input = screen.getByLabelText('Volume');
    fireEvent.change(input, { target: { value: '10' } });

    expect(onChange).toHaveBeenCalledWith(10);
  });
});
