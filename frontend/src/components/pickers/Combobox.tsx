import { useState, useRef, useEffect, useId, useCallback } from 'react';
import { useTranslation } from 'react-i18next';
import { playBumpSound } from '../../services/audioFeedback';
import './Combobox.css';

export interface ComboboxItem {
    value: string;
    label: string;
    sublabel?: string;
    disabled?: boolean;
}

export interface ComboboxProps {
    icon?: string;
    label?: string;
    description?: string;
    items: ComboboxItem[];
    selected: string;
    onSelect: (value: string, item: ComboboxItem) => void;
    placeholder?: string;
    disabled?: boolean;
    maxWidth?: string;
    onAnnounce?: (message: string) => void;
    onOpen?: () => void;
    allowFreeInput?: boolean;
    /** Called after an item is selected and the dropdown closes. Use to customize focus restoration. */
    onAfterSelect?: () => void;
}

export const Combobox = ({
    icon = '🔧',
    label,
    description,
    items,
    selected,
    onSelect,
    placeholder,
    disabled = false,
    maxWidth = '180px',
    onAnnounce,
    onOpen,
    allowFreeInput = false,
    onAfterSelect,
}: ComboboxProps) => {
    const { t } = useTranslation();
    const effectiveLabel = label ?? t('pickers.combobox.select');
    const effectivePlaceholder = placeholder ?? t('pickers.combobox.filterPlaceholder');
    const [isOpen, setIsOpen] = useState(false);
    const [filter, setFilter] = useState('');
    const [highlightIndex, setHighlightIndex] = useState(0);
    const [liveMessage, setLiveMessage] = useState('');

    const inputRef = useRef<HTMLInputElement>(null);
    const buttonRef = useRef<HTMLButtonElement>(null);
    const containerRef = useRef<HTMLDivElement>(null);
    const listboxRef = useRef<HTMLUListElement>(null);
    const uniqueId = useId();

    const filteredItems = items.filter(item =>
        item.label.toLowerCase().includes(filter.toLowerCase()) ||
        (item.sublabel && item.sublabel.toLowerCase().includes(filter.toLowerCase()))
    );

    const selectedItem = items.find(i => i.value === selected);
    const selectedLabel = selectedItem?.label || (allowFreeInput && selected ? selected : effectiveLabel);
    const displayLabel = selectedLabel.length > 20
        ? selectedLabel.substring(0, 17) + '...'
        : selectedLabel;

    const announceMessage = useCallback((msg: string) => {
        if (onAnnounce) {
            onAnnounce(msg);
        }
        setLiveMessage('');
        requestAnimationFrame(() => setLiveMessage(msg));
    }, [onAnnounce]);

    const announceHighlight = useCallback((index: number, list: ComboboxItem[]) => {
        if (index >= 0 && list[index]) {
            const item = list[index];
            const sublabel = item.sublabel ? `, ${item.sublabel}` : '';
            announceMessage(`${item.label}${sublabel}, ${index + 1} ${t('common.of')} ${list.length}`);
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
        setIsOpen(true);
        setFilter('');

        const currentIdx = items.findIndex(i => i.value === selected);
        setHighlightIndex(currentIdx >= 0 ? currentIdx : 0);

        setTimeout(() => {
            inputRef.current?.focus();
        }, 10);
    };

    const close = useCallback((reason: 'select' | 'dismiss' = 'dismiss') => {
        setIsOpen(false);
        setFilter('');
        setHighlightIndex(0);
        setLiveMessage('');

        setTimeout(() => {
            if (reason === 'select' && onAfterSelect) {
                onAfterSelect();
            } else {
                buttonRef.current?.focus();
            }
        }, 10);
    }, [onAfterSelect]);

    const selectItem = useCallback((item: ComboboxItem) => {
        if (item.disabled) {
            playBumpSound();
            return;
        }
        onSelect(item.value, item);
        close('select');
    }, [onSelect, close]);

    // Reset highlight when filter changes
    useEffect(() => {
        if (!isOpen) return;
        const currentIdx = filteredItems.findIndex(i => i.value === selected);
        const newIdx = currentIdx >= 0 ? currentIdx : 0;
        setHighlightIndex(newIdx);
    }, [filter]);

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

    // Announce and scroll when highlight changes
    useEffect(() => {
        if (isOpen) {
            announceHighlight(highlightIndex, filteredItems);
            scrollToOption(highlightIndex);
        }
    }, [highlightIndex, isOpen]);

    const activeDescendant = isOpen && highlightIndex >= 0 && filteredItems[highlightIndex]
        ? `${uniqueId}-option-${highlightIndex}`
        : undefined;

    return (
        <div
            ref={containerRef}
            className="combobox-picker"
            style={{ '--max-width': maxWidth } as React.CSSProperties}
        >
            {!isOpen ? (
                <button
                    ref={buttonRef}
                    className="picker-button"
                    onClick={open}
                    disabled={disabled}
                    aria-expanded={false}
                    aria-haspopup="listbox"
                    aria-label={`${effectiveLabel}: ${selectedLabel}`}
                    title={description || `${effectiveLabel}: ${selectedLabel}`}
                >
                    <span className="picker-icon" aria-hidden="true">{icon}</span>
                    <span className="picker-label" aria-hidden="true">{displayLabel}</span>
                    <span className="picker-arrow" aria-hidden="true">▼</span>
                </button>
            ) : (
                <div className="picker-dropdown">
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
                                <span className="option-label">{item.label}</span>
                                {item.sublabel && (
                                    <span className="option-sublabel">{item.sublabel}</span>
                                )}
                            </li>
                        ))}
                        {filteredItems.length === 0 && !allowFreeInput && (
                            <li className="no-results" role="status">
                                {t('pickers.combobox.noResults')}
                            </li>
                        )}
                        {filteredItems.length === 0 && allowFreeInput && filter.trim() && (
                            <li className="no-results free-input-hint" role="status">
                                {t('pickers.combobox.pressEnterToUse', { value: filter.trim() })}
                            </li>
                        )}
                        {filteredItems.length === 0 && allowFreeInput && !filter.trim() && (
                            <li className="no-results" role="status">
                                {t('pickers.combobox.typeToCreate')}
                            </li>
                        )}
                    </ul>
                </div>
            )}
            <div
                className="sr-only"
                aria-live="assertive"
                aria-atomic="true"
                role="log"
            >
                {liveMessage}
            </div>
        </div>
    );
};
