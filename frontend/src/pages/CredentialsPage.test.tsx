import { describe, expect, it, vi, beforeEach } from 'vitest';
import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';

const mockList = vi.fn();
const mockUpsert = vi.fn();
const mockDelete = vi.fn();

vi.mock('@wailsjs/go/main/App', () => ({
  ListCredentials: () => mockList(),
  UpsertCredential: (payload: any) => mockUpsert(payload),
  DeleteCredential: (pattern: string) => mockDelete(pattern),
}));

vi.mock('../hooks/useGridFocus', () => ({
  useGridFocus: () => ({
    focusFirstCell: vi.fn(),
    handleGridReady: vi.fn(),
  }),
}));

vi.mock('../hooks/useAnnouncer', () => ({
  useAnnouncer: () => ({
    announce: vi.fn(),
  }),
}));

vi.mock('../store/uiStore', () => ({
  useUIStore: () => ({
    addToast: vi.fn(),
  }),
}));

vi.mock('../components/ui/Toolbar', () => ({
  Toolbar: ({ left, right }: any) => (
    <div>
      {left}
      {right}
    </div>
  ),
}));

vi.mock('../components/ui/DataGrid', () => ({
  DataGrid: ({ items, onActivate }: any) => (
    <div>
      {items?.map((row: any) => (
        <button key={row.id} onClick={() => onActivate?.(row)}>{row.pattern}</button>
      ))}
    </div>
  ),
}));

vi.mock('../components/ui/Modal', () => ({
  Modal: ({ isOpen, children }: any) => (isOpen ? <div>{children}</div> : null),
}));

vi.mock('../components/ui/EditorPanel', () => ({
  EditorPanelFooter: ({ children }: any) => <div>{children}</div>,
}));

vi.mock('../components', () => ({
  Button: ({ children, onClick }: any) => <button onClick={onClick}>{children}</button>,
  Input: ({ label, value, onChange, type }: any) => (
    <label>
      {label}
      <input aria-label={label} value={value} onChange={onChange} type={type} />
    </label>
  ),
  Select: ({ label, value, options, onChange }: any) => (
    <label>
      {label}
      <select aria-label={label} value={value} onChange={onChange}>
        {options.map((opt: any) => (
          <option key={opt.value} value={opt.value}>{opt.label}</option>
        ))}
      </select>
    </label>
  ),
}));

import CredentialsPage from './CredentialsPage';

describe('CredentialsPage', () => {
  beforeEach(() => {
    mockList.mockResolvedValue([
      { pattern: '*.github.com', type: 'bearer', masked: '••••1234' },
    ]);
    mockUpsert.mockResolvedValue(undefined);
    mockDelete.mockResolvedValue(undefined);
  });

  it('carrega credenciais e abre editor', async () => {
    render(<CredentialsPage />);

    await waitFor(() => {
      expect(screen.getByText('*.github.com')).toBeInTheDocument();
    });

    await userEvent.click(screen.getByText('*.github.com'));

    expect(screen.getByLabelText('Pattern')).toBeInTheDocument();
    expect(screen.getByLabelText('Tipo')).toBeInTheDocument();
  });

  it('cria nova credencial', async () => {
    render(<CredentialsPage />);

    await userEvent.click(screen.getByText('Nova'));

    await userEvent.type(screen.getByLabelText('Pattern'), 'api.example.com');
    await userEvent.selectOptions(screen.getByLabelText('Tipo'), 'bearer');
    await userEvent.type(screen.getByLabelText('Token'), 'tok_123');

    await userEvent.click(screen.getByText('Criar'));

    expect(mockUpsert).toHaveBeenCalledWith(expect.objectContaining({
      pattern: 'api.example.com',
      type: 'bearer',
      token: 'tok_123',
    }));
  });
});
