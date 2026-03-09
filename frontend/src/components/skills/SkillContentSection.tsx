interface SkillContentSectionProps {
  content: string;
  onContentChange: (content: string) => void;
}

export function SkillContentSection({
  content,
  onContentChange,
}: SkillContentSectionProps) {
  return (
    <section className="skill-section" data-testid="skill-content-section">
      <h3 className="skill-section__title">Conteúdo</h3>
      <div className="skill-fields">
        <div className="skill-field">
          <label htmlFor="sk-content" className="skill-field__label">
            Conteúdo do Skill
          </label>
          <textarea
            id="sk-content"
            className="skill-field__textarea"
            rows={15}
            value={content || ''}
            onChange={(e) => onContentChange(e.target.value)}
            placeholder="Descreva como este skill deve ser usado, quais são suas limitações, exemplos de uso, etc."
          />
          <span className="skill-field__hint">
            Este conteúdo será incluído no system prompt quando o skill estiver ativo.
          </span>
        </div>
      </div>
    </section>
  );
}
