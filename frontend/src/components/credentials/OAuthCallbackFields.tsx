import { useId } from 'react';
import { useTranslation } from 'react-i18next';
import { Input, Select } from '../index';

interface OAuthCallbackFieldsProps {
  oauth2CallbackHost: string;
  oauth2CallbackPort: string;
  onOAuth2CallbackHostChange: (value: string) => void;
  onOAuth2CallbackPortChange: (value: string) => void;
}

export function OAuthCallbackFields({
  oauth2CallbackHost,
  oauth2CallbackPort,
  onOAuth2CallbackHostChange,
  onOAuth2CallbackPortChange,
}: OAuthCallbackFieldsProps) {
  const { t } = useTranslation();
  const callbackHintId = useId();
  return (
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
        <p id={callbackHintId} className="credentials-page__hint">
          {t('mcp.connection.redirectUriLabel')}{' '}
          <code>
            http://{oauth2CallbackHost || 'localhost'}:{oauth2CallbackPort}/callback
          </code>
        </p>
      )}
    </>
  );
}
