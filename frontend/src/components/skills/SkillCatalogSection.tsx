import { useTranslation } from 'react-i18next';

export interface SkillCatalogFields {
  autoLoad?: boolean;
  autoloadReason?: string;
  contextBudget?: number;
  requiresTools?: boolean;
  requiresFilesystem?: boolean;
  requiresNetwork?: boolean;
  requiresMcp?: boolean;
}

interface SkillCatalogSectionProps {
  item: SkillCatalogFields;
  onFieldChange: (field: keyof SkillCatalogFields, value: string | boolean | number) => void;
}

// SkillCatalogSection edita os metadados de catálogo/gating do AEP-0072 D4:
// auto_load + autoload_reason (D5), context_budget e pré-condições de
// capability (requires_*).
export function SkillCatalogSection({ item, onFieldChange }: SkillCatalogSectionProps) {
  const { t } = useTranslation();

  const capabilities: { field: keyof SkillCatalogFields; id: string; label: string }[] = [
    {
      field: 'requiresTools',
      id: 'sk-requires-tools',
      label: t('skills.catalogSection.requiresTools'),
    },
    {
      field: 'requiresFilesystem',
      id: 'sk-requires-filesystem',
      label: t('skills.catalogSection.requiresFilesystem'),
    },
    {
      field: 'requiresNetwork',
      id: 'sk-requires-network',
      label: t('skills.catalogSection.requiresNetwork'),
    },
    {
      field: 'requiresMcp',
      id: 'sk-requires-mcp',
      label: t('skills.catalogSection.requiresMcp'),
    },
  ];

  return (
    <section className="skill-section" data-testid="skill-catalog-section">
      <h3 className="skill-section__title">{t('skills.catalogSection.title')}</h3>
      <div className="skill-fields">
        <div className="skill-field skill-field--checkbox">
          <input
            id="sk-auto-load"
            type="checkbox"
            checked={Boolean(item.autoLoad)}
            onChange={(e) => onFieldChange('autoLoad', e.target.checked)}
          />
          <label htmlFor="sk-auto-load" className="skill-field__label">
            {t('skills.catalogSection.autoLoad')}
          </label>
          <span className="skill-field__hint">{t('skills.catalogSection.autoLoadHint')}</span>
        </div>

        <div className="skill-field">
          <label htmlFor="sk-autoload-reason" className="skill-field__label">
            {t('skills.catalogSection.autoloadReason')}
          </label>
          <textarea
            id="sk-autoload-reason"
            className="skill-field__input"
            rows={2}
            value={item.autoloadReason || ''}
            onChange={(e) => onFieldChange('autoloadReason', e.target.value)}
            placeholder={t('skills.catalogSection.autoloadReasonPlaceholder')}
          />
          <span className="skill-field__hint">
            {t('skills.catalogSection.autoloadReasonHint')}
          </span>
        </div>

        <div className="skill-field">
          <label htmlFor="sk-context-budget" className="skill-field__label">
            {t('skills.catalogSection.contextBudget')}
          </label>
          <input
            id="sk-context-budget"
            type="number"
            min={0}
            className="skill-field__input"
            value={item.contextBudget ?? 0}
            onChange={(e) => {
              const parsed = Number.parseInt(e.target.value, 10);
              onFieldChange('contextBudget', Number.isNaN(parsed) || parsed < 0 ? 0 : parsed);
            }}
          />
          <span className="skill-field__hint">{t('skills.catalogSection.contextBudgetHint')}</span>
        </div>

        <fieldset className="skill-fieldset">
          <legend className="skill-field__label">
            {t('skills.catalogSection.requiresLegend')}
          </legend>
          {capabilities.map((cap) => (
            <div key={cap.id} className="skill-field skill-field--checkbox">
              <input
                id={cap.id}
                type="checkbox"
                checked={Boolean(item[cap.field])}
                onChange={(e) => onFieldChange(cap.field, e.target.checked)}
              />
              <label htmlFor={cap.id} className="skill-field__label">
                {cap.label}
              </label>
            </div>
          ))}
          <span className="skill-field__hint">{t('skills.catalogSection.requiresHint')}</span>
        </fieldset>
      </div>
    </section>
  );
}
