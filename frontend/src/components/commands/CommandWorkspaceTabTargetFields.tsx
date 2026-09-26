import { useEffect, useRef } from 'react';
import { useTranslation } from 'react-i18next';
import { Input, Select } from '../ui';
import { useAnnouncer } from '../../hooks/useAnnouncer';
import type { WorkspaceTab } from '../../store/workspaceStore';

interface Props {
  workspaceID: string;
  workspaceName: string;
  tabs: readonly WorkspaceTab[];
  value: Record<string, unknown>;
  disabled?: boolean;
  onChange: (value: Record<string, unknown>) => void;
  onValidityChange: (valid: boolean) => void;
}

const KEYS = ['workspace_id', 'target_mode', 'position', 'tab_id'];

function isValidTarget(value: Record<string, unknown>, workspaceID: string, tabs: readonly WorkspaceTab[]): boolean {
  if (!workspaceID || value.workspace_id !== workspaceID || Object.keys(value).some(key => !KEYS.includes(key))) return false;
  if (value.target_mode === 'position') {
    return Object.keys(value).every(key => ['workspace_id', 'target_mode', 'position'].includes(key)) &&
      Number.isInteger(value.position) && (value.position as number) >= 1;
  }
  if (value.target_mode === 'specific') {
    return Object.keys(value).every(key => ['workspace_id', 'target_mode', 'tab_id'].includes(key)) &&
      typeof value.tab_id === 'string' && tabs.some(tab => tab.id === value.tab_id);
  }
  return false;
}

export function CommandWorkspaceTabTargetFields({ workspaceID, workspaceName, tabs, value, disabled, onChange, onValidityChange }: Props) {
  const { t } = useTranslation();
  const { announce } = useAnnouncer();
  const announcedWorkspaceMismatch = useRef<string | null>(null);
  const valid = isValidTarget(value, workspaceID, tabs);
  const hasWorkspaceMismatch = typeof value.workspace_id === 'string' && value.workspace_id !== workspaceID;
  useEffect(() => { onValidityChange(valid); }, [valid, onValidityChange]);
  useEffect(() => {
    if (!hasWorkspaceMismatch) {
      announcedWorkspaceMismatch.current = null;
      return;
    }
    const mismatch = `${value.workspace_id}\u0000${workspaceID}`;
    if (announcedWorkspaceMismatch.current === mismatch) return;
    announcedWorkspaceMismatch.current = mismatch;
    announce(t('commandSettings.tabTarget.otherWorkspace'));
  }, [announce, hasWorkspaceMismatch, t, value.workspace_id, workspaceID]);

  return (
    <fieldset disabled={disabled}>
      <legend>{t('commandSettings.tabTarget.title')}</legend>
      <p>{workspaceName
        ? t('commandSettings.tabTarget.workspace', { name: workspaceName })
        : t('commandSettings.tabTarget.noWorkspace')}</p>
      <Select
        label={t('commandSettings.tabTarget.mode')}
        value={value.target_mode === 'position' || value.target_mode === 'specific' ? value.target_mode : ''}
        options={[
          { value: '', label: t('commandSettings.tabTarget.chooseMode'), disabled: true },
          { value: 'position', label: t('commandSettings.tabTarget.byPosition') },
          { value: 'specific', label: t('commandSettings.tabTarget.bySpecificTab') },
        ]}
        onChange={event => {
          const target_mode = event.target.value;
          if (target_mode === 'position' || target_mode === 'specific') onChange({ workspace_id: workspaceID, target_mode });
        }}
      />
      {value.target_mode === 'position' && (
        <Input
          type="number"
          min={1}
          step={1}
          label={t('commandSettings.tabTarget.position')}
          value={typeof value.position === 'number' ? value.position : ''}
          onChange={event => onChange({ workspace_id: workspaceID, target_mode: 'position', position: event.target.value === '' ? 0 : Number(event.target.value) })}
        />
      )}
      {value.target_mode === 'specific' && (
        <Select
          label={t('commandSettings.tabTarget.specificTab')}
          value={typeof value.tab_id === 'string' ? value.tab_id : ''}
          options={[
            { value: '', label: t('commandSettings.tabTarget.chooseTab'), disabled: true },
            ...(typeof value.tab_id === 'string' && value.tab_id && !tabs.some(tab => tab.id === value.tab_id)
              ? [{ value: value.tab_id, label: t('commandSettings.tabTarget.unavailableTab'), disabled: true }]
              : []),
            ...tabs.map(tab => ({ value: tab.id, label: tab.title })),
          ]}
          onChange={event => {
            const tab = tabs.find(candidate => candidate.id === event.target.value);
            if (tab) onChange({ workspace_id: workspaceID, target_mode: 'specific', tab_id: tab.id });
          }}
        />
      )}
    </fieldset>
  );
}
