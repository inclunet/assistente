import { useEffect, useMemo, useState, type FormEvent, type ReactNode } from 'react';
import { useTranslation } from 'react-i18next';
import { Button, DialogActions, Input, Modal, Select, Textarea } from '../ui';
import type { SelectOption } from '../ui/Select';
import { useAnnouncer } from '../../hooks/useAnnouncer';

interface ArgumentSchema {
  type?: string | string[];
  required?: string[];
  properties?: Record<string, ArgumentSchema>;
  items?: ArgumentSchema;
  enum?: unknown[];
  nullable?: boolean;
  minimum?: number;
  maximum?: number;
  minLength?: number;
  maxLength?: number;
  minItems?: number;
  maxItems?: number;
  additionalProperties?: boolean;
  optional?: boolean;
}

interface ArgumentField {
  name: string;
  schema: ArgumentSchema;
  required: boolean;
}

type ArgumentSelection =
  | { kind: 'unset' }
  | { kind: 'value'; value: string }
  | { kind: 'null' }
  | { kind: 'enum'; index: number }
  | { kind: 'boolean'; value: boolean };

const UNSET_SELECTION: ArgumentSelection = { kind: 'unset' };

export interface ToolGuidance {
  available: boolean;
  displayName: string;
  description?: string;
  schema?: unknown;
}

export interface CommandArgumentsDialogProps {
  open: boolean;
  commandName: string;
  schema: unknown;
  busy?: boolean;
  /** Campos string cuyo contrato transporta JSON serializado, sem desserializar o persistir seu conteúdo. */
  jsonStringFields?: readonly string[];
  /** Read-only guidance from the runtime tool catalog; never authorizes execution. */
  toolGuidance?: ToolGuidance;
  onCancel: () => void;
  onSubmit: (argumentsValue: Record<string, unknown>) => void;
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return !!value && typeof value === 'object' && !Array.isArray(value);
}

function firstOwn(record: Record<string, unknown>, ...keys: string[]): unknown {
  for (const key of keys) if (Object.prototype.hasOwnProperty.call(record, key)) return record[key];
  return undefined;
}

/** Normalizes both JSON Schema-like catalog values and commandcatalog.Schema's Go JSON shape. */
function normalizeSchema(value: unknown): ArgumentSchema {
  if (!isRecord(value)) return {};
  const type = firstOwn(value, 'type', 'Type');
  const rawProperties = firstOwn(value, 'properties', 'Properties');
  const goShape = ['Type', 'Properties', 'Required', 'Optional', 'Nullable', 'Items', 'Enum'].some((key) => Object.prototype.hasOwnProperty.call(value, key));
  const properties = isRecord(rawProperties)
    ? Object.fromEntries(Object.entries(rawProperties).map(([key, child]) => [key, normalizeSchema(child)])) as Record<string, ArgumentSchema>
    : undefined;
  const rawRequired = firstOwn(value, 'required', 'Required');
  const required = Array.isArray(rawRequired)
    ? rawRequired.filter((item): item is string => typeof item === 'string')
    : goShape && isRecord(rawProperties)
      ? Object.entries(rawProperties).filter(([, child]) => !isRecord(child) || firstOwn(child, 'Optional', 'optional') !== true).map(([key]) => key)
      : undefined;
  const optional = firstOwn(value, 'optional', 'Optional');
  const nullable = firstOwn(value, 'nullable', 'Nullable');
  const items = firstOwn(value, 'items', 'Items');
  const enumeration = firstOwn(value, 'enum', 'Enum');
  const minimum = firstOwn(value, 'minimum', 'Minimum');
  const maximum = firstOwn(value, 'maximum', 'Maximum');
  const minLength = firstOwn(value, 'minLength', 'MinLength');
  const maxLength = firstOwn(value, 'maxLength', 'MaxLength');
  const minItems = firstOwn(value, 'minItems', 'MinItems');
  const maxItems = firstOwn(value, 'maxItems', 'MaxItems');
  return {
    type: typeof type === 'string' || (Array.isArray(type) && type.every((entry) => typeof entry === 'string')) ? type as string | string[] : undefined,
    required,
    properties,
    items: isRecord(items) ? normalizeSchema(items) : undefined,
    enum: Array.isArray(enumeration) ? enumeration : undefined,
    nullable: nullable === true || (Array.isArray(type) && type.includes('null')),
    optional: optional === true,
    minimum: typeof minimum === 'number' ? minimum : undefined,
    maximum: typeof maximum === 'number' ? maximum : undefined,
    minLength: typeof minLength === 'number' ? minLength : undefined,
    maxLength: typeof maxLength === 'number' ? maxLength : undefined,
    minItems: typeof minItems === 'number' ? minItems : undefined,
    maxItems: typeof maxItems === 'number' ? maxItems : undefined,
    additionalProperties: type === 'object' || type === 'Object' ? false : undefined,
  };
}

function ownValue<T>(value: Record<string, T>, key: string): T | undefined {
  return Object.prototype.hasOwnProperty.call(value, key) ? value[key] : undefined;
}

function schemaType(schema: ArgumentSchema): string {
  const types = Array.isArray(schema.type) ? schema.type : [schema.type ?? ''];
  return types.find((type) => type !== 'null') ?? 'null';
}

function isNullable(schema: ArgumentSchema): boolean {
  return schema.nullable === true || (Array.isArray(schema.type) && schema.type.includes('null'));
}

function fieldsFor(schema: ArgumentSchema): ArgumentField[] {
  if (schemaType(schema) !== 'object' || !isRecord(schema.properties)) return [];
  return Object.entries(schema.properties).map(([name, value]) => ({
    name,
    schema: value as ArgumentSchema,
    required: Array.isArray(schema.required)
      ? schema.required.includes(name)
      : false,
  }));
}

export function equalJSON(left: unknown, right: unknown): boolean {
  if (Object.is(left, right)) return true;
  if (Array.isArray(left) || Array.isArray(right)) {
    return Array.isArray(left) && Array.isArray(right) && left.length === right.length &&
      left.every((value, index) => equalJSON(value, right[index]));
  }
  if (isRecord(left) || isRecord(right)) {
    if (!isRecord(left) || !isRecord(right)) return false;
    const leftKeys = Object.keys(left).sort();
    const rightKeys = Object.keys(right).sort();
    return leftKeys.length === rightKeys.length && leftKeys.every((key, index) =>
      key === rightKeys[index] && equalJSON(left[key], right[key]));
  }
  return false;
}

function isValidJSON(value: string): boolean {
  if (!value.trim()) return false;
  try { JSON.parse(value); return true; } catch { return false; }
}

function validateValue(value: unknown, schema: ArgumentSchema, path: string): string | null {
  if (value === null) return isNullable(schema) || schemaType(schema) === 'null' ? null : path;
  const type = schemaType(schema);
  if (type === 'string' && typeof value === 'string') {
    if (schema.minLength !== undefined && [...value].length < schema.minLength) return path;
    if (schema.maxLength !== undefined && [...value].length > schema.maxLength) return path;
  } else if (type === 'number' && typeof value === 'number' && Number.isFinite(value)) {
    if (schema.minimum !== undefined && value < schema.minimum) return path;
    if (schema.maximum !== undefined && value > schema.maximum) return path;
  } else if (type === 'integer' && typeof value === 'number' && Number.isInteger(value)) {
    if (schema.minimum !== undefined && value < schema.minimum) return path;
    if (schema.maximum !== undefined && value > schema.maximum) return path;
  } else if (type === 'boolean' && typeof value === 'boolean') {
    // Type accepted; enum validation below remains authoritative for choices.
  } else if (type === 'object' && isRecord(value)) {
    const properties = isRecord(schema.properties) ? schema.properties as Record<string, ArgumentSchema> : {};
    if (schema.additionalProperties === false && Object.keys(value).some((key) => !Object.prototype.hasOwnProperty.call(properties, key))) return path;
    const required = Array.isArray(schema.required) ? schema.required : [];
    for (const name of required) if (!Object.prototype.hasOwnProperty.call(value, name)) return `${path}.${name}`;
    for (const [name, entry] of Object.entries(value)) {
      const child = Object.prototype.hasOwnProperty.call(properties, name) ? properties[name] : undefined;
      if (child) {
        const error = validateValue(entry, child, `${path}.${name}`);
        if (error) return error;
      }
    }
  } else if (type === 'array' && Array.isArray(value)) {
    if (schema.minItems !== undefined && value.length < schema.minItems) return path;
    if (schema.maxItems !== undefined && value.length > schema.maxItems) return path;
    if (schema.items) {
      for (let index = 0; index < value.length; index += 1) {
        const error = validateValue(value[index], schema.items, `${path}[${index}]`);
        if (error) return error;
      }
    }
  } else if (type === 'null' && value !== null) {
    return path;
  } else {
    return path;
  }
  if (schema.enum?.length && !schema.enum.some((candidate) => equalJSON(candidate, value))) return path;
  return null;
}

function parseField(selection: ArgumentSelection, schema: ArgumentSchema, required: boolean): { present: boolean; value?: unknown; error?: string } {
  const type = schemaType(schema);
  if (selection.kind === 'null') return isNullable(schema) ? { present: true, value: null } : { present: false, error: 'invalid' };
  if (selection.kind === 'enum') {
    const value = schema.enum?.[selection.index];
    return value === undefined ? { present: false, error: 'invalid' } : { present: true, value };
  }
  if (selection.kind === 'boolean') return type === 'boolean' ? { present: true, value: selection.value } : { present: false, error: 'invalid' };
  if (selection.kind === 'unset') {
    return type === 'string' && required && !isNullable(schema)
      ? { present: true, value: '' }
      : { present: false };
  }
  const valueRaw = selection.value;
  if (valueRaw === '' && type === 'string') return required ? { present: true, value: '' } : { present: false };
  if (valueRaw === '') return { present: false };
  if (type === 'string') return { present: true, value: valueRaw };
  if (type === 'number' || type === 'integer') {
    const value = Number(valueRaw);
    if (!Number.isFinite(value) || (type === 'integer' && !Number.isInteger(value))) return { present: false, error: 'invalid' };
    return { present: true, value };
  }
  if (type === 'boolean') {
    if (valueRaw !== 'true' && valueRaw !== 'false') return { present: false, error: 'invalid' };
    return { present: true, value: valueRaw === 'true' };
  }
  try { return { present: true, value: JSON.parse(valueRaw) as unknown }; }
  catch { return { present: false, error: 'invalid' }; }
}

export function CommandArgumentsDialog({
  open,
  commandName,
  schema: rawSchema,
  busy = false,
  jsonStringFields = [],
  toolGuidance,
  onCancel,
  onSubmit,
}: CommandArgumentsDialogProps) {
  const { t } = useTranslation();
  const { announce } = useAnnouncer();
  const schema = useMemo(() => normalizeSchema(rawSchema), [rawSchema]);
  const fields = useMemo(() => fieldsFor(schema), [schema]);
  const [values, setValues] = useState<Record<string, ArgumentSelection>>(() => Object.create(null) as Record<string, ArgumentSelection>);
  const [errors, setErrors] = useState<Record<string, string>>(() => Object.create(null) as Record<string, string>);

  useEffect(() => {
    if (open) {
      setValues(Object.create(null) as Record<string, ArgumentSelection>);
      setErrors(Object.create(null) as Record<string, string>);
    } else {
      setValues(Object.create(null) as Record<string, ArgumentSelection>);
      setErrors(Object.create(null) as Record<string, string>);
    }
  }, [open, schema]);

  useEffect(() => {
    if (!open) return;
    if (fields.length === 0) announce(t('commandPalette.arguments.unsupported'));
    if (toolGuidance && !toolGuidance.available) announce(t('commandPalette.arguments.toolSchemaUnavailable'));
  }, [announce, fields.length, open, t, toolGuidance]);

  const update = (name: string, value: ArgumentSelection) => {
    setValues((current) => Object.assign(Object.create(null) as Record<string, ArgumentSelection>, current, { [name]: value }));
    setErrors((current) => {
      if (!ownValue(current, name)) return current;
      const next = Object.assign(Object.create(null) as Record<string, string>, current);
      delete next[name];
      return next;
    });
  };

  const submit = (event: FormEvent) => {
    event.preventDefault();
    const result: Record<string, unknown> = Object.create(null) as Record<string, unknown>;
    const nextErrors: Record<string, string> = Object.create(null) as Record<string, string>;
    for (const field of fields) {
      const parsed = parseField(ownValue(values, field.name) ?? UNSET_SELECTION, field.schema, field.required);
      if (parsed.error || (field.required && !parsed.present)) {
        nextErrors[field.name] = t('commandPalette.arguments.invalid');
        continue;
      }
      if (!parsed.present) continue;
      const error = validateValue(parsed.value, field.schema, field.name);
      if (error || (jsonStringFields.includes(field.name) && typeof parsed.value === 'string' && !isValidJSON(parsed.value))) nextErrors[field.name] = t('commandPalette.arguments.invalid');
      else result[field.name] = parsed.value;
    }
    if (Object.keys(nextErrors).length > 0) {
      setErrors(nextErrors);
      return;
    }
    onSubmit(result);
  };

  const renderField = (field: ArgumentField) => {
    const { name, schema: fieldSchema, required } = field;
    const type = schemaType(fieldSchema);
    const label = name;
    const common = { id: `command-argument-${name}`, 'aria-required': required, error: ownValue(errors, name) };
    const selection = ownValue(values, name) ?? UNSET_SELECTION;
    let control: ReactNode;
    if (fieldSchema.enum?.length) {
      const options: SelectOption[] = [
        { value: '', label: t(required ? 'commandPalette.arguments.chooseRequired' : 'commandPalette.arguments.skip') },
        ...fieldSchema.enum.map((option, index) => ({ value: `enum:${index}`, label: String(option) })),
      ];
      if (isNullable(fieldSchema)) options.push({ value: 'null', label: t('commandPalette.arguments.null') });
      const selected = selection.kind === 'null' ? 'null' : selection.kind === 'enum' ? `enum:${selection.index}` : '';
      control = <Select {...common} label={label} value={selected} options={options} onChange={(event) => {
        if (event.target.value === 'null') update(name, { kind: 'null' });
        else if (event.target.value.startsWith('enum:')) update(name, { kind: 'enum', index: Number(event.target.value.slice('enum:'.length)) });
        else update(name, UNSET_SELECTION);
      }} />;
    } else if (type === 'boolean') {
      const options: SelectOption[] = [
        { value: '', label: t(required ? 'commandPalette.arguments.chooseRequired' : 'commandPalette.arguments.skip') },
        { value: 'true', label: t('common.yes') },
        { value: 'false', label: t('common.no') },
      ];
      if (isNullable(fieldSchema)) options.push({ value: 'null', label: t('commandPalette.arguments.null') });
      const selected = selection.kind === 'null' ? 'null' : selection.kind === 'boolean' ? String(selection.value) : '';
      control = <Select {...common} label={label} value={selected} options={options} onChange={(event) => {
        if (event.target.value === 'null') update(name, { kind: 'null' });
        else if (event.target.value === 'true' || event.target.value === 'false') update(name, { kind: 'boolean', value: event.target.value === 'true' });
        else update(name, UNSET_SELECTION);
      }} />;
    } else if (type === 'object' || type === 'array' || type === 'null' || (type === 'string' && jsonStringFields.includes(name))) {
      control = <Textarea {...common} label={label} value={selection.kind === 'value' ? selection.value : ''}
        placeholder={type === 'array' ? '[]' : '{}'}
        hint={t('commandPalette.arguments.jsonHint')}
        onChange={(event) => update(name, { kind: 'value', value: event.target.value })} />;
    } else {
      const options: SelectOption[] | null = isNullable(fieldSchema)
        ? [
            { value: '', label: t(required ? 'commandPalette.arguments.chooseRequired' : 'commandPalette.arguments.skip') },
            { value: 'null', label: t('commandPalette.arguments.null') },
            { value: 'value', label: t('commandPalette.arguments.provideValue') },
          ]
        : null;
      const nullableSelection = selection.kind === 'null' ? 'null' : selection.kind === 'value' ? 'value' : '';
      control = (
        <>
          {options && <Select label={label} value={nullableSelection} options={options}
            onChange={(event) => update(name, event.target.value === 'null' ? { kind: 'null' } : event.target.value === 'value' ? { kind: 'value', value: '' } : UNSET_SELECTION)} />}
          {(!options || selection.kind === 'value') && (
            <Input {...common} label={options ? t('commandPalette.arguments.value') : label}
              type={type === 'integer' || type === 'number' ? 'number' : 'text'}
              step={type === 'integer' ? 1 : 'any'}
              min={fieldSchema.minimum} max={fieldSchema.maximum}
              minLength={fieldSchema.minLength} maxLength={fieldSchema.maxLength}
              value={selection.kind === 'value' ? selection.value : ''}
              onChange={(event) => update(name, { kind: 'value', value: event.target.value })} />
          )}
        </>
      );
    }
    return <div className="command-arguments__field" key={name}>{control}</div>;
  };

  return (
    <Modal isOpen={open} onClose={onCancel} title={t('commandPalette.arguments.title', { command: commandName })} size="lg">
      <form className="command-arguments" onSubmit={submit}>
        <p>{t('commandPalette.arguments.description')}</p>
        {toolGuidance && (
          <section className="command-arguments__tool-guidance" aria-labelledby="command-arguments-tool-schema-title">
            <h2 id="command-arguments-tool-schema-title">{t('commandPalette.arguments.toolSchemaTitle', { tool: toolGuidance.displayName })}</h2>
            {toolGuidance.available ? (
              <>
                {toolGuidance.description && <p>{toolGuidance.description}</p>}
                <p>{t('commandPalette.arguments.toolSchemaHint')}</p>
                <pre tabIndex={0} aria-label={t('commandPalette.arguments.toolSchemaLabel')}><code>{JSON.stringify(toolGuidance.schema, null, 2)}</code></pre>
              </>
            ) : (
              <p>{t('commandPalette.arguments.toolSchemaUnavailable')}</p>
            )}
          </section>
        )}
        {fields.length === 0
          ? <p>{t('commandPalette.arguments.unsupported')}</p>
          : fields.map(renderField)}
        <DialogActions
          primary={<Button type="submit" loading={busy} disabled={busy || fields.length === 0}>{t('commandPalette.arguments.submit')}</Button>}
          secondary={<Button type="button" variant="secondary" disabled={busy} onClick={onCancel}>{t('common.cancel')}</Button>}
        />
      </form>
    </Modal>
  );
}

export default CommandArgumentsDialog;
