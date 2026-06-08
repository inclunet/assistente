import { describe, it, expect, vi } from 'vitest';
import { render, screen, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { McpConnectionSection } from './McpConnectionSection';
import type { ComponentProps } from 'react';
import ptBR from '../../locales/pt-BR';

function resolveLocaleString(key: string, vars?: Record<string, unknown>): string | undefined {
  const root = (ptBR as { translation: Record<string, unknown> }).translation;
  const value = key.split('.').reduce<unknown>((acc, part) => {
    if (!acc || typeof acc !== 'object') return undefined;
    return (acc as Record<string, unknown>)[part];
  }, root);

  if (typeof value !== 'string') return undefined;

  if (!vars) return value;
  return value.replace(/\{\{\s*(\w+)\s*\}\}/g, (_match, varName: string) => {
    const v = vars[varName];
    return v == null ? '' : String(v);
  });
}

vi.mock('react-i18next', async (importOriginal) => {
  const actual = await importOriginal<typeof import('react-i18next')>();
  return {
    ...actual,
    useTranslation: () => ({
      t: (key: string, options?: string | Record<string, unknown>) => {
        const vars = options && typeof options === 'object' ? (options as Record<string, unknown>) : undefined;
        return resolveLocaleString(key, vars) ?? key;
      },
    }),
  };
});

const noop = () => {};

const baseProps: ComponentProps<typeof McpConnectionSection> = {
  transport: 'streamable',
  command: '',
  args: '',
  url: 'https://example.com/mcp',
  envText: '',
  enabled: true,
  autoConnect: true,
  preferBridge: false,
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
  discoveryStatus: 'not_found',
  discoveredFields: new Set<string>(),
  discoveryResourceName: '',
  discoveryRegistrationUrl: '',
  onCommandChange: noop,
  onArgsChange: noop,
  onUrlChange: noop,
  onEnvTextChange: noop,
  onEnabledChange: noop,
  onAutoConnectChange: noop,
  onPreferBridgeChange: noop,
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
};

function renderWith(overrides: Partial<ComponentProps<typeof McpConnectionSection>> = {}) {
  return render(<McpConnectionSection {...baseProps} {...overrides} />);
}

describe('McpConnectionSection — Discovery states', () => {
  it('DCR disponível: não exibe campos OAuth, mostra mensagem de sucesso', () => {
    renderWith({
      discoveryStatus: 'found',
      discoveryRegistrationUrl: 'https://auth.example.com/register',
    });
    expect(screen.queryByLabelText('Client ID')).not.toBeInTheDocument();
    expect(screen.getByText(/Client ID será registrado via DCR/)).toBeInTheDocument();
  });

  it('sem DCR: exibe campos Client ID e Client Secret', () => {
    renderWith({
      discoveryStatus: 'found',
      discoveryRegistrationUrl: '',
    });
    expect(screen.getByLabelText(/Client ID/)).toBeInTheDocument();
    expect(screen.getByLabelText(/Client Secret/)).toBeInTheDocument();
    expect(screen.queryByLabelText('Tipo de autenticação')).not.toBeInTheDocument();
  });

  it('discovery falhou: exibe configuração manual completa', () => {
    renderWith({ discoveryStatus: 'not_found' });
    expect(screen.getByLabelText('Tipo de autenticação')).toBeInTheDocument();
  });

  it('idle: não exibe campos OAuth', () => {
    renderWith({ discoveryStatus: 'idle' });
    expect(screen.queryByLabelText('Client ID')).not.toBeInTheDocument();
    expect(screen.queryByLabelText('Tipo de autenticação')).not.toBeInTheDocument();
  });

  it('loading: mostra mensagem de verificação', () => {
    renderWith({ discoveryStatus: 'loading' });
    expect(screen.getByText(/Verificando configuração OAuth/)).toBeInTheDocument();
  });
});

describe('McpConnectionSection — Callback Host (manual mode)', () => {
  it('exibe Select de Callback Host no avançado quando PKCE manual', () => {
    renderWith({ discoveryStatus: 'not_found', authType: 'oauth2_pkce' });
    expect(screen.getByLabelText('Callback Host')).toBeInTheDocument();
  });

  it('não exibe Callback Host para bearer', () => {
    renderWith({ discoveryStatus: 'not_found', authType: 'bearer' });
    expect(screen.queryByLabelText('Callback Host')).not.toBeInTheDocument();
  });

  it('tem as 3 opções de host corretas', () => {
    renderWith({ discoveryStatus: 'not_found', authType: 'oauth2_pkce' });
    const select = screen.getByLabelText('Callback Host');
    const options = within(select).getAllByRole('option');
    expect(options).toHaveLength(3);
    expect(options[0]).toHaveValue('localhost');
    expect(options[1]).toHaveValue('127.0.0.1');
    expect(options[2]).toHaveValue('[::1]');
  });

  it('valor padrão é localhost quando vazio', () => {
    renderWith({ discoveryStatus: 'not_found', oauth2CallbackHost: '' });
    expect(screen.getByLabelText('Callback Host')).toHaveValue('localhost');
  });

  it('seleciona 127.0.0.1', () => {
    renderWith({ discoveryStatus: 'not_found', oauth2CallbackHost: '127.0.0.1' });
    expect(screen.getByLabelText('Callback Host')).toHaveValue('127.0.0.1');
  });

  it('chama onOAuth2CallbackHostChange ao mudar', async () => {
    const user = userEvent.setup();
    const onChange = vi.fn();
    renderWith({ discoveryStatus: 'not_found', onOAuth2CallbackHostChange: onChange });

    await user.selectOptions(screen.getByLabelText('Callback Host'), '127.0.0.1');
    expect(onChange).toHaveBeenCalledWith('127.0.0.1');
  });

  it('preview do redirect URI usa localhost como default', () => {
    renderWith({ discoveryStatus: 'not_found', oauth2CallbackPort: '3118', oauth2CallbackHost: '' });
    expect(screen.getByText(/http:\/\/localhost:3118\/callback/)).toBeInTheDocument();
  });

  it('preview do redirect URI reflete 127.0.0.1', () => {
    renderWith({ discoveryStatus: 'not_found', oauth2CallbackPort: '3118', oauth2CallbackHost: '127.0.0.1' });
    expect(screen.getByText(/http:\/\/127\.0\.0\.1:3118\/callback/)).toBeInTheDocument();
  });

  it('preview do redirect URI reflete [::1]', () => {
    renderWith({ discoveryStatus: 'not_found', oauth2CallbackPort: '8080', oauth2CallbackHost: '[::1]' });
    expect(screen.getByText(/http:\/\/\[::1\]:8080\/callback/)).toBeInTheDocument();
  });

  it('não exibe preview quando porta está vazia', () => {
    renderWith({ discoveryStatus: 'not_found', oauth2CallbackPort: '' });
    expect(screen.queryByText(/Redirect URI:/)).not.toBeInTheDocument();
  });
});

describe('McpConnectionSection — Callback Host (discovered, no DCR)', () => {
  it('exibe Callback Host no avançado quando discovery sem DCR', () => {
    renderWith({
      discoveryStatus: 'found',
      discoveryRegistrationUrl: '',
    });
    expect(screen.getByLabelText('Callback Host')).toBeInTheDocument();
  });
});
