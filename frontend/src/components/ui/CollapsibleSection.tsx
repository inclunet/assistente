import { ReactNode, useId, useRef } from 'react';
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
  const baseId = useId();
  const headerId = `collapsible-${baseId}-header`;
  const regionId = `collapsible-${baseId}-region`;
  const lastKeyToggleRef = useRef(false);

  const handleKeyDown: React.KeyboardEventHandler<HTMLButtonElement> = (event) => {
    if (event.key !== 'Enter' && event.key !== ' ') return;
    event.preventDefault();
    event.stopPropagation();
    lastKeyToggleRef.current = true;
    onToggle();
  };

  const handleClick: React.MouseEventHandler<HTMLButtonElement> = () => {
    if (lastKeyToggleRef.current) {
      lastKeyToggleRef.current = false;
      return;
    }
    onToggle();
  };
  return (
    <div className="collapsible-section" data-testid="collapsible-section">
      <button
        className="collapsible-section__header"
        onClick={handleClick}
        onKeyDown={handleKeyDown}
        aria-expanded={isOpen}
        aria-controls={regionId}
        aria-label={ariaLabel || `${title} - ${isOpen ? t('ui.collapsible.close') : t('ui.collapsible.open')}`}
        id={headerId}
        type="button"
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
      <div
        className="collapsible-section__content"
        role="region"
        id={regionId}
        aria-labelledby={headerId}
        hidden={!isOpen}
      >
        {children}
      </div>
    </div>
  );
}
