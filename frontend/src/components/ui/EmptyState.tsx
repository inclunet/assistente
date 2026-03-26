import type { ReactNode } from 'react';
import { useTranslation } from 'react-i18next';
import './EmptyState.css';

interface EmptyStateProps {
  icon?: ReactNode;
  title?: string;
  description?: string;
  action?: ReactNode;
  className?: string;
}

export function EmptyState({ icon, title, description, action, className = '' }: EmptyStateProps) {
  const { t } = useTranslation();
  return (
    <div className={`empty-state ${className}`} role="status">
      {icon && <span className="empty-state__icon" aria-hidden="true">{icon}</span>}
      {title && <h3 className="empty-state__title">{title}</h3>}
      <p className="empty-state__description">{description || t('common.emptyState', 'Nenhum item para exibir')}</p>
      {action && <div className="empty-state__action">{action}</div>}
    </div>
  );
}
