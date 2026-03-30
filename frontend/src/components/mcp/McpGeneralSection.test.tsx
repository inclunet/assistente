import { describe, expect, it, vi } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/react';
import { McpGeneralSection } from './McpGeneralSection';

vi.mock('react-i18next', async (importOriginal) => {
  const actual = await importOriginal<typeof import('react-i18next')>();
  const map: Record<string, string> = {
    'mcp.general.title': 'Geral',
    'common.name': 'Nome',
    'common.description': 'Descrição',
    'mcp.general.namePlaceholder': 'ex: GitHub Tools',
    'mcp.general.descriptionPlaceholder': 'Descrição opcional do servidor',
    'mcp.general.transport': 'Transporte',
    'mcp.general.transportStdio': 'stdio (processo local)',
    'mcp.general.transportHTTP': 'Streamable HTTP (recomendado)',
  };
  return {
    ...actual,
    useTranslation: () => ({
      t: (key: string, options?: string | Record<string, unknown>) => {
        if (typeof options === 'string') {
          return options;
        }
        if (options && typeof options === 'object' && options !== null) {
          const def = (options as { defaultValue?: string }).defaultValue;
          if (typeof def === 'string') {
            return def;
          }
        }
        return map[key] ?? key;
      },
    }),
  };
});

describe('McpGeneralSection', () => {
  it('renderiza campos e dispara handlers', () => {
    const onNameChange = vi.fn();
    const onDescriptionChange = vi.fn();
    const onTransportChange = vi.fn();

    render(
      <McpGeneralSection
        name=""
        description=""
        transport="stdio"
        onNameChange={onNameChange}
        onDescriptionChange={onDescriptionChange}
        onTransportChange={onTransportChange}
      />
    );

    fireEvent.change(screen.getByLabelText(/Nome/), { target: { value: 'GitHub' } });
    fireEvent.change(screen.getByLabelText(/Descrição/), { target: { value: 'Desc' } });
    fireEvent.change(screen.getByLabelText(/Transporte/), { target: { value: 'streamable' } });

    expect(onNameChange).toHaveBeenCalledWith('GitHub');
    expect(onDescriptionChange).toHaveBeenCalledWith('Desc');
    expect(onTransportChange).toHaveBeenCalledWith('streamable');
  });
});
