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

const isOAuth2Type = (t: string) =>
  t === 'oauth2_client_credentials' || t === 'oauth2_pkce';

function discoveryMessage(
  status: DiscoveryStatus,
  resourceName: string,
): string {
  switch (status) {
    case 'loading':
      return 'Verificando configuração OAuth do servidor…';
    case 'found':
      return `Configuração OAuth detectada automaticamente${resourceName ? ` (${resourceName})` : ''}.`;
    case 'not_found':
      return 'Servidor não expõe metadados OAuth. Configure manualmente se necessário.';
    default:
      return '';
  }
}

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
  discoveredFields,
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
  const isDiscovered = (field: string) => discoveredFields.has(field);
  const hasDiscovery = discoveredFields.size > 0;
  const uid = useId();
  const discoveryLiveId = `${uid}-discovery-live`;
  const authHintId = `${uid}-auth-hint`;
  const callbackHintId = `${uid}-callback-hint`;
  const pkceHintId = `${uid}-pkce-hint`;

  return (
    <section className="mcp-section" aria-labelledby="mcp-section-connection">
      <h3 id="mcp-section-connection">Conexão</h3>
      <div className="mcp-fields">
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
          </>
        )}

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

            {/* Live region persistente — conteúdo muda, elemento não é recriado */}
            <div
              id={discoveryLiveId}
              aria-live="polite"
              aria-atomic="true"
              className="mcp-discovery-live"
            >
              {discoveryMessage(discoveryStatus, discoveryResourceName)}
            </div>

            <fieldset className="mcp-fieldset">
              <legend className="mcp-fieldset__legend">Autenticação</legend>

              <Select
                label="Tipo de autenticação"
                value={authType}
                onChange={(e) => onAuthTypeChange(e.target.value)}
                disabled={isDiscovered('authType')}
                hint={isDiscovered('authType') ? 'Detectado automaticamente. Use "Editar manualmente" para alterar.' : undefined}
                fullWidth
                options={[
                  { value: 'none', label: 'Nenhuma' },
                  { value: 'bearer', label: 'Bearer Token (API Key)' },
                  { value: 'basic', label: 'Basic Auth (Usuário/Senha)' },
                  { value: 'oauth2_client_credentials', label: 'OAuth2 Client Credentials' },
                  { value: 'oauth2_pkce', label: 'OAuth2 Authorization Code (PKCE)' },
                ]}
              />

              {discoveryStatus === 'found' && hasDiscovery && (
                <p className="mcp-hint mcp-hint--success">
                  <button
                    type="button"
                    className="mcp-link-btn"
                    onClick={onManualOverride}
                    aria-label="Descartar configuração automática e editar campos OAuth manualmente"
                  >
                    Editar manualmente
                  </button>
                </p>
              )}

              {hasExistingAuth && authType !== 'none' && !isOAuth2Type(authType) && (
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
                    hint={hasExistingAuth ? 'Client secret configurado. Deixe vazio para manter o atual.' : undefined}
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
                    readOnly={isDiscovered('oauth2TokenUrl')}
                    className={isDiscovered('oauth2TokenUrl') ? 'mcp-input--discovered' : undefined}
                    hint={isDiscovered('oauth2TokenUrl') ? 'Preenchido automaticamente via discovery.' : undefined}
                    fullWidth
                  />
                  <Input
                    label="Scopes"
                    type="text"
                    value={oauth2Scopes}
                    onChange={(e) => onOAuth2ScopesChange(e.target.value)}
                    placeholder="read write"
                    readOnly={isDiscovered('oauth2Scopes')}
                    className={isDiscovered('oauth2Scopes') ? 'mcp-input--discovered' : undefined}
                    hint={isDiscovered('oauth2Scopes') ? 'Preenchido automaticamente via discovery. Separados por espaço.' : 'Separados por espaço'}
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
                    placeholder={discoveryRegistrationUrl ? '(será registrado automaticamente via DCR)' : 'seu-app-id'}
                    required={!discoveryRegistrationUrl}
                    hint={
                      discoveryRegistrationUrl && !oauth2ClientId
                        ? 'Servidor suporta registro dinâmico (RFC 7591). O Client ID será obtido automaticamente.'
                        : !discoveryRegistrationUrl && !oauth2ClientId
                          ? 'Sem registro dinâmico. Informe um Client ID pré-registrado no provedor OAuth.'
                          : undefined
                    }
                    autoComplete="off"
                    fullWidth
                  />
                  <Select
                    label="Callback Host"
                    value={oauth2CallbackHost || 'localhost'}
                    onChange={(e) => onOAuth2CallbackHostChange(e.target.value)}
                    hint="Host usado no redirect_uri do OAuth. Use localhost para compatibilidade com a maioria dos servidores MCP."
                    fullWidth
                    options={[
                      { value: 'localhost', label: 'localhost (padrão — compatível com Claude/Slack)' },
                      { value: '127.0.0.1', label: '127.0.0.1 (RFC 8252/OAuth 2.1 — compatível com Snowflake)' },
                      { value: '[::1]', label: '[::1] (IPv6 loopback)' },
                    ]}
                  />
                  <Input
                    label="Client Secret"
                    type="password"
                    value={oauth2ClientSecret}
                    onChange={(e) => onOAuth2ClientSecretChange(e.target.value)}
                    placeholder={hasExistingAuth ? '••••••••' : '(opcional para public clients)'}
                    hint={hasExistingAuth ? 'Secret configurado. Deixe vazio para manter o atual.' : 'Necessário apenas se o provedor OAuth exigir.'}
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
                    readOnly={isDiscovered('oauth2TokenUrl')}
                    className={isDiscovered('oauth2TokenUrl') ? 'mcp-input--discovered' : undefined}
                    hint={isDiscovered('oauth2TokenUrl') ? 'Preenchido automaticamente via discovery.' : undefined}
                    fullWidth
                  />
                  <Input
                    label="Authorization URL"
                    type="url"
                    value={oauth2AuthUrl}
                    onChange={(e) => onOAuth2AuthUrlChange(e.target.value)}
                    placeholder="https://auth.example.com/authorize"
                    required
                    readOnly={isDiscovered('oauth2AuthUrl')}
                    className={isDiscovered('oauth2AuthUrl') ? 'mcp-input--discovered' : undefined}
                    hint={isDiscovered('oauth2AuthUrl') ? 'Preenchido automaticamente via discovery.' : undefined}
                    fullWidth
                  />
                  <Input
                    label="Scopes"
                    type="text"
                    value={oauth2Scopes}
                    onChange={(e) => onOAuth2ScopesChange(e.target.value)}
                    placeholder="openid profile"
                    readOnly={isDiscovered('oauth2Scopes')}
                    className={isDiscovered('oauth2Scopes') ? 'mcp-input--discovered' : undefined}
                    hint={isDiscovered('oauth2Scopes') ? 'Preenchido automaticamente via discovery. Separados por espaço.' : 'Separados por espaço'}
                    fullWidth
                  />
                  <Input
                    label="Porta do callback"
                    type="number"
                    value={oauth2CallbackPort}
                    onChange={(e) => onOAuth2CallbackPortChange(e.target.value)}
                    placeholder="(aleatória)"
                    aria-describedby={oauth2CallbackPort ? callbackHintId : pkceHintId}
                    fullWidth
                  />
                  {oauth2CallbackPort && (
                    <p id={callbackHintId} className="mcp-hint mcp-hint--success" role="status">
                      Redirect URI: <code>http://{oauth2CallbackHost || 'localhost'}:{oauth2CallbackPort}/callback</code>
                      {' '}&mdash; registre este URI no provedor OAuth.
                    </p>
                  )}
                  <p id={pkceHintId} className="mcp-hint">
                    Na conexão, o browser abrirá para autorizar.
                    {!discoveryRegistrationUrl && ' O provedor não suporta DCR — informe Client ID e Secret do seu app OAuth.'}
                    {!oauth2CallbackPort && ' Preencha a porta se o provedor exigir redirect_uri exato.'}
                  </p>
                </>
              )}

              {authType !== 'none' && (
                <p id={authHintId} className="mcp-hint" role="note">
                  Credenciais armazenadas com criptografia no cofre do sistema.
                </p>
              )}
            </fieldset>
          </>
        )}

        {transport === 'stdio' && (
          <Textarea
            label="Variáveis de ambiente"
            rows={4}
            value={envText}
            onChange={(e) => onEnvTextChange(e.target.value)}
            placeholder={"GITHUB_TOKEN=ghp_xxx\nNODE_ENV=production"}
            hint="Uma variável por linha no formato KEY=VALUE. Linhas começando com # são ignoradas."
            fullWidth
          />
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
    </section>
  );
}
