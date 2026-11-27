import { useTranslation } from 'react-i18next';
import { Input } from './Input';

interface Props {
  scope: 'profile' | 'provider';
  value?: number;
  onChange: (seconds: number) => void;
  disabled?: boolean;
  testId?: string;
}

export function StreamIdleTimeoutInput({ scope, value, onChange, disabled, testId }: Props) {
  const { t } = useTranslation();
  return (
    <Input
      type="number"
      label={t('providerForm.streamIdleTimeout')}
      hint={t(scope === 'profile' ? 'providerForm.streamIdleProfileHint' : 'providerForm.streamIdleProviderHint')}
      value={value || ''}
      min={scope === 'profile' ? 10 : 1}
      max={scope === 'provider' ? 86400 : undefined}
      onChange={event => onChange(Number(event.target.value))}
      disabled={disabled}
      data-testid={testId}
    />
  );
}
