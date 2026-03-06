import { describe, it, expect, vi } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/react';
import { RangeSlider } from './RangeSlider';

describe('RangeSlider', () => {
  it('renderiza label corretamente', () => {
    const handleChange = vi.fn();
    render(
      <RangeSlider
        id="test-slider"
        label="Test Slider"
        value={50}
        min={0}
        max={100}
        step={1}
        onChange={handleChange}
      />
    );

    expect(screen.getByText('Test Slider')).toBeInTheDocument();
  });

  it('renderiza input range com valores corretos', () => {
    const handleChange = vi.fn();
    render(
      <RangeSlider
        id="test-slider"
        label="Test Slider"
        value={50}
        min={0}
        max={100}
        step={1}
        onChange={handleChange}
      />
    );

    const input = screen.getByRole('slider');
    expect(input).toHaveAttribute('min', '0');
    expect(input).toHaveAttribute('max', '100');
    expect(input).toHaveAttribute('step', '1');
    expect(input).toHaveAttribute('value', '50');
  });

  it('exibe o valor formatado', () => {
    const handleChange = vi.fn();
    render(
      <RangeSlider
        id="test-slider"
        label="Test Slider"
        value={0.7}
        min={0}
        max={1}
        step={0.1}
        onChange={handleChange}
        formatValue={(v) => v.toFixed(2)}
      />
    );

    expect(screen.getByTestId('slider-value')).toHaveTextContent('0.70');
  });

  it('chama onChange ao mover o slider', async () => {
    // Simular mudança diretamente no input (range sliders não suportam user.type)
    const handleChange = vi.fn();
    render(
      <RangeSlider
        id="test-slider"
        label="Test Slider"
        value={50}
        min={0}
        max={100}
        step={1}
        onChange={handleChange}
      />
    );

    const input = screen.getByRole('slider') as HTMLInputElement;
    // Simular mudança diretamente no input (range sliders não suportam user.type)
    fireEvent.change(input, { target: { value: '75' } });

    expect(handleChange).toHaveBeenCalled();
  });

  it('desabilita o slider quando disabled = true', () => {
    const handleChange = vi.fn();
    render(
      <RangeSlider
        id="test-slider"
        label="Test Slider"
        value={50}
        min={0}
        max={100}
        step={1}
        onChange={handleChange}
        disabled={true}
      />
    );

    const input = screen.getByRole('slider');
    expect(input).toBeDisabled();
  });

  it('usa formatValue customizado', () => {
    const handleChange = vi.fn();
    const customFormat = (v: number) => `${v}%`;

    render(
      <RangeSlider
        id="test-slider"
        label="Test Slider"
        value={75}
        min={0}
        max={100}
        step={1}
        onChange={handleChange}
        formatValue={customFormat}
      />
    );

    expect(screen.getByTestId('slider-value')).toHaveTextContent('75%');
  });

  it('possui aria-label com o label fornecido', () => {
    const handleChange = vi.fn();
    render(
      <RangeSlider
        id="test-slider"
        label="Temperature"
        value={0.7}
        min={0}
        max={2}
        step={0.1}
        onChange={handleChange}
      />
    );

    const input = screen.getByRole('slider');
    expect(input).toHaveAttribute('aria-label', 'Temperature');
  });

  it('possui aria-valuemin, aria-valuemax, aria-valuenow', () => {
    const handleChange = vi.fn();
    render(
      <RangeSlider
        id="test-slider"
        label="Test Slider"
        value={50}
        min={0}
        max={100}
        step={1}
        onChange={handleChange}
      />
    );

    const input = screen.getByRole('slider');
    expect(input).toHaveAttribute('aria-valuemin', '0');
    expect(input).toHaveAttribute('aria-valuemax', '100');
    expect(input).toHaveAttribute('aria-valuenow', '50');
  });

  it('atualiza aria-valuetext com formatValue', () => {
    const handleChange = vi.fn();
    render(
      <RangeSlider
        id="test-slider"
        label="Test Slider"
        value={0.7}
        min={0}
        max={1}
        step={0.1}
        onChange={handleChange}
        formatValue={(v) => v.toFixed(2)}
      />
    );

    const input = screen.getByRole('slider');
    expect(input).toHaveAttribute('aria-valuetext', '0.70');
  });
});
