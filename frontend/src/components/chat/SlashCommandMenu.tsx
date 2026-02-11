import React, { useEffect, useRef, useCallback } from 'react';
import type { skills } from '../../../wailsjs/go/models';
import './SlashCommandMenu.css';

export interface SlashCommandMenuProps {
  /** Lista de skills invocáveis pelo usuário */
  skills: skills.SkillInfo[];
  /** Texto de filtro digitado após o "/" */
  filter: string;
  /** Índice do item selecionado */
  selectedIndex: number;
  /** Callback quando um skill é selecionado */
  onSelect: (skill: skills.SkillInfo) => void;
  /** Callback quando o menu deve ser fechado */
  onClose: () => void;
  /** Posição do menu (referência ao textarea) */
  anchorRef: React.RefObject<HTMLTextAreaElement>;
}

export const SlashCommandMenu: React.FC<SlashCommandMenuProps> = ({
  skills: skillList,
  filter,
  selectedIndex,
  onSelect,
  onClose,
  anchorRef,
}) => {
  const menuRef = useRef<HTMLDivElement>(null);
  const itemRefs = useRef<(HTMLButtonElement | null)[]>([]);

  // Filtra skills pelo texto digitado
  const filteredSkills = skillList.filter((s) => {
    const searchText = filter.toLowerCase();
    if (!searchText) return true;
    const name = (s.displayName || s.name || '').toLowerCase();
    const slug = (s.slug || '').toLowerCase();
    const desc = (s.description || '').toLowerCase();
    return name.includes(searchText) || slug.includes(searchText) || desc.includes(searchText);
  });

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

  if (filteredSkills.length === 0) {
    return (
      <div className="slash-menu" ref={menuRef} role="listbox" aria-label="Slash commands">
        <div className="slash-menu__empty">Nenhum skill encontrado</div>
      </div>
    );
  }

  return (
    <div className="slash-menu" ref={menuRef} role="listbox" aria-label="Slash commands">
      <div className="slash-menu__header">Skills disponíveis</div>
      <div className="slash-menu__list">
        {filteredSkills.map((skill, index) => {
          const displayName = skill.displayName || skill.name || skill.slug;
          const isSelected = index === selectedIndex;

          return (
            <button
              key={skill.slug}
              ref={(el) => { itemRefs.current[index] = el; }}
              className={`slash-menu__item ${isSelected ? 'slash-menu__item--selected' : ''}`}
              role="option"
              aria-selected={isSelected}
              onClick={() => onSelect(skill)}
              onMouseEnter={() => {
                // O hover visual é controlado pelo CSS, mas o selectedIndex é controlado pelo pai
              }}
            >
              <div className="slash-menu__item-header">
                <span className="slash-menu__item-name">/{skill.slug}</span>
                {displayName !== skill.slug && (
                  <span className="slash-menu__item-display">{displayName}</span>
                )}
                {skill.argumentHint && (
                  <span className="slash-menu__item-hint">{skill.argumentHint}</span>
                )}
              </div>
              {skill.description && (
                <div className="slash-menu__item-desc">{skill.description}</div>
              )}
            </button>
          );
        })}
      </div>
    </div>
  );
};

/**
 * Retorna o número de skills filtrados para o texto dado.
 * Útil para controlar o selectedIndex no pai.
 */
export function countFilteredSkills(skillList: skills.SkillInfo[], filter: string): number {
  const searchText = filter.toLowerCase();
  if (!searchText) return skillList.length;
  return skillList.filter((s) => {
    const name = (s.displayName || s.name || '').toLowerCase();
    const slug = (s.slug || '').toLowerCase();
    const desc = (s.description || '').toLowerCase();
    return name.includes(searchText) || slug.includes(searchText) || desc.includes(searchText);
  }).length;
}
