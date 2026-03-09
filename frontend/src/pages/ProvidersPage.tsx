import { useState, useEffect } from 'react';
import {
  GetLLMProvidersWithStatus,
} from '@wailsjs/go/main/App';
import { DataGrid, DataGridColumn } from '../components/ui/DataGrid';
import { Toolbar } from '../components/ui/Toolbar';
import { Modal } from '../components/ui/Modal';
import { ProviderForm, ProviderFormData } from '../components/settings/ProviderForm';
import { useGridFocus } from '../hooks/useGridFocus';
import { useUIStore } from '../store/uiStore';
import './ProvidersPage.css';

interface Provider {
  id: string;
  name: string;
  type: string;
  base_url: string;
  credential_required: boolean;
  credential_status: 'none' | 'configured' | 'missing';
  credential_domain_patterns?: string[];
}

interface ProviderRow extends Provider {
  statusText: string;
}

export default function ProvidersPage() {
  const { addToast } = useUIStore();
  const { handleGridReady } = useGridFocus();

  const [providers, setProviders] = useState<ProviderRow[]>([]);
  const [loading, setLoading] = useState(true);
  const [searchTerm, setSearchTerm] = useState('');
  const [selectedIds, setSelectedIds] = useState<Set<string | number>>(new Set());
  const [isEditing, setIsEditing] = useState(false);
  const [editingProvider, setEditingProvider] = useState<ProviderFormData | undefined>(undefined);

  const loadProviders = async () => {
    setLoading(true);
    try {
      const result = await GetLLMProvidersWithStatus();
      const mapped = (result || []).map((p: any) => ({
        ...p,
        id: p.id,
        statusText: getStatusText(p.credential_status),
      })) as ProviderRow[];
      setProviders(mapped);
    } catch (error) {
      console.error('Erro ao carregar provedores:', error);
      addToast('Erro ao carregar provedores', 'error');
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    loadProviders();
  }, []);

  const getStatusText = (status: string): string => {
    switch (status) {
      case 'configured':
        return 'Configurado';
      case 'missing':
        return 'Credencial faltando';
      case 'none':
      default:
        return 'Não requer';
    }
  };

  const handleAddProvider = () => {
    setEditingProvider(undefined);
    setIsEditing(true);
  };

  const handleEditProvider = (provider: ProviderRow) => {
    setEditingProvider({
      id: provider.id,
      name: provider.name,
      type: provider.type,
      base_url: provider.base_url,
      api_key: '',
    });
    setIsEditing(true);
  };

  const handleSaveSuccess = async () => {
    setIsEditing(false);
    setEditingProvider(undefined);
    await loadProviders();
  };

  const handleCancelEdit = () => {
    setIsEditing(false);
    setEditingProvider(undefined);
  };

  const columns: DataGridColumn<ProviderRow>[] = [
    {
      key: 'name',
      label: 'Nome',
      width: '25%',
    },
    {
      key: 'type',
      label: 'Tipo',
      width: '15%',
    },
    {
      key: 'base_url',
      label: 'Base URL',
      width: '40%',
    },
    {
      key: 'statusText',
      label: 'Status Credencial',
      width: '20%',
      format: (value: string, row: ProviderRow) => (
        <span
          className={`provider-status provider-status--${row.credential_status}`}
        >
          {value}
        </span>
      ),
    },
  ];

  const filteredRows = providers.filter((row) =>
    searchTerm === ''
      ? true
      : row.name.toLowerCase().includes(searchTerm.toLowerCase()) ||
        row.type.toLowerCase().includes(searchTerm.toLowerCase()) ||
        row.base_url.toLowerCase().includes(searchTerm.toLowerCase())
  );

  return (
    <div className="providers-page">
      {loading && <div className="loading">Carregando...</div>}
      {!loading && (
        <>
          <Toolbar
            left={<h2>Provedores LLM</h2>}
            searchPlaceholder="Buscar provedores..."
            searchValue={searchTerm}
            onSearchChange={setSearchTerm}
            actions={[
              {
                key: 'add',
                label: 'Adicionar Provedor',
                onClick: handleAddProvider,
                variant: 'primary',
              },
            ]}
          />

          <DataGrid
            columns={columns}
            items={filteredRows}
            selectedIds={selectedIds}
            onSelectionChange={setSelectedIds}
            onActivate={handleEditProvider}
            onGridReady={handleGridReady}
            label="Lista de provedores LLM"
          />

          <Modal
            isOpen={isEditing}
            onClose={handleCancelEdit}
            title={editingProvider?.id ? 'Editar Provedor' : 'Novo Provedor'}
            size="md"
          >
            <div className="providers-editor">
              <ProviderForm
                provider={editingProvider}
                onSave={handleSaveSuccess}
                onCancel={handleCancelEdit}
              />
            </div>
          </Modal>
        </>
      )}
    </div>
  );
}
