import { useId } from 'react';
import { useTranslation } from 'react-i18next';
import { Checkbox, Input, Textarea } from '../index';
import { Select } from '../index';

type DiscoveryStatus = 'idle' | 'loading' | 'found' | 'not_found';

interface McpConnectionSectionProps {
  transport: string;
  command: string;
  args: string;
  url: string;
  envText: string;
  enabled: boolean;
  autoConnect: boolean;
  preferBridge: boolean;
  authType: string;
  authToken: string;
  authUsername: string;
  authPassword: string;
  hasExistingAuth: boolean;
  oauth2ClientId: string;
  oauth2ClientSecret: string;
  oauth2TokenUrl: string;
  oauth2AuthUrl: string;
  oauth2Scopes: string;
  oauth2CallbackPort: string;
  oauth2CallbackHost: string;
  discoveryStatus: DiscoveryStatus;
  discoveredFields: Set<string>;
  discoveryResourceName: string;
  discoveryRegistrationUrl: string;
  onCommandChange: (value: string) => void;
  onArgsChange: (value: string) => void;
  onUrlChange: (value: string) => void;
  onEnvTextChange: (value: string) => void;
  onEnabledChange: (value: boolean) => void;
  onAutoConnectChange: (value: boolean) => void;
  onPreferBridgeChange: (value: boolean) => void;
  onAuthTypeChange: (value: string) => void;
  onAuthTokenChange: (value: string) => void;
  onAuthUsernameChange: (value: string) => void;
  onAuthPasswordChange: (value: string) => void;
  onOAuth2ClientIdChange: (value: string) => void;
  onOAuth2ClientSecretChange: (value: string) => void;
  onOAuth2TokenUrlChange: (value: string) => void;
  onOAuth2AuthUrlChange: (value: string) => void;
  onOAuth2ScopesChange: (value: string) => void;
  onOAuth2CallbackPortChange: (value: string) => void;
  onOAuth2CallbackHostChange: (value: string) => void;
  onUrlBlur: () => void;
  onManualOverride: () => void;
}

const isHTTPTransport = (transportKind: string) =>
  transportKind === 'streamable' || transportKind === 'sse';

export function McpConnectionSection({
  transport,
  command,
  args,
  url,
  envText,
  enabled,
  autoConnect,
  preferBridge,
  authType,
  authToken,
  authUsername,
  authPassword,
  hasExistingAuth,
  oauth2ClientId,
  oauth2ClientSecret,
  oauth2TokenUrl,
  oauth2AuthUrl,
  oauth2Scopes,
  oauth2CallbackPort,
  oauth2CallbackHost,
  discoveryStatus,
  discoveredFields: _discoveredFields,
  discoveryResourceName,
  discoveryRegistrationUrl,
  onCommandChange,
  onArgsChange,
  onUrlChange,
  onEnvTextChange,
  onEnabledChange,
  onAutoConnectChange,
  onPreferBridgeChange,
  onAuthTypeChange,
  onAuthTokenChange,
  onAuthUsernameChange,
  onAuthPasswordChange,
  onOAuth2ClientIdChange,
  onOAuth2ClientSecretChange,
  onOAuth2TokenUrlChange,
  onOAuth2AuthUrlChange,
  onOAuth2ScopesChange,
  onOAuth2CallbackPortChange,
  onOAuth2CallbackHostChange,
  onUrlBlur,
  onManualOverride,
}: McpConnectionSectionProps) {
  const { t } = useTranslation();
  const uid = useId();
  const discoveryLiveId = `${uid}-discovery-live`;
  const callbackHintId = `${uid}-callback-hint`;

  const resourceSuffix = discoveryResourceName ? ` (${discoveryResourceName})` : '';

  const hasDCR = discoveryStatus === 'found' && !!discoveryRegistrationUrl;
  const discoveredNoDCR = discoveryStatus === 'found' && !discoveryRegistrationUrl;
  const isManualMode = discoveryStatus === 'not_found';

  const discoveryLiveText = (() => {
    switch (discoveryStatus) {
      case 'loading':
        return t('mcp.connection.checkingOAuth', 'Verificando configuração OAuth do servidor…');
      case 'found':
        return hasDCR
          ? t('mcp.connection.oauthAutoConfiguredDCR', {
              defaultValue: `OAuth configurado automaticamente${resourceSuffix}. Client ID será registrado via DCR.`,
              resourceName: resourceSuffix,
            })
          : t('mcp.connection.oauthDetectedNoDCR', {
              defaultValue: `OAuth detectado${resourceSuffix}, mas sem registro dinâmico. Informe o Client ID.`,
              resourceName: resourceSuffix,
            });
      case 'not_found':
        return t('mcp.connection.oauthNotDetected', 'Metadados OAuth não detectados. Configure manualmente.');
      default:
        return '';
    }
  })();

  return (
    <section className="mcp-section" aria-labelledby="mcp-section-connection">
      <h3 id="mcp-section-connection">{t('mcp.connection.title', 'Conexão')}</h3>
      <div className="mcp-fields">
        {/* === stdio === */}
        {transport === 'stdio' && (
          <>
            <Input
              label={t('mcp.connection.command', 'Comando')}
              type="text"
              value={command}
              onChange={(e) => onCommandChange(e.target.value)}
              placeholder={t('mcp.connection.commandPlaceholder', 'ex: npx, node, python')}
              required
              fullWidth
            />
            <Input
              label={t('mcp.connection.args', 'Argumentos')}
              type="text"
              value={args}
              onChange={(e) => onArgsChange(e.target.value)}
              placeholder={t('mcp.connection.argsPlaceholder', 'ex: -y @modelcontextprotocol/server-filesystem /home')}
              hint={t('mcp.connection.argsSeparated', 'Separados por espaço')}
              fullWidth
            />
            <Textarea
              label={t('mcp.connection.envVars', 'Variáveis de ambiente')}
              rows={4}
              value={envText}
              onChange={(e) => onEnvTextChange(e.target.value)}
              placeholder={t(
                'mcp.connection.envExample',
                'GITHUB_TOKEN=ghp_xxx\nNODE_ENV=production',
              )}
              hint={t(
                'mcp.connection.envVarsHint',
                'Uma variável por linha no formato KEY=VALUE. Linhas começando com # são ignoradas.',
              )}
              fullWidth
            />
          </>
        )}

        {/* === HTTP (remoto) === */}
        {isHTTPTransport(transport) && (
          <>
            <Input
              label={t('mcp.connection.serverUrl', 'URL do servidor')}
              type="url"
              value={url}
              onChange={(e) => onUrlChange(e.target.value)}
              onBlur={onUrlBlur}
              placeholder={t('mcp.connection.serverUrlPlaceholder', 'https://example.com/mcp')}
              required
              fullWidth
            />

            <div
              id={discoveryLiveId}
              aria-live="polite"
              aria-atomic="true"
              className="mcp-discovery-live"
            >
              {discoveryLiveText}
            </div>

            {/* Estado A: DCR disponível — nada a preencher */}
            {hasDCR && (
              <p className="mcp-hint mcp-hint--success" role="status">
                {t(
                  'mcp.connection.browserAuthHint',
                  'Na conexão, o browser abrirá para autorizar. Credenciais serão armazenadas com criptografia no cofre do sistema.',
                )}{' '}
                <button type="button" className="mcp-link-btn" onClick={onManualOverride}>
                  {t('mcp.connection.configureManually', 'Configurar manualmente')}
                </button>
              </p>
            )}

            {/* Estado B: Discovery sem DCR — pedir Client ID */}
            {discoveredNoDCR && (
              <fieldset className="mcp-fieldset">
                <legend className="mcp-fieldset__legend">
                  {t('mcp.connection.credentials', 'Credenciais')}
                </legend>
                <Input
                  label={t('mcp.connection.clientId', 'Client ID')}
                  type="text"
                  value={oauth2ClientId}
                  onChange={(e) => onOAuth2ClientIdChange(e.target.value)}
                  placeholder={t(
                    'mcp.connection.clientIdPlaceholder',
                    'ID do app registrado no provedor OAuth',
                  )}
                  required
                  autoComplete="off"
                  fullWidth
                />
                <Input
                  label={t('mcp.connection.clientSecret', 'Client Secret')}
                  type="password"
                  value={oauth2ClientSecret}
                  onChange={(e) => onOAuth2ClientSecretChange(e.target.value)}
                  placeholder={
                    hasExistingAuth
                      ? t('mcp.connection.passwordMask', '••••••••')
                      : t('mcp.connection.clientSecretOptional', '(opcional para public clients)')
                  }
                  hint={
                    hasExistingAuth
                      ? t('mcp.connection.keepExisting', 'Deixe vazio para manter o atual.')
                      : t(
                          'mcp.connection.clientSecretHint',
                          'Necessário apenas se exigido pelo provedor.',
                        )
                  }
                  autoComplete="off"
                  fullWidth
                />
                <p className="mcp-hint">
                  {t(
                    'mcp.connection.browserAuthHintBrief',
                    'Na conexão, o browser abrirá para autorizar. Credenciais armazenadas com criptografia.',
                  )}{' '}
                  <button type="button" className="mcp-link-btn" onClick={onManualOverride}>
                    {t('mcp.connection.configureManually', 'Configurar manualmente')}
                  </button>
                </p>
              </fieldset>
            )}

            {/* Estado C: Discovery falhou — configuração manual completa */}
            {isManualMode && (
              <fieldset className="mcp-fieldset">
                <legend className="mcp-fieldset__legend">{t('mcp.connection.auth', 'Autenticação')}</legend>

                <Select
                  label={t('mcp.connection.authType', 'Tipo de autenticação')}
                  value={authType}
                  onChange={(e) => onAuthTypeChange(e.target.value)}
                  fullWidth
                  options={[
                    { value: 'none', label: t('mcp.connection.authNone', 'Nenhuma') },
                    { value: 'bearer', label: t('mcp.connection.authBearer', 'Bearer Token (API Key)') },
                    { value: 'basic', label: t('mcp.connection.authBasic', 'Basic Auth (Usuário/Senha)') },
                    {
                      value: 'oauth2_client_credentials',
                      label: t('mcp.connection.authClientCredentials', 'OAuth2 Client Credentials'),
                    },
                    { value: 'oauth2_pkce', label: t('mcp.connection.authPKCE', 'OAuth2 Authorization Code (PKCE)') },
                  ]}
                />

                {hasExistingAuth && authType !== 'none' && authType !== 'oauth2_client_credentials' && authType !== 'oauth2_pkce' && (
                  <p className="mcp-hint mcp-hint--success" role="status">
                    {t(
                      'mcp.connection.credentialConfigured',
                      'Credencial configurada. Deixe os campos vazios para manter a atual.',
                    )}
                  </p>
                )}

                {authType === 'bearer' && (
                  <Input
                    label={t('mcp.connection.token', 'Token')}
                    type="password"
                    value={authToken}
                    onChange={(e) => onAuthTokenChange(e.target.value)}
                    placeholder={
                      hasExistingAuth
                        ? t('mcp.connection.passwordMask', '••••••••')
                        : t('mcp.connection.tokenExample', 'sk-xxx ou ghp_xxx')
                    }
                    autoComplete="off"
                    fullWidth
                  />
                )}

                {authType === 'basic' && (
                  <>
                    <Input
                      label={t('mcp.connection.username', 'Usuário')}
                      type="text"
                      value={authUsername}
                      onChange={(e) => onAuthUsernameChange(e.target.value)}
                      placeholder={t('mcp.connection.inputUsernamePlaceholder', 'username')}
                      autoComplete="username"
                      fullWidth
                    />
                    <Input
                      label={t('mcp.connection.password', 'Senha')}
                      type="password"
                      value={authPassword}
                      onChange={(e) => onAuthPasswordChange(e.target.value)}
                      placeholder={
                        hasExistingAuth
                          ? t('mcp.connection.passwordMask', '••••••••')
                          : t('mcp.connection.inputPasswordPlaceholder', 'password')
                      }
                      autoComplete="off"
                      fullWidth
                    />
                  </>
                )}

                {authType === 'oauth2_client_credentials' && (
                  <>
                    <Input
                      label={t('mcp.connection.clientId', 'Client ID')}
                      type="text"
                      value={oauth2ClientId}
                      onChange={(e) => onOAuth2ClientIdChange(e.target.value)}
                      placeholder={t('mcp.connection.ccClientIdPlaceholder', 'meu-app-id')}
                      required
                      autoComplete="off"
                      fullWidth
                    />
                    <Input
                      label={t('mcp.connection.clientSecret', 'Client Secret')}
                      type="password"
                      value={oauth2ClientSecret}
                      onChange={(e) => onOAuth2ClientSecretChange(e.target.value)}
                      placeholder={
                        hasExistingAuth
                          ? t('mcp.connection.passwordMask', '••••••••')
                          : t('mcp.connection.ccSecretPlaceholder', 'secret')
                      }
                      hint={
                        hasExistingAuth
                          ? t('mcp.connection.keepExisting', 'Deixe vazio para manter o atual.')
                          : undefined
                      }
                      required={!hasExistingAuth}
                      autoComplete="off"
                      fullWidth
                    />
                    <Input
                      label={t('mcp.connection.tokenUrl', 'Token URL')}
                      type="url"
                      value={oauth2TokenUrl}
                      onChange={(e) => onOAuth2TokenUrlChange(e.target.value)}
                      placeholder={t(
                        'mcp.connection.oauthTokenUrlPlaceholder',
                        'https://auth.example.com/oauth/token',
                      )}
                      required
                      fullWidth
                    />
                    <Input
                      label={t('mcp.connection.scopes', 'Scopes')}
                      type="text"
                      value={oauth2Scopes}
                      onChange={(e) => onOAuth2ScopesChange(e.target.value)}
                      placeholder={t('mcp.connection.scopesPlaceholderCc', 'read write')}
                      hint={t('mcp.connection.argsSeparated', 'Separados por espaço')}
                      fullWidth
                    />
                  </>
                )}

                {authType === 'oauth2_pkce' && (
                  <>
                    <Input
                      label={t('mcp.connection.clientId', 'Client ID')}
                      type="text"
                      value={oauth2ClientId}
                      onChange={(e) => onOAuth2ClientIdChange(e.target.value)}
                      placeholder={t('mcp.connection.pkceClientIdPlaceholder', 'seu-app-id')}
                      required
                      autoComplete="off"
                      fullWidth
                    />
                    <Input
                      label={t('mcp.connection.clientSecret', 'Client Secret')}
                      type="password"
                      value={oauth2ClientSecret}
                      onChange={(e) => onOAuth2ClientSecretChange(e.target.value)}
                      placeholder={
                        hasExistingAuth
                          ? t('mcp.connection.passwordMask', '••••••••')
                          : t('mcp.connection.clientSecretOptional', '(opcional para public clients)')
                      }
                      hint={
                        hasExistingAuth
                          ? t('mcp.connection.keepExisting', 'Deixe vazio para manter o atual.')
                          : t(
                              'mcp.connection.clientSecretHint',
                              'Necessário apenas se exigido pelo provedor.',
                            )
                      }
                      autoComplete="off"
                      fullWidth
                    />
                    <Input
                      label={t('mcp.connection.tokenUrl', 'Token URL')}
                      type="url"
                      value={oauth2TokenUrl}
                      onChange={(e) => onOAuth2TokenUrlChange(e.target.value)}
                      placeholder={t(
                        'mcp.connection.oauthTokenUrlPlaceholder',
                        'https://auth.example.com/oauth/token',
                      )}
                      required
                      fullWidth
                    />
                    <Input
                      label={t('mcp.connection.authorizationUrl', 'Authorization URL')}
                      type="url"
                      value={oauth2AuthUrl}
                      onChange={(e) => onOAuth2AuthUrlChange(e.target.value)}
                      placeholder={t(
                        'mcp.connection.oauthAuthUrlPlaceholder',
                        'https://auth.example.com/authorize',
                      )}
                      required
                      fullWidth
                    />
                    <Input
                      label={t('mcp.connection.scopes', 'Scopes')}
                      type="text"
                      value={oauth2Scopes}
                      onChange={(e) => onOAuth2ScopesChange(e.target.value)}
                      placeholder={t('mcp.connection.scopesPlaceholderPkce', 'openid profile')}
                      hint={t('mcp.connection.argsSeparated', 'Separados por espaço')}
                      fullWidth
                    />
                  </>
                )}

                {authType !== 'none' && (
                  <p className="mcp-hint" role="note">
                    {t(
                      'mcp.connection.encryptedStorage',
                      'Credenciais armazenadas com criptografia no cofre do sistema.',
                    )}
                  </p>
                )}
              </fieldset>
            )}
          </>
        )}

        {/* === Avançado === */}
        <details className="mcp-advanced">
          <summary>{t('mcp.connection.advanced', 'Avançado')}</summary>
          <div className="mcp-fields">
            {isHTTPTransport(transport) && (isManualMode || discoveredNoDCR) && (authType === 'oauth2_pkce' || discoveredNoDCR) && (
              <>
                <Select
                  label={t('mcp.connection.callbackHost', 'Callback Host')}
                  value={oauth2CallbackHost || 'localhost'}
                  onChange={(e) => onOAuth2CallbackHostChange(e.target.value)}
                  hint={t(
                    'mcp.connection.callbackHostHint',
                    'Host usado no redirect_uri do OAuth.',
                  )}
                  fullWidth
                  options={[
                    {
                      value: 'localhost',
                      label: t('mcp.connection.callbackHostLocalhost', 'localhost (padrão)'),
                    },
                    {
                      value: '127.0.0.1',
                      label: t('mcp.connection.callbackHostIPv4', '127.0.0.1 (RFC 8252)'),
                    },
                    { value: '[::1]', label: t('mcp.connection.callbackHostIPv6', '[::1] (IPv6)') },
                  ]}
                />
                <Input
                  label={t('mcp.connection.callbackPort', 'Porta do callback')}
                  type="number"
                  value={oauth2CallbackPort}
                  onChange={(e) => onOAuth2CallbackPortChange(e.target.value)}
                  placeholder={t('mcp.connection.callbackPortRandom', '(aleatória)')}
                  fullWidth
                />
                {oauth2CallbackPort && (
                  <p id={callbackHintId} className="mcp-hint mcp-hint--success" role="status">
                    {t('mcp.connection.redirectUriLabel', 'Redirect URI:')}{' '}
                    <code>
                      http://{oauth2CallbackHost || 'localhost'}:{oauth2CallbackPort}/callback
                    </code>
                  </p>
                )}
              </>
            )}
            <fieldset className="mcp-fieldset">
              <legend className="mcp-fieldset__legend">{t('mcp.connection.options', 'Opções')}</legend>
              <div className="mcp-options">
                <Checkbox
                  label={t('mcp.connection.enabled', 'Habilitado')}
                  checked={enabled}
                  onChange={(e) => onEnabledChange(e.target.checked)}
                />
                <Checkbox
                  label={t('mcp.connection.autoConnect', 'Conectar automaticamente no início')}
                  checked={autoConnect}
                  onChange={(e) => onAutoConnectChange(e.target.checked)}
                />
                {isHTTPTransport(transport) && (
                  <Checkbox
                    label={t('mcp.connection.preferBridge', 'Usar bridge local (não enviar ao provider via MCP nativo)')}
                    checked={preferBridge}
                    onChange={(e) => onPreferBridgeChange(e.target.checked)}
                  />
                )}
              </div>
            </fieldset>
          </div>
        </details>
      </div>
    </section>
  );
}
