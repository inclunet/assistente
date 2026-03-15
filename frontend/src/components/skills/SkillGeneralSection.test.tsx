import { describe, it, expect, vi } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { SkillGeneralSection } from './SkillGeneralSection';

vi.mock('react-i18next', () => ({
  initReactI18next: { type: '3rdParty', init: () => {} },
  useTranslation: () => ({
    t: (key: string) => {
      const translations: Record<string, string> = {
        'skills.generalSection.title': 'Geral',
        'skills.generalSection.name': 'Nome',
        'skills.generalSection.namePlaceholder': 'Ex: Criar Componente React',
        'skills.generalSection.description': 'Descrição',
        'skills.generalSection.descriptionPlaceholder': 'Quando este skill deve ser usado',
        'skills.generalSection.auto': 'Auto — injetar automaticamente no system prompt',
      };
      return translations[key] ?? key;
    },
  }),
}));

describe('SkillGeneralSection', () => {
  it('renderiza campos principais e checkbox', () => {
    const onFieldChange = vi.fn();
    render(
      <SkillGeneralSection
        item={{ name: 'Skill X', description: 'Desc', auto: true }}
        onFieldChange={onFieldChange}
      />
    );

    expect(screen.getByLabelText('Nome')).toHaveValue('Skill X');
    expect(screen.getByLabelText('Descrição')).toHaveValue('Desc');
    expect(screen.getByLabelText(/Auto —/)).toBeChecked();
  });

  it('dispara onFieldChange ao editar nome e descrição', async () => {
    const onFieldChange = vi.fn();
    render(
      <SkillGeneralSection
        item={{ name: '', description: '', auto: false }}
        onFieldChange={onFieldChange}
      />
    );

    fireEvent.change(screen.getByLabelText('Nome'), { target: { value: 'Nova' } });
    fireEvent.change(screen.getByLabelText('Descrição'), { target: { value: 'Detalhe' } });

    expect(onFieldChange).toHaveBeenCalledWith('name', 'Nova');
    expect(onFieldChange).toHaveBeenCalledWith('description', 'Detalhe');
  });

  it('dispara onFieldChange ao alternar auto', async () => {
    const user = userEvent.setup();
    const onFieldChange = vi.fn();
    render(
      <SkillGeneralSection
        item={{ auto: false }}
        onFieldChange={onFieldChange}
      />
    );

    await user.click(screen.getByLabelText(/Auto —/));

    expect(onFieldChange).toHaveBeenCalledWith('auto', true);
  });

  it('exibe placeholders esperados', () => {
    const onFieldChange = vi.fn();
    render(
      <SkillGeneralSection
        item={{ name: '', description: '', auto: false }}
        onFieldChange={onFieldChange}
      />
    );

    expect(screen.getByLabelText('Nome')).toHaveAttribute(
      'placeholder',
      'Ex: Criar Componente React'
    );
    expect(screen.getByLabelText('Descrição')).toHaveAttribute(
      'placeholder',
      'Quando este skill deve ser usado'
    );
  });
});
