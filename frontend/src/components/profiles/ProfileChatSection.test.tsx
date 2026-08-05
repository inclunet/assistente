import { describe, it, expect, vi } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { ProfileChatSection } from './ProfileChatSection';

vi.mock('react-i18next', () => ({
  initReactI18next: { type: '3rdParty', init: () => {} },
  useTranslation: () => ({
    t: (key: string) => {
      const translations: Record<string, string> = {
        'profiles.chatSection.groupProvider': 'Provedor e Modelo',
        'profiles.chatSection.groupGeneration': 'Parâmetros de Geração',
        'profiles.chatSection.groupContext': 'Contexto e Limites',
        'profiles.chatSection.groupRecovery': 'Recuperação de Streaming',
        'profiles.chatSection.groupPromptCache': 'Prompt Cache',
        'profiles.chatSection.groupDebug': 'Debug LLM',
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
        'profiles.chatSection.promptCacheEnabled': 'Habilitar mecanismos ativos de cache',
        'profiles.chatSection.promptCacheEnabledHint': 'Mantém o layout cache-friendly sempre ativo; controla apenas hints e cache control.',
        'profiles.chatSection.promptCacheProviderHints': 'Enviar provider hints',
        'profiles.chatSection.promptCacheProviderHintsHint': 'Permite hints neutros como prompt_cache_key quando suportado.',
        'profiles.chatSection.promptCacheExplicitCacheControl': 'Usar cache control explícito',
        'profiles.chatSection.promptCacheExplicitCacheControlHint': 'Permite marcação explícita de blocos para providers compatíveis.',
        'profiles.chatSection.debugDumpsEnabled': 'Salvar dumps OpenAI Responses para debug',
        'profiles.chatSection.debugDumpsEnabledHint': 'Grava dados do caminho OpenAI Responses localmente em ~/.assistente/debug/llm-dumps com campos sensíveis redigidos.',
        'profiles.chatSection.debugDumpRequests': 'Salvar requests completas',
        'profiles.chatSection.debugDumpResponses': 'Salvar responses finais',
        'profiles.chatSection.debugMaxFiles': 'Máximo de dumps por conversa',
        'profiles.chatSection.debugMaxFilesHint': 'Limita snapshots retidos por conversa.',
        'profiles.chatSection.streamingRecoveryEnabled': 'Tentar recuperar respostas interrompidas automaticamente',
        'profiles.chatSection.streamingRecoveryEnabledHint': 'Quando uma resposta falha ou é interrompida, tenta retomar automaticamente antes de marcar como falha.',
        'profiles.chatSection.streamingRecoveryMaxAttempts': 'Máximo de tentativas de recuperação',
        'profiles.chatSection.streamingRecoveryMaxAttemptsHint': 'Número máximo de tentativas automáticas antes de exigir uma ação manual.',
        'profiles.chatSection.streamingRecoveryShowContinue': 'Mostrar ação “Continuar resposta” quando falhar',
        'profiles.chatSection.streamingRecoveryShowContinueHint': 'Exibe a opção manual de continuação quando houver conteúdo parcial e o modelo suportar.',
      };
      return translations[key] ?? key;
    },
  }),
}));

vi.mock('../pickers/LLMProviderPicker', () => ({
  LLMProviderPicker: ({ value, onChange, label, disabled }: { value: string; onChange: (value: string) => void; label?: string; disabled?: boolean }) => (
    <div data-testid="llm-provider-picker-mock">
      <label>{label}</label>
      <button disabled={disabled} onClick={() => onChange('test-provider')}>
        {value || 'Selecionar provedor'}
      </button>
    </div>
  ),
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
    promptCache: {
      enabled: false,
      provider_hints: false,
      explicit_cache_control: false,
    },
    debug: {
      enabled: false,
      dump_requests: true,
      dump_responses: true,
      max_files: 200,
    },
    streamingRecoveryEnabled: true,
    streamingRecoveryMaxAttempts: 3,
    streamingRecoveryShowContinue: true,
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

  it('chama onChange ao habilitar prompt cache', () => {
    const handleChange = vi.fn();
    render(<ProfileChatSection {...defaultProps} onChange={handleChange} />);

    const checkbox = screen.getByLabelText('Habilitar mecanismos ativos de cache');
    fireEvent.click(checkbox);

    expect(handleChange).toHaveBeenCalledWith('prompt_cache.enabled', true);
  });

  it('desabilita controles dependentes quando prompt cache está desligado', () => {
    render(<ProfileChatSection {...defaultProps} />);

    expect(screen.getByLabelText('Enviar provider hints')).toBeDisabled();
    expect(screen.getByLabelText('Usar cache control explícito')).toBeDisabled();
  });

  it('chama onChange ao alternar controles dependentes de prompt cache', () => {
    const handleChange = vi.fn();
    render(
      <ProfileChatSection
        {...defaultProps}
        promptCache={{ enabled: true, provider_hints: false, explicit_cache_control: false }}
        onChange={handleChange}
      />
    );

    fireEvent.click(screen.getByLabelText('Enviar provider hints'));
    fireEvent.click(screen.getByLabelText('Usar cache control explícito'));

    expect(handleChange).toHaveBeenCalledWith('prompt_cache.provider_hints', true);
    expect(handleChange).toHaveBeenCalledWith('prompt_cache.explicit_cache_control', true);
  });

  it('limpa controles dependentes ao desabilitar prompt cache com onMultiChange', () => {
    const handleChange = vi.fn();
    const handleMultiChange = vi.fn();
    render(
      <ProfileChatSection
        {...defaultProps}
        promptCache={{ enabled: true, provider_hints: true, explicit_cache_control: true }}
        onChange={handleChange}
        onMultiChange={handleMultiChange}
      />
    );

    fireEvent.click(screen.getByLabelText('Habilitar mecanismos ativos de cache'));

    expect(handleMultiChange).toHaveBeenCalledWith({
      'prompt_cache.enabled': false,
      'prompt_cache.provider_hints': false,
      'prompt_cache.explicit_cache_control': false,
    });
    expect(handleChange).not.toHaveBeenCalled();
  });

  it('habilita dumps LLM sem sobrescrever preferências de requests/responses', () => {
    const handleChange = vi.fn();
    const handleMultiChange = vi.fn();
    render(
      <ProfileChatSection
        {...defaultProps}
        debug={{ enabled: false, dump_requests: false, dump_responses: true, max_files: 200 }}
        onChange={handleChange}
        onMultiChange={handleMultiChange}
      />
    );

    fireEvent.click(screen.getByLabelText('Salvar dumps OpenAI Responses para debug'));

    expect(handleMultiChange).toHaveBeenCalledWith({
      'debug.enabled': true,
      'debug.max_files': 200,
    });
    expect(handleChange).not.toHaveBeenCalled();
  });

  it('desabilita controles dependentes quando debug LLM está desligado', () => {
    render(<ProfileChatSection {...defaultProps} />);

    expect(screen.getByLabelText('Salvar requests completas')).toBeDisabled();
    expect(screen.getByLabelText('Salvar responses finais')).toBeDisabled();
    expect(screen.getByLabelText('Máximo de dumps por conversa')).toBeDisabled();
  });

  it('preserva zero no máximo de dumps para usar o default do backend', () => {
    const handleChange = vi.fn();
    render(
      <ProfileChatSection
        {...defaultProps}
        debug={{ enabled: true, dump_requests: true, dump_responses: true, max_files: 200 }}
        onChange={handleChange}
      />
    );

    const input = screen.getByLabelText('Máximo de dumps por conversa');
    fireEvent.change(input, { target: { value: '0' } });

    expect(handleChange).toHaveBeenCalledWith('debug.max_files', 0);
  });

  it('limita máximo de dumps ao intervalo aceito pelo backend', () => {
    const handleChange = vi.fn();
    render(
      <ProfileChatSection
        {...defaultProps}
        debug={{ enabled: true, dump_requests: true, dump_responses: true, max_files: 200 }}
        onChange={handleChange}
      />
    );

    const input = screen.getByLabelText('Máximo de dumps por conversa');
    fireEvent.change(input, { target: { value: '-1' } });
    fireEvent.change(input, { target: { value: '10001' } });

    expect(handleChange).toHaveBeenCalledWith('debug.max_files', 0);
    expect(handleChange).toHaveBeenCalledWith('debug.max_files', 10000);
  });

  it('chama onChange ao alternar auto-recuperação de streaming', () => {
    const handleChange = vi.fn();
    render(<ProfileChatSection {...defaultProps} onChange={handleChange} />);

    const checkbox = screen.getByLabelText('Tentar recuperar respostas interrompidas automaticamente');
    fireEvent.click(checkbox);

    expect(handleChange).toHaveBeenCalledWith('streaming_recovery_enabled', false);
  });

  it('chama onChange ao alterar máximo de tentativas de recuperação', () => {
    const handleChange = vi.fn();
    render(<ProfileChatSection {...defaultProps} onChange={handleChange} />);

    const input = screen.getByLabelText('Máximo de tentativas de recuperação');
    fireEvent.change(input, { target: { value: '5' } });

    expect(handleChange).toHaveBeenCalledWith('streaming_recovery_max_attempts', 5);
  });

  it('chama onChange ao alternar ação continuar resposta', () => {
    const handleChange = vi.fn();
    render(<ProfileChatSection {...defaultProps} onChange={handleChange} />);

    const checkbox = screen.getByLabelText('Mostrar ação “Continuar resposta” quando falhar');
    fireEvent.click(checkbox);

    expect(handleChange).toHaveBeenCalledWith('streaming_recovery_show_continue', false);
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
    expect(screen.getByLabelText('Habilitar mecanismos ativos de cache')).toBeDisabled();
    expect(screen.getByLabelText('Enviar provider hints')).toBeDisabled();
    expect(screen.getByLabelText('Usar cache control explícito')).toBeDisabled();
    expect(screen.getByLabelText('Salvar dumps OpenAI Responses para debug')).toBeDisabled();
    expect(screen.getByLabelText('Salvar requests completas')).toBeDisabled();

    const modelPickerButton = screen.getByTestId('model-picker-mock').querySelector('button');
    expect(modelPickerButton).toBeDisabled();
  });

  // Num perfil com agente o turno lê só o modelo: amostragem, cache, contexto e
  // recuperação não chegam a existir (AEP-0084, Fase 8). Mostrá-los pedindo
  // atenção de quem navega por teclado seria pedir atenção para nada.
  describe('com provedor de agente', () => {
    const comAgente = { ...defaultProps, llmProvider: 'cursor', agentProvider: true };

    it('mantém a escolha de provedor e modelo', () => {
      render(<ProfileChatSection {...comAgente} />);

      expect(screen.getByTestId('llm-provider-picker-mock')).toBeInTheDocument();
      expect(screen.getByTestId('model-picker-mock')).toBeInTheDocument();
    });

    it('esconde os ajustes que o turno do agente ignora', () => {
      render(<ProfileChatSection {...comAgente} />);

      expect(screen.queryByLabelText('Temperatura')).toBeNull();
      expect(screen.queryByLabelText('Top P')).toBeNull();
      expect(screen.queryByLabelText('Max Tokens')).toBeNull();
      expect(screen.queryByLabelText('Raciocínio (Reasoning)')).toBeNull();
      expect(screen.queryByLabelText('Janela de Contexto (tokens)')).toBeNull();
      expect(screen.queryByLabelText('Timeout (segundos)')).toBeNull();
      expect(screen.queryByLabelText('Habilitar mecanismos ativos de cache')).toBeNull();
      expect(screen.queryByLabelText('Salvar dumps OpenAI Responses para debug')).toBeNull();
      expect(
        screen.queryByLabelText('Tentar recuperar respostas interrompidas automaticamente'),
      ).toBeNull();
    });

    it('diz por que a guia é curta', () => {
      render(<ProfileChatSection {...comAgente} />);

      expect(screen.getByTestId('profile-chat-agent-hint')).toBeInTheDocument();
    });

    it('esconder campo não mexe no perfil', () => {
      const handleChange = vi.fn();
      const handleMultiChange = vi.fn();
      render(
        <ProfileChatSection
          {...comAgente}
          onChange={handleChange}
          onMultiChange={handleMultiChange}
        />,
      );

      expect(handleChange).not.toHaveBeenCalled();
      expect(handleMultiChange).not.toHaveBeenCalled();
    });

    it('provedor comum continua com tudo', () => {
      render(<ProfileChatSection {...defaultProps} />);

      expect(screen.getByLabelText('Temperatura')).toBeInTheDocument();
      expect(screen.queryByTestId('profile-chat-agent-hint')).toBeNull();
    });
  });
});
