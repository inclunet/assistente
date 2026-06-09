import { describe, it, expect, vi } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { SkillCatalogSection } from './SkillCatalogSection';

vi.mock('react-i18next', () => ({
  initReactI18next: { type: '3rdParty', init: () => {} },
  useTranslation: () => ({
    t: (key: string, fallback?: string) => fallback ?? key,
  }),
}));

describe('SkillCatalogSection', () => {
  it('renderiza budget e checkboxes de capacidade com valores atuais', () => {
    const onFieldChange = vi.fn();
    render(
      <SkillCatalogSection
        item={{ contextBudget: 42, requiresNetwork: true }}
        onFieldChange={onFieldChange}
      />
    );

    expect(screen.getByLabelText('Orçamento de contexto (tokens)')).toHaveValue(42);
    expect(screen.getByLabelText('Requer rede')).toBeChecked();
    expect(screen.getByLabelText('Requer ferramentas')).not.toBeChecked();
  });

  it('dispara onFieldChange ao editar budget e justificativa', () => {
    const onFieldChange = vi.fn();
    render(<SkillCatalogSection item={{}} onFieldChange={onFieldChange} />);

    fireEvent.change(screen.getByLabelText('Orçamento de contexto (tokens)'), {
      target: { value: '100' },
    });
    fireEvent.change(screen.getByLabelText('Justificativa do auto_load'), {
      target: { value: 'sempre necessário' },
    });

    expect(onFieldChange).toHaveBeenCalledWith('contextBudget', 100);
    expect(onFieldChange).toHaveBeenCalledWith('autoloadReason', 'sempre necessário');
  });

  it('normaliza budget negativo para zero', () => {
    const onFieldChange = vi.fn();
    render(<SkillCatalogSection item={{}} onFieldChange={onFieldChange} />);

    fireEvent.change(screen.getByLabelText('Orçamento de contexto (tokens)'), {
      target: { value: '-5' },
    });

    expect(onFieldChange).toHaveBeenCalledWith('contextBudget', 0);
  });

  it('dispara onFieldChange ao alternar capacidade', async () => {
    const user = userEvent.setup();
    const onFieldChange = vi.fn();
    render(<SkillCatalogSection item={{}} onFieldChange={onFieldChange} />);

    await user.click(screen.getByLabelText('Requer MCP'));

    expect(onFieldChange).toHaveBeenCalledWith('requiresMcp', true);
  });
});
