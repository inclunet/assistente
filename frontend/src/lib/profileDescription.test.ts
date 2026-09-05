import type { TFunction } from 'i18next';
import { describe, expect, it, vi } from 'vitest';
import { profileDisplayDescription } from './profileDescription';

describe('profileDisplayDescription', () => {
  it('localiza descrições builtin pelo slug', () => {
    const t = vi.fn((key: string) => key === 'profiles.builtinDescriptions.programacao'
      ? 'Localized programming description'
      : key) as unknown as TFunction;

    expect(profileDisplayDescription(t, {
      slug: 'programacao',
      description: 'Descrição canônica',
      builtin: true,
    })).toBe('Localized programming description');
    expect(t).toHaveBeenCalledWith(
      'profiles.builtinDescriptions.programacao',
      'Descrição canônica',
    );
  });

  it('preserva descrições de profiles customizados', () => {
    const t = vi.fn() as unknown as TFunction;

    expect(profileDisplayDescription(t, {
      slug: 'programacao',
      description: 'Minha descrição personalizada',
      builtin: false,
    })).toBe('Minha descrição personalizada');
    expect(t).not.toHaveBeenCalled();
  });
});
