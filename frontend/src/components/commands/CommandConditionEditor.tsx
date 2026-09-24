import { useLayoutEffect, useMemo, useRef } from 'react';
import { DeleteOutlined, PlusOutlined } from '@ant-design/icons';
import { useTranslation } from 'react-i18next';
import type {
  CommandCondition,
  CommandConditionClause,
  CommandConditionField,
  CommandConditionValue,
} from '../../types/commandSettingsTypes';
import { Button, Checkbox, Select } from '../ui';
import { Input } from '../ui/Input';
import './CommandConditionEditor.css';

export interface CommandConditionEditorProps {
  value: CommandCondition;
  fields: CommandConditionField[];
  onChange: (value: CommandCondition) => void;
  disabled?: boolean;
}

export function CommandConditionEditor({
  value,
  fields,
  onChange,
  disabled = false,
}: CommandConditionEditorProps) {
  const { t } = useTranslation();
  const clauseFieldRefs = useRef(new Map<number, HTMLSelectElement>());
  const addButtonRef = useRef<HTMLButtonElement>(null);
  const fieldsetRef = useRef<HTMLFieldSetElement>(null);
  const pendingFocusIndex = useRef<number | null>(null);
  const pendingFocusAdd = useRef(false);
  const fieldById = useMemo(() => new Map(fields.map((field) => [field.id, field])), [fields]);
  const canAddField = (field: CommandConditionField) => field.valueKind !== 'enum' || (field.options?.length ?? 0) > 0;

  useLayoutEffect(() => {
    const index = pendingFocusIndex.current;
    if (index === null && !pendingFocusAdd.current) return;
    pendingFocusIndex.current = null;
    pendingFocusAdd.current = false;
    if (index !== null) clauseFieldRefs.current.get(index)?.focus();
    else if (addButtonRef.current && !addButtonRef.current.disabled) addButtonRef.current.focus();
    else fieldsetRef.current?.focus();
  }, [value.clauses.length]);

  const updateClause = (index: number, patch: Partial<CommandConditionClause>) => {
    const clauses = value.clauses.map((clause, clauseIndex) =>
      clauseIndex === index ? { ...clause, ...patch } : clause
    );
    onChange({ version: 1, clauses });
  };

  const updateField = (index: number, fieldId: string) => {
    const field = fieldById.get(fieldId);
    if (!field || !canAddField(field)) return;
    const firstOption = field.options?.[0]?.value ?? '';
    const nextValue: CommandConditionValue = field.valueKind === 'boolean' ? false : firstOption;
    updateClause(index, { field: field.id, value: nextValue });
  };

  const removeClause = (index: number) => {
    const clauses = value.clauses.filter((_, clauseIndex) => clauseIndex !== index);
    pendingFocusIndex.current = clauses.length > 0 ? Math.min(index, clauses.length - 1) : null;
    pendingFocusAdd.current = clauses.length === 0;
    onChange({ version: 1, clauses });
  };

  const addClause = () => {
    const used = new Set(value.clauses.map((clause) => clause.field));
    const field = fields.find((candidate) => !used.has(candidate.id) && canAddField(candidate));
    if (!field) return;
    const firstOption = field.options?.[0]?.value ?? '';
    pendingFocusIndex.current = value.clauses.length;
    onChange({
      version: 1,
      clauses: [
        ...value.clauses,
        { field: field.id, value: field.valueKind === 'boolean' ? false : firstOption },
      ],
    });
  };

  return (
    <fieldset ref={fieldsetRef} tabIndex={-1} className="command-condition-editor" disabled={disabled}>
      <legend>{t('commandSettings.conditions.title')}</legend>
      <p className="command-condition-editor__hint">{t('commandSettings.conditions.help')}</p>
      {value.clauses.length === 0 ? (
        <p className="command-condition-editor__empty">{t('commandSettings.conditions.empty')}</p>
      ) : (
        <div className="command-condition-editor__list" aria-label={t('commandSettings.conditions.listLabel')}>
          {value.clauses.map((clause, index) => {
            const knownField = fieldById.get(clause.field);
            const field = knownField ?? { id: clause.field, label: clause.field, valueKind: 'opaque-id' as const };
            const unavailableOption = field.valueKind === 'enum' && !field.options?.some(option => option.value === clause.value);
            return (
              <div
                className="command-condition-editor__row"
                key={index}
                role="group"
                aria-label={t('commandSettings.conditionRowLabel', { index: index + 1, field: field.label })}
              >
                <Select
                  ref={(element) => {
                    if (element) clauseFieldRefs.current.set(index, element);
                    else clauseFieldRefs.current.delete(index);
                  }}
                  label={t('commandSettings.conditions.field')}
                  value={field.id}
                  options={[...(!knownField ? [{ value: field.id, label: t('commandSettings.unavailableCondition', { field: field.id }), disabled: true }] : []), ...fields.map((option) => ({
                    value: option.id,
                    label: option.label,
                    disabled: !canAddField(option) || (option.id !== field.id && value.clauses.some((other, otherIndex) => otherIndex !== index && other.field === option.id)),
                  }))]}
                  onChange={(event) => updateField(index, event.target.value)}
                />
                {field.valueKind === 'boolean' ? (
                  <Checkbox
                    label={t('commandSettings.conditions.value')}
                    checked={clause.value === true}
                    onChange={(event) => updateClause(index, { value: event.target.checked })}
                  />
                ) : field.valueKind === 'enum' ? (
                  <Select
                    label={t('commandSettings.conditions.value')}
                    value={typeof clause.value === 'string' ? clause.value : ''}
                    options={[...(unavailableOption ? [{ value: String(clause.value), label: t('commandSettings.unavailableConditionValue'), disabled: true }] : []), ...(field.options ?? [])]}
                    onChange={(event) => updateClause(index, { value: event.target.value })}
                  />
                ) : (
                  <Input
                    label={t('commandSettings.conditions.value')}
                    value={typeof clause.value === 'string' ? clause.value : ''}
                    disabled={!knownField}
                    onChange={(event) => updateClause(index, { value: event.target.value })}
                    hint={knownField?.hint ?? (field.valueKind === 'opaque-id' ? t('commandSettings.conditions.opaqueHint') : undefined)}
                  />
                )}
                <Button
                  type="button"
                  variant="ghost"
                  aria-label={t('commandSettings.conditions.remove')}
                  onClick={() => removeClause(index)}
                >
                  <DeleteOutlined aria-hidden="true" />
                </Button>
              </div>
            );
          })}
        </div>
      )}
      <Button ref={addButtonRef} type="button" variant="secondary" onClick={addClause} disabled={!fields.some(field => canAddField(field) && !value.clauses.some(clause => clause.field === field.id))}>
        <PlusOutlined aria-hidden="true" /> {t('commandSettings.conditions.add')}
      </Button>
    </fieldset>
  );
}

export default CommandConditionEditor;
