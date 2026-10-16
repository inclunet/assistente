import { useTranslation } from 'react-i18next';
import { Input, Select } from '../index';

export interface OAuthClientFieldsProps {
  authType: string;
  hasExistingAuth: boolean;
  oauth2ClientId: string;
  oauth2ClientSecret: string;
  oauth2TokenUrl: string;
  oauth2AuthUrl: string;
  oauth2Scopes: string;
  onOAuth2ClientIdChange: (value: string) => void;
  onOAuth2ClientSecretChange: (value: string) => void;
  onOAuth2TokenUrlChange: (value: string) => void;
  onOAuth2AuthUrlChange: (value: string) => void;
  onOAuth2ScopesChange: (value: string) => void;
}

// OAuth client configuration belongs to the credential editor; consumers only
// provide the draft and commit it through their existing atomic vault workflow.
export function OAuthClientFields({
  authType,
  hasExistingAuth,
  oauth2ClientId,
  oauth2ClientSecret,
  oauth2TokenUrl,
  oauth2AuthUrl,
  oauth2Scopes,
  onOAuth2ClientIdChange,
  onOAuth2ClientSecretChange,
  onOAuth2TokenUrlChange,
  onOAuth2AuthUrlChange,
  onOAuth2ScopesChange,
}: OAuthClientFieldsProps) {
  const { t } = useTranslation();
  return (
    <>
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
            hint={hasExistingAuth ? t('mcp.connection.keepExisting') : undefined}
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
    </>
  );
}

export function OAuthTokenAuthField({
  authType,
  value,
  onChange,
}: {
  authType: string;
  value: string;
  onChange?: (value: string) => void;
}) {
  const { t } = useTranslation();
  return (
    <Select
      label={t('mcp.connection.tokenAuthMethod')}
      value={value}
      onChange={(e) => onChange?.(e.target.value)}
      options={[
        ...(authType === 'oauth2_pkce'
          ? [{ value: 'none', label: t('mcp.connection.tokenAuthNone') }]
          : []),
        { value: 'client_secret_post', label: t('mcp.connection.tokenAuthPost') },
        { value: 'client_secret_basic', label: t('mcp.connection.tokenAuthBasic') },
      ]}
    />
  );
}
