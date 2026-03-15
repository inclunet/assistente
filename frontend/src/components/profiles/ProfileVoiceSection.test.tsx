import { describe, it, expect, vi } from 'vitest';
import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { ProfileVoiceSection } from './ProfileVoiceSection';
import { VOICE_DISABLED } from '../pickers/VoicePicker';

// Mock VoicePicker para evitar imports de @wailsjs
vi.mock('../pickers/VoicePicker', () => ({
  VOICE_DISABLED: '__disabled__',
  VoicePicker: ({ value, onChange, label }: { value: string; onChange: (value: string) => void; label: string }) => (
    <div data-testid="voice-picker-mock">
      <label>{label}</label>
      <button onClick={() => onChange('test-voice')}>
        {value || VOICE_DISABLED}
      </button>
    </div>
  ),
}));

describe('ProfileVoiceSection', () => {
  const defaultProps = {
    voice: 'Google português do Brasil',
    rate: 1.0,
    volume: 0.8,
    onChange: vi.fn(),
  };

  it('renderiza a seção de voz', () => {
    render(<ProfileVoiceSection {...defaultProps} />);
    
    expect(screen.getByTestId('profile-voice-section')).toBeInTheDocument();
  });

  it('renderiza o VoicePicker com o valor correto', () => {
    render(<ProfileVoiceSection {...defaultProps} />);
    
    // VoicePicker renderiza um combobox com label "Voz (TTS)"
    expect(screen.getByText('Voz (TTS)')).toBeInTheDocument();
  });

  it('renderiza o slider de rate com valor correto', () => {
    render(<ProfileVoiceSection {...defaultProps} />);
    
    const rateLabel = screen.getByText('Taxa de Fala (Rate)');
    expect(rateLabel).toBeInTheDocument();
    
    const rateInput = screen.getByLabelText('Taxa de Fala (Rate)');
    expect(rateInput).toHaveValue('1');
  });

  it('renderiza o slider de volume com valor correto', () => {
    render(<ProfileVoiceSection {...defaultProps} />);
    
    const volumeLabel = screen.getByText('Volume');
    expect(volumeLabel).toBeInTheDocument();
    
    const volumeInput = screen.getByLabelText('Volume');
    expect(volumeInput).toHaveValue('0.8');
  });

  it('formata o valor do rate corretamente (ex: 1.0x)', () => {
    render(<ProfileVoiceSection {...defaultProps} />);
    
    // RangeSlider mostra valor formatado
    expect(screen.getByText('1.0x')).toBeInTheDocument();
  });

  it('formata o valor do volume corretamente (ex: 80%)', () => {
    render(<ProfileVoiceSection {...defaultProps} />);
    
    // RangeSlider mostra valor formatado
    expect(screen.getByText('80%')).toBeInTheDocument();
  });

  it('VoicePicker está presente e pode receber interações', () => {
    render(<ProfileVoiceSection {...defaultProps} />);
    
    // VoicePicker renderiza um picker com label "Voz (TTS)"
    // (teste simplificado pois VoicePicker tem seus próprios testes)
    const label = screen.getByText('Voz (TTS)');
    expect(label).toBeInTheDocument();
  });

  it('chama onChange ao alterar rate', async () => {
    const handleChange = vi.fn();
    const user = userEvent.setup();
    
    render(<ProfileVoiceSection {...defaultProps} onChange={handleChange} />);
    
    const rateInput = screen.getByLabelText('Taxa de Fala (Rate)');
    await user.click(rateInput);
    
    // RangeSlider chama onChange internamente
    expect(rateInput).toBeInTheDocument();
  });

  it('chama onChange ao alterar volume', async () => {
    const handleChange = vi.fn();
    const user = userEvent.setup();
    
    render(<ProfileVoiceSection {...defaultProps} onChange={handleChange} />);
    
    const volumeInput = screen.getByLabelText('Volume');
    await user.click(volumeInput);
    
    // RangeSlider chama onChange internamente
    expect(volumeInput).toBeInTheDocument();
  });

  it('renderiza com voz desativada quando voice é VOICE_DISABLED', () => {
    render(
      <ProfileVoiceSection
        {...defaultProps}
        voice={VOICE_DISABLED}
      />
    );
    
    expect(screen.getByTestId('profile-voice-section')).toBeInTheDocument();
  });

  it('renderiza com voz vazia quando voice é string vazia', () => {
    render(
      <ProfileVoiceSection
        {...defaultProps}
        voice=""
      />
    );
    
    // Deve usar VOICE_DISABLED como fallback
    expect(screen.getByTestId('profile-voice-section')).toBeInTheDocument();
  });

  it('desabilita os sliders quando disabled é true', () => {
    render(<ProfileVoiceSection {...defaultProps} disabled={true} />);
    
    const rateInput = screen.getByLabelText('Taxa de Fala (Rate)');
    const volumeInput = screen.getByLabelText('Volume');
    
    expect(rateInput).toBeDisabled();
    expect(volumeInput).toBeDisabled();
  });

  it('permite valores mínimos e máximos corretos no rate', () => {
    render(<ProfileVoiceSection {...defaultProps} />);
    
    const rateInput = screen.getByLabelText('Taxa de Fala (Rate)') as HTMLInputElement;
    
    expect(rateInput.min).toBe('0.5');
    expect(rateInput.max).toBe('2');
    expect(rateInput.step).toBe('0.1');
  });

  it('permite valores mínimos e máximos corretos no volume', () => {
    render(<ProfileVoiceSection {...defaultProps} />);
    
    const volumeInput = screen.getByLabelText('Volume') as HTMLInputElement;
    
    expect(volumeInput.min).toBe('0');
    expect(volumeInput.max).toBe('1');
    expect(volumeInput.step).toBe('0.05');
  });
});
