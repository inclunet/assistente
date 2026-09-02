import React, { useEffect, useRef, useCallback } from 'react';
import { useTranslation } from 'react-i18next';
import type { apidto, skills } from '../../../wailsjs/go/models';
import { buildSlashItems, filterSlashItems, type SlashItem, type SlashItemSource } from './slashItems';
import './SlashCommandMenu.css';

export interface SlashCommandMenuProps {
  /** Lista de skills invocáveis pelo usuário */
  skills: skills.SkillInfo[];
  /** Comandos que o agente de código desta conversa oferece (AEP-0084 D8) */
  agentCommands?: apidto.AgentCommand[];
  /** Texto de filtro digitado após o "/" */
  filter: string;
  /** Índice do item selecionado */
  selectedIndex: number;
  /** Callback quando um item é selecionado */
  onSelect: (item: SlashItem) => void;
  /** Callback quando o menu deve ser fechado */
  onClose: () => void;
  /** Posição do menu (referência ao textarea) */
  anchorRef: React.RefObject<HTMLTextAreaElement>;
  /** ID estável referenciado pelo combobox. */
  listboxId: string;
}

export const SlashCommandMenu: React.FC<SlashCommandMenuProps> = ({
  skills: skillList,
  agentCommands = [],
  filter,
  selectedIndex,
  onSelect,
  onClose,
  anchorRef,
  listboxId,
}) => {
  const { t } = useTranslation();
  const menuRef = useRef<HTMLDivElement>(null);
  const itemRefs = useRef<(HTMLDivElement | null)[]>([]);

  const filtered = filterSlashItems(buildSlashItems(skillList, agentCommands), filter);

  // Scroll para item selecionado
  useEffect(() => {
    const item = itemRefs.current[selectedIndex];
    if (item) {
      item.scrollIntoView({ block: 'nearest' });
    }
  }, [selectedIndex]);

  // Fecha menu se clicar fora
  const handleClickOutside = useCallback(
    (e: MouseEvent) => {
      if (
        menuRef.current &&
        !menuRef.current.contains(e.target as Node) &&
        anchorRef.current &&
        !anchorRef.current.contains(e.target as Node)
      ) {
        onClose();
      }
    },
    [onClose, anchorRef]
  );

  useEffect(() => {
    document.addEventListener('mousedown', handleClickOutside);
    return () => document.removeEventListener('mousedown', handleClickOutside);
  }, [handleClickOutside]);

  if (filtered.length === 0) {
    return (
      <div
        className="slash-menu"
        ref={menuRef}
        id={listboxId}
        role="listbox"
        aria-label={t('chat.slashCommands')}
      >
        {/* A lista tem skills do app e comandos do agente: falar só de skills
            aqui deixaria de fora justamente o que a pessoa pode estar
            procurando. */}
        <div className="slash-menu__empty">{t('chat.noSlashItemsFound')}</div>
      </div>
    );
  }

  // Os grupos preservam a ordem da lista: o índice de cada item continua sendo o
  // da lista inteira, que é o mesmo que as setas percorrem. Separar em dois
  // arrays independentes faria a seta e o rótulo discordarem.
  const groups: Array<{ source: SlashItemSource; label: string; items: Array<{ item: SlashItem; index: number }> }> = [
    { source: 'skill', label: t('chat.availableSkills'), items: [] },
    { source: 'agent', label: t('chat.agentCommands', 'Comandos do agente'), items: [] },
  ];
  filtered.forEach((item, index) => {
    const group = groups.find((candidate) => candidate.source === item.source);
    group?.items.push({ item, index });
  });

  return (
    <div
      className="slash-menu"
      ref={menuRef}
      id={listboxId}
      role="listbox"
      aria-label={t('chat.slashCommands')}
    >
      {groups.map((group) => {
        if (group.items.length === 0) return null;
        return (
          <div key={group.source} role="group" aria-label={group.label} className="slash-menu__group">
            <div className="slash-menu__header">{group.label}</div>
            <div className="slash-menu__list">
              {group.items.map(({ item, index }) => {
                const isSelected = index === selectedIndex;
                return (
                  <div
                    key={item.key}
                    id={getSlashOptionId(listboxId, item)}
                    ref={(el) => { itemRefs.current[index] = el; }}
                    className={`slash-menu__item ${isSelected ? 'slash-menu__item--selected' : ''}`}
                    role="option"
                    aria-selected={isSelected}
                    onMouseDown={(event) => event.preventDefault()}
                    onClick={() => onSelect(item)}
                  >
                    <div className="slash-menu__item-header">
                      <span className="slash-menu__item-name">/{item.token}</span>
                      {item.label !== item.token && (
                        <span className="slash-menu__item-display">{item.label}</span>
                      )}
                      {item.argumentHint && (
                        <span className="slash-menu__item-hint">{item.argumentHint}</span>
                      )}
                      {!item.argumentHint && item.acceptsInput && (
                        <span className="slash-menu__item-hint">
                          {t('chat.agentCommandAcceptsInput', 'aceita texto depois do nome')}
                        </span>
                      )}
                    </div>
                    {item.description && (
                      <div className="slash-menu__item-desc">{item.description}</div>
                    )}
                  </div>
                );
              })}
            </div>
          </div>
        );
      })}
    </div>
  );
};

export function getSlashOptionId(listboxId: string, item: SlashItem): string {
  const stableItemId = item.key.replace(/[^a-zA-Z0-9_-]/g, '-');
  return `${listboxId}-option-${stableItemId}`;
}
