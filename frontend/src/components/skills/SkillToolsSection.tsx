import { useEffect } from 'react';
import { useAnnouncer } from '../../hooks/useAnnouncer';
import { useTranslation } from 'react-i18next';

interface SkillToolsSectionProps {
  toolsString: string;
  declaresShellPrograms?: boolean;
  deniesShell?: boolean;
  onToolsChange: (tools: string) => void;
}

export function SkillToolsSection({
  toolsString,
  declaresShellPrograms,
  deniesShell,
  onToolsChange,
}: SkillToolsSectionProps) {
  const { t } = useTranslation();
  const allowed = toolsString.split(',').map(name => name.trim()).filter(Boolean);
  const shellConflict = declaresShellPrograms && (deniesShell || (allowed.length > 0 && !allowed.includes('run_command')));
  const { announce } = useAnnouncer();
  const warning = shellConflict ? t('skills.toolsSection.shellConflict') : '';
  useEffect(() => {
    if (warning) announce(warning);
  }, [announce, warning]);
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
            aria-describedby={shellConflict ? 'sk-shell-conflict' : undefined}
            value={toolsString}
            onChange={(e) => onToolsChange(e.target.value)}
            placeholder={t('skills.toolsSection.placeholder')}
          />
          {shellConflict && <p id="sk-shell-conflict">{warning}</p>}
          <span className="skills-field__hint">
            {t('skills.toolsSection.hint')}
          </span>
        </div>
      </div>
    </section>
  );
}
