import { describe, it, expect, vi } from 'vitest';
import { render, screen, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { McpConnectionSection } from './McpConnectionSection';

const noop = () => {};

function renderPKCE(overrides: Record<string, any> = {}) {
  const props = {
    transport: 'streamable',
    command: '',
    args: '',
    url: 'https://example.com/mcp',
    envText: '',
    enabled: true,
    autoConnect: true,
    authType: 'oauth2_pkce',
    authToken: '',
    authUsername: '',
    authPassword: '',
    hasExistingAuth: false,
    oauth2ClientId: 'my-client',
    oauth2ClientSecret: '',
    oauth2TokenUrl: 'https://auth.example.com/token',
    oauth2AuthUrl: 'https://auth.example.com/authorize',
    oauth2Scopes: 'openid',
    oauth2CallbackPort: '',
    oauth2CallbackHost: '',
    discoveryStatus: 'idle' as const,
    discoveredFields: new Set<string>(),
    discoveryResourceName: '',
    discoveryRegistrationUrl: '',
    onCommandChange: noop,
    onArgsChange: noop,
    onUrlChange: noop,
    onEnvTextChange: noop,
    onEnabledChange: noop,
    onAutoConnectChange: noop,
    onAuthTypeChange: noop,
    onAuthTokenChange: noop,
    onAuthUsernameChange: noop,
    onAuthPasswordChange: noop,
    onOAuth2ClientIdChange: noop,
    onOAuth2ClientSecretChange: noop,
    onOAuth2TokenUrlChange: noop,
    onOAuth2AuthUrlChange: noop,
    onOAuth2ScopesChange: noop,
    onOAuth2CallbackPortChange: noop,
    onOAuth2CallbackHostChange: noop,
    onUrlBlur: noop,
    onManualOverride: noop,
    ...overrides,
  };
  return render(<McpConnectionSection {...props} />);
}

describe('McpConnectionSection — Callback Host', () => {
  it('exibe Select de Callback Host quando authType é oauth2_pkce', () => {
    renderPKCE();
    expect(screen.getByLabelText('Callback Host')).toBeInTheDocument();
  });

  it('não exibe Select de Callback Host quando authType é bearer', () => {
    renderPKCE({ authType: 'bearer' });
    expect(screen.queryByLabelText('Callback Host')).not.toBeInTheDocument();
  });

  it('não exibe Select de Callback Host quando authType é oauth2_client_credentials', () => {
    renderPKCE({ authType: 'oauth2_client_credentials' });
    expect(screen.queryByLabelText('Callback Host')).not.toBeInTheDocument();
  });

  it('não exibe Select de Callback Host quando authType é none', () => {
    renderPKCE({ authType: 'none' });
    expect(screen.queryByLabelText('Callback Host')).not.toBeInTheDocument();
  });

  it('tem as 3 opções de host corretas', () => {
    renderPKCE();
    const select = screen.getByLabelText('Callback Host');
    const options = within(select).getAllByRole('option');
    expect(options).toHaveLength(3);
    expect(options[0]).toHaveValue('localhost');
    expect(options[1]).toHaveValue('127.0.0.1');
    expect(options[2]).toHaveValue('[::1]');
  });

  it('valor padrão é localhost quando oauth2CallbackHost está vazio', () => {
    renderPKCE({ oauth2CallbackHost: '' });
    expect(screen.getByLabelText('Callback Host')).toHaveValue('localhost');
  });

  it('seleciona 127.0.0.1 quando oauth2CallbackHost é "127.0.0.1"', () => {
    renderPKCE({ oauth2CallbackHost: '127.0.0.1' });
    expect(screen.getByLabelText('Callback Host')).toHaveValue('127.0.0.1');
  });

  it('seleciona [::1] quando oauth2CallbackHost é "[::1]"', () => {
    renderPKCE({ oauth2CallbackHost: '[::1]' });
    expect(screen.getByLabelText('Callback Host')).toHaveValue('[::1]');
  });

  it('chama onOAuth2CallbackHostChange ao mudar seleção', async () => {
    const user = userEvent.setup();
    const onChange = vi.fn();
    renderPKCE({ onOAuth2CallbackHostChange: onChange });

    await user.selectOptions(screen.getByLabelText('Callback Host'), '127.0.0.1');
    expect(onChange).toHaveBeenCalledWith('127.0.0.1');
  });

  it('exibe hint descritivo sobre o redirect_uri', () => {
    renderPKCE();
    expect(
      screen.getByText(/Host usado no redirect_uri do OAuth/),
    ).toBeInTheDocument();
  });

  it('preview do redirect URI usa localhost como default', () => {
    renderPKCE({ oauth2CallbackPort: '3118', oauth2CallbackHost: '' });
    expect(screen.getByText(/http:\/\/localhost:3118\/callback/)).toBeInTheDocument();
  });

  it('preview do redirect URI reflete 127.0.0.1', () => {
    renderPKCE({ oauth2CallbackPort: '3118', oauth2CallbackHost: '127.0.0.1' });
    expect(screen.getByText(/http:\/\/127\.0\.0\.1:3118\/callback/)).toBeInTheDocument();
  });

  it('preview do redirect URI reflete [::1]', () => {
    renderPKCE({ oauth2CallbackPort: '8080', oauth2CallbackHost: '[::1]' });
    expect(screen.getByText(/http:\/\/\[::1\]:8080\/callback/)).toBeInTheDocument();
  });

  it('não exibe preview do redirect URI quando porta está vazia', () => {
    renderPKCE({ oauth2CallbackPort: '', oauth2CallbackHost: 'localhost' });
    expect(screen.queryByText(/Redirect URI:/)).not.toBeInTheDocument();
  });
});
