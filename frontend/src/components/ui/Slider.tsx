import { useId, forwardRef } from 'react';
import './Slider.css';

export interface SliderProps {
  /** Valor atual */
  value: number;
  /** Callback quando valor muda */
  onChange: (value: number) => void;
  /** Valor mínimo */
  min?: number;
  /** Valor máximo */
  max?: number;
  /** Passo */
  step?: number;
  /** Label acessível */
  label?: string;
  /** Desabilitado */
  disabled?: boolean;
  /** Classe CSS adicional */
  className?: string;
  /** Mostra o valor atual */
  showValue?: boolean;
  /** Formato do valor */
  formatValue?: (value: number) => string;
}

export const Slider = forwardRef<HTMLInputElement, SliderProps>(
  (
    {
      value,
      onChange,
      min = 0,
      max = 100,
      step = 1,
      label,
      disabled = false,
      className = '',
      showValue = false,
      formatValue = (v) => v.toString(),
    },
    ref
  ) => {
    const id = useId();

    const handleChange = (e: React.ChangeEvent<HTMLInputElement>) => {
      onChange(parseFloat(e.target.value));
    };

    // Calcula a porcentagem para o gradiente
    const percentage = ((value - min) / (max - min)) * 100;

    return (
      <div className={`slider ${className}`}>
        {label && (
          <label htmlFor={id} className="slider__label">
            {label}
          </label>
        )}
        <div className="slider__container">
          <input
            ref={ref}
            id={id}
            type="range"
            className="slider__input"
            value={value}
            onChange={handleChange}
            min={min}
            max={max}
            step={step}
            disabled={disabled}
            aria-label={label}
            style={{
              '--slider-percentage': `${percentage}%`,
            } as React.CSSProperties}
          />
          {showValue && (
            <span className="slider__value" aria-hidden="true">
              {formatValue(value)}
            </span>
          )}
        </div>
      </div>
    );
  }
);

Slider.displayName = 'Slider';

export default Slider;
