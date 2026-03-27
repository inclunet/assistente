import type { KeyboardEventHandler, MouseEventHandler, Ref } from 'react';
import { ReactNode, createContext, useContext, useId, useMemo, useRef } from 'react';
import { useTabsKeyboardNav, type TabsActivationMode } from './useTabsKeyboardNav';
import './Tabs.css';

interface TabsContextValue {
  value: string;
  onValueChange: (value: string) => void;
  activationMode: TabsActivationMode;
  idBase: string;
  onBump?: () => void;
  onDelete?: (value: string) => void;
  onActivate?: () => boolean;
  pageJump: number;
}

const TabsContext = createContext<TabsContextValue | null>(null);

function useTabsContext() {
  const ctx = useContext(TabsContext);
  if (!ctx) throw new Error('Tabs components must be used within <Tabs>.');
  return ctx;
}

function sanitizeIdBase(idBase: string) {
  // React useId pode incluir ':'; é válido em HTML, mas fica feio/ruim pra debug.
  return idBase.replace(/[^a-zA-Z0-9_-]/g, '');
}

export interface TabsProps {
  value: string;
  onValueChange: (value: string) => void;
  activationMode?: TabsActivationMode;
  idBase?: string;
  pageJump?: number;
  onBump?: () => void;
  onDelete?: (value: string) => void;
  /** Called on Enter. Return true to suppress default (restoreDefaultFocus). */
  onActivate?: () => boolean;
  className?: string;
  children: ReactNode;
}

export function Tabs({
  value,
  onValueChange,
  activationMode = 'auto',
  idBase,
  pageJump = 10,
  onBump,
  onDelete,
  onActivate,
  className,
  children,
}: TabsProps) {
  const reactId = useId();
  const computedIdBase = useMemo(() => {
    const base = idBase?.trim() ? idBase.trim() : `tabs-${reactId}`;
    return sanitizeIdBase(base);
  }, [idBase, reactId]);

  const ctx = useMemo<TabsContextValue>(
    () => ({
      value,
      onValueChange,
      activationMode,
      idBase: computedIdBase,
      onBump,
      onDelete,
      onActivate,
      pageJump,
    }),
    [activationMode, computedIdBase, onActivate, onBump, onDelete, onValueChange, pageJump, value]
  );

  return (
    <TabsContext.Provider value={ctx}>
      <div className={`tabs${className ? ` ${className}` : ''}`}>{children}</div>
    </TabsContext.Provider>
  );
}

export interface TabListProps {
  ariaLabel: string;
  className?: string;
  listRef?: Ref<HTMLDivElement>;
  onKeyDown?: KeyboardEventHandler<HTMLDivElement>;
  children: ReactNode;
}

export function TabList({ ariaLabel, className, listRef, onKeyDown, children }: TabListProps) {
  const { activationMode, onBump, onValueChange, onDelete, onActivate, pageJump } = useTabsContext();
  const tabListRef = useRef<HTMLDivElement | null>(null);

  const combinedRef: React.RefCallback<HTMLDivElement> = (node) => {
    tabListRef.current = node;
    if (!listRef) return;
    if (typeof listRef === 'function') {
      listRef(node);
      return;
    }
    (listRef as React.MutableRefObject<HTMLDivElement | null>).current = node;
  };

  const { onKeyDown: onKeyDownInternal } = useTabsKeyboardNav({
    tabListRef,
    activationMode,
    onBump,
    onValueChange,
    onDelete,
    onActivate,
    pageJump,
  });

  const handleKeyDown: KeyboardEventHandler<HTMLDivElement> = (event) => {
    onKeyDownInternal(event);
    onKeyDown?.(event);
  };

  return (
    <div
      ref={combinedRef}
      className={`tabs__list${className ? ` ${className}` : ''}`}
      role="tablist"
      aria-label={ariaLabel}
      onKeyDown={handleKeyDown}
    >
      {children}
    </div>
  );
}

export interface TabProps {
  value: string;
  className?: string;
  activeClassName?: string;
  /**
   * Por padrão, o Tab aponta para um TabPanel gerado a partir de `idBase` e `value`.
   * Se o consumidor não renderiza TabPanels (ex.: tabs que só mudam estado fora daqui),
   * passe `controlsId={null}` para omitir `aria-controls`.
   */
  controlsId?: string | null;
  ariaLabel?: string;
  ariaDescription?: string;
  disabled?: boolean;
  title?: string;
  onClick?: MouseEventHandler<HTMLButtonElement>;
  onDoubleClick?: MouseEventHandler<HTMLButtonElement>;
  onContextMenu?: MouseEventHandler<HTMLButtonElement>;
  children: ReactNode;
}

export function Tab({
  value,
  className,
  activeClassName,
  controlsId,
  ariaLabel,
  ariaDescription,
  disabled,
  title,
  onClick,
  onDoubleClick,
  onContextMenu,
  children,
}: TabProps) {
  const { value: activeValue, onValueChange, idBase } = useTabsContext();
  const isActive = value === activeValue;
  const ariaControls = controlsId === null ? undefined : (controlsId ?? `${idBase}-tabpanel-${value}`);

  return (
    <button
      type="button"
      role="tab"
      data-tab-value={value}
      id={`${idBase}-tab-${value}`}
      aria-selected={isActive}
      aria-controls={ariaControls}
      aria-label={ariaLabel}
      aria-description={ariaDescription}
      tabIndex={isActive ? 0 : -1}
      className={`tabs__tab${className ? ` ${className}` : ''}${isActive && activeClassName ? ` ${activeClassName}` : ''}`}
      onClick={(event) => {
        onValueChange(value);
        onClick?.(event);
      }}
      onDoubleClick={onDoubleClick}
      onContextMenu={onContextMenu}
      disabled={disabled}
      title={title}
    >
      {children}
    </button>
  );
}

export interface TabPanelProps {
  value: string;
  className?: string;
  children: ReactNode;
}

export function TabPanel({ value, className, children }: TabPanelProps) {
  const { value: activeValue, idBase } = useTabsContext();
  const isActive = activeValue === value;

  return (
    <div
      id={`${idBase}-tabpanel-${value}`}
      role="tabpanel"
      aria-labelledby={`${idBase}-tab-${value}`}
      className={className}
      hidden={!isActive}
    >
      {isActive ? children : null}
    </div>
  );
}
