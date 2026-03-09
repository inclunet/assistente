import './RangeSlider.css';

export interface RangeSliderProps {
  /** ID único do input */
  id: string;
  /** Label exibido antes do slider */
  label: string;
  /** Valor atual */
  value: number;
  /** Valor mínimo */
  min: number;
  /** Valor máximo */
  max: number;
  /** Incremento (step) */
  step: number;
  /** Callback quando o valor muda */
  onChange: (value: number) => void;
  /** Função customizada para formatar o valor exibido */
  formatValue?: (value: number) => string;
  /** Se o slider está desabilitado */
  disabled?: boolean;
  /** Classe CSS customizada */
  className?: string;
}

/**
 * Componente reutilizável para range slider.
 * Usado em ProfileChatSection (temperature, top_p),
 * ProfileVoiceSection (rate, volume), etc.
 *
 * @example
 * ```tsx
 * <RangeSlider
 *   id="temperature"
 *   label="Temperature"
 *   value={0.7}
 *   min={0}
 *   max={2}
 *   step={0.1}
 *   onChange={(value) => setTemperature(value)}
 *   formatValue={(v) => v.toFixed(2)}
 * />
 * ```
 */
export function RangeSlider({
  id,
  label,
  value,
  min,
  max,
  step,
  onChange,
  formatValue = (v) => v.toString(),
  disabled = false,
  className,
}: RangeSliderProps) {
  return (
    <div className={`range-slider ${className || ''}`} data-testid="range-slider">
      <label htmlFor={id} className="range-slider__label">
        {label}
      </label>
      <div className="range-slider__container">
        <input
          id={id}
          type="range"
          min={min}
          max={max}
          step={step}
          value={value}
          onChange={(e) => onChange(parseFloat(e.target.value))}
          disabled={disabled}
          className="range-slider__input"
          aria-label={label}
          aria-valuemin={min}
          aria-valuemax={max}
          aria-valuenow={value}
          aria-valuetext={formatValue(value)}
        />
        <span className="range-slider__value" data-testid="slider-value">
          {formatValue(value)}
        </span>
      </div>
    </div>
  );
}
