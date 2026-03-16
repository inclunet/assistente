import { describe, expect, it, vi } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/react';
import { SlashCommandMenu } from './SlashCommandMenu';
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
      />
    );

    fireEvent.click(screen.getByRole('option'));
    expect(onSelect).toHaveBeenCalledWith(expect.objectContaining({ slug: 'skill' }));
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
      />
    );

    expect(screen.getByText('chat.noSkillsFound')).toBeInTheDocument();
  });
});
