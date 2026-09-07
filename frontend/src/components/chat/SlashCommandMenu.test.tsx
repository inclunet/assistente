import { describe, expect, it, vi } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/react';
import { getSlashOptionId, SlashCommandMenu } from './SlashCommandMenu';
import { skills } from '../../../wailsjs/go/models';

vi.mock('react-i18next', () => ({
  useTranslation: () => ({ t: (key: string) => key }),
}));

describe('SlashCommandMenu', () => {
  it('renderiza skills e seleciona', () => {
    const onSelect = vi.fn();
    const onClose = vi.fn();
    const anchorRef = { current: document.createElement('textarea') };
    const skill = new skills.SkillInfo({
      slug: 'skill',
      name: 'Skill',
      version: '1.0.0',
      description: 'Desc',
      source: 'builtin',
    });

    render(
      <SlashCommandMenu
        skills={[skill]}
        filter=""
        selectedIndex={0}
        onSelect={onSelect}
        onClose={onClose}
        anchorRef={anchorRef}
        listboxId="slash-listbox"
      />
    );

    fireEvent.click(screen.getByRole('option'));
    expect(screen.getByRole('listbox')).toHaveAttribute('id', 'slash-listbox');
    expect(screen.getByRole('option')).toHaveAttribute('id', 'slash-listbox-option-skill%3Askill');
    expect(screen.getByRole('option')).toHaveAttribute('aria-selected', 'true');
    expect(onSelect).toHaveBeenCalledWith(
      expect.objectContaining({ source: 'skill', token: 'skill', skill: expect.objectContaining({ slug: 'skill' }) }),
    );
  });

  it('mostra estado vazio quando nao encontra', () => {
    const anchorRef = { current: document.createElement('textarea') };
    const skill = new skills.SkillInfo({
      slug: 'skill',
      name: 'Skill',
      version: '1.0.0',
      description: 'Desc',
      source: 'builtin',
    });

    render(
      <SlashCommandMenu
        skills={[skill]}
        filter="x"
        selectedIndex={0}
        onSelect={() => {}}
        onClose={() => {}}
        anchorRef={anchorRef}
        listboxId="slash-listbox"
      />
    );

    expect(screen.getByText('chat.noSlashItemsFound')).toBeInTheDocument();
  });

  it('gera IDs distintos para chaves com pontuação diferente', () => {
    const baseItem = {
      source: 'agent' as const,
      token: 'a-b',
      label: 'a-b',
      acceptsInput: false,
    };

    expect(getSlashOptionId('slash-listbox', { ...baseItem, key: 'agent:a-b' }))
      .not.toBe(getSlashOptionId('slash-listbox', { ...baseItem, key: 'agent:a:b' }));
  });
});
