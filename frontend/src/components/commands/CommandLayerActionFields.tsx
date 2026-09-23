import { useEffect } from 'react';
import { useTranslation } from 'react-i18next';
import { Select } from '../ui';
import { Input } from '../ui/Input';
import type { CommandLayer, CommandSettingsRule, CommandSettingsScope } from '../../types/commandSettingsTypes';

interface Props {
  commandID: string;
  scope: CommandSettingsScope;
  layers: readonly CommandLayer[];
  rules: readonly CommandSettingsRule[];
  value: Record<string, unknown>;
  disabled?: boolean;
  onChange: (value: Record<string, unknown>) => void;
  onValidityChange: (valid: boolean) => void;
}

export function CommandLayerActionFields({ commandID, scope, layers, rules, value, disabled, onChange, onValidityChange }: Props) {
  const { t } = useTranslation();
  const back = commandID === 'layer.back';
  const candidates = rules.filter(rule => !rule.inherited && rule.enabled && rule.reviewStatus === 'active' &&
    (rule.mode === 'manual' || rule.mode === 'toggle') && rule.condition.clauses.length === 0 &&
    ['persistent', 'session', 'temporary'].includes(rule.lifecycle) &&
    layers.some(layer => layer.id === rule.layerId && !layer.builtin && layer.enabled));
  const ruleID = typeof value.rule_id === 'string' ? value.rule_id : '';
  const selected = value.scope === scope ? candidates.find(rule => rule.id === ruleID) : undefined;
  const temporary = selected?.lifecycle === 'temporary';
  const duration = value.duration_seconds;
  const valid = value.scope === scope && typeof value.rule_id === 'string' && Object.keys(value).every(key => ['scope', 'rule_id', 'duration_seconds'].includes(key)) &&
    (back ? ruleID === '' && duration === 0 : !!selected && Number.isInteger(duration) &&
      (temporary ? typeof duration === 'number' && duration >= 1 && duration <= 86400 : duration === 0));
  useEffect(() => { onValidityChange(valid); }, [valid, onValidityChange]);
  const update = (id: string, seconds: number) => onChange({ scope, rule_id: id, duration_seconds: seconds });
  return (
    <fieldset disabled={disabled}>
      <legend>{t('commandSettings.layerActionTarget')}</legend>
      <p>{t('commandSettings.layerActionOriginHelp')}</p>
      {back ? (
        <Select label={t('commandSettings.layerActionScope')} value={valid ? scope : ''}
          options={[{ value: '', label: t('commandSettings.layerActionChoose') }, { value: scope, label: t(`commandSettings.scope.${scope}`) }]}
          onChange={event => { if (event.target.value === scope) update('', 0); }} />
      ) : (
        <>
          <Select label={t('commandSettings.layerActionTarget')} value={selected ? ruleID : ruleID ? 'unavailable' : ''}
            options={[
              { value: '', label: t('commandSettings.layerActionChoose') },
              ...(ruleID && !selected ? [{ value: 'unavailable', label: t('commandSettings.unavailableConditionValue'), disabled: true }] : []),
              ...candidates.map(rule => ({ value: rule.id, label: `${layers.find(layer => layer.id === rule.layerId)!.name} — ${t(`commandSettings.rules.lifecycles.${rule.lifecycle}`)} — ${t(rule.mode === 'toggle' ? 'commandSettings.manualToggle' : 'commandSettings.rules.modes.manual')}` })),
            ]}
            onChange={event => {
              const rule = candidates.find(candidate => candidate.id === event.target.value);
              if (rule) update(rule.id, rule.lifecycle === 'temporary' ? 60 : 0);
            }} />
          {temporary && <Input type="number" min={1} max={86400} label={t('commandSettings.manualDurationSeconds')}
            value={typeof duration === 'number' ? duration : ''}
            onChange={event => update(ruleID, event.target.value === '' ? 0 : Number(event.target.value))} />}
        </>
      )}
    </fieldset>
  );
}
