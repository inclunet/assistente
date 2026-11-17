import { beforeEach, describe, expect, it, vi } from 'vitest';
import { act, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { LegacyPermissionAssignment } from './LegacyPermissionAssignment';

const api = vi.hoisted(() => ({ load: vi.fn(), assign: vi.fn(), toast: vi.fn() }));
vi.mock('@wailsjs/go/wailsapi/Permissions', () => ({
  GetPermissionMigration: api.load, AssignPermissionOrigin: api.assign,
}));
vi.mock('../../store/uiStore', () => ({
  useUIStore: (selector: (s: { addToast: typeof api.toast }) => unknown) => selector({ addToast: api.toast }),
}));
const translate = (key: string) => key;
vi.mock('react-i18next', () => ({ useTranslation: () => ({ t: translate }) }));

describe('atribuição exclusiva de origens legadas', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    api.load.mockResolvedValue({
      origins: [{ id: 'origin-id', domain: 'network', source: '/legacy/network.json' }],
      owners: [{ id: 'owner-id', name: 'Maria' }],
    });
    api.assign.mockResolvedValue(undefined);
  });

  async function choose() {
    await screen.findByRole('option', { name: 'Maria' });
    fireEvent.change(screen.getByLabelText('permissions.originLabel'), { target: { value: 'origin-id' } });
    fireEvent.change(screen.getByLabelText('permissions.owner'), { target: { value: 'owner-id' } });
  }

  it('exige os dois seletores, envia IDs e só conclui depois do sucesso', async () => {
    let complete!: () => void;
    api.assign.mockImplementationOnce(() => new Promise<void>((resolve) => { complete = resolve; }));
    const saved = vi.fn();
    render(<LegacyPermissionAssignment onSaved={saved} onCancel={vi.fn()} />);
    const save = screen.getByRole('button', { name: 'permissions.assign' });
    expect(save).toBeDisabled();
    await screen.findByRole('option', { name: 'Maria' });
    fireEvent.change(screen.getByLabelText('permissions.originLabel'), { target: { value: 'origin-id' } });
    expect(save).toBeDisabled();
    fireEvent.change(screen.getByLabelText('permissions.owner'), { target: { value: 'owner-id' } });
    fireEvent.click(save);
    expect(api.assign).toHaveBeenCalledExactlyOnceWith('origin-id', 'owner-id');
    expect(save).toBeDisabled();
    expect(screen.getByRole('button', { name: 'common.cancel' })).toBeDisabled();
    expect(saved).not.toHaveBeenCalled();
    await act(async () => complete());
    expect(api.toast).toHaveBeenCalledWith('permissions.assigned', 'success');
    expect(saved).toHaveBeenCalledOnce();
  });

  it('mantém a escolha após erro e permite tentar novamente sem anunciar sucesso falso', async () => {
    api.assign.mockRejectedValueOnce(new Error('db unavailable'));
    const saved = vi.fn();
    render(<LegacyPermissionAssignment onSaved={saved} onCancel={vi.fn()} />);
    await choose();
    const save = screen.getByRole('button', { name: 'permissions.assign' });
    fireEvent.click(save);
    await waitFor(() => expect(api.toast).toHaveBeenCalledWith('permissions.assignmentFailed', 'error'));
    expect(saved).not.toHaveBeenCalled();
    expect(screen.getByLabelText('permissions.originLabel')).toHaveValue('origin-id');
    expect(screen.getByLabelText('permissions.owner')).toHaveValue('owner-id');
    expect(save).toBeEnabled();
    fireEvent.click(save);
    await waitFor(() => expect(saved).toHaveBeenCalledOnce());
    expect(api.assign).toHaveBeenNthCalledWith(2, 'origin-id', 'owner-id');
  });

  it('informa falha de leitura, não permite atribuir e permite cancelar', async () => {
    api.load.mockRejectedValueOnce(new Error('forbidden'));
    const cancel = vi.fn();
    render(<LegacyPermissionAssignment onSaved={vi.fn()} onCancel={cancel} />);
    await waitFor(() => expect(api.toast).toHaveBeenCalledWith('permissions.loadFailed', 'error'));
    expect(screen.getByRole('button', { name: 'permissions.assign' })).toBeDisabled();
    fireEvent.click(screen.getByRole('button', { name: 'common.cancel' }));
    expect(cancel).toHaveBeenCalledOnce();
    expect(api.assign).not.toHaveBeenCalled();
  });
});
