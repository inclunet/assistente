import { afterEach, describe, expect, it, vi } from 'vitest';
import { render, screen } from '@testing-library/react';
import { FormField } from './FormField';
import { Input } from './Input';

const announceMock = vi.hoisted(() => vi.fn());

vi.mock('../../hooks/useAnnouncer', () => ({
  useAnnouncer: () => ({
    announce: announceMock,
  }),
}));

describe('FormField', () => {
  afterEach(() => {
    announceMock.mockClear();
  });

  it('associa label ao campo e mostra descricao', () => {
    render(
      <FormField label="Nome" description="Ajuda" required>
        <input />
      </FormField>
    );

    const input = screen.getByLabelText(/Nome/);
    expect(input).toBeInTheDocument();
    expect(screen.getByText('Ajuda')).toBeInTheDocument();
  });

  it('prioriza erro sobre descricao', () => {
    render(
      <FormField label="Nome" description="Ajuda" error="Obrigatorio">
        <input />
      </FormField>
    );

    expect(screen.getByText('Obrigatorio')).toBeInTheDocument();
    expect(screen.queryByText('Ajuda')).toBeNull();
  });

  it('anuncia erro quando o filho não anuncia por conta própria', () => {
    render(
      <FormField label="Nome" description="Ajuda" error="Obrigatorio">
        <input />
      </FormField>
    );

    expect(announceMock).toHaveBeenCalledTimes(1);
    expect(announceMock).toHaveBeenCalledWith('Obrigatorio', 'assertive');
  });

  it('não duplica anúncio quando o filho recebe o mesmo erro', () => {
    render(
      <FormField label="Nome" description="Ajuda" error="Obrigatorio">
        <Input error="Obrigatorio" />
      </FormField>
    );

    expect(announceMock).toHaveBeenCalledTimes(1);
    expect(announceMock).toHaveBeenCalledWith('Obrigatorio', 'assertive');
  });
});
