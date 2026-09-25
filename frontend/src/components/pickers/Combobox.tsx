import { useState, useRef, useEffect, useLayoutEffect, useId, useCallback, useMemo, useImperativeHandle, type ReactNode, type Ref } from 'react';
import { useTranslation } from 'react-i18next';
import { playBumpSound } from '../../services/audioFeedback';
import { useAnnouncer } from '../../hooks/useAnnouncer';
import './Combobox.css';

export interface ComboboxItem {
    value: string;
    label: string;
    sublabel?: string;
    /** Texto adicional usado apenas pela busca, sem poluir a opção visível. */
    searchText?: string;
    /** Nome completo da opção quando label + sublabel não bastam. */
    accessibleLabel?: string;
    disabled?: boolean;
}

export type ComboboxDismissReason = 'dismiss' | 'focus-leave';

export interface ComboboxProps {
    icon?: ReactNode;
    label?: string;
    description?: string;
    items: ComboboxItem[];
    selected: string;
    onSelect: (value: string, item: ComboboxItem) => void;
    placeholder?: string;
    disabled?: boolean;
    maxWidth?: string;
    onAnnounce?: (message: string) => void;
    shortcut?: string;
    /** Indica carregamento sem bloquear o trigger nem alterar o foco. */
    busy?: boolean;
    onOpen?: () => void;
    triggerAriaLabel?: string;
    /** Controlled visibility. O modo uncontrolled continua sendo usado quando omitido. */
    open?: boolean;
    onOpenChange?: (open: boolean) => void;
    triggerRef?: Ref<HTMLButtonElement>;
    allowFreeInput?: boolean;
    /** Called after an item is selected and the dropdown closes. Use to customize focus restoration. */
    onAfterSelect?: () => void;
    /** Called after dismissal cleanup; focus-leave must not restore focus to the trigger. */
    onAfterDismiss?: (reason: ComboboxDismissReason) => void;
    /** Ações de teclado fora das opções; recebem a opção atualmente destacada. */
    renderActiveItemActions?: (item: ComboboxItem | null) => ReactNode;
}

export const Combobox = ({
    icon,
    label,
    description,
    items,
    selected,
    onSelect,
    placeholder,
    disabled = false,
    busy = false,
    maxWidth = '180px',
    onAnnounce,
    shortcut,
    onOpen,
    triggerAriaLabel,
    open: controlledOpen,
    onOpenChange,
    triggerRef,
    allowFreeInput = false,
    onAfterSelect,
    onAfterDismiss,
    renderActiveItemActions,
}: ComboboxProps) => {
    const { t } = useTranslation();
    const { announce: announceGlobally } = useAnnouncer();
    const effectiveLabel = label ?? t('pickers.combobox.select');
    const effectivePlaceholder = placeholder ?? t('pickers.combobox.filterPlaceholder');
    const [uncontrolledOpen, setUncontrolledOpen] = useState(false);
    const [filter, setFilter] = useState('');
    const [highlightIndex, setHighlightIndex] = useState(0);

    const inputRef = useRef<HTMLInputElement>(null);
    const buttonRef = useRef<HTMLButtonElement | null>(null);
    const containerRef = useRef<HTMLDivElement>(null);
    const listboxRef = useRef<HTMLUListElement>(null);
    const closeReasonRef = useRef<'select' | 'dismiss' | 'focus-leave' | null>(null);
    const tabNavigationRef = useRef(false);
    const wasOpenRef = useRef(false);
    const focusTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
    const onAfterSelectRef = useRef(onAfterSelect);
    const onAfterDismissRef = useRef(onAfterDismiss);
    const itemsRef = useRef(items);
    const selectedRef = useRef(selected);
    onAfterSelectRef.current = onAfterSelect;
    onAfterDismissRef.current = onAfterDismiss;
    itemsRef.current = items;
    selectedRef.current = selected;
    const previousEmptyResultsAnnouncementKeyRef = useRef('');
    const uniqueId = useId();
    const isControlled = controlledOpen !== undefined;
    const isOpen = isControlled ? controlledOpen : uncontrolledOpen;

    const setButtonRef = useCallback((node: HTMLButtonElement | null) => {
        buttonRef.current = node;
    }, []);
    useImperativeHandle(triggerRef, () => buttonRef.current as HTMLButtonElement, [triggerRef, isOpen]);

    const filteredItems = useMemo(() => items.filter(item =>
        item.label.toLowerCase().includes(filter.toLowerCase()) ||
        (item.sublabel && item.sublabel.toLowerCase().includes(filter.toLowerCase())) ||
        (item.searchText && item.searchText.toLowerCase().includes(filter.toLowerCase()))
    ), [filter, items]);

    const selectedItem = items.find(i => i.value === selected);
    const selectedLabel = selectedItem?.label || (selected ? selected : effectiveLabel);
    const displayLabel = selectedLabel.length > 20
        ? selectedLabel.substring(0, 17) + '...'
        : selectedLabel;
    const hasSelectedItem = Boolean(selected);
    const emptyResultsMessage = filteredItems.length === 0
        ? allowFreeInput && filter.trim()
            ? t('pickers.combobox.pressEnterToUse', { value: filter.trim() })
            : allowFreeInput
                ? t('pickers.combobox.typeToCreate')
                : t('pickers.combobox.noResults')
        : '';
    const emptyResultsAnnouncementKey = filteredItems.length === 0
        ? allowFreeInput && filter.trim()
            ? 'free-input-value'
            : allowFreeInput
                ? 'free-input-empty'
                : 'no-results'
        : '';

    const announceMessage = useCallback((msg: string, priority: 'polite' | 'assertive' = 'assertive') => {
        if (onAnnounce) {
            onAnnounce(msg);
            return;
        }
        announceGlobally(msg, priority);
    }, [announceGlobally, onAnnounce]);

    const announceHighlight = useCallback((index: number, list: ComboboxItem[]) => {
        if (index >= 0 && list[index]) {
            const item = list[index];
            const optionLabel = item.accessibleLabel ||
                `${item.label}${item.sublabel ? `, ${item.sublabel}` : ''}`;
            announceMessage(`${optionLabel}, ${index + 1} ${t('common.of')} ${list.length}`);
        }
    }, [announceMessage, t]);

    const scrollToOption = useCallback((index: number) => {
        const option = document.getElementById(`${uniqueId}-option-${index}`);
        if (option && typeof option.scrollIntoView === 'function') {
            try {
                option.scrollIntoView({ block: 'nearest' });
            } catch {
                // scrollIntoView may fail in jsdom
            }
        }
    }, [uniqueId]);

    const open = () => {
        if (disabled) return;
        onOpen?.();
        onOpenChange?.(true);
        if (!isControlled) setUncontrolledOpen(true);
        setFilter('');

        const currentIdx = items.findIndex(i => i.value === selected);
        setHighlightIndex(currentIdx >= 0 ? currentIdx : 0);

    };

    const close = useCallback((reason: 'select' | 'dismiss' | 'focus-leave' = 'dismiss') => {
        closeReasonRef.current = reason;
        onOpenChange?.(false);
        if (!isControlled) setUncontrolledOpen(false);
        setFilter('');
        setHighlightIndex(0);
    }, [isControlled, onOpenChange]);

    const selectItem = useCallback((item: ComboboxItem) => {
        if (item.disabled) {
            playBumpSound();
            return;
        }
        onSelect(item.value, item);
        close('select');
    }, [onSelect, close]);

    // Keep highlighted option aligned with filter, controlled selection, and refreshed items.
    useEffect(() => {
        if (focusTimerRef.current !== null) {
            clearTimeout(focusTimerRef.current);
            focusTimerRef.current = null;
        }
        if (isOpen) {
            wasOpenRef.current = true;
            setFilter('');
            const currentIdx = itemsRef.current.findIndex(i => i.value === selectedRef.current);
            setHighlightIndex(currentIdx >= 0 ? currentIdx : 0);
            focusTimerRef.current = setTimeout(() => {
                inputRef.current?.focus();
                focusTimerRef.current = null;
            }, 10);
            return () => {
                if (focusTimerRef.current !== null) clearTimeout(focusTimerRef.current);
            };
        }
        if (!wasOpenRef.current) return;
        wasOpenRef.current = false;
        const reason = closeReasonRef.current ?? 'dismiss';
        closeReasonRef.current = null;
        focusTimerRef.current = setTimeout(() => {
            if (reason === 'select' && onAfterSelectRef.current) {
                onAfterSelectRef.current();
            } else if (reason === 'dismiss' || reason === 'focus-leave') {
                if (onAfterDismissRef.current) onAfterDismissRef.current(reason);
                else if (reason === 'dismiss') buttonRef.current?.focus();
            } else {
                buttonRef.current?.focus();
            }
            focusTimerRef.current = null;
        }, 10);
        return () => {
            if (focusTimerRef.current !== null) clearTimeout(focusTimerRef.current);
        };
    }, [isOpen]);

    useEffect(() => {
        if (!isOpen) return;
        const currentIdx = filteredItems.findIndex(i => i.value === selected);
        const newIdx = currentIdx >= 0 ? currentIdx : 0;
        setHighlightIndex(newIdx);
    }, [filteredItems, isOpen, selected]);

    useEffect(() => {
        if (!isOpen || !emptyResultsMessage) {
            previousEmptyResultsAnnouncementKeyRef.current = '';
            return;
        }
        if (emptyResultsAnnouncementKey === previousEmptyResultsAnnouncementKeyRef.current) return;

        announceMessage(emptyResultsMessage, 'polite');
        previousEmptyResultsAnnouncementKeyRef.current = emptyResultsAnnouncementKey;
    }, [announceMessage, emptyResultsAnnouncementKey, emptyResultsMessage, isOpen]);

    useEffect(() => {
        if (!isOpen) return;

        const handleClickOutside = (event: MouseEvent) => {
            if (containerRef.current && !containerRef.current.contains(event.target as Node)) {
                close();
            }
        };

        const timer = setTimeout(() => {
            document.addEventListener('mousedown', handleClickOutside);
        }, 100);

        return () => {
            clearTimeout(timer);
            document.removeEventListener('mousedown', handleClickOutside);
        };
    }, [isOpen, close]);

    const handleKeyDown = (event: React.KeyboardEvent) => {
        if (event.key === 'ArrowDown') {
            event.preventDefault();
            event.stopPropagation();
            if (filteredItems.length > 0) {
                if (highlightIndex >= filteredItems.length - 1) {
                    playBumpSound();
                    return;
                }
                setHighlightIndex(prev => Math.min(prev + 1, filteredItems.length - 1));
            }
        } else if (event.key === 'ArrowUp') {
            event.preventDefault();
            event.stopPropagation();
            if (highlightIndex <= 0) {
                playBumpSound();
                return;
            }
            setHighlightIndex(prev => Math.max(prev - 1, 0));
        } else if (event.key === 'PageDown') {
            event.preventDefault();
            event.stopPropagation();
            if (filteredItems.length > 0) {
                if (highlightIndex >= filteredItems.length - 1) {
                    playBumpSound();
                    return;
                }
                setHighlightIndex(prev => Math.min(prev + 10, filteredItems.length - 1));
            }
        } else if (event.key === 'PageUp') {
            event.preventDefault();
            event.stopPropagation();
            if (highlightIndex <= 0) {
                playBumpSound();
                return;
            }
            setHighlightIndex(prev => Math.max(prev - 10, 0));
        } else if (event.key === 'Enter') {
            event.preventDefault();
            event.stopPropagation();
            if (highlightIndex >= 0 && filteredItems[highlightIndex]) {
                selectItem(filteredItems[highlightIndex]);
            } else if (filteredItems.length === 1) {
                selectItem(filteredItems[0]);
            } else if (allowFreeInput && filter.trim()) {
                onSelect(filter.trim(), { value: filter.trim(), label: filter.trim() });
                close('select');
            }
        } else if (event.key === 'Escape') {
            event.preventDefault();
            event.stopPropagation();
            close();
        } else if (event.key === 'Tab') {
            if (renderActiveItemActions) return;
            if (allowFreeInput && filter.trim() && filteredItems.length === 0) {
                onSelect(filter.trim(), { value: filter.trim(), label: filter.trim() });
            }
            close(allowFreeInput && filter.trim() && filteredItems.length === 0 ? 'select' : 'dismiss');
        } else if (event.key === 'Home') {
            event.preventDefault();
            event.stopPropagation();
            if (filteredItems.length > 0) {
                if (highlightIndex === 0) {
                    playBumpSound();
                    return;
                }
                setHighlightIndex(0);
            }
        } else if (event.key === 'End') {
            event.preventDefault();
            event.stopPropagation();
            if (filteredItems.length > 0) {
                if (highlightIndex >= filteredItems.length - 1) {
                    playBumpSound();
                    return;
                }
                setHighlightIndex(filteredItems.length - 1);
            }
        }
    };

    // Reposiciona dropdown se transbordar a viewport (horizontal e vertical)
    useLayoutEffect(() => {
        if (!isOpen || !containerRef.current) return;
        const dropdown = containerRef.current.querySelector('.picker-dropdown') as HTMLElement;
        if (!dropdown) return;

        dropdown.style.left = '0';
        dropdown.style.right = 'auto';
        dropdown.style.top = '';
        dropdown.style.bottom = '';

        const rect = dropdown.getBoundingClientRect();
        if (rect.right > window.innerWidth - 8) {
            dropdown.style.left = 'auto';
            dropdown.style.right = '0';
        }
        if (rect.bottom > window.innerHeight - 8) {
            dropdown.style.top = 'auto';
            dropdown.style.bottom = 'calc(100% + 4px)';
        }
    }, [isOpen]);

    // Anunciar quando highlightIndex mudar
    useEffect(() => {
        if (isOpen) {
            announceHighlight(highlightIndex, filteredItems);
            scrollToOption(highlightIndex);
        }
    }, [highlightIndex, isOpen]);

    const activeDescendant = isOpen && highlightIndex >= 0 && filteredItems[highlightIndex]
        ? `${uniqueId}-option-${highlightIndex}`
        : undefined;
    const activeItem = isOpen && highlightIndex >= 0 ? filteredItems[highlightIndex] ?? null : null;

    return (
        <div
            ref={containerRef}
            className="combobox-picker"
            style={{ '--max-width': maxWidth } as React.CSSProperties}
        >
            {!isOpen ? (
                <button
                    ref={setButtonRef}
                    type="button"
                    className={`picker-button${hasSelectedItem ? ' picker-button--selected' : ''}`}
                    onClick={open}
                    disabled={disabled}
                        aria-expanded={false}
                        aria-busy={busy || undefined}
                    aria-haspopup="listbox"
                    aria-label={triggerAriaLabel ?? `${effectiveLabel}, ${selectedLabel}`}
                    title={`${description || `${effectiveLabel}: ${selectedLabel}`}${shortcut ? ` (${shortcut})` : ''}`}
                    data-shortcut={shortcut}
                >
                    {icon && <span className="picker-icon" aria-hidden="true">{icon}</span>}
                    <span className="picker-label" aria-hidden="true">{displayLabel}</span>
                    <span className="picker-arrow" aria-hidden="true">▼</span>
                </button>
            ) : (
                <div className="picker-dropdown" onKeyDownCapture={(event) => {
                    tabNavigationRef.current = event.key === 'Tab';
                    if (event.key === 'Escape' && event.target !== inputRef.current) {
                        event.preventDefault();
                        event.stopPropagation();
                        close('dismiss');
                    }
                }} onKeyUpCapture={() => { tabNavigationRef.current = false; }} onBlurCapture={(event) => {
                    const leavingByTab = tabNavigationRef.current;
                    tabNavigationRef.current = false;
                    // Tab deve sair das ações sem prender o foco. Mudanças de
                    // foco programáticas preservam a seleção contextual capturada.
                    if (leavingByTab && renderActiveItemActions && !event.currentTarget.contains(event.relatedTarget as Node | null)) close('focus-leave');
                }}>
                    <input
                        ref={inputRef}
                        type="text"
                        value={filter}
                        onChange={(e) => setFilter(e.target.value)}
                        onKeyDown={handleKeyDown}
                        placeholder={effectivePlaceholder}
                        role="combobox"
                        aria-expanded="true"
                        aria-haspopup="listbox"
                        aria-controls={`${uniqueId}-listbox`}
                        aria-activedescendant={activeDescendant}
                        aria-autocomplete="list"
                        aria-label={`${effectiveLabel} - ${t('pickers.combobox.filterLabel')}`}
                    />
                    {renderActiveItemActions && activeItem && (
                        <div className="picker-active-actions" aria-label={activeItem.accessibleLabel || activeItem.label}>
                            {renderActiveItemActions(activeItem)}
                        </div>
                    )}
                    <ul
                        ref={listboxRef}
                        id={`${uniqueId}-listbox`}
                        role="listbox"
                        aria-label={`${effectiveLabel} ${t('pickers.combobox.available')}`}
                        tabIndex={-1}
                    >
                        {filteredItems.map((item, i) => (
                            <li
                                key={item.value}
                                id={`${uniqueId}-option-${i}`}
                                role="option"
                                aria-selected={item.value === selected}
                                aria-disabled={item.disabled ? 'true' : undefined}
                                aria-label={item.accessibleLabel}
                                className={`${i === highlightIndex ? 'highlighted' : ''} ${item.value === selected ? 'selected' : ''} ${item.disabled ? 'disabled' : ''}`}
                                onMouseDown={(e) => {
                                    e.preventDefault();
                                    if (item.disabled) {
                                        playBumpSound();
                                        return;
                                    }
                                    selectItem(item);
                                }}
                                onMouseEnter={() => setHighlightIndex(i)}
                            >
                                {item.value === selected && (
                                    <span className="option-check" aria-hidden="true" />
                                )}
                                <span className="option-label">{item.label}</span>
                                {item.sublabel && (
                                    <span className="option-sublabel">{item.sublabel}</span>
                                )}
                            </li>
                        ))}
                        {filteredItems.length === 0 && !allowFreeInput && (
                            <li className="no-results">
                                {t('pickers.combobox.noResults')}
                            </li>
                        )}
                        {filteredItems.length === 0 && allowFreeInput && filter.trim() && (
                            <li className="no-results free-input-hint">
                                {t('pickers.combobox.pressEnterToUse', { value: filter.trim() })}
                            </li>
                        )}
                        {filteredItems.length === 0 && allowFreeInput && !filter.trim() && (
                            <li className="no-results">
                                {t('pickers.combobox.typeToCreate')}
                            </li>
                        )}
                    </ul>
                </div>
            )}
        </div>
    );
};
