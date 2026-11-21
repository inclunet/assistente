import { describe, it, expect, vi } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/react';
import { SkillToolsSection } from './SkillToolsSection';

const announce = vi.hoisted(() => vi.fn());
vi.mock('../../hooks/useAnnouncer', () => ({ useAnnouncer: () => ({ announce }) }));

vi.mock('react-i18next', () => ({
  initReactI18next: { type: '3rdParty', init: () => {} },
  useTranslation: () => ({
    t: (key: string) => {
      const translations: Record<string, string> = {
        'skills.toolsSection.title': 'Ferramentas Associadas',
        'skills.toolsSection.label': 'Ferramentas (separadas por vírgula)',
        'skills.toolsSection.placeholder': 'Ex: tool1, tool2, tool3',
        'skills.toolsSection.hint': 'Liste as ferramentas (tool calling) que podem ser usadas neste skill.',
      };
      return translations[key] ?? key;
    },
  }),
}));

describe('SkillToolsSection', () => {
  it('avisa quando as ferramentas impedem o uso dos programas de shell declarados', () => {
    announce.mockClear();
    const { rerender } = render(<SkillToolsSection toolsString="read_file" declaresShellPrograms onToolsChange={vi.fn()} />);
    expect(screen.getByText('skills.toolsSection.shellConflict')).toHaveTextContent('skills.toolsSection.shellConflict');
    rerender(<SkillToolsSection toolsString="read_file, run_command" declaresShellPrograms onToolsChange={vi.fn()} />);
    expect(screen.queryByText('skills.toolsSection.shellConflict')).toBeNull();
    rerender(<SkillToolsSection toolsString="run_command" declaresShellPrograms deniesShell onToolsChange={vi.fn()} />);
    expect(screen.getByText('skills.toolsSection.shellConflict')).toBeInTheDocument();
    expect(announce).toHaveBeenCalledTimes(1);
  });

  it('descreve o conflito inicial e anuncia uma nova incompatibilidade durante a edição', () => {
    announce.mockClear();
    const { rerender } = render(<SkillToolsSection toolsString="read_file" declaresShellPrograms onToolsChange={vi.fn()} />);
    expect(screen.getByRole('textbox')).toHaveAccessibleDescription('skills.toolsSection.shellConflict');
    expect(announce).not.toHaveBeenCalled();
    rerender(<SkillToolsSection toolsString="run_command" declaresShellPrograms onToolsChange={vi.fn()} />);
    rerender(<SkillToolsSection toolsString="read_file" declaresShellPrograms onToolsChange={vi.fn()} />);
    expect(announce).toHaveBeenCalledTimes(1);
  });

  it('anuncia uma vez ao trocar de skill sem desmontar a seção', () => {
    announce.mockClear();
    const onToolsChange = vi.fn();
    const primeira = { toolsString: 'run_command', declaresShellPrograms: true };
    const segunda = { toolsString: 'read_file', declaresShellPrograms: true };
    const { rerender } = render(<SkillToolsSection {...primeira} onToolsChange={onToolsChange} />);
    const campo = screen.getByRole('textbox');
    expect(announce).not.toHaveBeenCalled();

    rerender(<SkillToolsSection {...segunda} onToolsChange={onToolsChange} />);
    expect(screen.getByRole('textbox')).toBe(campo);
    expect(campo).toHaveAccessibleDescription('skills.toolsSection.shellConflict');
    expect(announce).toHaveBeenCalledWith('skills.toolsSection.shellConflict');
    rerender(<SkillToolsSection {...segunda} onToolsChange={onToolsChange} />);
    expect(announce).toHaveBeenCalledTimes(1);
  });

  it('renderiza string de ferramentas e dica', () => {
    const onToolsChange = vi.fn();
    render(
      <SkillToolsSection
        toolsString="tool1, tool2"
        onToolsChange={onToolsChange}
      />
    );

    expect(screen.getByLabelText('Ferramentas (separadas por vírgula)')).toHaveValue(
      'tool1, tool2'
    );
    expect(
      screen.getByText('Liste as ferramentas (tool calling) que podem ser usadas neste skill.')
    ).toBeInTheDocument();
    expect(screen.getByTestId('skill-tools-section')).toHaveClass('skills-section');
    expect(screen.getByLabelText('Ferramentas (separadas por vírgula)')).toHaveClass('skills-field__input');
  });

  it('dispara onToolsChange ao editar', async () => {
    const onToolsChange = vi.fn();
    render(
      <SkillToolsSection
        toolsString=""
        onToolsChange={onToolsChange}
      />
    );

    fireEvent.change(screen.getByLabelText('Ferramentas (separadas por vírgula)'), {
      target: { value: 'toolA, toolB' },
    });

    expect(onToolsChange).toHaveBeenCalledWith('toolA, toolB');
  });

  it('exibe placeholder esperado', () => {
    const onToolsChange = vi.fn();
    render(
      <SkillToolsSection
        toolsString=""
        onToolsChange={onToolsChange}
      />
    );

    expect(screen.getByLabelText('Ferramentas (separadas por vírgula)')).toHaveAttribute(
      'placeholder',
      'Ex: tool1, tool2, tool3'
    );
  });
});
