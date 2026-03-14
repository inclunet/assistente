import { useTranslation } from 'react-i18next';

interface SkillToolsSectionProps {
  toolsString: string;
  onToolsChange: (tools: string) => void;
}

export function SkillToolsSection({
  toolsString,
  onToolsChange,
}: SkillToolsSectionProps) {
  const { t } = useTranslation();
  return (
    <section className="skill-section" data-testid="skill-tools-section">
      <h3 className="skill-section__title">{t('skills.toolsSection.title')}</h3>
      <div className="skill-fields">
        <div className="skill-field">
          <label htmlFor="sk-tools" className="skill-field__label">
            {t('skills.toolsSection.label')}
          </label>
          <input
            id="sk-tools"
            type="text"
            className="skill-field__input"
            value={toolsString}
            onChange={(e) => onToolsChange(e.target.value)}
            placeholder={t('skills.toolsSection.placeholder')}
          />
          <span className="skill-field__hint">
            {t('skills.toolsSection.hint')}
          </span>
        </div>
      </div>
    </section>
  );
}
