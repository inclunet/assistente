import { skills } from '@wailsjs/go/models';
import { useTranslation } from 'react-i18next';
import { CollapsibleSection } from '../ui/CollapsibleSection';

export interface ProfileSkillsSectionProps {
  availableSkills: skills.SkillInfo[];
  enabledSkills?: string[];
  disableOnDemand?: boolean;
  skillsDisabled?: boolean;
  onChange: (field: 'enabled_skills' | 'disable_on_demand_skills' | 'disable_skills', value: any) => void;
  disabled?: boolean;
}

export function ProfileSkillsSection({
  availableSkills,
  enabledSkills = [],
  disableOnDemand = false,
  skillsDisabled = false,
  onChange,
  disabled = false,
}: ProfileSkillsSectionProps) {
  const { t } = useTranslation();

  const handleToggleSkill = (slug: string) => {
    const allSlugs = availableSkills.map(s => s.slug);
    const autoloadSet = new Set(enabledSkills);

    if (autoloadSet.has(slug)) {
      const newList = enabledSkills.filter(s => s !== slug);
      onChange('enabled_skills', newList);
    } else {
      const newList = [...enabledSkills, slug];
      // If all skills selected, send all slugs
      onChange('enabled_skills', newList.length === allSlugs.length ? allSlugs : newList);
    }
  };

  const allSlugs = availableSkills.map(s => s.slug);
  const autoloadSet = new Set(enabledSkills);
  const allAutoloaded = enabledSkills.length === allSlugs.length && allSlugs.length > 0;
  const noneAutoloaded = enabledSkills.length === 0;
  const showSelectAll = !allAutoloaded;
  const showDeselectAll = !noneAutoloaded;

  const autoloadSkills = (enabledSkills || [])
    .map(slug => availableSkills.find(s => s.slug === slug))
    .filter(Boolean) as skills.SkillInfo[];
  const onDemandSkills = availableSkills.filter(s => !autoloadSet.has(s.slug));
  const sortedSkills = [...autoloadSkills, ...onDemandSkills];

  return (
    <CollapsibleSection
      title={t('profiles.collapseSkills', 'Skills')}
      isOpen={!skillsDisabled}
      onToggle={() => onChange('disable_skills', !skillsDisabled)}
      disabled={disabled}
      badge={skillsDisabled ? 'off' : 'on'}
    >
      {availableSkills.length > 0 ? (
        <>
          <p className="profiles-field__hint">
            {t('profiles.skillsHint', 'Marque skills para autoload (injetados no system prompt em ordem). Desmarcados ficam disponíveis sob demanda.')}
          </p>
          <div
            className="profiles-field__tools-actions"
            role="toolbar"
            aria-label={t('profiles.skillsActionsLabel', 'Ações de seleção de skills')}
            data-testid="skills-toolbar"
          >
            {showSelectAll && (
              <button
                type="button"
                className="profiles-field__tools-toggle"
                tabIndex={0}
                onClick={() => onChange('enabled_skills', allSlugs)}
                disabled={disabled}
                data-testid="skills-select-all"
              >
                {t('profiles.skillsSelectAll', 'Selecionar todas')}
              </button>
            )}
            {showDeselectAll && (
              <button
                type="button"
                className="profiles-field__tools-toggle"
                tabIndex={showSelectAll ? -1 : 0}
                onClick={() => onChange('enabled_skills', [])}
                disabled={disabled}
                data-testid="skills-deselect-all"
              >
                {t('profiles.skillsDeselectAll', 'Desmarcar todas')}
              </button>
            )}
            <button
              type="button"
              className={`profiles-field__tools-toggle ${disableOnDemand ? 'profiles-field__tools-toggle--active' : ''}`}
              tabIndex={-1}
              onClick={() => onChange('disable_on_demand_skills', !disableOnDemand)}
              disabled={disabled}
              aria-pressed={disableOnDemand}
              data-testid="skills-toggle-on-demand"
            >
              {disableOnDemand
                ? t('profiles.skillsOnDemandOff', 'Sob demanda: desativado')
                : t('profiles.skillsOnDemandOn', 'Sob demanda: ativado')}
            </button>
          </div>
          <div
            className="profiles-field__tools-grid"
            role="group"
            aria-label={t('profiles.skillsGridLabel', 'Lista de skills')}
            data-testid="skills-grid"
          >
            {sortedSkills.map((skill) => {
              const isAutoload = autoloadSet.has(skill.slug);
              return (
                <div key={skill.slug} className={`profiles-field__tool-item ${isAutoload ? 'profiles-field__tool-item--autoload' : ''}`}>
                  <input
                    type="checkbox"
                    id={`pf-skill-${skill.slug}`}
                    checked={isAutoload}
                    onChange={() => handleToggleSkill(skill.slug)}
                    disabled={disabled}
                    aria-labelledby={`pf-skill-name-${skill.slug}`}
                    aria-describedby={`pf-skill-desc-${skill.slug}`}
                    data-testid={`skill-checkbox-${skill.slug}`}
                  />
                  <label htmlFor={`pf-skill-${skill.slug}`} className="profiles-field__tool-label">
                    <span id={`pf-skill-name-${skill.slug}`} className="profiles-field__tool-name">
                      {skill.name}
                    </span>
                    <span id={`pf-skill-desc-${skill.slug}`} className="profiles-field__tool-desc">
                      {skill.description}
                      {isAutoload && <span className="profiles-field__skill-badge"> (autoload)</span>}
                    </span>
                  </label>
                </div>
              );
            })}
          </div>
        </>
      ) : (
        <p className="profiles-field__hint" style={{ margin: 0 }}>
          {t('profiles.noSkillsAvailable', 'Nenhum skill encontrado.')}
        </p>
      )}
    </CollapsibleSection>
  );
}
