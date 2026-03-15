import { allowlist } from '../../../wailsjs/go/models';

interface AllowlistGeneralSectionProps {
  item: allowlist.Allowlist;
  onFieldChange: <K extends keyof allowlist.Allowlist>(field: K, value: allowlist.Allowlist[K]) => void;
}

export function AllowlistGeneralSection({
  item,
  onFieldChange,
}: AllowlistGeneralSectionProps) {
  return (
    <section className="allowlist-section" data-testid="allowlist-general-section">
      <h3 className="allowlist-section__title">Geral</h3>
      <div className="allowlist-fields">
        <div className="allowlist-field">
          <label htmlFor="al-name" className="allowlist-field__label">
            Nome
          </label>
          <input
            id="al-name"
            type="text"
            className="allowlist-field__input"
            value={item.name || ''}
            onChange={(e) => onFieldChange('name', e.target.value)}
            placeholder="Nome da allowlist"
          />
        </div>

        <div className="allowlist-field">
          <label htmlFor="al-description" className="allowlist-field__label">
            Descrição
          </label>
          <input
            id="al-description"
            type="text"
            className="allowlist-field__input"
            value={item.description || ''}
            onChange={(e) => onFieldChange('description', e.target.value)}
            placeholder="Para que serve esta allowlist"
          />
        </div>

        <div className="allowlist-field">
          <label htmlFor="al-default-action" className="allowlist-field__label">
            Ação Padrão
          </label>
          <select
            id="al-default-action"
            className="allowlist-field__input"
            value={item.default_action || 'confirm'}
            onChange={(e) => onFieldChange('default_action', e.target.value)}
          >
            <option value="confirm">Confirmar (pede aprovação ao usuário)</option>
            <option value="deny">Negar (bloqueia comandos desconhecidos)</option>
          </select>
        </div>
      </div>
    </section>
  );
}
