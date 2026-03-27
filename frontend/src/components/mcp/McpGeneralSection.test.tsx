import { describe, expect, it, vi } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/react';
import { McpGeneralSection } from './McpGeneralSection';

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
    fireEvent.change(screen.getByLabelText(/Tipo/), { target: { value: 'streamable' } });

    expect(onNameChange).toHaveBeenCalledWith('GitHub');
    expect(onDescriptionChange).toHaveBeenCalledWith('Desc');
    expect(onTransportChange).toHaveBeenCalledWith('streamable');
  });
});
