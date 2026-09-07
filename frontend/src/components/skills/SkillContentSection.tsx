import { useTranslation } from 'react-i18next';

interface SkillContentSectionProps {
  content: string;
  onContentChange: (content: string) => void;
}

export function SkillContentSection({
  content,
  onContentChange,
}: SkillContentSectionProps) {
  const { t } = useTranslation();
  return (
    <section className="skills-section" data-testid="skill-content-section">
      <h3 className="skills-section__title">{t('skills.contentSection.title')}</h3>
      <div className="skills-fields">
        <div className="skills-field">
          <label htmlFor="sk-content" className="skills-field__label">
            {t('skills.contentSection.label')}
          </label>
          <textarea
            id="sk-content"
            className="skills-field__textarea"
            rows={15}
            value={content || ''}
            onChange={(e) => onContentChange(e.target.value)}
            placeholder={t('skills.contentSection.placeholder')}
          />
          <span className="skills-field__hint">
            {t('skills.contentSection.hint')}
          </span>
        </div>
      </div>
    </section>
  );
}
