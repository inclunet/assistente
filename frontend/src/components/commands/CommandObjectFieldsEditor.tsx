import { useEffect, useId, useMemo, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { DeleteOutlined, PlusOutlined } from '@ant-design/icons';
import { Button, Checkbox } from '../ui';
import { Input } from '../ui/Input';
import { Select } from '../ui/Select';

type FieldValueType = 'string' | 'number' | 'boolean' | 'json';
interface FieldEntry { id: number; key: string; raw: string; type: FieldValueType }

export interface CommandObjectFieldsEditorProps {
  label: string;
  value: Record<string, unknown>;
  onChange: (value: Record<string, unknown>) => void;
  onValidityChange?: (valid: boolean) => void;
  valueLabel: string;
  typeLabel: string;
  removeLabel: (index: number) => string;
  addLabel: string;
  typeOptions?: Record<FieldValueType, string>;
  disabled?: boolean;
}

function valueType(value: unknown): FieldValueType {
  if (typeof value === 'boolean') return 'boolean';
  if (typeof value === 'number') return 'number';
  if (typeof value === 'object') return 'json';
  return 'string';
}

function decodeEntries(entries: FieldEntry[]): Record<string, unknown> | null {
  const keys = new Set<string>();
  const pairs: [string, unknown][] = [];
  for (const entry of entries) {
    if (!entry.key.trim() || entry.key !== entry.key.trim() || keys.has(entry.key)) return null;
    keys.add(entry.key);
    let value: unknown = entry.raw;
    if (entry.type === 'number') {
      if (!entry.raw.trim() || !Number.isFinite(Number(entry.raw))) return null;
      value = Number(entry.raw);
    } else if (entry.type === 'boolean') {
      if (entry.raw !== 'true' && entry.raw !== 'false') return null;
      value = entry.raw === 'true';
    } else if (entry.type === 'json') {
      try { value = JSON.parse(entry.raw); } catch { return null; }
      if (typeof value !== 'object') return null;
    }
    pairs.push([entry.key, value]);
  }
  return Object.fromEntries(pairs);
}

export function CommandObjectFieldsEditor({
  label, value, onChange, onValidityChange, valueLabel, typeLabel,
  removeLabel, addLabel, typeOptions, disabled = false,
}: CommandObjectFieldsEditorProps) {
  const { t } = useTranslation();
  const errorId = useId();
  const nextID = useRef(0);
  const received = useRef(value);
  const [entries, setEntries] = useState<FieldEntry[]>(() => Object.entries(value).map(([key, entryValue]) => {
    const type = valueType(entryValue);
    return { id: nextID.current++, key, type, raw: type === 'json' ? JSON.stringify(entryValue) : String(entryValue) };
  }));
  useEffect(() => {
    if (value === received.current) return;
    received.current = value;
    setEntries(Object.entries(value).map(([key, entryValue]) => {
      const type = valueType(entryValue);
      return { id: nextID.current++, key, type, raw: type === 'json' ? JSON.stringify(entryValue) : String(entryValue) };
    }));
  }, [value]);
  const valid = useMemo(() => decodeEntries(entries) !== null, [entries]);
  useEffect(() => { onValidityChange?.(valid); }, [valid, onValidityChange]);
  const update = (next: FieldEntry[]) => {
    setEntries(next);
    const decoded = decodeEntries(next);
    if (decoded !== null) {
      received.current = decoded;
      onChange(decoded);
    }
  };
  const updateEntry = (id: number, patch: Partial<FieldEntry>) => update(entries.map((entry) => entry.id === id ? { ...entry, ...patch } : entry));
  const options = typeOptions ?? {
    string: t('commandSettings.objectFields.types.string'), number: t('commandSettings.objectFields.types.number'),
    boolean: t('commandSettings.objectFields.types.boolean'), json: t('commandSettings.objectFields.types.json'),
  };
  return (
    <fieldset className="command-object-fields" disabled={disabled} aria-describedby={!valid ? errorId : undefined}>
      <legend>{label}</legend>
      {entries.map((entry, index) => (
        <div className="command-object-fields__row" key={entry.id}>
          <Input aria-label={`${t('commandSettings.argumentEditor.key')} ${index + 1}`} value={entry.key} onChange={(event) => updateEntry(entry.id, { key: event.target.value })} />
          <Select
            aria-label={`${typeLabel} ${index + 1}`} label={typeLabel} value={entry.type}
            onChange={(event) => {
              const type = event.target.value as FieldValueType;
              const raw = { string: '', number: '0', boolean: 'false', json: '{}' }[type];
              updateEntry(entry.id, { type, raw });
            }}
            options={Object.entries(options).map(([type, typeName]) => ({ value: type, label: typeName }))}
          />
          {entry.type === 'boolean' ? (
            <Checkbox label={`${valueLabel} ${index + 1}`} checked={entry.raw === 'true'} onChange={(event) => updateEntry(entry.id, { raw: String(event.target.checked) })} />
          ) : (
            <Input aria-label={`${valueLabel} ${index + 1}`} value={entry.raw} onChange={(event) => updateEntry(entry.id, { raw: event.target.value })} />
          )}
          <Button type="button" variant="ghost" aria-label={removeLabel(index + 1)} onClick={() => update(entries.filter((row) => row.id !== entry.id))}>
            <DeleteOutlined aria-hidden="true" />
          </Button>
        </div>
      ))}
      {!valid && <p id={errorId}>{t('commandSettings.argumentEditor.invalid')}</p>}
      <Button type="button" variant="secondary" aria-label={addLabel} onClick={() => {
        let suffix = 1;
        while (entries.some((entry) => entry.key === `argument_${suffix}`)) suffix++;
        update([...entries, { id: nextID.current++, key: `argument_${suffix}`, type: 'string', raw: '' }]);
      }}>
        <PlusOutlined aria-hidden="true" />
      </Button>
    </fieldset>
  );
}

export default CommandObjectFieldsEditor;
