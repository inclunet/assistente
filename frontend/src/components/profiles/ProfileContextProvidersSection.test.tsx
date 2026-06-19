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

const workspaceProvider = {
  name: 'workspace',
  display_name: 'Workspace',
  description: 'Workspace context',
  default_enabled: true,
  default_budget: 500,
  supports_settings: false,
};

describe('ProfileContextProvidersSection', () => {
  it('remove provider config when clearing the last effective override', () => {
    const onChange = vi.fn();

    render(
      <ProfileContextProvidersSection
        providers={[workspaceProvider]}
        value={{ workspace: { budget: 1200 } }}
        onChange={onChange}
      />,
    );

    fireEvent.change(screen.getByLabelText('Budget em caracteres para Workspace'), {
      target: { value: '' },
    });

    expect(onChange).toHaveBeenCalledWith({});
  });

  it('alternates provider enabled state with Space on the enabled cell', () => {
    const onChange = vi.fn();

    render(
      <ProfileContextProvidersSection
        providers={[workspaceProvider]}
        value={{}}
        onChange={onChange}
      />,
    );

    const grid = screen.getByRole('grid', { name: 'Lista de Context Providers' });
    fireEvent.focus(grid);
    fireEvent.keyDown(grid, { key: ' ' });

    expect(onChange).toHaveBeenCalledWith({ workspace: { enabled: false } });
  });
});
