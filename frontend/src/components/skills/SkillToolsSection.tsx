interface SkillToolsSectionProps {
  toolsString: string;
  onToolsChange: (tools: string) => void;
}

export function SkillToolsSection({
  toolsString,
  onToolsChange,
}: SkillToolsSectionProps) {
  return (
    <section className="skill-section" data-testid="skill-tools-section">
      <h3 className="skill-section__title">Ferramentas Associadas</h3>
      <div className="skill-fields">
        <div className="skill-field">
          <label htmlFor="sk-tools" className="skill-field__label">
            Ferramentas (separadas por vírgula)
          </label>
          <input
            id="sk-tools"
            type="text"
            className="skill-field__input"
            value={toolsString}
            onChange={(e) => onToolsChange(e.target.value)}
            placeholder="Ex: tool1, tool2, tool3"
          />
          <span className="skill-field__hint">
            Liste as ferramentas (tool calling) que podem ser usadas neste skill.
          </span>
        </div>
      </div>
    </section>
  );
}
