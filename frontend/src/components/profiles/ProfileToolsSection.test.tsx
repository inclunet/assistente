import { describe, it, expect, vi, beforeAll } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/react';
import { ProfileToolsSection } from './ProfileToolsSection';
import { allowlist } from '../../../wailsjs/go/models';

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

const mockTools: Array<{ name: string; description: string }> = [
  { name: 'Tool 1', description: 'First tool' },
  { name: 'Tool 2', description: 'Second tool' },
  { name: 'Tool 3', description: 'Third tool' },
];

const mockAllowlists: Array<allowlist.AllowlistInfo> = [
  { slug: 'allowlist-1', name: 'Allowlist 1', ruleCount: 5 },
  { slug: 'allowlist-2', name: 'Allowlist 2', ruleCount: 10 },
];

describe('ProfileToolsSection', () => {
  it('renderiza a seção de ferramentas com DataGrid', () => {
    const onChange = vi.fn();
    render(
      <ProfileToolsSection
        availableTools={mockTools}
        enabledTools={null}
        availableAllowlists={mockAllowlists}
        onChange={onChange}
      />
    );
    expect(screen.getByRole('grid')).toBeInTheDocument();
  });

  it('renderiza todas as ferramentas no grid', () => {
    const onChange = vi.fn();
    render(
      <ProfileToolsSection
        availableTools={mockTools}
        enabledTools={null}
        availableAllowlists={mockAllowlists}
        onChange={onChange}
      />
    );
    expect(screen.getByText('Tool 1')).toBeInTheDocument();
    expect(screen.getByText('Tool 2')).toBeInTheDocument();
    expect(screen.getByText('Tool 3')).toBeInTheDocument();
  });

  it('mostra nomes e descrições das ferramentas', () => {
    const onChange = vi.fn();
    render(
      <ProfileToolsSection
        availableTools={mockTools}
        enabledTools={null}
        availableAllowlists={mockAllowlists}
        onChange={onChange}
      />
    );
    expect(screen.getByText('Tool 1')).toBeInTheDocument();
    expect(screen.getByText('Second tool')).toBeInTheDocument();
    expect(screen.getByText('Third tool')).toBeInTheDocument();
  });

  it('marca todas as ferramentas quando enabledTools é null', () => {
    const onChange = vi.fn();
    render(
      <ProfileToolsSection
        availableTools={mockTools}
        enabledTools={null}
        availableAllowlists={mockAllowlists}
        onChange={onChange}
      />
    );
    const rows = screen.getAllByRole('row');
    // row[0] is header, rows 1-3 are tools — all selected
    expect(rows[1]).toHaveAttribute('aria-selected', 'true');
    expect(rows[2]).toHaveAttribute('aria-selected', 'true');
    expect(rows[3]).toHaveAttribute('aria-selected', 'true');
  });

  it('marca apenas ferramentas em enabledTools', () => {
    const onChange = vi.fn();
    render(
      <ProfileToolsSection
        availableTools={mockTools}
        enabledTools={['Tool 1', 'Tool 3']}
        availableAllowlists={mockAllowlists}
        onChange={onChange}
      />
    );
    const rows = screen.getAllByRole('row');
    expect(rows[1]).toHaveAttribute('aria-selected', 'true');
    expect(rows[2]).toHaveAttribute('aria-selected', 'false');
    expect(rows[3]).toHaveAttribute('aria-selected', 'true');
  });

  it('renderiza checkboxes acessíveis dentro das células', () => {
    const onChange = vi.fn();
    render(
      <ProfileToolsSection
        availableTools={mockTools}
        enabledTools={['Tool 1']}
        availableAllowlists={mockAllowlists}
        onChange={onChange}
      />
    );
    const checkboxes = screen.getAllByRole('checkbox');
    const tool1Cb = checkboxes.find(cb => cb.getAttribute('aria-label')?.includes('Tool 1'));
    expect(tool1Cb).toBeTruthy();
    expect((tool1Cb as HTMLInputElement).checked).toBe(true);

    const tool2Cb = checkboxes.find(cb => cb.getAttribute('aria-label')?.includes('Tool 2'));
    expect(tool2Cb).toBeTruthy();
    expect((tool2Cb as HTMLInputElement).checked).toBe(false);
  });

  it('chama onChange ao desmarcar ferramenta de todas selecionadas via Space', () => {
    const onChange = vi.fn();
    render(
      <ProfileToolsSection
        availableTools={mockTools}
        enabledTools={null}
        availableAllowlists={mockAllowlists}
        onChange={onChange}
      />
    );
    const grid = screen.getByRole('grid');
    // Space toggles first tool (Tool 1) — removes it from "all"
    fireEvent.keyDown(grid, { key: ' ' });
    expect(onChange).toHaveBeenCalledWith('enabled_tools', ['Tool 2', 'Tool 3']);
  });

  it('chama onChange ao desmarcar uma ferramenta específica via Space', () => {
    const onChange = vi.fn();
    render(
      <ProfileToolsSection
        availableTools={mockTools}
        enabledTools={['Tool 1', 'Tool 2']}
        availableAllowlists={mockAllowlists}
        onChange={onChange}
      />
    );
    const grid = screen.getByRole('grid');
    fireEvent.keyDown(grid, { key: ' ' });
    expect(onChange).toHaveBeenCalledWith('enabled_tools', ['Tool 2']);
  });

  it('chama onChange ao marcar uma ferramenta desmarcada', () => {
    const onChange = vi.fn();
    render(
      <ProfileToolsSection
        availableTools={mockTools}
        enabledTools={['Tool 1']}
        availableAllowlists={mockAllowlists}
        onChange={onChange}
      />
    );
    const grid = screen.getByRole('grid');
    // Navigate to Tool 2 (row 1) then Space
    fireEvent.keyDown(grid, { key: 'ArrowDown' });
    fireEvent.keyDown(grid, { key: ' ' });
    expect(onChange).toHaveBeenCalledWith('enabled_tools', ['Tool 1', 'Tool 2']);
  });

  it('envia null quando todas as ferramentas são selecionadas', () => {
    const onChange = vi.fn();
    render(
      <ProfileToolsSection
        availableTools={mockTools}
        enabledTools={['Tool 1', 'Tool 2']}
        availableAllowlists={mockAllowlists}
        onChange={onChange}
      />
    );
    const grid = screen.getByRole('grid');
    // Navigate to Tool 3 (row 2) then Space to select all
    fireEvent.keyDown(grid, { key: 'ArrowDown' });
    fireEvent.keyDown(grid, { key: 'ArrowDown' });
    fireEvent.keyDown(grid, { key: ' ' });
    expect(onChange).toHaveBeenCalledWith('enabled_tools', null);
  });

  it('mostra badge com status da feature', () => {
    const onChange = vi.fn();
    render(
      <ProfileToolsSection
        availableTools={mockTools}
        enabledTools={null}
        toolsDisabled={false}
        availableAllowlists={mockAllowlists}
        onChange={onChange}
      />
    );
    expect(screen.getByTestId('badge-on')).toBeInTheDocument();
  });

  it('mostra badge "off" quando toolsDisabled', () => {
    const onChange = vi.fn();
    render(
      <ProfileToolsSection
        availableTools={mockTools}
        enabledTools={null}
        toolsDisabled={true}
        availableAllowlists={mockAllowlists}
        onChange={onChange}
      />
    );
    expect(screen.getByTestId('badge-off')).toBeInTheDocument();
  });

  it('mostra botão "Selecionar todas" quando há ferramentas desmarcadas', () => {
    const onChange = vi.fn();
    render(
      <ProfileToolsSection
        availableTools={mockTools}
        enabledTools={[]}
        availableAllowlists={mockAllowlists}
        onChange={onChange}
      />
    );
    expect(screen.getByTestId('tools-select-all')).toBeInTheDocument();
  });

  it('chama onChange com null ao clicar "Selecionar todas"', () => {
    const onChange = vi.fn();
    render(
      <ProfileToolsSection
        availableTools={mockTools}
        enabledTools={[]}
        availableAllowlists={mockAllowlists}
        onChange={onChange}
      />
    );
    const selectAllBtn = screen.getByTestId('tools-select-all');
    fireEvent.click(selectAllBtn);
    expect(onChange).toHaveBeenCalledWith('enabled_tools', null);
  });

  it('mostra botão "Desmarcar todas" quando pelo menos uma ferramenta selecionada', () => {
    const onChange = vi.fn();
    render(
      <ProfileToolsSection
        availableTools={mockTools}
        enabledTools={['Tool 1']}
        availableAllowlists={mockAllowlists}
        onChange={onChange}
      />
    );
    expect(screen.getByTestId('tools-deselect-all')).toBeInTheDocument();
  });

  it('chama onChange com array vazio ao clicar "Desmarcar todas"', () => {
    const onChange = vi.fn();
    render(
      <ProfileToolsSection
        availableTools={mockTools}
        enabledTools={['Tool 1', 'Tool 2']}
        availableAllowlists={mockAllowlists}
        onChange={onChange}
      />
    );
    const deselectAllBtn = screen.getByTestId('tools-deselect-all');
    fireEvent.click(deselectAllBtn);
    expect(onChange).toHaveBeenCalledWith('enabled_tools', []);
  });

  it('renderiza select de allowlist com opções', () => {
    const onChange = vi.fn();
    render(
      <ProfileToolsSection
        availableTools={mockTools}
        enabledTools={null}
        commandAllowlist=""
        availableAllowlists={mockAllowlists}
        onChange={onChange}
      />
    );
    expect(screen.getByTestId('allowlist-select')).toBeInTheDocument();
    expect(screen.getByText('Padrão')).toBeInTheDocument();
    expect(screen.getByText('Allowlist 1 (5 regras)')).toBeInTheDocument();
    expect(screen.getByText('Allowlist 2 (10 regras)')).toBeInTheDocument();
  });

  it('chama onChange ao alterar select de allowlist', () => {
    const onChange = vi.fn();
    render(
      <ProfileToolsSection
        availableTools={mockTools}
        enabledTools={null}
        commandAllowlist=""
        availableAllowlists={mockAllowlists}
        onChange={onChange}
      />
    );
    const select = screen.getByTestId('allowlist-select') as HTMLSelectElement;
    fireEvent.change(select, { target: { value: 'allowlist-1' } });
    expect(onChange).toHaveBeenCalledWith('command_allowlist', 'allowlist-1');
  });

  it('renderiza mensagem quando nenhuma ferramenta disponível', () => {
    const onChange = vi.fn();
    render(
      <ProfileToolsSection
        availableTools={[]}
        enabledTools={null}
        availableAllowlists={mockAllowlists}
        onChange={onChange}
      />
    );
    expect(screen.getByText('Nenhuma ferramenta encontrada.')).toBeInTheDocument();
  });

  it('desabilita botões da toolbar quando disabled é true', () => {
    const onChange = vi.fn();
    render(
      <ProfileToolsSection
        availableTools={mockTools}
        enabledTools={[]}
        availableAllowlists={mockAllowlists}
        onChange={onChange}
        disabled={true}
      />
    );
    expect(screen.getByTestId('tools-select-all')).toBeDisabled();
    expect(screen.getByTestId('allowlist-select')).toBeDisabled();
  });

  // Agentic Loop tests
  describe('Agentic Loop Fields', () => {
    it('renderiza slider para maxAgenticIterations', () => {
      const onChange = vi.fn();
      render(
        <ProfileToolsSection
          availableTools={mockTools}
          enabledTools={null}
          availableAllowlists={mockAllowlists}
          maxAgenticIterations={25}
          responseTimeout={180}
          onChange={onChange}
        />
      );
      const slider = screen.getByRole('slider', { name: /máximo de iterações/i });
      expect(slider).toBeInTheDocument();
      expect(slider).toHaveAttribute('min', '0');
      expect(slider).toHaveAttribute('max', '1000');
    });

    it('renderiza input para responseTimeout', () => {
      const onChange = vi.fn();
      render(
        <ProfileToolsSection
          availableTools={mockTools}
          enabledTools={null}
          availableAllowlists={mockAllowlists}
          maxAgenticIterations={25}
          responseTimeout={180}
          onChange={onChange}
        />
      );
      const input = screen.getByTestId('response-timeout-input');
      expect(input).toBeInTheDocument();
      expect(input).toHaveAttribute('type', 'number');
      expect(input).toHaveAttribute('min', '5');
      expect(input).toHaveAttribute('max', '600');
    });

    it('exibe valor padrão de maxAgenticIterations', () => {
      const onChange = vi.fn();
      render(
        <ProfileToolsSection
          availableTools={mockTools}
          enabledTools={null}
          availableAllowlists={mockAllowlists}
          maxAgenticIterations={0}
          responseTimeout={180}
          onChange={onChange}
        />
      );
      const slider = screen.getByRole('slider', { name: /máximo de iterações/i });
      expect(slider).toHaveValue('0');
    });

    it('exibe valor customizado de maxAgenticIterations', () => {
      const onChange = vi.fn();
      render(
        <ProfileToolsSection
          availableTools={mockTools}
          enabledTools={null}
          availableAllowlists={mockAllowlists}
          maxAgenticIterations={50}
          responseTimeout={180}
          onChange={onChange}
        />
      );
      const slider = screen.getByRole('slider', { name: /máximo de iterações/i });
      expect(slider).toHaveValue('50');
    });

    it('exibe valor padrão de responseTimeout', () => {
      const onChange = vi.fn();
      render(
        <ProfileToolsSection
          availableTools={mockTools}
          enabledTools={null}
          availableAllowlists={mockAllowlists}
          maxAgenticIterations={25}
          responseTimeout={180}
          onChange={onChange}
        />
      );
      const input = screen.getByTestId('response-timeout-input') as HTMLInputElement;
      expect(input.value).toBe('180');
    });

    it('exibe valor customizado de responseTimeout', () => {
      const onChange = vi.fn();
      render(
        <ProfileToolsSection
          availableTools={mockTools}
          enabledTools={null}
          availableAllowlists={mockAllowlists}
          maxAgenticIterations={25}
          responseTimeout={300}
          onChange={onChange}
        />
      );
      const input = screen.getByTestId('response-timeout-input') as HTMLInputElement;
      expect(input.value).toBe('300');
    });

    it('chama onChange com max_agentic_iterations ao mudar slider', () => {
      const onChange = vi.fn();
      render(
        <ProfileToolsSection
          availableTools={mockTools}
          enabledTools={null}
          availableAllowlists={mockAllowlists}
          maxAgenticIterations={25}
          responseTimeout={180}
          onChange={onChange}
        />
      );
      const slider = screen.getByRole('slider', { name: /máximo de iterações/i });
      fireEvent.change(slider, { target: { value: '50' } });
      expect(onChange).toHaveBeenCalledWith('max_agentic_iterations', 50);
    });

    it('chama onChange com response_timeout ao mudar input', () => {
      const onChange = vi.fn();
      render(
        <ProfileToolsSection
          availableTools={mockTools}
          enabledTools={null}
          availableAllowlists={mockAllowlists}
          maxAgenticIterations={25}
          responseTimeout={180}
          onChange={onChange}
        />
      );
      const input = screen.getByTestId('response-timeout-input');
      fireEvent.change(input, { target: { value: '250' } });
      expect(onChange).toHaveBeenCalledWith('response_timeout', 250);
    });

    it('aplica fallback para 180 com valor vazio no responseTimeout', () => {
      const onChange = vi.fn();
      render(
        <ProfileToolsSection
          availableTools={mockTools}
          enabledTools={null}
          availableAllowlists={mockAllowlists}
          maxAgenticIterations={25}
          responseTimeout={180}
          onChange={onChange}
        />
      );
      const input = screen.getByTestId('response-timeout-input');
      fireEvent.change(input, { target: { value: '' } });
      expect(onChange).toHaveBeenCalledWith('response_timeout', 180);
    });

    it('desabilita campos agentic loop quando disabled é true', () => {
      const onChange = vi.fn();
      render(
        <ProfileToolsSection
          availableTools={mockTools}
          enabledTools={null}
          availableAllowlists={mockAllowlists}
          maxAgenticIterations={25}
          responseTimeout={180}
          onChange={onChange}
          disabled={true}
        />
      );
      const slider = screen.getByRole('slider', { name: /máximo de iterações/i });
      const input = screen.getByTestId('response-timeout-input');
      expect(slider).toHaveAttribute('disabled');
      expect(input).toHaveAttribute('disabled');
    });

    it('respeita limites de responseTimeout (min=5, max=600)', () => {
      const onChange = vi.fn();
      render(
        <ProfileToolsSection
          availableTools={mockTools}
          enabledTools={null}
          availableAllowlists={mockAllowlists}
          maxAgenticIterations={25}
          responseTimeout={180}
          onChange={onChange}
        />
      );
      const input = screen.getByTestId('response-timeout-input');
      expect(input).toHaveAttribute('min', '5');
      expect(input).toHaveAttribute('max', '600');
    });
  });
});
