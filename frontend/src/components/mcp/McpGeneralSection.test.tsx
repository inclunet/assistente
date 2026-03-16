import { describe, expect, it, vi } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/react';
import { McpGeneralSection } from './McpGeneralSection';

describe('McpGeneralSection', () => {
  it('renderiza campos e dispara handlers', () => {
    const onSlugChange = vi.fn();
    const onNameChange = vi.fn();
    const onDescriptionChange = vi.fn();
    const onTransportChange = vi.fn();

    render(
      <McpGeneralSection
        isNew={true}
        slug=""
        name=""
        description=""
        transport="stdio"
        onSlugChange={onSlugChange}
        onNameChange={onNameChange}
        onDescriptionChange={onDescriptionChange}
        onTransportChange={onTransportChange}
      />
    );

    fireEvent.change(screen.getByLabelText(/Slug/), { target: { value: 'github' } });
    fireEvent.change(screen.getByLabelText(/Nome/), { target: { value: 'GitHub' } });
    fireEvent.change(screen.getByLabelText(/Descrição/), { target: { value: 'Desc' } });
    fireEvent.change(screen.getByLabelText(/Transporte/), { target: { value: 'sse' } });

    expect(onSlugChange).toHaveBeenCalledWith('github');
    expect(onNameChange).toHaveBeenCalledWith('GitHub');
    expect(onDescriptionChange).toHaveBeenCalledWith('Desc');
    expect(onTransportChange).toHaveBeenCalledWith('sse');
  });
});
