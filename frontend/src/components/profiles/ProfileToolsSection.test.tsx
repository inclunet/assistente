import { describe, it, expect, vi, beforeAll } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/react';
import { ProfileToolsSection } from './ProfileToolsSection';
import { allowlist } from '../../../wailsjs/go/models';

vi.mock('@wailsjs/go/models', () => ({}));

vi.mock('react-i18next', () => ({
  initReactI18next: { type: '3rdParty', init: () => {} },
  useTranslation: () => ({
    t: (_key: string, defaultValue: string, options?: Record<string, string>) => {
      if (!options) return defaultValue;
      return Object.entries(options).reduce(
        (text, [key, value]) => text.replace(new RegExp(`{{${key}}}`, 'g'), value),
        defaultValue,
      );
    },
  }),
}));

vi.mock('../../services/audioFeedback', () => ({
  playBumpSound: vi.fn(),
}));

vi.mock('../../store/mcpStore', () => ({
  useMCPStore: (selector: (s: { servers: Array<{ slug: string; name: string }> }) => unknown) =>
    selector({ servers: [
      { slug: 'atlassian', name: 'Atlassian' },
      { slug: 'slack', name: 'Slack' },
    ] }),
}));

beforeAll(() => {
  Element.prototype.scrollIntoView = vi.fn();
});

function tool(name: string, description: string, sourceType = 'local', sourceLabel = 'Local', displayName?: string) {
  return { name, display_name: displayName ?? name, description, source_type: sourceType, source_label: sourceLabel };
}

const mockTools = [
  tool('Tool 1', 'First tool'),
  tool('Tool 2', 'Second tool'),
  tool('Tool 3', 'Third tool'),
];

const mockToolsMixed = [
  tool('local_tool', 'Local tool'),
  tool('mcp_atlassian__search', 'Atlassian search', 'mcp', 'Atlassian', 'search'),
  tool('mcp_atlassian__create', 'Atlassian create', 'mcp', 'Atlassian', 'create'),
  tool('mcp_slack__send', 'Slack send', 'mcp', 'Slack', 'send'),
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

  it('reflete o estado tri-state de MCP nativo (auto/on/off)', () => {
    const onChange = vi.fn();
    const { rerender } = render(
      <ProfileToolsSection
        availableTools={mockTools}
        enabledTools={null}
        availableAllowlists={mockAllowlists}
        nativeMcp={null}
        onChange={onChange}
      />
    );
    const select = screen.getByTestId('native-mcp-select') as HTMLSelectElement;
    expect(select.value).toBe('auto');

    rerender(
      <ProfileToolsSection
        availableTools={mockTools}
        enabledTools={null}
        availableAllowlists={mockAllowlists}
        nativeMcp={true}
        onChange={onChange}
      />
    );
    expect((screen.getByTestId('native-mcp-select') as HTMLSelectElement).value).toBe('on');

    rerender(
      <ProfileToolsSection
        availableTools={mockTools}
        enabledTools={null}
        availableAllowlists={mockAllowlists}
        nativeMcp={false}
        onChange={onChange}
      />
    );
    expect((screen.getByTestId('native-mcp-select') as HTMLSelectElement).value).toBe('off');
  });

  it('emite native_mcp com null/true/false ao trocar a seleção', () => {
    const onChange = vi.fn();
    render(
      <ProfileToolsSection
        availableTools={mockTools}
        enabledTools={null}
        availableAllowlists={mockAllowlists}
        nativeMcp={null}
        onChange={onChange}
      />
    );
    const select = screen.getByTestId('native-mcp-select');
    fireEvent.change(select, { target: { value: 'on' } });
    expect(onChange).toHaveBeenCalledWith('native_mcp', true);
    fireEvent.change(select, { target: { value: 'off' } });
    expect(onChange).toHaveBeenCalledWith('native_mcp', false);
    fireEvent.change(select, { target: { value: 'auto' } });
    expect(onChange).toHaveBeenCalledWith('native_mcp', null);
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

  it('renderiza estados tri-state acessíveis dentro das células', () => {
    const onChange = vi.fn();
    render(
      <ProfileToolsSection
        availableTools={mockTools}
        enabledTools={['Tool 1']}
        availableAllowlists={mockAllowlists}
        onChange={onChange}
      />
    );
    expect(screen.getByLabelText('Tool 1: Pré-carregada')).toBeInTheDocument();
    expect(screen.getByLabelText('Tool 2: Desabilitada')).toBeInTheDocument();
  });

  it('chama onChange ao promover ferramenta sob demanda via Space', () => {
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
    // Perfil legado aberto: tools começam sob demanda; Space promove para preloaded.
    fireEvent.focus(grid);
    fireEvent.keyDown(grid, { key: ' ' });
    expect(onChange).toHaveBeenCalledWith('tool_policy', {
      'Tool 1': 'preloaded',
      'Tool 2': 'on_demand',
      'Tool 3': 'on_demand',
    });
  });

  it('chama onChange ao desabilitar ferramenta preloaded via Space', () => {
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
    fireEvent.focus(grid);
    fireEvent.keyDown(grid, { key: ' ' });
    expect(onChange).toHaveBeenCalledWith('tool_policy', {
      'Tool 1': 'disabled',
      'Tool 2': 'preloaded',
      'Tool 3': 'disabled',
    });
  });

  it('chama onChange ao mover ferramenta disabled para on_demand', () => {
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
    fireEvent.focus(grid);
    fireEvent.keyDown(grid, { key: 'ArrowDown' });
    fireEvent.keyDown(grid, { key: ' ' });
    expect(onChange).toHaveBeenCalledWith('tool_policy', {
      'Tool 1': 'preloaded',
      'Tool 2': 'on_demand',
      'Tool 3': 'disabled',
    });
  });

  it('cicla ferramenta disabled para on_demand sem promover lote inteiro', () => {
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
    fireEvent.focus(grid);
    fireEvent.keyDown(grid, { key: 'ArrowDown' });
    fireEvent.keyDown(grid, { key: 'ArrowDown' });
    fireEvent.keyDown(grid, { key: ' ' });
    expect(onChange).toHaveBeenCalledWith('tool_policy', {
      'Tool 1': 'preloaded',
      'Tool 2': 'preloaded',
      'Tool 3': 'on_demand',
    });
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

  it('mostra botão de pré-carregar quando há ferramentas não carregadas', () => {
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

  it('chama onChange com tool_policy preloaded ao clicar pré-carregar', () => {
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
    expect(onChange).toHaveBeenCalledWith('tool_policy', {
      'Tool 1': 'preloaded',
      'Tool 2': 'preloaded',
      'Tool 3': 'preloaded',
    });
  });

  it('mostra botão de desabilitar quando pelo menos uma ferramenta está disponível', () => {
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

  it('chama onChange com tool_policy disabled ao clicar desabilitar', () => {
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
    expect(onChange).toHaveBeenCalledWith('tool_policy', {
      'Tool 1': 'disabled',
      'Tool 2': 'disabled',
      'Tool 3': 'disabled',
    });
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
        <ProfileToolsSection
          availableTools={mockTools}
          enabledTools={null}
          availableAllowlists={mockAllowlists}
          onChange={vi.fn()}
        />
      );
      expect(screen.getByTestId('tools-search')).toBeInTheDocument();
    });

    it('renderiza picker de filtro na toolbar', () => {
      render(
        <ProfileToolsSection
          availableTools={mockTools}
          enabledTools={null}
          availableAllowlists={mockAllowlists}
          onChange={vi.fn()}
        />
      );
      expect(screen.getByTestId('tools-filter')).toBeInTheDocument();
    });

    it('filtra ferramentas por texto de busca', () => {
      render(
        <ProfileToolsSection
          availableTools={mockTools}
          enabledTools={null}
          availableAllowlists={mockAllowlists}
          onChange={vi.fn()}
        />
      );
      const searchInput = screen.getByTestId('tools-search');
      fireEvent.change(searchInput, { target: { value: 'First' } });
      expect(screen.getByText('Tool 1')).toBeInTheDocument();
      expect(screen.queryByText('Tool 2')).not.toBeInTheDocument();
      expect(screen.queryByText('Tool 3')).not.toBeInTheDocument();
    });

    it('filtra por "Locais" mostrando somente ferramentas não-MCP', () => {
      render(
        <ProfileToolsSection
          availableTools={mockToolsMixed}
          enabledTools={null}
          availableAllowlists={mockAllowlists}
          onChange={vi.fn()}
        />
      );
      selectFilter('tools-filter', 'Locais');
      expect(screen.getByText('local_tool')).toBeInTheDocument();
      expect(screen.queryByText('mcp_atlassian__search')).not.toBeInTheDocument();
      expect(screen.queryByText('mcp_slack__send')).not.toBeInTheDocument();
    });

    it('filtra por "Todas MCP" mostrando somente ferramentas MCP', () => {
      render(
        <ProfileToolsSection
          availableTools={mockToolsMixed}
          enabledTools={null}
          availableAllowlists={mockAllowlists}
          onChange={vi.fn()}
        />
      );
      selectFilter('tools-filter', 'Todas MCP');
      expect(screen.queryByText('local_tool')).not.toBeInTheDocument();
      expect(screen.getByText('search')).toBeInTheDocument();
      expect(screen.getByText('send')).toBeInTheDocument();
    });

    it('filtra por servidor MCP específico', () => {
      render(
        <ProfileToolsSection
          availableTools={mockToolsMixed}
          enabledTools={null}
          availableAllowlists={mockAllowlists}
          onChange={vi.fn()}
        />
      );
      selectFilter('tools-filter', 'Atlassian');
      expect(screen.getByText('search')).toBeInTheDocument();
      expect(screen.getByText('create')).toBeInTheDocument();
      expect(screen.queryByText('send')).not.toBeInTheDocument();
      expect(screen.queryByText('local_tool')).not.toBeInTheDocument();
    });

    it('mostra mensagem quando filtro não retorna resultados', () => {
      render(
        <ProfileToolsSection
          availableTools={mockTools}
          enabledTools={null}
          availableAllowlists={mockAllowlists}
          onChange={vi.fn()}
        />
      );
      fireEvent.change(screen.getByTestId('tools-search'), { target: { value: 'inexistente' } });
      expect(screen.getByText('Nenhuma ferramenta corresponde ao filtro.')).toBeInTheDocument();
    });

    it('"Pré-carregar filtradas" altera apenas itens filtrados', () => {
      const onChange = vi.fn();
      render(
        <ProfileToolsSection
          availableTools={mockToolsMixed}
          enabledTools={[]}
          availableAllowlists={mockAllowlists}
          onChange={onChange}
        />
      );
      selectFilter('tools-filter', 'Atlassian');
      fireEvent.click(screen.getByTestId('tools-select-all'));
      expect(onChange).toHaveBeenCalledWith('tool_policy', {
        local_tool: 'disabled',
        mcp_atlassian__search: 'preloaded',
        mcp_atlassian__create: 'preloaded',
        mcp_slack__send: 'disabled',
      });
    });

    it('"Pré-carregar filtradas" aparece para itens sob demanda filtrados', () => {
      const onChange = vi.fn();
      render(
        <ProfileToolsSection
          availableTools={mockToolsMixed}
          enabledTools={null}
          availableAllowlists={mockAllowlists}
          onChange={onChange}
        />
      );
      selectFilter('tools-filter', 'Atlassian');
      fireEvent.click(screen.getByTestId('tools-select-all'));
      expect(onChange).toHaveBeenCalledWith('tool_policy', {
        local_tool: 'on_demand',
        mcp_atlassian__search: 'preloaded',
        mcp_atlassian__create: 'preloaded',
        mcp_slack__send: 'on_demand',
      });
    });

    it('"Desabilitar filtradas" altera apenas itens filtrados', () => {
      const onChange = vi.fn();
      render(
        <ProfileToolsSection
          availableTools={mockToolsMixed}
          enabledTools={null}
          availableAllowlists={mockAllowlists}
          onChange={onChange}
        />
      );
      selectFilter('tools-filter', 'Atlassian');
      fireEvent.click(screen.getByTestId('tools-deselect-all'));
      const call = onChange.mock.calls[0];
      expect(call[0]).toBe('tool_policy');
      expect(call[1]).toEqual({
        local_tool: 'on_demand',
        mcp_atlassian__search: 'disabled',
        mcp_atlassian__create: 'disabled',
        mcp_slack__send: 'on_demand',
      });
    });

    it('combina busca de texto com filtro de origem', () => {
      render(
        <ProfileToolsSection
          availableTools={mockToolsMixed}
          enabledTools={null}
          availableAllowlists={mockAllowlists}
          onChange={vi.fn()}
        />
      );
      selectFilter('tools-filter', 'Todas MCP');
      fireEvent.change(screen.getByTestId('tools-search'), { target: { value: 'search' } });
      expect(screen.getByText('search')).toBeInTheDocument();
      expect(screen.queryByText('create')).not.toBeInTheDocument();
      expect(screen.queryByText('send')).not.toBeInTheDocument();
    });
  });

  describe('Display names e coluna de origem', () => {
    it('exibe displayName curto em vez do nome namespaced para tools MCP', () => {
      render(
        <ProfileToolsSection
          availableTools={mockToolsMixed}
          enabledTools={null}
          availableAllowlists={mockAllowlists}
          onChange={vi.fn()}
        />
      );
      expect(screen.getByText('search')).toBeInTheDocument();
      expect(screen.getByText('create')).toBeInTheDocument();
      expect(screen.getByText('send')).toBeInTheDocument();
      expect(screen.queryByText('mcp_atlassian__search')).not.toBeInTheDocument();
      expect(screen.queryByText('mcp_slack__send')).not.toBeInTheDocument();
    });

    it('exibe nome original para tools locais', () => {
      render(
        <ProfileToolsSection
          availableTools={mockToolsMixed}
          enabledTools={null}
          availableAllowlists={mockAllowlists}
          onChange={vi.fn()}
        />
      );
      expect(screen.getByText('local_tool')).toBeInTheDocument();
    });

    it('exibe coluna Origem com labels corretos', () => {
      render(
        <ProfileToolsSection
          availableTools={mockToolsMixed}
          enabledTools={null}
          availableAllowlists={mockAllowlists}
          onChange={vi.fn()}
        />
      );
      expect(screen.getByText('Origem')).toBeInTheDocument();
      expect(screen.getByText('Local')).toBeInTheDocument();
      const atlassianBadges = screen.getAllByText('Atlassian');
      expect(atlassianBadges.length).toBeGreaterThanOrEqual(1);
      const slackBadges = screen.getAllByText('Slack');
      expect(slackBadges.length).toBeGreaterThanOrEqual(1);
    });

    it('busca encontra tools pelo nome completo namespaced', () => {
      render(
        <ProfileToolsSection
          availableTools={mockToolsMixed}
          enabledTools={null}
          availableAllowlists={mockAllowlists}
          onChange={vi.fn()}
        />
      );
      fireEvent.change(screen.getByTestId('tools-search'), { target: { value: 'mcp_atlassian' } });
      expect(screen.getByText('search')).toBeInTheDocument();
      expect(screen.getByText('create')).toBeInTheDocument();
      expect(screen.queryByText('send')).not.toBeInTheDocument();
      expect(screen.queryByText('local_tool')).not.toBeInTheDocument();
    });

    it('busca encontra tools pelo sourceLabel', () => {
      render(
        <ProfileToolsSection
          availableTools={mockToolsMixed}
          enabledTools={null}
          availableAllowlists={mockAllowlists}
          onChange={vi.fn()}
        />
      );
      fireEvent.change(screen.getByTestId('tools-search'), { target: { value: 'Slack' } });
      expect(screen.getByText('send')).toBeInTheDocument();
      expect(screen.queryByText('search')).not.toBeInTheDocument();
      expect(screen.queryByText('local_tool')).not.toBeInTheDocument();
    });

    it('aria-label do estado inclui displayName e sourceLabel para MCP', () => {
      render(
        <ProfileToolsSection
          availableTools={mockToolsMixed}
          enabledTools={null}
          availableAllowlists={mockAllowlists}
          onChange={vi.fn()}
        />
      );
      expect(screen.getByLabelText('search (Atlassian): Sob demanda')).toBeInTheDocument();
      expect(screen.getByLabelText('send (Slack): Sob demanda')).toBeInTheDocument();
      expect(screen.getByLabelText('local_tool: Sob demanda')).toBeInTheDocument();
    });

    it('enabled_tools continua usando nomes namespaced internos', () => {
      const onChange = vi.fn();
      render(
        <ProfileToolsSection
          availableTools={mockToolsMixed}
          enabledTools={['mcp_atlassian__search', 'local_tool']}
          availableAllowlists={mockAllowlists}
          onChange={onChange}
        />
      );
      expect(screen.getByLabelText('search (Atlassian): Pré-carregada')).toBeInTheDocument();
      expect(screen.getByLabelText('local_tool: Pré-carregada')).toBeInTheDocument();
      expect(screen.getByLabelText('create (Atlassian): Desabilitada')).toBeInTheDocument();
      expect(screen.getByLabelText('send (Slack): Desabilitada')).toBeInTheDocument();
    });
  });
});
