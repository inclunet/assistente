import { describe, it, expect, vi } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { SkillCatalogSection } from './SkillCatalogSection';

vi.mock('react-i18next', () => ({
  initReactI18next: { type: '3rdParty', init: () => {} },
  useTranslation: () => ({
    t: (key: string) => {
      const translations: Record<string, string> = {
        'skills.catalogSection.title': 'Catálogo e carregamento',
        'skills.catalogSection.autoLoad': 'Auto-load — injetar automaticamente no system prompt',
        'skills.catalogSection.autoLoadHint':
          'Quando ativo, o corpo do skill é injetado em toda conversa. Use com parcimônia e justifique abaixo.',
        'skills.catalogSection.autoloadReason': 'Justificativa do auto_load',
        'skills.catalogSection.autoloadReasonPlaceholder':
          'Por que esta skill precisa estar sempre no prompt?',
        'skills.catalogSection.autoloadReasonHint':
          'Obrigatória quando auto_load está ativo: sem a justificativa, o salvamento é rejeitado.',
        'skills.catalogSection.contextBudget': 'Orçamento de contexto (tokens)',
        'skills.catalogSection.contextBudgetHint':
          'Custo aproximado do corpo. 0 = estimado automaticamente pelo tamanho.',
        'skills.catalogSection.requiresLegend': 'Pré-condições de capacidade',
        'skills.catalogSection.requiresTools': 'Requer ferramentas',
        'skills.catalogSection.requiresFilesystem': 'Requer sistema de arquivos',
        'skills.catalogSection.requiresNetwork': 'Requer rede',
        'skills.catalogSection.requiresMcp': 'Requer MCP',
        'skills.catalogSection.requiresHint':
          'Skills que exigem uma capacidade desabilitada são omitidas do prompt.',
      };
      return translations[key] ?? key;
    },
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

  it('reflete e alterna auto_load', async () => {
    const user = userEvent.setup();
    const onFieldChange = vi.fn();
    const { rerender } = render(
      <SkillCatalogSection item={{ autoLoad: true }} onFieldChange={onFieldChange} />
    );

    const checkbox = screen.getByLabelText('Auto-load — injetar automaticamente no system prompt');
    expect(checkbox).toBeChecked();

    rerender(<SkillCatalogSection item={{ autoLoad: false }} onFieldChange={onFieldChange} />);
    await user.click(
      screen.getByLabelText('Auto-load — injetar automaticamente no system prompt')
    );
    expect(onFieldChange).toHaveBeenCalledWith('autoLoad', true);
  });
});
