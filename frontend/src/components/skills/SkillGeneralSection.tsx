import { useTranslation } from 'react-i18next';

interface SkillGeneralSectionProps {
  item: {
    name?: string;
    version?: string;
    description?: string;
    auto?: boolean;
  };
  onFieldChange: (field: string, value: string | boolean) => void;
}

export function SkillGeneralSection({
  item,
  onFieldChange,
}: SkillGeneralSectionProps) {
  const { t } = useTranslation();
  return (
    <section className="skills-section" data-testid="skill-general-section">
      <h3 className="skills-section__title">{t('skills.generalSection.title')}</h3>
      <div className="skills-fields">
        <div className="skills-field">
          <label htmlFor="sk-name" className="skills-field__label">
            {t('skills.generalSection.name')}
          </label>
          <input
            id="sk-name"
            type="text"
            className="skills-field__input"
            value={item.name || ''}
            onChange={(e) => onFieldChange('name', e.target.value)}
            placeholder={t('skills.generalSection.namePlaceholder')}
          />
        </div>

        <div className="skills-field">
          <label htmlFor="sk-version" className="skills-field__label">
            {t('skills.generalSection.version')}
          </label>
          <input
            id="sk-version"
            type="text"
            className="skills-field__input"
            value={item.version || ''}
            onChange={(e) => onFieldChange('version', e.target.value)}
            placeholder={t('skills.generalSection.versionPlaceholder')}
            pattern="\d+\.\d+\.\d+"
            aria-describedby="sk-version-hint"
          />
          <span id="sk-version-hint" className="skills-field__hint">
            {t('skills.generalSection.versionHint')}
          </span>
        </div>

        <div className="skills-field">
          <label htmlFor="sk-description" className="skills-field__label">
            {t('skills.generalSection.description')}
          </label>
          <input
            id="sk-description"
            type="text"
            className="skills-field__input"
            value={item.description || ''}
            onChange={(e) => onFieldChange('description', e.target.value)}
            placeholder={t('skills.generalSection.descriptionPlaceholder')}
          />
        </div>

        <div className="skills-field skills-field--checkbox">
          <input
            id="sk-auto"
            type="checkbox"
            checked={item.auto || false}
            onChange={(e) => onFieldChange('auto', e.target.checked)}
          />
          <label htmlFor="sk-auto" className="skills-field__label">
            {t('skills.generalSection.auto')}
          </label>
        </div>
      </div>
    </section>
  );
}
