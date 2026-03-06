import { describe, it, expect, vi } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/react';
import { SkillToolsSection } from './SkillToolsSection';

describe('SkillToolsSection', () => {
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
