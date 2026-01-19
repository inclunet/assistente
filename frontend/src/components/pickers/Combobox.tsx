import { useState, useRef, useEffect, useId } from 'react';
import './Combobox.css';

export interface ComboboxItem {
    value: string;
    label: string;
    sublabel?: string;
}

export interface ComboboxProps {
    icon?: string;
    label?: string;
    items: ComboboxItem[];
    selected: string;
    onSelect: (value: string, item: ComboboxItem) => void;
    placeholder?: string;
    disabled?: boolean;
    maxWidth?: string;
    onAnnounce?: (message: string) => void;
}

export const Combobox = ({
    icon = '🔧',
    label = 'Selecionar',
    items,
    selected,
    onSelect,
    placeholder = 'Filtrar...',
    disabled = false,
    maxWidth = '180px',
    onAnnounce
}: ComboboxProps) => {
    const [isOpen, setIsOpen] = useState(false);
    const [filter, setFilter] = useState('');
    const [highlightIndex, setHighlightIndex] = useState(0);
    
    const inputRef = useRef<HTMLInputElement>(null);
    const buttonRef = useRef<HTMLButtonElement>(null);
    const uniqueId = useId();

    // Filtra items
    const filteredItems = items.filter(item =>
        item.label.toLowerCase().includes(filter.toLowerCase()) ||
        (item.sublabel && item.sublabel.toLowerCase().includes(filter.toLowerCase()))
    );

    // Label do item selecionado
    const selectedItem = items.find(i => i.value === selected);
    const selectedLabel = selectedItem?.label || label;
    const displayLabel = selectedLabel.length > 20
        ? selectedLabel.substring(0, 17) + '...'
        : selectedLabel;

    const open = () => {
        if (disabled) return;

        setIsOpen(true);
        setFilter('');

        // Inicia no item atual ou no primeiro
        const currentIdx = filteredItems.findIndex(i => i.value === selected);
        setHighlightIndex(currentIdx >= 0 ? currentIdx : 0);

        setTimeout(() => {
            inputRef.current?.focus();
            announce();
        }, 10);
    };

    const close = () => {
        setIsOpen(false);
        setFilter('');
        setHighlightIndex(0);

        setTimeout(() => {
            buttonRef.current?.focus();
        }, 10);
    };

    const selectItem = (item: ComboboxItem) => {
        onSelect(item.value, item);
        close();
    };

    const announce = () => {
        if (highlightIndex >= 0 && filteredItems[highlightIndex] && onAnnounce) {
            const item = filteredItems[highlightIndex];
            const sublabel = item.sublabel ? `, ${item.sublabel}` : '';
            onAnnounce(`${item.label}${sublabel}, ${highlightIndex + 1} de ${filteredItems.length}`);
        }
    };

    const scrollToOption = () => {
        const option = document.getElementById(`${uniqueId}-option-${highlightIndex}`);
        option?.scrollIntoView({ block: 'nearest' });
    };

    const handleKeyDown = (event: React.KeyboardEvent) => {
        if (event.key === 'ArrowDown') {
            event.preventDefault();
            event.stopPropagation();
            if (filteredItems.length > 0) {
                const newIndex = Math.min(highlightIndex + 1, filteredItems.length - 1);
                setHighlightIndex(newIndex);
            }
        } else if (event.key === 'ArrowUp') {
            event.preventDefault();
            event.stopPropagation();
            if (highlightIndex > 0) {
                setHighlightIndex(highlightIndex - 1);
            }
        } else if (event.key === 'Enter') {
            event.preventDefault();
            event.stopPropagation();
            if (highlightIndex >= 0 && filteredItems[highlightIndex]) {
                selectItem(filteredItems[highlightIndex]);
            } else if (filteredItems.length === 1) {
                selectItem(filteredItems[0]);
            }
        } else if (event.key === 'Escape') {
            event.preventDefault();
            event.stopPropagation();
            close();
        } else if (event.key === 'Tab') {
            close();
        }
    };

    const handleBlur = () => {
        // Delay para permitir clique nas opções
        setTimeout(() => {
            if (isOpen) {
                close();
            }
        }, 150);
    };

    // Anunciar quando highlightIndex mudar
    useEffect(() => {
        if (isOpen) {
            announce();
            scrollToOption();
        }
    }, [highlightIndex, isOpen]);

    return (
        <div className="combobox-picker" style={{ '--max-width': maxWidth } as React.CSSProperties}>
            {!isOpen ? (
                <button
                    ref={buttonRef}
                    className="picker-button"
                    onClick={open}
                    disabled={disabled}
                    aria-haspopup="listbox"
                    aria-expanded={isOpen}
                    aria-label={`${label}: ${selectedLabel}`}
                    title={`${label}: ${selectedLabel}`}
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
                        onBlur={handleBlur}
                        placeholder={placeholder}
                        role="combobox"
                        aria-expanded="true"
                        aria-controls={`${uniqueId}-listbox`}
                        aria-activedescendant={highlightIndex >= 0 ? `${uniqueId}-option-${highlightIndex}` : ''}
                        aria-autocomplete="list"
                    />
                    <ul
                        id={`${uniqueId}-listbox`}
                        role="listbox"
                        aria-label={`${label} disponíveis`}
                    >
                        {filteredItems.map((item, i) => (
                            <li
                                key={item.value}
                                id={`${uniqueId}-option-${i}`}
                                role="option"
                                aria-selected={i === highlightIndex}
                                className={`${i === highlightIndex ? 'highlighted' : ''} ${item.value === selected ? 'selected' : ''}`}
                                onClick={() => selectItem(item)}
                                onMouseEnter={() => setHighlightIndex(i)}
                            >
                                <span className="option-label">{item.label}</span>
                                {item.sublabel && (
                                    <span className="option-sublabel">{item.sublabel}</span>
                                )}
                            </li>
                        ))}
                        {filteredItems.length === 0 && (
                            <li className="no-results" role="option" aria-disabled="true">
                                Nenhum resultado
                            </li>
                        )}
                    </ul>
                </div>
            )}
        </div>
    );
};
