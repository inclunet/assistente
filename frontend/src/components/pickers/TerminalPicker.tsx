import { CodeOutlined } from '@ant-design/icons';
import { useTranslation } from 'react-i18next';
import { BasePicker } from './BasePicker';
import type { SessionInfo } from '../../store/terminalStore';

interface TerminalPickerProps {
  sessions: SessionInfo[];
  value?: string;
  disabled?: boolean;
  onChange: (sessionId: string) => void;
  onOpen?: () => void;
  onAnnounce?: (message: string) => void;
}

export function TerminalPicker({
  sessions,
  value,
  disabled,
  onChange,
  onOpen,
  onAnnounce,
}: TerminalPickerProps) {
  const { t } = useTranslation();
  const items = sessions.map((session) => ({
    value: session.id,
    label: session.name || session.id,
    sublabel: t('terminal.picker.itemDescription', {
      cwd: session.cwd,
      state: t(`terminal.states.${session.state}`),
    }),
  }));

  return (
    <BasePicker
      variant="toolbar"
      items={items}
      selected={value || ''}
      onSelect={(sessionId) => onChange(sessionId)}
      label={t('terminal.picker.label')}
      description={t('terminal.picker.description')}
      icon={<CodeOutlined />}
      placeholder={sessions.length > 0
        ? t('terminal.picker.placeholder')
        : t('terminal.picker.empty')}
      disabled={disabled}
      maxWidth="260px"
      onAnnounce={onAnnounce}
      onOpen={onOpen}
      showLoadingState={false}
      showEmptyState
      wrapCombobox={false}
    />
  );
}
