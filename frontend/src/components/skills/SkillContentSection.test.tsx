import { describe, it, expect, vi } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/react';
import { SkillContentSection } from './SkillContentSection';

vi.mock('react-i18next', () => ({
  initReactI18next: { type: '3rdParty', init: () => {} },
  useTranslation: () => ({
    t: (key: string) => {
      const translations: Record<string, string> = {
        'skills.contentSection.title': 'Conteúdo',
        'skills.contentSection.label': 'Conteúdo do Skill',
        'skills.contentSection.placeholder': 'Descreva como este skill deve ser usado, quais são suas limitações, exemplos de uso, etc.',
        'skills.contentSection.hint': 'Este conteúdo será incluído no system prompt quando o skill estiver ativo.',
      };
      return translations[key] ?? key;
    },
  }),
}));

describe('SkillContentSection', () => {
  it('renderiza conteúdo e dica', () => {
    const onContentChange = vi.fn();
    render(
      <SkillContentSection
        content="Use este skill com cuidado."
        onContentChange={onContentChange}
      />
    );

    expect(screen.getByLabelText('Conteúdo do Skill')).toHaveValue(
      'Use este skill com cuidado.'
    );
    expect(
      screen.getByText('Este conteúdo será incluído no system prompt quando o skill estiver ativo.')
    ).toBeInTheDocument();
  });

  it('dispara onContentChange ao editar', async () => {
    const onContentChange = vi.fn();
    render(
      <SkillContentSection
        content=""
        onContentChange={onContentChange}
      />
    );

    fireEvent.change(screen.getByLabelText('Conteúdo do Skill'), {
      target: { value: 'Novo conteúdo' },
    });

    expect(onContentChange).toHaveBeenCalledWith('Novo conteúdo');
  });

  it('exibe placeholder esperado', () => {
    const onContentChange = vi.fn();
    render(
      <SkillContentSection
        content=""
        onContentChange={onContentChange}
      />
    );

    expect(screen.getByLabelText('Conteúdo do Skill')).toHaveAttribute(
      'placeholder',
      'Descreva como este skill deve ser usado, quais são suas limitações, exemplos de uso, etc.'
    );
  });
});
