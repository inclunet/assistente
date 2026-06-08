import { describe, it, expect, vi } from 'vitest';
import { useState } from 'react';
import { render, screen, fireEvent, waitFor } from '@testing-library/react';
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
      {voiceOverrides && voiceOverrides.length > 0 && <span data-testid="voice-overrides-present" />}
    </div>
  ),
}));

vi.mock('../../services/tts', () => ({
  ttsService: {
    getConfig: vi.fn(() => ({
      enabled: false,
      autoRead: false,
      enabledForUser: false,
      provider: 'webspeech',
      rate: 1,
      pitch: 1,
      volume: 1,
    })),
    hasVoiceConfig: vi.fn(() => false),
    getVoices: vi.fn().mockResolvedValue([]),
    getModelsForProvider: vi.fn().mockResolvedValue([
      { id: 'voice-pt_BR-cadu-medium', name: 'voice-pt_BR-cadu-medium', provider: 'localai', selectionMode: 'model_only' },
      { id: 'qwen3-tts-0.6b-custom-voice', name: 'qwen3-tts-0.6b-custom-voice', provider: 'localai', selectionMode: 'model_and_voice' },
    ]),
    on: vi.fn(),
    off: vi.fn(),
    stop: vi.fn(),
    pause: vi.fn(),
    resume: vi.fn(),
    speakWithOverride: vi.fn().mockResolvedValue(undefined),
    setEnabled: vi.fn(),
    setAutoRead: vi.fn(),
    setRate: vi.fn().mockResolvedValue(undefined),
    setPitch: vi.fn(),
    setVolume: vi.fn().mockResolvedValue(undefined),
    setVoice: vi.fn().mockResolvedValue(undefined),
    isSupported: vi.fn(() => true),
  },
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

  it('chama onChange ao alterar rate', () => {
    const handleChange = vi.fn();
    
    render(<ProfileVoiceSection {...defaultProps} onChange={handleChange} />);
    
    const rateInput = screen.getByLabelText('profiles.voiceSection.rateLabel');
    fireEvent.change(rateInput, { target: { value: '1.5' } });
    
    expect(handleChange).toHaveBeenCalledWith('rate', 1.5);
  });

  it('chama onChange ao alterar volume', () => {
    const handleChange = vi.fn();
    
    render(<ProfileVoiceSection {...defaultProps} onChange={handleChange} />);
    
    const volumeInput = screen.getByLabelText('profiles.voiceSection.volumeLabel');
    fireEvent.change(volumeInput, { target: { value: '0.5' } });
    
    expect(handleChange).toHaveBeenCalledWith('volume', 0.5);
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

  it('mostra seletor de modelo TTS para OpenAI e mantém voz separada', () => {
    render(
      <ProfileVoiceSection
        {...defaultProps}
        providerId="openai-default"
        providerType="openai"
        ttsModel="tts-1"
      />
    );

    expect(screen.getByText('profiles.voiceSection.modelLabel')).toBeInTheDocument();
    expect(screen.getByTestId('voice-overrides-present')).toBeInTheDocument();
  });

  it('NÃO mostra seletor de modelo TTS para webspeech', () => {
    render(
      <ProfileVoiceSection
        {...defaultProps}
        providerId="webspeech"
        ttsModel="tts-1"
      />
    );

    expect(screen.queryByText('profiles.voiceSection.modelLabel')).not.toBeInTheDocument();
  });

  it('NÃO mostra seletor de modelo TTS para sapi5', () => {
    render(
      <ProfileVoiceSection
        {...defaultProps}
        providerId="sapi5"
        ttsModel="tts-1"
      />
    );

    expect(screen.queryByText('profiles.voiceSection.modelLabel')).not.toBeInTheDocument();
  });

  it('atualiza modelo e selectionMode ao selecionar modelo OpenAI', () => {
    const handleChange = vi.fn();
    render(
      <ProfileVoiceSection
        {...defaultProps}
        providerId="openai-default"
        providerType="openai"
        ttsModel="tts-1"
        onChange={handleChange}
      />
    );

    const modelSelect = screen.getByDisplayValue('tts-1');
    fireEvent.change(modelSelect, { target: { value: 'tts-1-hd' } });

    expect(handleChange).toHaveBeenCalledWith('model', 'tts-1-hd');
    expect(handleChange).toHaveBeenCalledWith('selectionMode', 'model_and_voice');
  });

  it('limpa modelo, selectionMode e voz ao limpar o seletor de modelo', () => {
    const handleChange = vi.fn();
    render(
      <ProfileVoiceSection
        {...defaultProps}
        providerId="openai-default"
        providerType="openai"
        ttsModel="tts-1"
        voice="nova"
        onChange={handleChange}
      />
    );

    const modelSelect = screen.getByDisplayValue('tts-1');
    fireEvent.change(modelSelect, { target: { value: '' } });

    expect(handleChange).toHaveBeenCalledWith('model', '');
    expect(handleChange).toHaveBeenCalledWith('selectionMode', '');
    expect(handleChange).toHaveBeenCalledWith('voice', '');
  });

  it('não permite escolher voz HTTP antes do modelo TTS', () => {
    render(
      <ProfileVoiceSection
        {...defaultProps}
        providerId="openai-default"
        providerType="openai"
        ttsModel=""
        voice=""
      />
    );

    expect(screen.getByText('profiles.voiceSection.modelLabel')).toBeInTheDocument();
    expect(screen.queryByTestId('voice-picker-mock')).not.toBeInTheDocument();
  });

  it('NÃO mostra voiceOverrides quando providerType não é fornecido', () => {
    render(
      <ProfileVoiceSection
        {...defaultProps}
        providerId="openai-default"
        ttsModel="tts-1"
      />
    );

    // Sem providerType, getTTSCapabilities retorna DYNAMIC_TTS (sem staticVoices)
    // Portanto o botão HD (voiceOverrides) NÃO deve existir
    expect(screen.queryByTestId('voice-overrides-present')).not.toBeInTheDocument();
  });

  it('oculta o seletor de voz quando o modelo selecionado é model_only', () => {
    render(
      <ProfileVoiceSection
        {...defaultProps}
        providerId="localai-default"
        providerType="localai"
        ttsModel="voice-pt_BR-cadu-medium"
        selectionMode="model_only"
      />
    );

    expect(screen.getByText('profiles.voiceSection.modelLabel')).toBeInTheDocument();
    expect(screen.queryByTestId('voice-picker-mock')).not.toBeInTheDocument();
  });

  it('gera ids únicos para seletores de modelo no mesmo perfil', () => {
    render(
      <>
        <ProfileVoiceSection
          {...defaultProps}
          providerId="localai-default"
          providerType="localai"
          profileId="profile-1"
          label="Assistant voice"
        />
        <ProfileVoiceSection
          {...defaultProps}
          providerId="localai-default"
          providerType="localai"
          profileId="profile-1"
          label="User voice"
        />
      </>
    );

    const modelSelects = screen.getAllByLabelText('profiles.voiceSection.modelLabel');
    expect(modelSelects[0].id).toMatch(/^tts-model-profile-1-assistant-voice-/);
    expect(modelSelects[1].id).toMatch(/^tts-model-profile-1-user-voice-/);
    expect(modelSelects[0].id).not.toBe(modelSelects[1].id);
  });

  it('infere model_only para modelos voice-* antes da listagem dinâmica carregar', () => {
    render(
      <ProfileVoiceSection
        {...defaultProps}
        providerId="localai-default"
        providerType="localai"
        ttsModel="voice-pt_BR-cadu-medium"
      />
    );

    expect(screen.queryByTestId('voice-picker-mock')).not.toBeInTheDocument();
  });

  it('mantém o modelo selecionado e mostra vozes quando updates dependentes são aplicados em lote', async () => {
    function StatefulSection() {
      const [state, setState] = useState({
        model: '',
        voice: '',
        selectionMode: undefined as 'model_and_voice' | 'model_only' | undefined,
      });
      return (
        <ProfileVoiceSection
          {...defaultProps}
          providerId="localai-default"
          providerType="localai"
          ttsModel={state.model}
          voice={state.voice}
          selectionMode={state.selectionMode}
          onChange={(field, value) => setState((prev) => ({ ...prev, [field === 'selectionMode' ? 'selectionMode' : field]: value as string }))}
          onChangeMany={(updates) => setState((prev) => ({
            ...prev,
            ...(updates.model !== undefined ? { model: String(updates.model) } : {}),
            ...(updates.voice !== undefined ? { voice: String(updates.voice) } : {}),
            ...(updates.selectionMode !== undefined ? { selectionMode: updates.selectionMode as 'model_and_voice' | 'model_only' } : {}),
          }))}
        />
      );
    }

    render(<StatefulSection />);

    const modelSelect = await screen.findByLabelText('profiles.voiceSection.modelLabel') as HTMLSelectElement;
    await waitFor(() => expect(modelSelect.options.length).toBeGreaterThan(1));

    fireEvent.change(modelSelect, { target: { value: 'qwen3-tts-0.6b-custom-voice' } });

    await waitFor(() => expect(modelSelect.value).toBe('qwen3-tts-0.6b-custom-voice'));
    expect(screen.getByTestId('voice-picker-mock')).toBeInTheDocument();
  });
});
