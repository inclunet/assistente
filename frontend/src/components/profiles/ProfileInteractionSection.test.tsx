import { describe, it, expect, vi } from 'vitest';
import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { ProfileInteractionSection } from './ProfileInteractionSection';

vi.mock('react-i18next', () => ({
  initReactI18next: { type: '3rdParty', init: () => {} },
  useTranslation: () => ({
    t: (key: string) => {
      const translations: Record<string, string> = {
        'profiles.interactionSection.sttTitle': 'Reconhecimento de Fala (STT)',
        'profiles.interactionSection.sttDescription': 'Selecione o provedor de reconhecimento de fala',
        'profiles.interactionSection.sttLanguage': 'Idioma (STT)',
        'profiles.interactionSection.ptBR': 'Português (Brasil)',
        'profiles.interactionSection.enUS': 'English (US)',
        'profiles.interactionSection.es': 'Español',
        'profiles.interactionSection.fr': 'Français',
        'profiles.interactionSection.de': 'Deutsch',
        'profiles.interactionSection.it': 'Italiano',
        'profiles.interactionSection.feedbackSounds': 'Ativar sons de feedback',
        'profiles.interactionSection.feedbackSoundsHint': 'Sons ao iniciar/parar gravação e outros eventos de interação',
      };
      return translations[key] ?? key;
    },
  }),
}));

// Mock STTProviderPicker para evitar imports de @wailsjs
vi.mock('../pickers/STTProviderPicker', () => ({
  STT_WEBSPEECH: 'webspeech',
  STT_WHISPER: 'whisper_api',
  STTProviderPicker: ({ value, onChange, label }: { value: string; onChange: (provider: string, llmProviderId?: string) => void; label: string }) => (
    <div data-testid="stt-provider-picker-mock">
      <label>{label}</label>
      <button onClick={() => onChange('whisper_api', 'openai-1')}>
        {value}
      </button>
    </div>
  ),
}));

describe('ProfileInteractionSection', () => {
  const defaultProps = {
    sttProvider: 'webspeech',
    sttLLMProviderId: '',
    sttModel: '',
    sttLanguage: 'pt-BR',
    enableFeedbackSounds: true,
    onChange: vi.fn(),
  };

  it('renderiza a seção de interação', () => {
    render(<ProfileInteractionSection {...defaultProps} />);
    
    expect(screen.getByTestId('profile-interaction-section')).toBeInTheDocument();
  });

  it('renderiza o STTProviderPicker com label correto', () => {
    render(<ProfileInteractionSection {...defaultProps} />);
    
    expect(screen.getByText('Reconhecimento de Fala (STT)')).toBeInTheDocument();
  });

  it('renderiza o select de idioma com valor correto', () => {
    render(<ProfileInteractionSection {...defaultProps} />);
    
    const select = screen.getByTestId('stt-language-select') as HTMLSelectElement;
    expect(select).toHaveValue('pt-BR');
  });

  it('renderiza o checkbox de feedback sounds com estado correto', () => {
    render(<ProfileInteractionSection {...defaultProps} />);
    
    const checkbox = screen.getByTestId('feedback-sounds-checkbox') as HTMLInputElement;
    expect(checkbox).toBeChecked();
  });

  it('renderiza todas as opções de idioma', () => {
    render(<ProfileInteractionSection {...defaultProps} />);
    
    expect(screen.getByRole('option', { name: 'Português (Brasil)' })).toBeInTheDocument();
    expect(screen.getByRole('option', { name: 'English (US)' })).toBeInTheDocument();
    expect(screen.getByRole('option', { name: 'Español' })).toBeInTheDocument();
    expect(screen.getByRole('option', { name: 'Français' })).toBeInTheDocument();
    expect(screen.getByRole('option', { name: 'Deutsch' })).toBeInTheDocument();
    expect(screen.getByRole('option', { name: 'Italiano' })).toBeInTheDocument();
  });

  it('STTProviderPicker está presente e renderiza corretamente', () => {
    render(<ProfileInteractionSection {...defaultProps} />);
    
    // STTProviderPicker renderiza um picker com label
    // (teste simplificado pois STTProviderPicker tem seus próprios testes)
    const label = screen.getByText('Reconhecimento de Fala (STT)');
    expect(label).toBeInTheDocument();
  });

  it('chama onChange ao alterar sttLanguage', async () => {
    const handleChange = vi.fn();
    const user = userEvent.setup();
    
    render(<ProfileInteractionSection {...defaultProps} onChange={handleChange} />);
    
    const select = screen.getByTestId('stt-language-select');
    await user.selectOptions(select, 'en-US');
    
    expect(handleChange).toHaveBeenCalledWith('sttLanguage', 'en-US');
  });

  it('chama onChange ao marcar checkbox de feedback sounds', async () => {
    const handleChange = vi.fn();
    const user = userEvent.setup();
    
    render(
      <ProfileInteractionSection
        {...defaultProps}
        enableFeedbackSounds={false}
        onChange={handleChange}
      />
    );
    
    const checkbox = screen.getByTestId('feedback-sounds-checkbox');
    await user.click(checkbox);
    
    expect(handleChange).toHaveBeenCalledWith('enableFeedbackSounds', true);
  });

  it('chama onChange ao desmarcar checkbox de feedback sounds', async () => {
    const handleChange = vi.fn();
    const user = userEvent.setup();
    
    render(
      <ProfileInteractionSection
        {...defaultProps}
        enableFeedbackSounds={true}
        onChange={handleChange}
      />
    );
    
    const checkbox = screen.getByTestId('feedback-sounds-checkbox');
    await user.click(checkbox);
    
    expect(handleChange).toHaveBeenCalledWith('enableFeedbackSounds', false);
  });

  it('renderiza com valores padrão quando props estão vazias', () => {
    render(
      <ProfileInteractionSection
        sttProvider=""
        sttLLMProviderId=""
        sttModel=""
        sttLanguage=""
        enableFeedbackSounds={false}
        onChange={vi.fn()}
      />
    );
    
    // Deve usar fallbacks
    const select = screen.getByTestId('stt-language-select') as HTMLSelectElement;
    expect(select).toHaveValue('pt-BR');
  });

  it('desabilita os campos quando disabled é true', () => {
    render(<ProfileInteractionSection {...defaultProps} disabled={true} />);
    
    const select = screen.getByTestId('stt-language-select');
    const checkbox = screen.getByTestId('feedback-sounds-checkbox');
    
    expect(select).toBeDisabled();
    expect(checkbox).toBeDisabled();
  });

  it('renderiza a hint do checkbox de feedback sounds', () => {
    render(<ProfileInteractionSection {...defaultProps} />);
    
    expect(
      screen.getByText(/Sons ao iniciar\/parar gravação e outros eventos/)
    ).toBeInTheDocument();
  });

  it('renderiza checkbox desmarcado quando enableFeedbackSounds é false', () => {
    render(
      <ProfileInteractionSection
        {...defaultProps}
        enableFeedbackSounds={false}
      />
    );
    
    const checkbox = screen.getByTestId('feedback-sounds-checkbox') as HTMLInputElement;
    expect(checkbox).not.toBeChecked();
  });

  it('mantém estado do checkbox quando label é clicado', async () => {
    const handleChange = vi.fn();
    const user = userEvent.setup();
    
    render(
      <ProfileInteractionSection
        {...defaultProps}
        enableFeedbackSounds={false}
        onChange={handleChange}
      />
    );
    
    // Clicar no label deve marcar o checkbox
    const label = screen.getByText('Ativar sons de feedback');
    await user.click(label);
    
    expect(handleChange).toHaveBeenCalledWith('enableFeedbackSounds', true);
  });
});
