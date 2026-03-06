interface SkillGeneralSectionProps {
  item: any;
  onFieldChange: (field: string, value: any) => void;
}

export function SkillGeneralSection({
  item,
  onFieldChange,
}: SkillGeneralSectionProps) {
  return (
    <section className="skill-section" data-testid="skill-general-section">
      <h3 className="skill-section__title">Geral</h3>
      <div className="skill-fields">
        <div className="skill-field">
          <label htmlFor="sk-name" className="skill-field__label">
            Nome
          </label>
          <input
            id="sk-name"
            type="text"
            className="skill-field__input"
            value={item.name || ''}
            onChange={(e) => onFieldChange('name', e.target.value)}
            placeholder="Ex: Criar Componente React"
          />
        </div>

        <div className="skill-field">
          <label htmlFor="sk-description" className="skill-field__label">
            Descrição
          </label>
          <input
            id="sk-description"
            type="text"
            className="skill-field__input"
            value={item.description || ''}
            onChange={(e) => onFieldChange('description', e.target.value)}
            placeholder="Quando este skill deve ser usado"
          />
        </div>

        <div className="skill-field skill-field--checkbox">
          <input
            id="sk-auto"
            type="checkbox"
            checked={item.auto || false}
            onChange={(e) => onFieldChange('auto', e.target.checked)}
          />
          <label htmlFor="sk-auto" className="skill-field__label">
            Auto — injetar automaticamente no system prompt
          </label>
        </div>
      </div>
    </section>
  );
}
