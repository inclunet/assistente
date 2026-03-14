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
    <section className="skill-section" data-testid="skill-content-section">
      <h3 className="skill-section__title">{t('skills.contentSection.title')}</h3>
      <div className="skill-fields">
        <div className="skill-field">
          <label htmlFor="sk-content" className="skill-field__label">
            {t('skills.contentSection.label')}
          </label>
          <textarea
            id="sk-content"
            className="skill-field__textarea"
            rows={15}
            value={content || ''}
            onChange={(e) => onContentChange(e.target.value)}
            placeholder={t('skills.contentSection.placeholder')}
          />
          <span className="skill-field__hint">
            {t('skills.contentSection.hint')}
          </span>
        </div>
      </div>
    </section>
  );
}
