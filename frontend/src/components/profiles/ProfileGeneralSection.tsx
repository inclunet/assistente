import { useTranslation } from 'react-i18next';
import './ProfileGeneralSection.css';

export interface ProfileGeneralSectionProps {
  /** Nome do perfil */
  name: string;
  /** Descrição opcional do perfil */
  description?: string;
  /** Nome do ícone (Ionicons) */
  icon?: string;
  /** Callback quando um campo é alterado */
  onChange: (field: 'name' | 'description' | 'icon', value: string) => void;
  /** Se os campos estão desabilitados */
  disabled?: boolean;
}

/**
 * Componente para seção de campos gerais do perfil.
 * Usado em ProfilesPage editor para campos básicos: name, description, icon.
 *
 * @example
 * ```tsx
 * <ProfileGeneralSection
 *   name={profile.name}
 *   description={profile.description}
 *   icon={profile.icon}
 *   onChange={(field, value) => updateField(field, value)}
 * />
 * ```
 */
export function ProfileGeneralSection({
  name,
  description = '',
  icon = '',
  onChange,
  disabled = false,
}: ProfileGeneralSectionProps) {
  const { t } = useTranslation();
  return (
    <section className="profile-general-section" data-testid="profile-general-section">
      <div className="profile-general-section__field">
        <label htmlFor="profile-name" className="profile-general-section__label">
          {t('profiles.generalSection.name')}
        </label>
        <input
          id="profile-name"
          type="text"
          className="profile-general-section__input"
          value={name}
          onChange={(e) => onChange('name', e.target.value)}
          disabled={disabled}
          required
          aria-required="true"
          data-testid="input-name"
        />
      </div>

      <div className="profile-general-section__field">
        <label htmlFor="profile-description" className="profile-general-section__label">
          {t('profiles.generalSection.description')}
        </label>
        <input
          id="profile-description"
          type="text"
          className="profile-general-section__input"
          value={description}
          onChange={(e) => onChange('description', e.target.value)}
          disabled={disabled}
          data-testid="input-description"
        />
      </div>

      <div className="profile-general-section__field">
        <label htmlFor="profile-icon" className="profile-general-section__label">
          {t('profiles.generalSection.icon')}
        </label>
        <input
          id="profile-icon"
          type="text"
          className="profile-general-section__input"
          value={icon}
          onChange={(e) => onChange('icon', e.target.value)}
          placeholder={t('profiles.generalSection.iconPlaceholder')}
          disabled={disabled}
          data-testid="input-icon"
        />
      </div>
    </section>
  );
}
