import { ReactNode } from 'react';
import { useTranslation } from 'react-i18next';
import './CollapsibleSection.css';

export interface CollapsibleSectionProps {
  /** Título da seção */
  title: string;
  /** Se a seção está aberta ou fechada */
  isOpen: boolean;
  /** Callback quando o usuário clica em abrir/fechar */
  onToggle: () => void;
  /** Se o botão está desabilitado */
  disabled?: boolean;
  /** Badge opcional para indicar status: 'on' (verde) ou 'off' (cinza) */
  badge?: 'on' | 'off';
  /** Conteúdo da seção */
  children: ReactNode;
  /** Customizar aria-label do botão */
  ariaLabel?: string;
}

/**
 * Componente reutilizável para seções colapsáveis.
 * Usado em ProfileSkillsSection, ProfileToolsSection, ProfileVoiceSection, etc.
 *
 * @example
 * ```tsx
 * <CollapsibleSection
 *   title="Skills"
 *   isOpen={openSkills}
 *   onToggle={() => setOpenSkills(!openSkills)}
 *   badge="on"
 * >
 *   <SkillsGrid {...props} />
 * </CollapsibleSection>
 * ```
 */
export function CollapsibleSection({
  title,
  isOpen,
  onToggle,
  disabled = false,
  badge,
  children,
  ariaLabel,
}: CollapsibleSectionProps) {
  const { t } = useTranslation();
  return (
    <div className="collapsible-section" data-testid="collapsible-section">
      <button
        className="collapsible-section__header"
        onClick={onToggle}
        aria-expanded={isOpen}
        aria-label={ariaLabel || `${title} - ${isOpen ? t('ui.collapsible.close') : t('ui.collapsible.open')}`}
        disabled={disabled}
      >
        <span className="collapsible-section__icon">
          {isOpen ? '▼' : '▶'}
        </span>
        <span className="collapsible-section__title">{title}</span>
        {badge && (
          <span
            className={`collapsible-section__badge collapsible-section__badge--${badge}`}
            data-testid={`badge-${badge}`}
          >
            {badge === 'on' ? '●' : '○'}
          </span>
        )}
      </button>
      {isOpen && (
        <div className="collapsible-section__content" role="region">
          {children}
        </div>
      )}
    </div>
  );
}
