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
        return t('mcp.connection.checkingOAuth');
      case 'found':
        return hasDCR
          ? t('mcp.connection.oauthAutoConfiguredDCR', {
              resourceName: resourceSuffix,
            })
          : t('mcp.connection.oauthDetectedNoDCR', {
              resourceName: resourceSuffix,
            });
      case 'not_found':
        return t('mcp.connection.oauthNotDetected');
      default:
        return '';
    }
  })();

  return (
    <section className="mcp-section" aria-labelledby="mcp-section-connection">
      <h3 id="mcp-section-connection">{t('mcp.connection.title')}</h3>
      <div className="mcp-fields">
        {/* === stdio === */}
        {transport === 'stdio' && (
          <>
            <Input
              label={t('mcp.connection.command')}
              type="text"
              value={command}
              onChange={(e) => onCommandChange(e.target.value)}
              placeholder={t('mcp.connection.commandPlaceholder')}
              required
              fullWidth
            />
            <Input
              label={t('mcp.connection.args')}
              type="text"
              value={args}
              onChange={(e) => onArgsChange(e.target.value)}
              placeholder={t('mcp.connection.argsPlaceholder')}
              hint={t('mcp.connection.argsSeparated')}
              fullWidth
            />
            <Textarea
              label={t('mcp.connection.envVars')}
              rows={4}
              value={envText}
              onChange={(e) => onEnvTextChange(e.target.value)}
              placeholder={t('mcp.connection.envExample')}
              hint={t('mcp.connection.envVarsHint')}
              fullWidth
            />
          </>
        )}

        {/* === HTTP (remoto) === */}
        {isHTTPTransport(transport) && (
          <>
            <Input
              label={t('mcp.connection.serverUrl')}
              type="url"
              value={url}
              onChange={(e) => onUrlChange(e.target.value)}
              onBlur={onUrlBlur}
              placeholder={t('mcp.connection.serverUrlPlaceholder')}
              required
              fullWidth
            />

            <div
              id={discoveryLiveId}


              className="mcp-discovery-live"
            >
              {discoveryLiveText}
            </div>

            {/* Estado A: DCR disponível — nada a preencher */}
            {hasDCR && (
              <p className="mcp-hint mcp-hint--success">
                {t('mcp.connection.browserAuthHint')}{' '}
                <button type="button" className="mcp-link-btn" onClick={onManualOverride}>
                  {t('mcp.connection.configureManually')}
                </button>
              </p>
            )}

            {/* Estado B: Discovery sem DCR — pedir Client ID */}
            {discoveredNoDCR && (
              <fieldset className="mcp-fieldset">
                <legend className="mcp-fieldset__legend">
                  {t('mcp.connection.credentials')}
                </legend>
                <Input
                  label={t('mcp.connection.clientId')}
                  type="text"
                  value={oauth2ClientId}
                  onChange={(e) => onOAuth2ClientIdChange(e.target.value)}
                  placeholder={t('mcp.connection.clientIdPlaceholder')}
                  required
                  autoComplete="off"
                  fullWidth
                />
                <Input
                  label={t('mcp.connection.clientSecret')}
                  type="password"
                  value={oauth2ClientSecret}
                  onChange={(e) => onOAuth2ClientSecretChange(e.target.value)}
                  placeholder={
                    hasExistingAuth
                      ? t('mcp.connection.passwordMask')
                      : t('mcp.connection.clientSecretOptional')
                  }
                  hint={
                    hasExistingAuth
                      ? t('mcp.connection.keepExisting')
                      : t('mcp.connection.clientSecretHint')
                  }
                  autoComplete="off"
                  fullWidth
                />
                <p className="mcp-hint">
                  {t('mcp.connection.browserAuthHintBrief')}{' '}
                  <button type="button" className="mcp-link-btn" onClick={onManualOverride}>
                    {t('mcp.connection.configureManually')}
                  </button>
                </p>
              </fieldset>
            )}

            {/* Estado C: Discovery falhou — configuração manual completa */}
            {isManualMode && (
              <fieldset className="mcp-fieldset">
                <legend className="mcp-fieldset__legend">{t('mcp.connection.auth')}</legend>

                <Select
                  label={t('mcp.connection.authType')}
                  value={authType}
                  onChange={(e) => onAuthTypeChange(e.target.value)}
                  fullWidth
                  options={[
                    { value: 'none', label: t('mcp.connection.authNone') },
                    { value: 'bearer', label: t('mcp.connection.authBearer') },
                    { value: 'basic', label: t('mcp.connection.authBasic') },
                    {
                      value: 'oauth2_client_credentials',
                      label: t('mcp.connection.authClientCredentials'),
                    },
                    { value: 'oauth2_pkce', label: t('mcp.connection.authPKCE') },
                  ]}
                />

                {hasExistingAuth && authType !== 'none' && authType !== 'oauth2_client_credentials' && authType !== 'oauth2_pkce' && (
                  <p className="mcp-hint mcp-hint--success">
                    {t('mcp.connection.credentialConfigured')}
                  </p>
                )}

                {authType === 'bearer' && (
                  <Input
                    label={t('mcp.connection.token')}
                    type="password"
                    value={authToken}
                    onChange={(e) => onAuthTokenChange(e.target.value)}
                    placeholder={
                      hasExistingAuth
                        ? t('mcp.connection.passwordMask')
                        : t('mcp.connection.tokenExample')
                    }
                    autoComplete="off"
                    fullWidth
                  />
                )}

                {authType === 'basic' && (
                  <>
                    <Input
                      label={t('mcp.connection.username')}
                      type="text"
                      value={authUsername}
                      onChange={(e) => onAuthUsernameChange(e.target.value)}
                      placeholder={t('mcp.connection.inputUsernamePlaceholder')}
                      autoComplete="username"
                      fullWidth
                    />
                    <Input
                      label={t('mcp.connection.password')}
                      type="password"
                      value={authPassword}
                      onChange={(e) => onAuthPasswordChange(e.target.value)}
                      placeholder={
                        hasExistingAuth
                          ? t('mcp.connection.passwordMask')
                          : t('mcp.connection.inputPasswordPlaceholder')
                      }
                      autoComplete="off"
                      fullWidth
                    />
                  </>
                )}

                {authType === 'oauth2_client_credentials' && (
                  <>
                    <Input
                      label={t('mcp.connection.clientId')}
                      type="text"
                      value={oauth2ClientId}
                      onChange={(e) => onOAuth2ClientIdChange(e.target.value)}
                      placeholder={t('mcp.connection.ccClientIdPlaceholder')}
                      required
                      autoComplete="off"
                      fullWidth
                    />
                    <Input
                      label={t('mcp.connection.clientSecret')}
                      type="password"
                      value={oauth2ClientSecret}
                      onChange={(e) => onOAuth2ClientSecretChange(e.target.value)}
                      placeholder={
                        hasExistingAuth
                          ? t('mcp.connection.passwordMask')
                          : t('mcp.connection.ccSecretPlaceholder')
                      }
                      hint={
                        hasExistingAuth
                          ? t('mcp.connection.keepExisting')
                          : undefined
                      }
                      required={!hasExistingAuth}
                      autoComplete="off"
                      fullWidth
                    />
                    <Input
                      label={t('mcp.connection.tokenUrl')}
                      type="url"
                      value={oauth2TokenUrl}
                      onChange={(e) => onOAuth2TokenUrlChange(e.target.value)}
                      placeholder={t('mcp.connection.oauthTokenUrlPlaceholder')}
                      required
                      fullWidth
                    />
                    <Input
                      label={t('mcp.connection.scopes')}
                      type="text"
                      value={oauth2Scopes}
                      onChange={(e) => onOAuth2ScopesChange(e.target.value)}
                      placeholder={t('mcp.connection.scopesPlaceholderCc')}
                      hint={t('mcp.connection.argsSeparated')}
                      fullWidth
                    />
                  </>
                )}

                {authType === 'oauth2_pkce' && (
                  <>
                    <Input
                      label={t('mcp.connection.clientId')}
                      type="text"
                      value={oauth2ClientId}
                      onChange={(e) => onOAuth2ClientIdChange(e.target.value)}
                      placeholder={t('mcp.connection.pkceClientIdPlaceholder')}
                      required
                      autoComplete="off"
                      fullWidth
                    />
                    <Input
                      label={t('mcp.connection.clientSecret')}
                      type="password"
                      value={oauth2ClientSecret}
                      onChange={(e) => onOAuth2ClientSecretChange(e.target.value)}
                      placeholder={
                        hasExistingAuth
                          ? t('mcp.connection.passwordMask')
                          : t('mcp.connection.clientSecretOptional')
                      }
                      hint={
                        hasExistingAuth
                          ? t('mcp.connection.keepExisting')
                          : t('mcp.connection.clientSecretHint')
                      }
                      autoComplete="off"
                      fullWidth
                    />
                    <Input
                      label={t('mcp.connection.tokenUrl')}
                      type="url"
                      value={oauth2TokenUrl}
                      onChange={(e) => onOAuth2TokenUrlChange(e.target.value)}
                      placeholder={t('mcp.connection.oauthTokenUrlPlaceholder')}
                      required
                      fullWidth
                    />
                    <Input
                      label={t('mcp.connection.authorizationUrl')}
                      type="url"
                      value={oauth2AuthUrl}
                      onChange={(e) => onOAuth2AuthUrlChange(e.target.value)}
                      placeholder={t('mcp.connection.oauthAuthUrlPlaceholder')}
                      required
                      fullWidth
                    />
                    <Input
                      label={t('mcp.connection.scopes')}
                      type="text"
                      value={oauth2Scopes}
                      onChange={(e) => onOAuth2ScopesChange(e.target.value)}
                      placeholder={t('mcp.connection.scopesPlaceholderPkce')}
                      hint={t('mcp.connection.argsSeparated')}
                      fullWidth
                    />
                  </>
                )}

                {authType !== 'none' && (
                  <p className="mcp-hint" role="note">
                    {t('mcp.connection.encryptedStorage')}
                  </p>
                )}
              </fieldset>
            )}
          </>
        )}

        {/* === Avançado === */}
        <details className="mcp-advanced">
          <summary>{t('mcp.connection.advanced')}</summary>
          <div className="mcp-fields">
            {isHTTPTransport(transport) && (isManualMode || discoveredNoDCR) && (authType === 'oauth2_pkce' || discoveredNoDCR) && (
              <>
                <Select
                  label={t('mcp.connection.callbackHost')}
                  value={oauth2CallbackHost || 'localhost'}
                  onChange={(e) => onOAuth2CallbackHostChange(e.target.value)}
                  hint={t('mcp.connection.callbackHostHint')}
                  fullWidth
                  options={[
                    {
                      value: 'localhost',
                      label: t('mcp.connection.callbackHostLocalhost'),
                    },
                    {
                      value: '127.0.0.1',
                      label: t('mcp.connection.callbackHostIPv4'),
                    },
                    { value: '[::1]', label: t('mcp.connection.callbackHostIPv6') },
                  ]}
                />
                <Input
                  label={t('mcp.connection.callbackPort')}
                  type="number"
                  value={oauth2CallbackPort}
                  onChange={(e) => onOAuth2CallbackPortChange(e.target.value)}
                  placeholder={t('mcp.connection.callbackPortRandom')}
                  fullWidth
                />
                {oauth2CallbackPort && (
                  <p id={callbackHintId} className="mcp-hint mcp-hint--success">
                    {t('mcp.connection.redirectUriLabel')}{' '}
                    <code>
                      http://{oauth2CallbackHost || 'localhost'}:{oauth2CallbackPort}/callback
                    </code>
                  </p>
                )}
              </>
            )}
            <fieldset className="mcp-fieldset">
              <legend className="mcp-fieldset__legend">{t('mcp.connection.options')}</legend>
              <div className="mcp-options">
                <Checkbox
                  label={t('mcp.connection.enabled')}
                  checked={enabled}
                  onChange={(e) => onEnabledChange(e.target.checked)}
                />
                <Checkbox
                  label={t('mcp.connection.autoConnect')}
                  checked={autoConnect}
                  onChange={(e) => onAutoConnectChange(e.target.checked)}
                />
                {isHTTPTransport(transport) && (
                  <Checkbox
                    label={t('mcp.connection.preferBridge')}
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
