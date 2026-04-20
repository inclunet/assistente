import { useCallback, useEffect, useRef } from 'react';
import { useTranslation } from 'react-i18next';
import { Input } from '../../ui/Input';
import { Select, SelectOption } from '../../ui/Select';
import { Button } from '../../ui/Button';
import { FormField } from '../../ui/FormField';
import { Combobox, type ComboboxItem } from '../../pickers/Combobox';
import { InferEventSchema } from '@wailsjs/go/app/App';
import './TriggerEditor.css';

export interface TriggerData {
  type: string;
  expression?: string;
  every?: string;
  listen?: string;
  keys?: string;
  when?: string;
}

interface TriggerEditorProps {
  triggers: TriggerData[];
  onChange: (triggers: TriggerData[]) => void;
  onEventSchemaResolved?: (schema: Record<string, unknown> | null) => void;
  knownEvents?: ComboboxItem[];
}

const TRIGGER_TYPES: SelectOption[] = [
  { value: 'manual', label: 'Manual' },
  { value: 'cron', label: 'Cron' },
  { value: 'interval', label: 'Interval' },
  { value: 'event', label: 'Event' },
  { value: 'hotkey', label: 'Hotkey' },
];

function TriggerRow({
  trigger,
  index,
  total,
  onUpdate,
  onRemove,
  canRemove,
  knownEvents,
}: {
  trigger: TriggerData;
  index: number;
  total: number;
  onUpdate: (idx: number, field: string, value: string) => void;
  onRemove: (idx: number) => void;
  canRemove: boolean;
  knownEvents?: ComboboxItem[];
}) {
  const { t } = useTranslation();

  const triggerLabel = t('jobs.builder.triggerN', { n: index + 1, total });

  return (
    <div
      className="trigger-row"
      role="listitem"
      aria-label={triggerLabel}
    >
      <div className="trigger-row__type">
        <FormField label={t('jobs.builder.triggerType')} visuallyHidden>
          <Select
            options={TRIGGER_TYPES}
            value={trigger.type}
            onChange={(e) => onUpdate(index, 'type', e.target.value)}
            fullWidth
            aria-label={`${triggerLabel} — ${t('jobs.builder.triggerType')}`}
          />
        </FormField>
      </div>

      <div className="trigger-row__config">
        {trigger.type === 'cron' && (
          <FormField label={t('jobs.builder.cronExpression')} visuallyHidden description={t('jobs.builder.cronHint')}>
            <Input
              value={trigger.expression ?? ''}
              onChange={(e) => onUpdate(index, 'expression', e.target.value)}
              placeholder="0 9 * * 1-5"
              fullWidth
              aria-label={`${triggerLabel} — ${t('jobs.builder.cronExpression')}`}
            />
          </FormField>
        )}

        {trigger.type === 'interval' && (
          <FormField label={t('jobs.builder.intervalValue')} visuallyHidden description={t('jobs.builder.intervalHint')}>
            <Input
              value={trigger.every ?? ''}
              onChange={(e) => onUpdate(index, 'every', e.target.value)}
              placeholder="30m"
              fullWidth
              aria-label={`${triggerLabel} — ${t('jobs.builder.intervalValue')}`}
            />
          </FormField>
        )}

        {trigger.type === 'event' && (
          <FormField label={t('jobs.builder.eventName')} visuallyHidden description={t('jobs.builder.eventHint')}>
            <Combobox
              icon="⚡"
              items={knownEvents ?? []}
              selected={trigger.listen ?? ''}
              onSelect={(value) => onUpdate(index, 'listen', value)}
              placeholder={t('jobs.builder.eventNamePlaceholder')}
              allowFreeInput
              maxWidth="100%"
            />
          </FormField>
        )}

        {trigger.type === 'hotkey' && (
          <FormField label={t('jobs.builder.hotkeyKeys')} visuallyHidden description={t('jobs.builder.hotkeyHint')}>
            <Input
              value={trigger.keys ?? ''}
              onChange={(e) => onUpdate(index, 'keys', e.target.value)}
              placeholder="Ctrl+Shift+J"
              fullWidth
              aria-label={`${triggerLabel} — ${t('jobs.builder.hotkeyKeys')}`}
            />
          </FormField>
        )}

        {trigger.type === 'event' && (
          <FormField label={t('jobs.builder.triggerWhen')} visuallyHidden description={t('jobs.builder.triggerWhenHint')}>
            <Input
              value={trigger.when ?? ''}
              onChange={(e) => onUpdate(index, 'when', e.target.value)}
              placeholder={'{{ eq .event.type "issue_updated" }}'}
              fullWidth
              aria-label={`${triggerLabel} — ${t('jobs.builder.triggerWhen')}`}
            />
          </FormField>
        )}
      </div>

      {canRemove && (
        <Button
          variant="ghost"
          size="sm"
          onClick={() => onRemove(index)}
          aria-label={t('jobs.builder.removeTrigger', { n: index + 1 })}
          className="trigger-row__remove"
        >
          <span aria-hidden="true">✕</span>
        </Button>
      )}
    </div>
  );
}

export function TriggerEditor({ triggers, onChange, onEventSchemaResolved, knownEvents }: TriggerEditorProps) {
  const { t } = useTranslation();
  const debounceRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const lastEventNameRef = useRef<string>('');

  const firstEventTrigger = triggers.find((tr) => tr.type === 'event');
  const eventName = firstEventTrigger?.listen ?? '';

  useEffect(() => {
    if (!onEventSchemaResolved) return;
    if (!eventName || eventName === lastEventNameRef.current) return;

    lastEventNameRef.current = eventName;

    if (debounceRef.current) clearTimeout(debounceRef.current);

    debounceRef.current = setTimeout(async () => {
      try {
        const schema = await InferEventSchema(eventName);
        if (schema && Object.keys(schema).length > 0) {
          onEventSchemaResolved(schema);
        } else {
          onEventSchemaResolved(null);
        }
      } catch {
        onEventSchemaResolved(null);
      }
    }, 500);

    return () => {
      if (debounceRef.current) clearTimeout(debounceRef.current);
    };
  }, [eventName, onEventSchemaResolved]);

  useEffect(() => {
    if (!onEventSchemaResolved) return;
    if (!firstEventTrigger) {
      onEventSchemaResolved(null);
      lastEventNameRef.current = '';
    }
  }, [firstEventTrigger, onEventSchemaResolved]);

  const handleUpdate = useCallback(
    (idx: number, field: string, value: string) => {
      const updated = triggers.map((tr, i) => {
        if (i !== idx) return tr;
        const next = { ...tr, [field]: value };
        if (field === 'type') {
          delete next.expression;
          delete next.every;
          delete next.listen;
          delete next.keys;
          delete next.when;
        }
        return next;
      });
      onChange(updated);
    },
    [triggers, onChange]
  );

  const handleRemove = useCallback(
    (idx: number) => {
      onChange(triggers.filter((_, i) => i !== idx));
    },
    [triggers, onChange]
  );

  const handleAdd = useCallback(() => {
    onChange([...triggers, { type: 'manual' }]);
  }, [triggers, onChange]);

  return (
    <div
      className="trigger-editor"
      role="list"
      aria-label={t('jobs.builder.triggerList')}
    >
      {triggers.map((trigger, idx) => (
        <TriggerRow
          key={idx}
          trigger={trigger}
          index={idx}
          total={triggers.length}
          onUpdate={handleUpdate}
          onRemove={handleRemove}
          canRemove={triggers.length > 1}
          knownEvents={knownEvents}
        />
      ))}

      <Button
        variant="ghost"
        size="sm"
        onClick={handleAdd}
        className="trigger-editor__add"
        aria-label={t('jobs.builder.addTrigger')}
      >
        <span aria-hidden="true">+</span> {t('jobs.builder.addTrigger')}
      </Button>
    </div>
  );
}
