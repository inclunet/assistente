import { useId } from 'react';
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

const isHTTPTransport = (t: string) => t === 'streamable' || t === 'sse';

export function McpConnectionSection({
  transport,
  command,
  args,
  url,
  envText,
  enabled,
  autoConnect,
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
  const uid = useId();
  const discoveryLiveId = `${uid}-discovery-live`;
  const callbackHintId = `${uid}-callback-hint`;

  const hasDCR = discoveryStatus === 'found' && !!discoveryRegistrationUrl;
  const discoveredNoDCR = discoveryStatus === 'found' && !discoveryRegistrationUrl;
  const isManualMode = discoveryStatus === 'not_found';

  const discoveryLiveText = (() => {
    switch (discoveryStatus) {
      case 'loading':
        return 'Verificando configuração OAuth do servidor…';
      case 'found':
        return hasDCR
          ? `OAuth configurado automaticamente${discoveryResourceName ? ` (${discoveryResourceName})` : ''}. Client ID será registrado via DCR.`
          : `OAuth detectado${discoveryResourceName ? ` (${discoveryResourceName})` : ''}, mas sem registro dinâmico. Informe o Client ID.`;
      case 'not_found':
        return 'Metadados OAuth não detectados. Configure manualmente.';
      default:
        return '';
    }
  })();

  return (
    <section className="mcp-section" aria-labelledby="mcp-section-connection">
      <h3 id="mcp-section-connection">Conexão</h3>
      <div className="mcp-fields">
        {/* === stdio === */}
        {transport === 'stdio' && (
          <>
            <Input
              label="Comando"
              type="text"
              value={command}
              onChange={(e) => onCommandChange(e.target.value)}
              placeholder="ex: npx, node, python"
              required
              fullWidth
            />
            <Input
              label="Argumentos"
              type="text"
              value={args}
              onChange={(e) => onArgsChange(e.target.value)}
              placeholder="ex: -y @modelcontextprotocol/server-filesystem /home"
              hint="Separados por espaço"
              fullWidth
            />
            <Textarea
              label="Variáveis de ambiente"
              rows={4}
              value={envText}
              onChange={(e) => onEnvTextChange(e.target.value)}
              placeholder={"GITHUB_TOKEN=ghp_xxx\nNODE_ENV=production"}
              hint="Uma variável por linha no formato KEY=VALUE. Linhas começando com # são ignoradas."
              fullWidth
            />
          </>
        )}

        {/* === HTTP (remoto) === */}
        {isHTTPTransport(transport) && (
          <>
            <Input
              label="URL do servidor"
              type="url"
              value={url}
              onChange={(e) => onUrlChange(e.target.value)}
              onBlur={onUrlBlur}
              placeholder="https://example.com/mcp"
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
                Na conexão, o browser abrirá para autorizar. Credenciais serão armazenadas com criptografia no cofre do sistema.
                {' '}
                <button type="button" className="mcp-link-btn" onClick={onManualOverride}>
                  Configurar manualmente
                </button>
              </p>
            )}

            {/* Estado B: Discovery sem DCR — pedir Client ID */}
            {discoveredNoDCR && (
              <fieldset className="mcp-fieldset">
                <legend className="mcp-fieldset__legend">Credenciais</legend>
                <Input
                  label="Client ID"
                  type="text"
                  value={oauth2ClientId}
                  onChange={(e) => onOAuth2ClientIdChange(e.target.value)}
                  placeholder="ID do app registrado no provedor OAuth"
                  required
                  autoComplete="off"
                  fullWidth
                />
                <Input
                  label="Client Secret"
                  type="password"
                  value={oauth2ClientSecret}
                  onChange={(e) => onOAuth2ClientSecretChange(e.target.value)}
                  placeholder={hasExistingAuth ? '••••••••' : '(opcional para public clients)'}
                  hint={hasExistingAuth ? 'Deixe vazio para manter o atual.' : 'Necessário apenas se exigido pelo provedor.'}
                  autoComplete="off"
                  fullWidth
                />
                <p className="mcp-hint">
                  Na conexão, o browser abrirá para autorizar. Credenciais armazenadas com criptografia.
                  {' '}
                  <button type="button" className="mcp-link-btn" onClick={onManualOverride}>
                    Configurar manualmente
                  </button>
                </p>
              </fieldset>
            )}

            {/* Estado C: Discovery falhou — configuração manual completa */}
            {isManualMode && (
              <fieldset className="mcp-fieldset">
                <legend className="mcp-fieldset__legend">Autenticação</legend>

                <Select
                  label="Tipo de autenticação"
                  value={authType}
                  onChange={(e) => onAuthTypeChange(e.target.value)}
                  fullWidth
                  options={[
                    { value: 'none', label: 'Nenhuma' },
                    { value: 'bearer', label: 'Bearer Token (API Key)' },
                    { value: 'basic', label: 'Basic Auth (Usuário/Senha)' },
                    { value: 'oauth2_client_credentials', label: 'OAuth2 Client Credentials' },
                    { value: 'oauth2_pkce', label: 'OAuth2 Authorization Code (PKCE)' },
                  ]}
                />

                {hasExistingAuth && authType !== 'none' && authType !== 'oauth2_client_credentials' && authType !== 'oauth2_pkce' && (
                  <p className="mcp-hint mcp-hint--success" role="status">
                    Credencial configurada. Deixe os campos vazios para manter a atual.
                  </p>
                )}

                {authType === 'bearer' && (
                  <Input
                    label="Token"
                    type="password"
                    value={authToken}
                    onChange={(e) => onAuthTokenChange(e.target.value)}
                    placeholder={hasExistingAuth ? '••••••••' : 'sk-xxx ou ghp_xxx'}
                    autoComplete="off"
                    fullWidth
                  />
                )}

                {authType === 'basic' && (
                  <>
                    <Input
                      label="Usuário"
                      type="text"
                      value={authUsername}
                      onChange={(e) => onAuthUsernameChange(e.target.value)}
                      placeholder="username"
                      autoComplete="username"
                      fullWidth
                    />
                    <Input
                      label="Senha"
                      type="password"
                      value={authPassword}
                      onChange={(e) => onAuthPasswordChange(e.target.value)}
                      placeholder={hasExistingAuth ? '••••••••' : 'password'}
                      autoComplete="off"
                      fullWidth
                    />
                  </>
                )}

                {authType === 'oauth2_client_credentials' && (
                  <>
                    <Input
                      label="Client ID"
                      type="text"
                      value={oauth2ClientId}
                      onChange={(e) => onOAuth2ClientIdChange(e.target.value)}
                      placeholder="meu-app-id"
                      required
                      autoComplete="off"
                      fullWidth
                    />
                    <Input
                      label="Client Secret"
                      type="password"
                      value={oauth2ClientSecret}
                      onChange={(e) => onOAuth2ClientSecretChange(e.target.value)}
                      placeholder={hasExistingAuth ? '••••••••' : 'secret'}
                      hint={hasExistingAuth ? 'Deixe vazio para manter o atual.' : undefined}
                      required={!hasExistingAuth}
                      autoComplete="off"
                      fullWidth
                    />
                    <Input
                      label="Token URL"
                      type="url"
                      value={oauth2TokenUrl}
                      onChange={(e) => onOAuth2TokenUrlChange(e.target.value)}
                      placeholder="https://auth.example.com/oauth/token"
                      required
                      fullWidth
                    />
                    <Input
                      label="Scopes"
                      type="text"
                      value={oauth2Scopes}
                      onChange={(e) => onOAuth2ScopesChange(e.target.value)}
                      placeholder="read write"
                      hint="Separados por espaço"
                      fullWidth
                    />
                  </>
                )}

                {authType === 'oauth2_pkce' && (
                  <>
                    <Input
                      label="Client ID"
                      type="text"
                      value={oauth2ClientId}
                      onChange={(e) => onOAuth2ClientIdChange(e.target.value)}
                      placeholder="seu-app-id"
                      required
                      autoComplete="off"
                      fullWidth
                    />
                    <Input
                      label="Client Secret"
                      type="password"
                      value={oauth2ClientSecret}
                      onChange={(e) => onOAuth2ClientSecretChange(e.target.value)}
                      placeholder={hasExistingAuth ? '••••••••' : '(opcional para public clients)'}
                      hint={hasExistingAuth ? 'Deixe vazio para manter o atual.' : 'Necessário apenas se exigido pelo provedor.'}
                      autoComplete="off"
                      fullWidth
                    />
                    <Input
                      label="Token URL"
                      type="url"
                      value={oauth2TokenUrl}
                      onChange={(e) => onOAuth2TokenUrlChange(e.target.value)}
                      placeholder="https://auth.example.com/oauth/token"
                      required
                      fullWidth
                    />
                    <Input
                      label="Authorization URL"
                      type="url"
                      value={oauth2AuthUrl}
                      onChange={(e) => onOAuth2AuthUrlChange(e.target.value)}
                      placeholder="https://auth.example.com/authorize"
                      required
                      fullWidth
                    />
                    <Input
                      label="Scopes"
                      type="text"
                      value={oauth2Scopes}
                      onChange={(e) => onOAuth2ScopesChange(e.target.value)}
                      placeholder="openid profile"
                      hint="Separados por espaço"
                      fullWidth
                    />
                  </>
                )}

                {authType !== 'none' && (
                  <p className="mcp-hint" role="note">
                    Credenciais armazenadas com criptografia no cofre do sistema.
                  </p>
                )}
              </fieldset>
            )}
          </>
        )}

        {/* === Avançado === */}
        <details className="mcp-advanced">
          <summary>Avançado</summary>
          <div className="mcp-fields">
            {isHTTPTransport(transport) && (isManualMode || discoveredNoDCR) && (authType === 'oauth2_pkce' || discoveredNoDCR) && (
              <>
                <Select
                  label="Callback Host"
                  value={oauth2CallbackHost || 'localhost'}
                  onChange={(e) => onOAuth2CallbackHostChange(e.target.value)}
                  hint="Host usado no redirect_uri do OAuth."
                  fullWidth
                  options={[
                    { value: 'localhost', label: 'localhost (padrão)' },
                    { value: '127.0.0.1', label: '127.0.0.1 (RFC 8252)' },
                    { value: '[::1]', label: '[::1] (IPv6)' },
                  ]}
                />
                <Input
                  label="Porta do callback"
                  type="number"
                  value={oauth2CallbackPort}
                  onChange={(e) => onOAuth2CallbackPortChange(e.target.value)}
                  placeholder="(aleatória)"
                  fullWidth
                />
                {oauth2CallbackPort && (
                  <p id={callbackHintId} className="mcp-hint mcp-hint--success" role="status">
                    Redirect URI: <code>http://{oauth2CallbackHost || 'localhost'}:{oauth2CallbackPort}/callback</code>
                  </p>
                )}
              </>
            )}
            <fieldset className="mcp-fieldset">
              <legend className="mcp-fieldset__legend">Opções</legend>
              <div className="mcp-options">
                <Checkbox
                  label="Habilitado"
                  checked={enabled}
                  onChange={(e) => onEnabledChange(e.target.checked)}
                />
                <Checkbox
                  label="Conectar automaticamente no início"
                  checked={autoConnect}
                  onChange={(e) => onAutoConnectChange(e.target.checked)}
                />
              </div>
            </fieldset>
          </div>
        </details>
      </div>
    </section>
  );
}
