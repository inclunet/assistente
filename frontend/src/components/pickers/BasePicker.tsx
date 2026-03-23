import type { HTMLAttributes, ReactNode } from 'react';
import { Combobox, ComboboxItem } from './Combobox';

type Variant = 'toolbar' | 'form';

type VariantValue<T> = T | { toolbar?: T; form?: T };

type VariantClassName = VariantValue<string>;

type VariantBoolean = VariantValue<boolean>;

type VariantString = VariantValue<string>;

type VariantIcon = VariantValue<string>;

interface BasePickerProps {
  items: ComboboxItem[];
  selected: string;
  onSelect: (value: string, item?: ComboboxItem) => void;
  label: string;
  description?: string;
  icon?: string;
  variant?: Variant;
  placeholder?: string;
  disabled?: boolean;
  maxWidth?: string;
  helpText?: string;
  onAnnounce?: (message: string) => void;
  onOpen?: () => void;
  loading?: boolean;
  error?: string | null;
  emptyState?: ReactNode;
  loadingState?: ReactNode;
  errorState?: ReactNode;
  loadingLabel?: VariantString;
  errorLabel?: VariantString;
  emptyLabel?: VariantString;
  errorIcon?: VariantIcon;
  loadingLabelVisuallyHidden?: VariantBoolean;
  errorLabelVisuallyHidden?: VariantBoolean;
  showFormLabel?: boolean;
  showFormLabelIcon?: boolean;
  showLoadingState?: boolean;
  showErrorState?: boolean;
  showEmptyState?: boolean;
  onRetry?: () => void;
  retryLabel?: string;
  formClassName?: string;
  toolbarClassName?: string;
  formLabelClassName?: string;
  formLabelIconClassName?: string;
  helpTextClassName?: string;
  loadingClassName?: VariantClassName;
  errorClassName?: VariantClassName;
  emptyClassName?: VariantClassName;
  retryClassName?: string;
  wrapCombobox?: boolean;
  comboboxWrapperClassName?: string;
  comboboxWrapperProps?: HTMLAttributes<HTMLDivElement>;
  allowFreeInput?: boolean;
  onAfterSelect?: () => void;
}

const resolveVariantValue = <T,>(value: VariantValue<T> | undefined, variant: Variant): T | undefined => {
  if (value && typeof value === 'object' && ('toolbar' in value || 'form' in value)) {
    return value[variant];
  }
  return value as T | undefined;
};

const resolveVariantClassName = (value: VariantClassName | undefined, variant: Variant, fallback?: string) => {
  const resolved = resolveVariantValue(value, variant);
  return resolved ?? fallback;
};

const renderLabelText = (
  labelText: string | undefined,
  visuallyHidden: boolean | undefined
) => {
  if (labelText === undefined || labelText === null || labelText === '') {
    return null;
  }

  if (visuallyHidden) {
    return <span className="sr-only">{labelText}</span>;
  }

  return <span>{labelText}</span>;
};

export const BasePicker = ({
  items,
  selected,
  onSelect,
  label,
  description,
  icon,
  variant = 'form',
  placeholder,
  disabled,
  maxWidth,
  helpText,
  onAnnounce,
  onOpen,
  loading = false,
  error,
  emptyState,
  loadingState,
  errorState,
  loadingLabel = 'Carregando...',
  errorLabel,
  emptyLabel = 'Nenhuma opção disponível',
  errorIcon,
  loadingLabelVisuallyHidden = false,
  errorLabelVisuallyHidden = false,
  showFormLabel = true,
  showFormLabelIcon = true,
  showLoadingState = true,
  showErrorState = true,
  showEmptyState = true,
  onRetry,
  retryLabel = 'Tentar novamente',
  formClassName,
  toolbarClassName,
  formLabelClassName,
  formLabelIconClassName,
  helpTextClassName,
  loadingClassName,
  errorClassName,
  emptyClassName,
  retryClassName = 'retry-btn',
  wrapCombobox = false,
  comboboxWrapperClassName,
  comboboxWrapperProps,
  allowFreeInput = false,
  onAfterSelect,
}: BasePickerProps) => {
  const resolvedLoadingLabel = resolveVariantValue(loadingLabel, variant) ?? 'Carregando...';
  const resolvedErrorLabel = resolveVariantValue(errorLabel, variant) ?? error ?? 'Erro ao carregar';
  const resolvedEmptyLabel = resolveVariantValue(emptyLabel, variant) ?? 'Nenhuma opção disponível';
  const resolvedErrorIcon = resolveVariantValue(errorIcon, variant);
  const resolvedLoadingHidden = resolveVariantValue(loadingLabelVisuallyHidden, variant) ?? false;
  const resolvedErrorHidden = resolveVariantValue(errorLabelVisuallyHidden, variant) ?? false;

  const effectiveLoadingClassName = resolveVariantClassName(
    loadingClassName,
    variant,
    toolbarClassName || formClassName
  );
  const effectiveErrorClassName = resolveVariantClassName(
    errorClassName,
    variant,
    toolbarClassName || formClassName
  );
  const effectiveEmptyClassName = resolveVariantClassName(
    emptyClassName,
    variant,
    toolbarClassName || formClassName
  );

  if (showLoadingState && loading) {
    if (loadingState) {
      return <>{loadingState}</>;
    }
    return (
      <div className={effectiveLoadingClassName} role="status" aria-live="polite">
        <span className="loading-spinner" aria-hidden="true" />
        {renderLabelText(resolvedLoadingLabel, resolvedLoadingHidden)}
      </div>
    );
  }

  if (showErrorState && error) {
    if (errorState) {
      return <>{errorState}</>;
    }
    return (
      <div className={effectiveErrorClassName} role="alert">
        {resolvedErrorIcon && <span className="error-icon">{resolvedErrorIcon}</span>}
        {renderLabelText(resolvedErrorLabel, resolvedErrorHidden)}
        {onRetry && (
          <button type="button" className={retryClassName} onClick={onRetry}>
            {retryLabel}
          </button>
        )}
      </div>
    );
  }

  if (showEmptyState && items.length === 0) {
    if (emptyState) {
      return <>{emptyState}</>;
    }

    return (
      <div className={effectiveEmptyClassName} role="status" aria-live="polite">
        {renderLabelText(resolvedEmptyLabel, false)}
      </div>
    );
  }

  const combobox = (
    <Combobox
      items={items}
      selected={selected}
      onSelect={(value, item) => onSelect(value, item)}
      label={label}
      description={description}
      icon={icon}
      maxWidth={maxWidth}
      placeholder={placeholder}
      disabled={disabled}
      onAnnounce={onAnnounce}
      onOpen={onOpen}
      allowFreeInput={allowFreeInput}
      onAfterSelect={onAfterSelect}
    />
  );

  if (variant === 'toolbar') {
    if (wrapCombobox) {
      return (
        <div className={comboboxWrapperClassName} {...comboboxWrapperProps}>
          {combobox}
        </div>
      );
    }

    return combobox;
  }

  return (
    <div className={formClassName}>
      {showFormLabel && (
        <label className={formLabelClassName}>
          {showFormLabelIcon && icon && (
            <span className={formLabelIconClassName}>{icon}</span>
          )}
          {label}
        </label>
      )}
      {wrapCombobox ? (
        <div className={comboboxWrapperClassName} {...comboboxWrapperProps}>
          {combobox}
        </div>
      ) : (
        combobox
      )}
      {helpText && <p className={helpTextClassName}>{helpText}</p>}
    </div>
  );
};
