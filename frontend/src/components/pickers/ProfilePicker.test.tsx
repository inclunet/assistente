import { describe, expect, it, vi } from 'vitest';
import { render, screen, fireEvent, waitFor } from '@testing-library/react';
import { ProfilePicker } from './ProfilePicker';

const getProfilesSpy = vi.fn();
const getActiveSpy = vi.fn();
const setActiveSpy = vi.fn();

vi.mock('@wailsjs/go/main/App', () => ({
  GetProfiles: () => getProfilesSpy(),
  GetActiveProfileSlug: () => getActiveSpy(),
  SetActiveProfile: (slug: string) => setActiveSpy(slug),
}));

vi.mock('@wailsjs/runtime/runtime', () => ({
  EventsOn: () => () => {},
}));

vi.mock('./BasePicker', () => ({
  BasePicker: (props: { items: Array<{ value: string }>; onSelect: (value: string) => void }) => (
    <div>
      {props.items.map((item, i) => (
        <button key={item.value} onClick={() => props.onSelect(item.value)}>
          {i === 0 ? 'Selecionar' : `Selecionar-${item.value}`}
        </button>
      ))}
      <div data-testid="base-picker" data-items={props.items.length} />
    </div>
  ),
}));

describe('ProfilePicker', () => {
  it('dispara onChange no modo controlado', async () => {
    getProfilesSpy.mockResolvedValueOnce([
      { name: 'Padrao', slug: 'padrao', description: '', icon: '', source: 'local' },
    ]);
    getActiveSpy.mockResolvedValueOnce('padrao');

    const onChange = vi.fn();

    render(<ProfilePicker value="padrao" onChange={onChange} />);

    await waitFor(() => {
      // 2 items: "usar perfil ativo global" + "padrao"
      expect(screen.getByTestId('base-picker')).toHaveAttribute('data-items', '2');
    });

    // Seleciona o perfil "padrao" (segundo item)
    fireEvent.click(screen.getByRole('button', { name: 'Selecionar-padrao' }));
    expect(onChange).toHaveBeenCalledWith('padrao');
    expect(setActiveSpy).not.toHaveBeenCalled();
  });

  it('dispara onChange com valor vazio ao selecionar perfil global', async () => {
    getProfilesSpy.mockResolvedValueOnce([
      { name: 'Padrao', slug: 'padrao', description: '', icon: '', source: 'local' },
    ]);
    getActiveSpy.mockResolvedValueOnce('padrao');

    const onChange = vi.fn();

    render(<ProfilePicker value="padrao" onChange={onChange} />);

    await waitFor(() => {
      expect(screen.getByTestId('base-picker')).toHaveAttribute('data-items', '2');
    });

    // Seleciona "usar perfil ativo global" (primeiro item, valor vazio)
    fireEvent.click(screen.getByRole('button', { name: 'Selecionar' }));
    expect(onChange).toHaveBeenCalledWith('');
    expect(setActiveSpy).not.toHaveBeenCalled();
  });
});
