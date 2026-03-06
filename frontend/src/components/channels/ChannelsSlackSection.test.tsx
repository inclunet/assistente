import { describe, it, expect, vi } from 'vitest';
import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { ChannelsSlackSection } from './ChannelsSlackSection';

vi.mock('../pickers/ProfilePicker', () => ({
  ProfilePicker: ({ value, onChange, label }: any) => (
    <div>
      <label htmlFor="profile-picker">{label}</label>
      <select
        id="profile-picker"
        value={value}
        onChange={(e) => onChange(e.target.value)}
      >
        <option value="">Nenhum</option>
        <option value="default">Default</option>
      </select>
    </div>
  ),
}));

describe('ChannelsSlackSection', () => {
  const mockOnChange = vi.fn();
  const mockOnAnnounce = vi.fn();
  const mockOnToggleVault = vi.fn();
  const mockOnRemoveBotToken = vi.fn();
  const mockOnRemoveAppToken = vi.fn();

  const defaultForm = {
    enabled: false,
    botToken: '',
    appToken: '',
    profile: '',
    maxHistory: 50,
    maxContacts: 10,
  };

  it('renderiza checkbox de habilitado', () => {
    render(
      <ChannelsSlackSection
        form={defaultForm}
        onChange={mockOnChange}
        onAnnounce={mockOnAnnounce}
        vaultEnabled={true}
        onToggleVault={mockOnToggleVault}
        botTokenStored={false}
        botTokenMasked=""
        appTokenStored={false}
        appTokenMasked=""
        onRemoveBotToken={mockOnRemoveBotToken}
        onRemoveAppToken={mockOnRemoveAppToken}
      />
    );

    expect(screen.getByLabelText('Habilitado')).toBeInTheDocument();
  });

  it('não mostra campos quando desabilitado', () => {
    render(
      <ChannelsSlackSection
        form={defaultForm}
        onChange={mockOnChange}
        onAnnounce={mockOnAnnounce}
        vaultEnabled={true}
        onToggleVault={mockOnToggleVault}
        botTokenStored={false}
        botTokenMasked=""
        appTokenStored={false}
        appTokenMasked=""
        onRemoveBotToken={mockOnRemoveBotToken}
        onRemoveAppToken={mockOnRemoveAppToken}
      />
    );

    expect(screen.queryByText('Bot Token')).not.toBeInTheDocument();
  });

  it('mostra campos quando habilitado', () => {
    render(
      <ChannelsSlackSection
        form={{ ...defaultForm, enabled: true }}
        onChange={mockOnChange}
        onAnnounce={mockOnAnnounce}
        vaultEnabled={true}
        onToggleVault={mockOnToggleVault}
        botTokenStored={false}
        botTokenMasked=""
        appTokenStored={false}
        appTokenMasked=""
        onRemoveBotToken={mockOnRemoveBotToken}
        onRemoveAppToken={mockOnRemoveAppToken}
      />
    );

    expect(screen.getByText('Bot Token')).toBeInTheDocument();
    expect(screen.getByText('App Token (Socket Mode)')).toBeInTheDocument();
    expect(screen.getByText('Max. contatos autorizados')).toBeInTheDocument();
  });

  it('chama onChange ao habilitar/desabilitar', async () => {
    const user = userEvent.setup();
    render(
      <ChannelsSlackSection
        form={defaultForm}
        onChange={mockOnChange}
        onAnnounce={mockOnAnnounce}
        vaultEnabled={true}
        onToggleVault={mockOnToggleVault}
        botTokenStored={false}
        botTokenMasked=""
        appTokenStored={false}
        appTokenMasked=""
        onRemoveBotToken={mockOnRemoveBotToken}
        onRemoveAppToken={mockOnRemoveAppToken}
      />
    );

    const checkbox = screen.getByLabelText('Habilitado');
    await user.click(checkbox);

    expect(mockOnChange).toHaveBeenCalledWith({
      ...defaultForm,
      enabled: true,
    });
  });

  it('chama onChange ao alterar botToken', async () => {
    const user = userEvent.setup();
    render(
      <ChannelsSlackSection
        form={{ ...defaultForm, enabled: true }}
        onChange={mockOnChange}
        onAnnounce={mockOnAnnounce}
        vaultEnabled={true}
        onToggleVault={mockOnToggleVault}
        botTokenStored={false}
        botTokenMasked=""
        appTokenStored={false}
        appTokenMasked=""
        onRemoveBotToken={mockOnRemoveBotToken}
        onRemoveAppToken={mockOnRemoveAppToken}
      />
    );

    const input = screen.getByPlaceholderText('xoxb-...');
    await user.type(input, 'xoxb-123456');

    expect(mockOnChange).toHaveBeenCalledWith(
      expect.objectContaining({
        botToken: expect.any(String),
      })
    );
  });

  it('mostra opção de salvar no cofre quando habilitado', () => {
    render(
      <ChannelsSlackSection
        form={{ ...defaultForm, enabled: true }}
        onChange={mockOnChange}
        onAnnounce={mockOnAnnounce}
        vaultEnabled={true}
        onToggleVault={mockOnToggleVault}
        botTokenStored={false}
        botTokenMasked=""
        appTokenStored={false}
        appTokenMasked=""
        onRemoveBotToken={mockOnRemoveBotToken}
        onRemoveAppToken={mockOnRemoveAppToken}
      />
    );

    expect(screen.getByLabelText('Salvar tokens no cofre de credenciais')).toBeInTheDocument();
  });
});
