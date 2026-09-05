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
    <section className="skills-section" data-testid="skill-tools-section">
      <h3 className="skills-section__title">{t('skills.toolsSection.title')}</h3>
      <div className="skills-fields">
        <div className="skills-field">
          <label htmlFor="sk-tools" className="skills-field__label">
            {t('skills.toolsSection.label')}
          </label>
          <input
            id="sk-tools"
            type="text"
            className="skills-field__input"
            value={toolsString}
            onChange={(e) => onToolsChange(e.target.value)}
            placeholder={t('skills.toolsSection.placeholder')}
          />
          <span className="skills-field__hint">
            {t('skills.toolsSection.hint')}
          </span>
        </div>
      </div>
    </section>
  );
}
