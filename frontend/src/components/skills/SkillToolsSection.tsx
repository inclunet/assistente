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
  function conflictsWithShell(value: string) {
    const allowed = value.split(',').map(name => name.trim()).filter(Boolean);
    return declaresShellPrograms && (deniesShell || (allowed.length > 0 && !allowed.includes('run_command')));
  }
  const shellConflict = conflictsWithShell(toolsString);
  const { announce } = useAnnouncer();
  const warning = t('skills.toolsSection.shellConflict');
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
            onChange={(e) => {
              const next = e.target.value;
              if (!shellConflict && conflictsWithShell(next)) announce(warning);
              onToolsChange(next);
            }}
            placeholder={t('skills.toolsSection.placeholder')}
          />
          {shellConflict && <p id="sk-shell-conflict" className="skills-field-warning">{warning}</p>}
          <span className="skills-field__hint">
            {t('skills.toolsSection.hint')}
          </span>
        </div>
      </div>
    </section>
  );
}
