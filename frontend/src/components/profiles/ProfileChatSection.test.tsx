import { describe, it, expect, vi } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { ProfileChatSection } from './ProfileChatSection';

vi.mock('react-i18next', () => ({
  initReactI18next: { type: '3rdParty', init: () => {} },
  useTranslation: () => ({
    t: (key: string) => {
      const translations: Record<string, string> = {
        'profiles.chatSection.provider': 'Provedor LLM',
        'profiles.chatSection.model': 'Modelo',
        'profiles.chatSection.temperature': 'Temperatura',
        'profiles.chatSection.maxTokens': 'Max Tokens',
        'profiles.chatSection.maxTokensFormat': 'Formato do parâmetro max_tokens',
        'profiles.chatSection.maxTokensLegacy': 'Legacy (max_tokens) - Padrão',
        'profiles.chatSection.maxTokensCompletion': 'Completion Tokens (max_completion_tokens) - GPT-4o, o1, etc',
        'profiles.chatSection.maxTokensHint': '"legacy" usa max_tokens (maioria dos modelos). "completion_tokens" usa max_completion_tokens (GPT-4o, o1, modelos novos OpenAI).',
        'profiles.chatSection.contextWindow': 'Janela de Contexto (tokens)',
        'profiles.chatSection.contextWindowHint': 'Total de tokens suportados pelo modelo (ex: 128000). 0 = não definido. Quando definido, ativa sumarização automática.',
        'profiles.chatSection.maxMessages': 'Máx. Mensagens no Contexto',
        'profiles.chatSection.maxMessagesHint': 'Limite de mensagens enviadas ao modelo. 0 = padrão (50).',
        'profiles.chatSection.minPreserved': 'Mín. Mensagens Preservadas',
        'profiles.chatSection.minPreservedHint': 'Mínimo de mensagens mantidas após sumarização. 0 = padrão (10).',
        'profiles.chatSection.topP': 'Top P',
        'profiles.chatSection.timeout': 'Timeout (segundos)',
        'profiles.chatSection.reasoning': 'Raciocínio (Reasoning)',
        'profiles.chatSection.reasoningOllama': 'Ativado (Ollama)',
        'profiles.chatSection.reasoningOff': 'Desativado',
        'profiles.chatSection.reasoningNone': 'Mínimo (none)',
        'profiles.chatSection.reasoningLow': 'Baixo (low)',
        'profiles.chatSection.reasoningMedium': 'Médio (medium)',
        'profiles.chatSection.reasoningHigh': 'Alto (high)',
        'profiles.chatSection.reasoningMax': 'Máximo (max)',
        'profiles.chatSection.reasoningHint': 'Define como o modelo usa tokens de raciocínio interno.',
      };
      return translations[key] ?? key;
    },
  }),
}));

vi.mock('../pickers/ModelPicker', () => ({
  ModelPicker: ({ value, onChange, label, disabled }: { value: string; onChange: (value: string) => void; label?: string; disabled?: boolean }) => (
    <div data-testid="model-picker-mock">
      <label>{label}</label>
      <button disabled={disabled} onClick={() => onChange('test-model')}>
        {value}
      </button>
    </div>
  ),
}));

describe('ProfileChatSection', () => {
  const defaultProps = {
    llmProvider: '',
    model: 'gpt-4o-mini',
    temperature: 0.7,
    maxTokens: 4096,
    maxTokensMode: 'legacy',
    contextWindow: 0,
    maxContextMessages: 0,
    minContextMessages: 0,
    topP: 1.0,
    responseTimeout: 180,
    reasoningEffort: '',
    onChange: vi.fn(),
  };

  it('renderiza a seção de chat', () => {
    render(<ProfileChatSection {...defaultProps} />);
    
    expect(screen.getByTestId('profile-chat-section')).toBeInTheDocument();
  });

  it('renderiza o ModelPicker com label correto', () => {
    render(<ProfileChatSection {...defaultProps} />);
    
    expect(screen.getByText('Modelo')).toBeInTheDocument();
    expect(screen.getByTestId('model-picker-mock')).toBeInTheDocument();
  });

  it('renderiza os sliders com valores formatados', () => {
    render(<ProfileChatSection {...defaultProps} />);
    
    expect(screen.getByText('0.70')).toBeInTheDocument();
    expect(screen.getByText('1.00')).toBeInTheDocument();
  });

  it('renderiza os campos numéricos com valores corretos', () => {
    render(<ProfileChatSection {...defaultProps} />);
    
    expect(screen.getByLabelText('Max Tokens')).toHaveValue(4096);
    expect(screen.getByLabelText('Janela de Contexto (tokens)')).toHaveValue(0);
    expect(screen.getByLabelText('Máx. Mensagens no Contexto')).toHaveValue(0);
    expect(screen.getByLabelText('Mín. Mensagens Preservadas')).toHaveValue(0);
    expect(screen.getByLabelText('Timeout (segundos)')).toHaveValue(180);
  });

  it('renderiza o select de reasoning com valor padrão off', () => {
    render(<ProfileChatSection {...defaultProps} />);
    
    const select = screen.getByLabelText('Raciocínio (Reasoning)') as HTMLSelectElement;
    expect(select.value).toBe('off');
  });

  it('chama onChange ao alterar temperatura', () => {
    const handleChange = vi.fn();
    render(<ProfileChatSection {...defaultProps} onChange={handleChange} />);
    
    const slider = screen.getByLabelText('Temperatura');
    fireEvent.change(slider, { target: { value: '1.2' } });
    
    expect(handleChange).toHaveBeenCalledWith('temperature', 1.2);
  });

  it('chama onChange ao alterar top_p', () => {
    const handleChange = vi.fn();
    render(<ProfileChatSection {...defaultProps} onChange={handleChange} />);
    
    const slider = screen.getByLabelText('Top P');
    fireEvent.change(slider, { target: { value: '0.5' } });
    
    expect(handleChange).toHaveBeenCalledWith('top_p', 0.5);
  });

  it('chama onChange ao alterar max_tokens', () => {
    const handleChange = vi.fn();
    render(<ProfileChatSection {...defaultProps} onChange={handleChange} />);
    
    const input = screen.getByLabelText('Max Tokens');
    fireEvent.change(input, { target: { value: '2048' } });
    
    expect(handleChange).toHaveBeenCalledWith('max_tokens', 2048);
  });

  it('chama onChange ao alterar context_window', () => {
    const handleChange = vi.fn();
    render(<ProfileChatSection {...defaultProps} onChange={handleChange} />);
    
    const input = screen.getByLabelText('Janela de Contexto (tokens)');
    fireEvent.change(input, { target: { value: '120000' } });
    
    expect(handleChange).toHaveBeenCalledWith('context_window', 120000);
  });

  it('chama onChange ao alterar max_context_messages', () => {
    const handleChange = vi.fn();
    render(<ProfileChatSection {...defaultProps} onChange={handleChange} />);
    
    const input = screen.getByLabelText('Máx. Mensagens no Contexto');
    fireEvent.change(input, { target: { value: '75' } });
    
    expect(handleChange).toHaveBeenCalledWith('max_context_messages', 75);
  });

  it('chama onChange ao alterar min_context_messages', () => {
    const handleChange = vi.fn();
    render(<ProfileChatSection {...defaultProps} onChange={handleChange} />);
    
    const input = screen.getByLabelText('Mín. Mensagens Preservadas');
    fireEvent.change(input, { target: { value: '12' } });
    
    expect(handleChange).toHaveBeenCalledWith('min_context_messages', 12);
  });

  it('chama onChange ao alterar response_timeout', () => {
    const handleChange = vi.fn();
    render(<ProfileChatSection {...defaultProps} onChange={handleChange} />);
    
    const input = screen.getByLabelText('Timeout (segundos)');
    fireEvent.change(input, { target: { value: '240' } });
    
    expect(handleChange).toHaveBeenCalledWith('response_timeout', 240);
  });

  it('chama onChange ao alterar reasoning_effort', async () => {
    const handleChange = vi.fn();
    const user = userEvent.setup();
    render(<ProfileChatSection {...defaultProps} onChange={handleChange} />);
    
    const select = screen.getByLabelText('Raciocínio (Reasoning)');
    await user.selectOptions(select, 'medium');
    
    expect(handleChange).toHaveBeenCalledWith('reasoning_effort', 'medium');
  });

  it('envia string vazia quando reasoning for off', async () => {
    const handleChange = vi.fn();
    const user = userEvent.setup();
    render(
      <ProfileChatSection
        {...defaultProps}
        reasoningEffort="low"
        onChange={handleChange}
      />
    );
    
    const select = screen.getByLabelText('Raciocínio (Reasoning)');
    await user.selectOptions(select, 'off');
    
    expect(handleChange).toHaveBeenCalledWith('reasoning_effort', '');
  });

  it('desabilita campos quando disabled é true', () => {
    render(<ProfileChatSection {...defaultProps} disabled={true} />);
    
    expect(screen.getByLabelText('Temperatura')).toBeDisabled();
    expect(screen.getByLabelText('Top P')).toBeDisabled();
    expect(screen.getByLabelText('Max Tokens')).toBeDisabled();
    expect(screen.getByLabelText('Janela de Contexto (tokens)')).toBeDisabled();
    expect(screen.getByLabelText('Máx. Mensagens no Contexto')).toBeDisabled();
    expect(screen.getByLabelText('Mín. Mensagens Preservadas')).toBeDisabled();
    expect(screen.getByLabelText('Timeout (segundos)')).toBeDisabled();
    expect(screen.getByLabelText('Raciocínio (Reasoning)')).toBeDisabled();

    const modelPickerButton = screen.getByTestId('model-picker-mock').querySelector('button');
    expect(modelPickerButton).toBeDisabled();
  });
});
