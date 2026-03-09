import { describe, it, expect, vi } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/react';
import { ProfileSkillsSection } from './ProfileSkillsSection';

vi.mock('@wailsjs/go/models', () => ({}));

vi.mock('react-i18next', () => ({
  useTranslation: () => ({
    t: (_key: string, defaultValue: string) => defaultValue,
  }),
}));

const mockSkills = [
  { name: 'Skill 1', slug: 'skill-1', description: 'First skill' },
  { name: 'Skill 2', slug: 'skill-2', description: 'Second skill' },
  { name: 'Skill 3', slug: 'skill-3', description: 'Third skill' },
] as any;

describe('ProfileSkillsSection', () => {
  it('renderiza a seção de skills', () => {
    const onChange = vi.fn();
    render(
      <ProfileSkillsSection
        availableSkills={mockSkills}
        enabledSkills={[]}
        onChange={onChange}
      />
    );
    expect(screen.getByTestId('collapsible-section')).toBeInTheDocument();
  });

  it('renderiza todas as skills do grid', () => {
    const onChange = vi.fn();
    render(
      <ProfileSkillsSection
        availableSkills={mockSkills}
        enabledSkills={[]}
        onChange={onChange}
      />
    );
    expect(screen.getByTestId('skill-checkbox-skill-1')).toBeInTheDocument();
    expect(screen.getByTestId('skill-checkbox-skill-2')).toBeInTheDocument();
    expect(screen.getByTestId('skill-checkbox-skill-3')).toBeInTheDocument();
  });

  it('mostra nomes e descrições das skills', () => {
    const onChange = vi.fn();
    render(
      <ProfileSkillsSection
        availableSkills={mockSkills}
        enabledSkills={[]}
        onChange={onChange}
      />
    );
    expect(screen.getByText('Skill 1')).toBeInTheDocument();
    expect(screen.getByText('Second skill')).toBeInTheDocument();
    expect(screen.getByText('Third skill')).toBeInTheDocument();
  });

  it('marca checkboxes com skills enabled', () => {
    const onChange = vi.fn();
    render(
      <ProfileSkillsSection
        availableSkills={mockSkills}
        enabledSkills={['skill-1', 'skill-3']}
        onChange={onChange}
      />
    );
    expect((screen.getByTestId('skill-checkbox-skill-1') as HTMLInputElement).checked).toBe(true);
    expect((screen.getByTestId('skill-checkbox-skill-2') as HTMLInputElement).checked).toBe(false);
    expect((screen.getByTestId('skill-checkbox-skill-3') as HTMLInputElement).checked).toBe(true);
  });

  it('chama onChange ao marcar uma skill', () => {
    const onChange = vi.fn();
    render(
      <ProfileSkillsSection
        availableSkills={mockSkills}
        enabledSkills={[]}
        onChange={onChange}
      />
    );
    const checkbox = screen.getByTestId('skill-checkbox-skill-1');
    fireEvent.click(checkbox);
    expect(onChange).toHaveBeenCalledWith('enabled_skills', ['skill-1']);
  });

  it('chama onChange ao desmarcar uma skill', () => {
    const onChange = vi.fn();
    render(
      <ProfileSkillsSection
        availableSkills={mockSkills}
        enabledSkills={['skill-1', 'skill-2']}
        onChange={onChange}
      />
    );
    const checkbox = screen.getByTestId('skill-checkbox-skill-1');
    fireEvent.click(checkbox);
    expect(onChange).toHaveBeenCalledWith('enabled_skills', ['skill-2']);
  });

  it('mostra badge com status da feature', () => {
    const onChange = vi.fn();
    render(
      <ProfileSkillsSection
        availableSkills={mockSkills}
        enabledSkills={[]}
        skillsDisabled={false}
        onChange={onChange}
      />
    );
    expect(screen.getByTestId('badge-on')).toBeInTheDocument();
  });

  it('mostra badge "off" quando skillsDisabled', () => {
    const onChange = vi.fn();
    render(
      <ProfileSkillsSection
        availableSkills={mockSkills}
        enabledSkills={[]}
        skillsDisabled={true}
        onChange={onChange}
      />
    );
    expect(screen.getByTestId('badge-off')).toBeInTheDocument();
  });

  it('mostra botão "Selecionar todas" quando nenhuma skill selecionada', () => {
    const onChange = vi.fn();
    render(
      <ProfileSkillsSection
        availableSkills={mockSkills}
        enabledSkills={[]}
        onChange={onChange}
      />
    );
    expect(screen.getByTestId('skills-select-all')).toBeInTheDocument();
  });

  it('chama onChange com todas as slugs ao clicar "Selecionar todas"', () => {
    const onChange = vi.fn();
    render(
      <ProfileSkillsSection
        availableSkills={mockSkills}
        enabledSkills={[]}
        onChange={onChange}
      />
    );
    const selectAllBtn = screen.getByTestId('skills-select-all');
    fireEvent.click(selectAllBtn);
    expect(onChange).toHaveBeenCalledWith('enabled_skills', ['skill-1', 'skill-2', 'skill-3']);
  });

  it('mostra botão "Desmarcar todas" quando pelo menos uma skill selecionada', () => {
    const onChange = vi.fn();
    render(
      <ProfileSkillsSection
        availableSkills={mockSkills}
        enabledSkills={['skill-1']}
        onChange={onChange}
      />
    );
    expect(screen.getByTestId('skills-deselect-all')).toBeInTheDocument();
  });

  it('chama onChange com array vazio ao clicar "Desmarcar todas"', () => {
    const onChange = vi.fn();
    render(
      <ProfileSkillsSection
        availableSkills={mockSkills}
        enabledSkills={['skill-1', 'skill-2']}
        onChange={onChange}
      />
    );
    const deselectAllBtn = screen.getByTestId('skills-deselect-all');
    fireEvent.click(deselectAllBtn);
    expect(onChange).toHaveBeenCalledWith('enabled_skills', []);
  });

  it('mostra botão toggle de on-demand com estado correto', () => {
    const onChange = vi.fn();
    render(
      <ProfileSkillsSection
        availableSkills={mockSkills}
        enabledSkills={[]}
        disableOnDemand={false}
        onChange={onChange}
      />
    );
    const toggleBtn = screen.getByTestId('skills-toggle-on-demand');
    expect(toggleBtn).toHaveTextContent('Sob demanda: ativado');
  });

  it('chama onChange ao clicar toggle on-demand', () => {
    const onChange = vi.fn();
    render(
      <ProfileSkillsSection
        availableSkills={mockSkills}
        enabledSkills={[]}
        disableOnDemand={false}
        onChange={onChange}
      />
    );
    const toggleBtn = screen.getByTestId('skills-toggle-on-demand');
    fireEvent.click(toggleBtn);
    expect(onChange).toHaveBeenCalledWith('disable_on_demand_skills', true);
  });

  it('renderiza mensagem quando nenhuma skill disponível', () => {
    const onChange = vi.fn();
    render(
      <ProfileSkillsSection
        availableSkills={[]}
        enabledSkills={[]}
        onChange={onChange}
      />
    );
    expect(screen.getByText('Nenhum skill encontrado.')).toBeInTheDocument();
  });

  it('desabilita todos os campos quando disabled é true', () => {
    const onChange = vi.fn();
    render(
      <ProfileSkillsSection
        availableSkills={mockSkills}
        enabledSkills={[]}
        onChange={onChange}
        disabled={true}
      />
    );
    expect(screen.getByTestId('skill-checkbox-skill-1')).toBeDisabled();
    expect(screen.getByTestId('skills-select-all')).toBeDisabled();
    expect(screen.getByTestId('skills-toggle-on-demand')).toBeDisabled();
  });

  it('ordena skills colocando autoload primeiro', () => {
    const onChange = vi.fn();
    const { container } = render(
      <ProfileSkillsSection
        availableSkills={mockSkills}
        enabledSkills={['skill-3']}
        onChange={onChange}
      />
    );
    const grid = container.querySelector('[data-testid="skills-grid"]');
    const items = grid?.querySelectorAll('.profiles-field__tool-item');
    // Primeiro item deve ser skill-3 (autoload)
    expect(items?.[0]?.querySelector('[data-testid="skill-checkbox-skill-3"]')).toBeInTheDocument();
  });

  it('mostra badge "autoload" para skills enabled', () => {
    const onChange = vi.fn();
    render(
      <ProfileSkillsSection
        availableSkills={mockSkills}
        enabledSkills={['skill-1']}
        onChange={onChange}
      />
    );
    const badges = screen.getAllByText('(autoload)');
    expect(badges.length).toBe(1);
    expect(badges[0]).toBeInTheDocument();
  });
});
