import { describe, it, expect, vi, beforeAll } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/react';
import { ProfileSkillsSection } from './ProfileSkillsSection';

vi.mock('@wailsjs/go/models', () => ({}));

vi.mock('react-i18next', () => ({
  initReactI18next: { type: '3rdParty', init: () => {} },
  useTranslation: () => ({
    t: (_key: string, defaultValue: string) => defaultValue,
  }),
}));

vi.mock('../../services/audioFeedback', () => ({
  playBumpSound: vi.fn(),
}));

beforeAll(() => {
  Element.prototype.scrollIntoView = vi.fn();
});

const mockSkills: Array<{ name: string; slug: string; description: string; source?: string; autoLoad?: boolean }> = [
  { name: 'Skill 1', slug: 'skill-1', description: 'First skill', source: 'exe' },
  { name: 'Skill 2', slug: 'skill-2', description: 'Second skill', source: 'home' },
  { name: 'Skill 3', slug: 'skill-3', description: 'Third skill', source: 'exe' },
];

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

  it('renderiza DataGrid com todas as skills', () => {
    const onChange = vi.fn();
    render(
      <ProfileSkillsSection
        availableSkills={mockSkills}
        enabledSkills={[]}
        onChange={onChange}
      />
    );
    expect(screen.getByRole('grid')).toBeInTheDocument();
    expect(screen.getByText('Skill 1')).toBeInTheDocument();
    expect(screen.getByText('Skill 2')).toBeInTheDocument();
    expect(screen.getByText('Skill 3')).toBeInTheDocument();
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

  it('marca rows como selecionados para skills enabled', () => {
    const onChange = vi.fn();
    render(
      <ProfileSkillsSection
        availableSkills={mockSkills}
        enabledSkills={['skill-1', 'skill-3']}
        onChange={onChange}
      />
    );
    const rows = screen.getAllByRole('row');
    // row[0] is header, row[1] is skill-1 (enabled), row[2] is skill-3 (enabled), row[3] is skill-2 (not enabled)
    expect(rows[1]).toHaveAttribute('aria-selected', 'true');
    expect(rows[2]).toHaveAttribute('aria-selected', 'true');
    expect(rows[3]).toHaveAttribute('aria-selected', 'false');
  });

  it('chama onChange ao selecionar uma skill via Space', () => {
    const onChange = vi.fn();
    render(
      <ProfileSkillsSection
        availableSkills={mockSkills}
        enabledSkills={[]}
        onChange={onChange}
      />
    );
    const grid = screen.getByRole('grid');
    fireEvent.focus(grid);
    fireEvent.keyDown(grid, { key: ' ' });
    expect(onChange).toHaveBeenCalledWith('enabled_skills', ['skill-1']);
  });

  it('chama onChange ao desmarcar uma skill via Space', () => {
    const onChange = vi.fn();
    render(
      <ProfileSkillsSection
        availableSkills={mockSkills}
        enabledSkills={['skill-1', 'skill-2']}
        onChange={onChange}
      />
    );
    const grid = screen.getByRole('grid');
    fireEvent.focus(grid);
    // First item is skill-1 (enabled, focused by default)
    fireEvent.keyDown(grid, { key: ' ' });
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

  it('preserva a ordem visível ao selecionar todas', () => {
    const onChange = vi.fn();
    render(
      <ProfileSkillsSection
        availableSkills={mockSkills}
        enabledSkills={['skill-3']}
        onChange={onChange}
      />
    );
    fireEvent.click(screen.getByTestId('skills-select-all'));
    expect(onChange).toHaveBeenCalledWith('enabled_skills', ['skill-3', 'skill-1', 'skill-2']);
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

  it('desabilita botões da toolbar quando disabled é true', () => {
    const onChange = vi.fn();
    render(
      <ProfileSkillsSection
        availableSkills={mockSkills}
        enabledSkills={[]}
        onChange={onChange}
        disabled={true}
      />
    );
    expect(screen.getByTestId('skills-select-all')).toBeDisabled();
    expect(screen.getByTestId('skills-toggle-on-demand')).toBeDisabled();
  });

  it('ordena skills habilitadas primeiro', () => {
    const onChange = vi.fn();
    render(
      <ProfileSkillsSection
        availableSkills={mockSkills}
        enabledSkills={['skill-3']}
        onChange={onChange}
      />
    );
    const rows = screen.getAllByRole('row');
    // row[0] = header, row[1] = skill-3 (habilitada), row[2] = skill-1, row[3] = skill-2
    expect(rows[1]).toHaveTextContent('Skill 3');
    expect(rows[1]).toHaveAttribute('aria-selected', 'true');
  });

  it('mostra checkboxes e modo efetivo para skills enabled', () => {
    const onChange = vi.fn();
    render(
      <ProfileSkillsSection
        availableSkills={mockSkills}
        enabledSkills={['skill-2', 'skill-1']}
        onChange={onChange}
      />
    );
    const checkboxes = screen.getAllByRole('checkbox');
    const skill2Cb = checkboxes.find(cb => cb.getAttribute('aria-label')?.includes('Skill 2'));
    expect(skill2Cb).toBeTruthy();
    expect((skill2Cb as HTMLInputElement).checked).toBe(true);

    const skill3Cb = checkboxes.find(cb => cb.getAttribute('aria-label')?.includes('Skill 3'));
    expect(skill3Cb).toBeTruthy();
    expect((skill3Cb as HTMLInputElement).checked).toBe(false);

    const rows = screen.getAllByRole('row');
    // row[1] = skill-2 (base), row[2] = skill-1 (sob demanda), row[3] = skill-3 (desabilitada)
    expect(rows[1]).toHaveTextContent('base');
    expect(rows[2]).toHaveTextContent('sob demanda');
    expect(rows[3]).toHaveTextContent('desabilitada');
  });

  it('usa autoLoad para representar perfis legados sem enabledSkills', () => {
    const onChange = vi.fn();
    render(
      <ProfileSkillsSection
        availableSkills={[
          { name: 'Auto', slug: 'auto', description: 'Auto skill', autoLoad: true },
          { name: 'Manual', slug: 'manual', description: 'Manual skill' },
        ]}
        enabledSkills={undefined}
        onChange={onChange}
      />
    );
    const rows = screen.getAllByRole('row');
    expect(rows[1]).toHaveTextContent('Auto');
    expect(rows[1]).toHaveTextContent('base');
    expect(rows[2]).toHaveTextContent('Manual');
    expect(rows[2]).toHaveTextContent('sob demanda');
  });

  it('mostra skills marcadas após a base como desabilitadas quando sob demanda está desligado', () => {
    const onChange = vi.fn();
    render(
      <ProfileSkillsSection
        availableSkills={mockSkills}
        enabledSkills={['skill-2', 'skill-1']}
        disableOnDemand={true}
        onChange={onChange}
      />
    );
    const rows = screen.getAllByRole('row');
    expect(rows[1]).toHaveTextContent('base');
    expect(rows[2]).toHaveTextContent('desabilitada');
  });

  // ─── Move buttons ──────────────────────────────────────────────

  it('mostra botões ↑ e ↓ na toolbar', () => {
    const onChange = vi.fn();
    render(
      <ProfileSkillsSection
        availableSkills={mockSkills}
        enabledSkills={['skill-1', 'skill-2']}
        onChange={onChange}
      />
    );
    expect(screen.getByTestId('skills-move-up')).toBeInTheDocument();
    expect(screen.getByTestId('skills-move-down')).toBeInTheDocument();
  });

  it('botões ↑/↓ desabilitados quando nenhum item focado é enabled', () => {
    const onChange = vi.fn();
    render(
      <ProfileSkillsSection
        availableSkills={mockSkills}
        enabledSkills={[]}
        onChange={onChange}
      />
    );
    expect(screen.getByTestId('skills-move-up')).toBeDisabled();
    expect(screen.getByTestId('skills-move-down')).toBeDisabled();
  });

  it('move item via Alt+Down no DataGrid', () => {
    const onChange = vi.fn();
    render(
      <ProfileSkillsSection
        availableSkills={mockSkills}
        enabledSkills={['skill-1', 'skill-2', 'skill-3']}
        onChange={onChange}
      />
    );
    const grid = screen.getByRole('grid');
    fireEvent.focus(grid);
    fireEvent.keyDown(grid, { key: 'ArrowDown', altKey: true });
    expect(onChange).toHaveBeenCalledWith('enabled_skills', ['skill-2', 'skill-1', 'skill-3']);
  });

  it('move item via Alt+Up no DataGrid', () => {
    const onChange = vi.fn();
    render(
      <ProfileSkillsSection
        availableSkills={mockSkills}
        enabledSkills={['skill-1', 'skill-2', 'skill-3']}
        onChange={onChange}
      />
    );
    const grid = screen.getByRole('grid');
    fireEvent.focus(grid);
    // Navigate to row 1 (skill-2) first
    fireEvent.keyDown(grid, { key: 'ArrowDown' });
    fireEvent.keyDown(grid, { key: 'ArrowUp', altKey: true });
    expect(onChange).toHaveBeenCalledWith('enabled_skills', ['skill-2', 'skill-1', 'skill-3']);
  });

  // ─── Filtro e Busca ──────────────────────────────────────────────

  describe('Filtro e Busca', () => {
    function selectFilter(testId: string, optionLabel: string) {
      const container = screen.getByTestId(testId);
      const pickerBtn = container.querySelector('.picker-button') as HTMLElement;
      fireEvent.click(pickerBtn);
      const option = screen.getByRole('option', { name: new RegExp(optionLabel) });
      fireEvent.mouseDown(option);
    }

    it('renderiza campo de busca na toolbar', () => {
      render(
        <ProfileSkillsSection
          availableSkills={mockSkills}
          enabledSkills={[]}
          onChange={vi.fn()}
        />
      );
      expect(screen.getByTestId('skills-search')).toBeInTheDocument();
    });

    it('filtra skills por texto de busca', () => {
      render(
        <ProfileSkillsSection
          availableSkills={mockSkills}
          enabledSkills={[]}
          onChange={vi.fn()}
        />
      );
      fireEvent.change(screen.getByTestId('skills-search'), { target: { value: 'First' } });
      expect(screen.getByText('Skill 1')).toBeInTheDocument();
      expect(screen.queryByText('Skill 2')).not.toBeInTheDocument();
      expect(screen.queryByText('Skill 3')).not.toBeInTheDocument();
    });

    it('renderiza picker de filtro quando há múltiplas fontes', () => {
      render(
        <ProfileSkillsSection
          availableSkills={mockSkills}
          enabledSkills={[]}
          onChange={vi.fn()}
        />
      );
      expect(screen.getByTestId('skills-filter')).toBeInTheDocument();
    });

    it('filtra por fonte específica', () => {
      render(
        <ProfileSkillsSection
          availableSkills={mockSkills}
          enabledSkills={[]}
          onChange={vi.fn()}
        />
      );
      selectFilter('skills-filter', 'Home');
      expect(screen.getByText('Skill 2')).toBeInTheDocument();
      expect(screen.queryByText('Skill 1')).not.toBeInTheDocument();
      expect(screen.queryByText('Skill 3')).not.toBeInTheDocument();
    });

    it('mostra mensagem quando filtro não retorna resultados', () => {
      render(
        <ProfileSkillsSection
          availableSkills={mockSkills}
          enabledSkills={[]}
          onChange={vi.fn()}
        />
      );
      fireEvent.change(screen.getByTestId('skills-search'), { target: { value: 'inexistente' } });
      expect(screen.getByText('Nenhum skill corresponde ao filtro.')).toBeInTheDocument();
    });

    it('"Selecionar todas" com filtro ativo seleciona apenas itens filtrados', () => {
      const onChange = vi.fn();
      render(
        <ProfileSkillsSection
          availableSkills={mockSkills}
          enabledSkills={[]}
          onChange={onChange}
        />
      );
      selectFilter('skills-filter', 'Builtin');
      fireEvent.click(screen.getByTestId('skills-select-all'));
      expect(onChange).toHaveBeenCalledWith('enabled_skills', ['skill-1', 'skill-3']);
    });

    it('"Desmarcar todas" com filtro ativo desmarca apenas itens filtrados', () => {
      const onChange = vi.fn();
      render(
        <ProfileSkillsSection
          availableSkills={mockSkills}
          enabledSkills={['skill-1', 'skill-2', 'skill-3']}
          onChange={onChange}
        />
      );
      selectFilter('skills-filter', 'Builtin');
      fireEvent.click(screen.getByTestId('skills-deselect-all'));
      expect(onChange).toHaveBeenCalledWith('enabled_skills', ['skill-2']);
    });
  });
});
