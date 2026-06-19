import { describe, expect, it, vi } from 'vitest';
import { fireEvent, render, screen } from '@testing-library/react';
import { ProfileContextProvidersSection } from './ProfileContextProvidersSection';

vi.mock('react-i18next', () => ({
  useTranslation: () => ({
    t: (_key: string, fallback?: string, values?: Record<string, string>) => {
      if (!fallback) return _key;
      return Object.entries(values ?? {}).reduce(
        (text, [name, value]) => text.replace(`{{${name}}}`, value),
        fallback,
      );
    },
  }),
}));

vi.mock('@wailsjs/go/models', () => ({
  contextprovider: {},
  profiles: {},
}));

describe('ProfileContextProvidersSection', () => {
  it('remove provider config when clearing the last effective override', () => {
    const onChange = vi.fn();

    render(
      <ProfileContextProvidersSection
        providers={[{
          name: 'workspace',
          display_name: 'Workspace',
          description: 'Workspace context',
          default_enabled: true,
          default_budget: 500,
          supports_settings: false,
        }]}
        value={{ workspace: { budget: 1200 } }}
        onChange={onChange}
      />,
    );

    fireEvent.change(screen.getByLabelText('Budget em caracteres para Workspace'), {
      target: { value: '' },
    });

    expect(onChange).toHaveBeenCalledWith({});
  });
});
