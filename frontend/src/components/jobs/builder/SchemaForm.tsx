import { useMemo, useState, useEffect, useCallback } from 'react';
import { useTranslation } from 'react-i18next';
import { Input } from '../../ui/Input';
import { Select, SelectOption } from '../../ui/Select';
import { Checkbox } from '../../ui/Checkbox';
import { Textarea } from '../../ui/Textarea';
import { FormField } from '../../ui/FormField';
import { Combobox, type ComboboxItem } from '../../pickers/Combobox';
import { GetAllTaskLists } from '@wailsjs/go/main/App';
import { TemplateEditor, type TemplateEditorContext } from './TemplateEditor';
import './SchemaForm.css';

interface JSONSchemaProperty {
  type?: string;
  description?: string;
  enum?: string[];
  default?: unknown;
  items?: JSONSchemaProperty;
  properties?: Record<string, JSONSchemaProperty>;
  required?: string[];
  minimum?: number;
  maximum?: number;
  pattern?: string;
  format?: string;
}

interface JSONSchema {
  type?: string;
  properties?: Record<string, JSONSchemaProperty>;
  required?: string[];
}

interface SchemaFormProps {
  schema: JSONSchema | null;
  values: Record<string, unknown>;
  onChange: (key: string, value: unknown) => void;
  templateMode?: boolean;
  templateContext?: TemplateEditorContext;
}

function placeholderFor(prop: JSONSchemaProperty): string {
  if (prop.enum && prop.enum.length > 0) return prop.enum.join(' | ');
  switch (prop.type) {
    case 'boolean': return 'true | false | {{ .event.flag }}';
    case 'integer':
    case 'number': return '42 | {{ .event.count }}';
    case 'array': return '["a","b"] | {{ .event.items }}';
    case 'object': return '{"k":"v"} | {{ .event.data }}';
    default:
      if (prop.format === 'uri') return 'https://... | {{ .event.url }}';
      return '{{ .event.field }}';
  }
}

function TaskListField({ value, onChange }: { value: unknown; onChange: (val: unknown) => void }) {
  const { t } = useTranslation();
  const [items, setItems] = useState<ComboboxItem[]>([]);

  const load = useCallback(async () => {
    try {
      const lists = await GetAllTaskLists();
      if (!lists) return;
      setItems(lists.map((tl) => ({
        value: tl.id.toString(),
        label: tl.title || t('tasklist.noTitle', 'Sem título'),
        sublabel: `${tl.tasks?.length ?? 0} ${t('tasklist.totalTasks', 'tarefas')}`,
      })));
    } catch { /* ignore */ }
  }, [t]);

  useEffect(() => { load(); }, [load]);

  const selected = value != null && value !== '' ? String(value) : '';

  return (
    <Combobox
      icon="📋"
      items={items}
      selected={selected}
      onSelect={(v) => onChange(parseInt(v, 10))}
      placeholder={t('jobs.builder.selectTaskList', 'Buscar task list...')}
      maxWidth="100%"
      onOpen={load}
    />
  );
}

function SchemaField({
  name,
  prop,
  value,
  onChange,
  required,
  templateMode,
  templateContext,
}: {
  name: string;
  prop: JSONSchemaProperty;
  value: unknown;
  onChange: (val: unknown) => void;
  required: boolean;
  templateMode: boolean;
  templateContext?: TemplateEditorContext;
}) {
  const { t } = useTranslation();

  if (name === 'task_list_id' && (prop.type === 'integer' || prop.type === 'number')) {
    return (
      <FormField label={name} description={prop.description} required={required}>
        <TaskListField value={value} onChange={onChange} />
      </FormField>
    );
  }

  if (templateMode) {
    return (
      <FormField label={name} description={prop.description} required={required}>
        <TemplateEditor
          value={String(value ?? '')}
          onChange={(v) => onChange(v)}
          context={templateContext}
          singleLine
          placeholder={placeholderFor(prop)}
          ariaLabel={name}
        />
      </FormField>
    );
  }

  if (prop.enum && prop.enum.length > 0) {
    const options: SelectOption[] = [
      { value: '', label: t('jobs.builder.selectOption') },
      ...prop.enum.map((v) => ({ value: v, label: v })),
    ];
    return (
      <FormField label={name} description={prop.description} required={required}>
        <Select
          options={options}
          value={String(value ?? '')}
          onChange={(e) => onChange(e.target.value)}
          fullWidth
        />
      </FormField>
    );
  }

  switch (prop.type) {
    case 'boolean':
      return (
        <FormField description={prop.description}>
          <Checkbox
            label={name}
            checked={Boolean(value)}
            onChange={(e) => onChange(e.target.checked)}
          />
        </FormField>
      );

    case 'integer':
    case 'number':
      return (
        <FormField label={name} description={prop.description} required={required}>
          <Input
            type="number"
            value={value !== undefined && value !== null ? String(value) : ''}
            onChange={(e) => {
              const v = e.target.value;
              onChange(v === '' ? undefined : prop.type === 'integer' ? parseInt(v, 10) : parseFloat(v));
            }}
            min={prop.minimum}
            max={prop.maximum}
            fullWidth
            required={required}
          />
        </FormField>
      );

    case 'array':
      return (
        <FormField label={name} description={prop.description ?? t('jobs.builder.arrayHint')} required={required}>
          <Textarea
            value={Array.isArray(value) ? value.join('\n') : String(value ?? '')}
            onChange={(e) => {
              const lines = e.target.value.split('\n').filter(Boolean);
              onChange(lines);
            }}
            placeholder={t('jobs.builder.onePerLine')}
            rows={3}
            fullWidth
          />
        </FormField>
      );

    case 'object':
      return (
        <FormField label={name} description={prop.description ?? t('jobs.builder.jsonHint')} required={required}>
          <Textarea
            value={typeof value === 'object' ? JSON.stringify(value, null, 2) : String(value ?? '{}')}
            onChange={(e) => {
              try {
                onChange(JSON.parse(e.target.value));
              } catch {
                onChange(e.target.value);
              }
            }}
            placeholder="{}"
            rows={4}
            fullWidth
            className="schema-form__json-field"
          />
        </FormField>
      );

    default:
      return (
        <FormField label={name} description={prop.description} required={required}>
          <Input
            value={String(value ?? '')}
            onChange={(e) => onChange(e.target.value)}
            placeholder={
              prop.format === 'uri' ? 'https://...'
              : prop.pattern ? `Pattern: ${prop.pattern}`
              : ''
            }
            fullWidth
            required={required}
          />
        </FormField>
      );
  }
}

export function SchemaForm({ schema, values, onChange, templateMode = false, templateContext }: SchemaFormProps) {
  const { t } = useTranslation();

  const fields = useMemo(() => {
    if (!schema?.properties) return [];
    const requiredSet = new Set(schema.required ?? []);
    return Object.entries(schema.properties).map(([name, prop]) => ({
      name,
      prop,
      required: requiredSet.has(name),
    }));
  }, [schema]);

  if (!schema || !schema.properties || fields.length === 0) {
    return (
      <div className="schema-form__empty" role="status">
        {t('jobs.builder.noInputs')}
      </div>
    );
  }

  return (
    <div className="schema-form" role="form" aria-label={t('jobs.builder.inputsForm')}>
      {templateMode && (
        <p className="schema-form__hint" role="note">{t('jobs.builder.templateHint')}</p>
      )}
      {fields.map(({ name, prop, required }) => (
        <SchemaField
          key={name}
          name={name}
          prop={prop}
          value={values[name]}
          onChange={(val) => onChange(name, val)}
          required={required}
          templateMode={templateMode}
          templateContext={templateContext}
        />
      ))}
    </div>
  );
}
