import { describe, it, expect, vi } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { ChannelsSignalSection } from './ChannelsSignalSection';

vi.mock('react-i18next', () => ({
  initReactI18next: { type: '3rdParty', init: () => {} },
  useTranslation: () => ({
    t: (key: string) =>
      (
        ({
          'channels.signal.enabled': 'Habilitado',
          'channels.signal.apiUrl': 'URL da API',
          'channels.signal.apiUrlPlaceholder': 'http://localhost:8080',
          'channels.signal.apiToken': 'Token da API (opcional)',
          'channels.signal.apiTokenPlaceholder': 'Bearer token',
          'channels.signal.saveVault': 'Salvar token no cofre de credenciais',
          'channels.signal.vaultHint':
            'Use apenas se sua instância exigir autenticação. O token fica criptografado no cofre.',
          'channels.signal.tokenStored': 'Token salvo no cofre',
          'channels.signal.removeVault': 'Remover do cofre',
          'channels.signal.testConnection': 'Testar Conexão',
          'channels.signal.testHint': 'Teste a conexão para avançar.',
          'channels.signal.connectAccount': 'Conectar Conta',
          'channels.signal.connectHint':
            'Cadastre um novo número ou conecte uma conta existente via QR Code.',
          'channels.signal.phoneNumber': 'Novo número de telefone',
          'channels.signal.phonePlaceholder': '+5511999999999',
          'channels.signal.registerNumber': 'Cadastrar número',
          'channels.signal.connectExisting': 'Conectar conta existente',
          'channels.signal.maxContacts': 'Max. contatos autorizados',
          'channels.signal.maxContactsHint':
            'Ao atingir o limite, novos contatos são ignorados silenciosamente.',
          'channels.signal.channelProfile': 'Perfil do Canal',
          'channels.signal.channelProfileHint':
            'Perfil usado para conversas deste canal. Define modelo, voz, STT e comportamento. Vazio usa o perfil ativo global.',
          'channels.signal.maxHistory': 'Máximo de Histórico',
        }) as Record<string, string>
      )[key] ?? key,
  }),
}));

vi.mock('../pickers/ProfilePicker', () => ({
  ProfilePicker: ({
    value,
    onChange,
    label,
  }: {
    value: string;
    onChange: (value: string) => void;
    label: string;
  }) => (
    <div>
      <label htmlFor="profile-picker">{label}</label>
      <select id="profile-picker" value={value} onChange={(e) => onChange(e.target.value)}>
        <option value="">Nenhum</option>
        <option value="default">Default</option>
      </select>
    </div>
  ),
}));

describe('ChannelsSignalSection', () => {
  const mockOnChange = vi.fn();
  const mockOnAnnounce = vi.fn();
  const mockOnCheckAPI = vi.fn();
  const mockOnRegister = vi.fn();
  const mockOnVerify = vi.fn();
  const mockOnLink = vi.fn();
  const mockOnUnregister = vi.fn();
  const mockOnStopLinkPolling = vi.fn();
  const mockSetApiReady = vi.fn();
  const mockSetApiInfo = vi.fn();
  const mockSetRegError = vi.fn();
  const mockSetRegStep = vi.fn();
  const mockSetRegCode = vi.fn();
  const mockSetRegCaptcha = vi.fn();
  const mockSetSmsSent = vi.fn();
  const mockSetAccounts = vi.fn();
  const mockSetConnectionMode = vi.fn();
  const mockSetLinkQR = vi.fn();
  const mockSetLinking = vi.fn();
  const mockOnToggleVault = vi.fn();
  const mockOnRemoveToken = vi.fn();

  const defaultForm = {
    enabled: false,
    apiURL: '',
    account: '',
    apiToken: '',
    profile: '',
    maxHistory: 50,
    maxContacts: 10,
  };

  const defaultProps = {
    form: defaultForm,
    onChange: mockOnChange,
    onAnnounce: mockOnAnnounce,
    vaultEnabled: false,
    onToggleVault: mockOnToggleVault,
    tokenStored: false,
    tokenMasked: '',
    onRemoveToken: mockOnRemoveToken,
    apiReady: false,
    apiInfo: '',
    regError: '',
    regStep: 'idle' as const,
    regCode: '',
    regCaptcha: '',
    smsSent: false,
    accounts: [],
    connectionMode: 'register' as const,
    linkQR: '',
    linking: false,
    unregistering: null,
    checkingAPI: false,
    onSetApiReady: mockSetApiReady,
    onSetApiInfo: mockSetApiInfo,
    onSetRegError: mockSetRegError,
    onSetRegStep: mockSetRegStep,
    onSetRegCode: mockSetRegCode,
    onSetRegCaptcha: mockSetRegCaptcha,
    onSetSmsSent: mockSetSmsSent,
    onSetAccounts: mockSetAccounts,
    onSetConnectionMode: mockSetConnectionMode,
    onSetLinkQR: mockSetLinkQR,
    onSetLinking: mockSetLinking,
    onCheckAPI: mockOnCheckAPI,
    onRegister: mockOnRegister,
    onVerify: mockOnVerify,
    onLink: mockOnLink,
    onUnregister: mockOnUnregister,
    onStopLinkPolling: mockOnStopLinkPolling,
  };

  it('renderiza checkbox de habilitado', () => {
    render(<ChannelsSignalSection {...defaultProps} />);
    expect(screen.getByLabelText('Habilitado')).toBeInTheDocument();
  });

  it('não mostra campos quando desabilitado', () => {
    render(<ChannelsSignalSection {...defaultProps} />);
    expect(screen.queryByLabelText('URL da API')).not.toBeInTheDocument();
  });

  it('mostra campos quando habilitado', () => {
    render(<ChannelsSignalSection {...defaultProps} form={{ ...defaultForm, enabled: true }} />);

    expect(screen.getByText('URL da API')).toBeInTheDocument();
    expect(screen.getByPlaceholderText('http://localhost:8080')).toBeInTheDocument();
    expect(screen.getByText('Testar Conexão')).toBeInTheDocument();
  });

  it('mostra mensagem de teste de conexão quando API não está pronta', () => {
    render(
      <ChannelsSignalSection
        {...defaultProps}
        form={{ ...defaultForm, enabled: true }}
        apiReady={false}
      />
    );

    expect(screen.getByText('Teste a conexão para avançar.')).toBeInTheDocument();
  });

  it('mostra gerenciamento de conta quando há contas conectadas', () => {
    render(
      <ChannelsSignalSection
        {...defaultProps}
        form={{ ...defaultForm, enabled: true }}
        apiReady={true}
        accounts={['+5511999999999']}
      />
    );

    expect(screen.getByText('Conta Conectada')).toBeInTheDocument();
    expect(screen.getByText('+5511999999999')).toBeInTheDocument();
    expect(screen.getByText('Desconectar')).toBeInTheDocument();
  });

  it('mostra seção de conectar conta quando API pronta e sem contas', () => {
    render(
      <ChannelsSignalSection
        {...defaultProps}
        form={{ ...defaultForm, enabled: true }}
        apiReady={true}
        accounts={[]}
      />
    );

    expect(screen.getByText('Conectar Conta')).toBeInTheDocument();
    expect(screen.getByText('Cadastrar número')).toBeInTheDocument();
    expect(screen.getByText('Conectar conta existente')).toBeInTheDocument();
  });

  it('chama onChange ao habilitar/desabilitar', async () => {
    const user = userEvent.setup();
    render(<ChannelsSignalSection {...defaultProps} />);

    const checkbox = screen.getByLabelText('Habilitado');
    await user.click(checkbox);

    expect(mockOnChange).toHaveBeenCalledWith({
      ...defaultForm,
      enabled: true,
    });
  });

  it('limpa estado ao alterar URL da API', () => {
    render(<ChannelsSignalSection {...defaultProps} form={{ ...defaultForm, enabled: true }} />);

    const input = screen.getByPlaceholderText('http://localhost:8080');
    fireEvent.change(input, { target: { value: 'http://localhost:8080' } });

    expect(mockSetApiReady).toHaveBeenCalledWith(false);
    expect(mockSetApiInfo).toHaveBeenCalledWith('');
    expect(mockSetRegError).toHaveBeenCalledWith('');
    expect(mockSetAccounts).toHaveBeenCalledWith([]);
  });
});
