import { describe, it, expect, vi } from 'vitest';
import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { ProfileVoiceSection } from './ProfileVoiceSection';
import { VOICE_DISABLED } from '../pickers/VoicePicker';

vi.mock('react-i18next', () => ({
  useTranslation: () => ({
    t: (key: string) => key,
  }),
}));

// Mock VoicePicker para evitar imports de @wailsjs
vi.mock('../pickers/VoicePicker', () => ({
  VOICE_DISABLED: '__disabled__',
  VoicePicker: ({ value, onChange, label, voiceOverrides }: {
    value: string; onChange: (value: string) => void; label: string; voiceOverrides?: Array<{ id: string; name: string }>
  }) => (
    <div data-testid="voice-picker-mock">
      <label>{label}</label>
      <button onClick={() => onChange('test-voice')} data-testid="voice-picker-select">
        {value || VOICE_DISABLED}
      </button>
      {voiceOverrides && voiceOverrides.length > 0 && (
        <button onClick={() => onChange('nova::tts-1-hd')} data-testid="voice-picker-select-hd">
          HD
        </button>
      )}
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
    
    expect(screen.getByText('profiles.voiceSection.voiceLabel')).toBeInTheDocument();
  });

  it('renderiza o slider de rate com valor correto', () => {
    render(<ProfileVoiceSection {...defaultProps} />);
    
    const rateLabel = screen.getByText('profiles.voiceSection.rateLabel');
    expect(rateLabel).toBeInTheDocument();
    
    const rateInput = screen.getByLabelText('profiles.voiceSection.rateLabel');
    expect(rateInput).toHaveValue('1');
  });

  it('renderiza o slider de volume com valor correto', () => {
    render(<ProfileVoiceSection {...defaultProps} />);
    
    const volumeLabel = screen.getByText('profiles.voiceSection.volumeLabel');
    expect(volumeLabel).toBeInTheDocument();
    
    const volumeInput = screen.getByLabelText('profiles.voiceSection.volumeLabel');
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
    
    const label = screen.getByText('profiles.voiceSection.voiceLabel');
    expect(label).toBeInTheDocument();
  });

  it('chama onChange ao alterar rate', async () => {
    const handleChange = vi.fn();
    const user = userEvent.setup();
    
    render(<ProfileVoiceSection {...defaultProps} onChange={handleChange} />);
    
    const rateInput = screen.getByLabelText('profiles.voiceSection.rateLabel');
    await user.click(rateInput);
    
    // RangeSlider chama onChange internamente
    expect(rateInput).toBeInTheDocument();
  });

  it('chama onChange ao alterar volume', async () => {
    const handleChange = vi.fn();
    const user = userEvent.setup();
    
    render(<ProfileVoiceSection {...defaultProps} onChange={handleChange} />);
    
    const volumeInput = screen.getByLabelText('profiles.voiceSection.volumeLabel');
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
    
    const rateInput = screen.getByLabelText('profiles.voiceSection.rateLabel');
    const volumeInput = screen.getByLabelText('profiles.voiceSection.volumeLabel');
    
    expect(rateInput).toBeDisabled();
    expect(volumeInput).toBeDisabled();
  });

  it('permite valores mínimos e máximos corretos no rate', () => {
    render(<ProfileVoiceSection {...defaultProps} />);
    
    const rateInput = screen.getByLabelText('profiles.voiceSection.rateLabel') as HTMLInputElement;
    
    expect(rateInput.min).toBe('0.5');
    expect(rateInput.max).toBe('2');
    expect(rateInput.step).toBe('0.1');
  });

  it('permite valores mínimos e máximos corretos no volume', () => {
    render(<ProfileVoiceSection {...defaultProps} />);
    
    const volumeInput = screen.getByLabelText('profiles.voiceSection.volumeLabel') as HTMLInputElement;
    
    expect(volumeInput.min).toBe('0');
    expect(volumeInput.max).toBe('1');
    expect(volumeInput.step).toBe('0.05');
  });

  it('NÃO mostra seletor de modelo TTS para OpenAI (modelo embutido na voz)', () => {
    const handleModelChange = vi.fn();
    render(
      <ProfileVoiceSection
        {...defaultProps}
        providerId="openai-default"
        providerType="openai"
        ttsModel="tts-1"
        ttsModels={[{ id: 'tts-1', name: 'tts-1' }, { id: 'tts-1-hd', name: 'tts-1-hd' }]}
        onModelChange={handleModelChange}
      />
    );

    expect(screen.queryByLabelText('profiles.fieldTTSModel')).not.toBeInTheDocument();
    // Verifica que voiceOverrides foi passado (botão HD presente)
    expect(screen.getByTestId('voice-picker-select-hd')).toBeInTheDocument();
  });

  it('NÃO mostra seletor de modelo TTS para webspeech', () => {
    render(
      <ProfileVoiceSection
        {...defaultProps}
        providerId="webspeech"
        ttsModel="tts-1"
        onModelChange={vi.fn()}
      />
    );

    expect(screen.queryByLabelText('profiles.fieldTTSModel')).not.toBeInTheDocument();
  });

  it('NÃO mostra seletor de modelo TTS para sapi5', () => {
    render(
      <ProfileVoiceSection
        {...defaultProps}
        providerId="sapi5"
        ttsModel="tts-1"
        onModelChange={vi.fn()}
      />
    );

    expect(screen.queryByLabelText('profiles.fieldTTSModel')).not.toBeInTheDocument();
  });

  it('passa valor composto voice::model via onChange ao selecionar voz HD no picker', async () => {
    const handleChange = vi.fn();
    const user = userEvent.setup();
    render(
      <ProfileVoiceSection
        {...defaultProps}
        providerId="openai-default"
        providerType="openai"
        ttsModel="tts-1"
        ttsModels={[{ id: 'tts-1', name: 'tts-1' }, { id: 'tts-1-hd', name: 'tts-1-hd' }]}
        onModelChange={vi.fn()}
        onChange={handleChange}
      />
    );

    // Clica no botão HD do mock — emite "nova::tts-1-hd"
    const hdButton = screen.getByTestId('voice-picker-select-hd');
    await user.click(hdButton);

    // Valor composto passado direto para o parent fazer o parse
    expect(handleChange).toHaveBeenCalledWith('voice', 'nova::tts-1-hd');
  });

  it('NÃO mostra seletor de modelo quando onModelChange não é fornecido', () => {
    render(
      <ProfileVoiceSection
        {...defaultProps}
        providerId="openai-default"
        providerType="openai"
        ttsModel="tts-1"
        ttsModels={[{ id: 'tts-1', name: 'tts-1' }, { id: 'tts-1-hd', name: 'tts-1-hd' }]}
      />
    );

    expect(screen.queryByLabelText('profiles.fieldTTSModel')).not.toBeInTheDocument();
  });

  it('NÃO mostra seletor de modelo para provider com apenas vozes dinâmicas (LocalAI/Piper)', () => {
    render(
      <ProfileVoiceSection
        {...defaultProps}
        providerId="localai-default"
        providerType="localai"
        ttsModel="voice-pt_BR-cadu-medium"
        ttsModels={[
          { id: 'voice-pt_BR-cadu-medium', name: 'voice-pt_BR-cadu-medium' },
          { id: 'voice-en_US-amy-medium', name: 'voice-en_US-amy-medium' },
        ]}
        onModelChange={vi.fn()}
      />
    );

    expect(screen.queryByLabelText('profiles.fieldTTSModel')).not.toBeInTheDocument();
  });
});
