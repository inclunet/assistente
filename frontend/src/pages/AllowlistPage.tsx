import { useState, useEffect, useCallback } from 'react';
import {
  GetAllowlists,
  GetAllowlist,
  CreateAllowlist,
  UpdateAllowlist,
  DeleteAllowlist,
} from '@wailsjs/go/main/App';

import { allowlist } from '../../wailsjs/go/models';
import { DataGrid, DataGridColumn } from '../components/ui/DataGrid';
import { Toolbar } from '../components/ui/Toolbar';
import { Button } from '../components';
import { ConfirmDialog } from '../components/ui/ConfirmDialog';
import { SimpleModal } from '../components/ui/SimpleModal';
import { useUIStore } from '../store/uiStore';
import { useGridFocus } from '../hooks/useGridFocus';
import './AllowlistPage.css';

type AllowlistInfo = allowlist.AllowlistInfo;
type Allowlist = allowlist.Allowlist;

interface AllowlistRow {
  id: string;
  slug: string;
  name: string;
  description: string;
  ruleCount: number;
}

export default function AllowlistPage() {
  const { addToast } = useUIStore();
  const { focusFirstCell, handleGridReady } = useGridFocus();

  const [rows, setRows] = useState<AllowlistRow[]>([]);
  const [_loading, setLoading] = useState(true);
  const [editing, setEditing] = useState<Allowlist | null>(null);
  const [editingSlug, setEditingSlug] = useState<string | null>(null);
  const [isNew, setIsNew] = useState(false);
  const [saving, setSaving] = useState(false);
  const [deleteOpen, setDeleteOpen] = useState(false);
  const [deleteTarget, setDeleteTarget] = useState<AllowlistRow | null>(null);

  const loadAllowlists = useCallback(async () => {
    setLoading(true);
    try {
      const list = await GetAllowlists();
      setRows((list || []).map((a: AllowlistInfo) => ({
        id: a.slug,
        slug: a.slug,
        name: a.name,
        description: a.description || '',
        ruleCount: a.ruleCount,
      })));
    } catch (error) {
      console.error('Erro ao carregar allowlists:', error);
      addToast('Erro ao carregar allowlists', 'error');
    } finally {
      setLoading(false);
    }
  }, [addToast]);

  useEffect(() => {
    loadAllowlists();
  }, [loadAllowlists]);

  const handleEdit = async (row: AllowlistRow) => {
    try {
      const al = await GetAllowlist(row.slug);
      setEditing(al);
      setEditingSlug(row.slug);
      setIsNew(false);
    } catch (error) {
      console.error('Erro ao carregar allowlist:', error);
      addToast('Erro ao carregar allowlist', 'error');
    }
  };

  const handleNew = () => {
    setEditing(allowlist.Allowlist.createFrom({
      name: '',
      description: '',
      auto_approve: [],
      always_deny: [],
      default_action: 'confirm',
    }));
    setEditingSlug(null);
    setIsNew(true);
  };

  const handleSave = async () => {
    if (!editing) return;
    if (!editing.name.trim()) {
      addToast('Nome é obrigatório', 'error');
      return;
    }

    setSaving(true);
    try {
      if (isNew) {
        await CreateAllowlist(editing);
        addToast('Allowlist criada!', 'success');
      } else if (editingSlug) {
        await UpdateAllowlist(editingSlug, editing);
        addToast('Allowlist atualizada!', 'success');
      }
      setEditing(null);
      setEditingSlug(null);
      setIsNew(false);
      await loadAllowlists();
    } catch (error: any) {
      addToast(error?.message || 'Erro ao salvar', 'error');
    } finally {
      setSaving(false);
    }
  };

  const handleDelete = async () => {
    if (!deleteTarget) return;
    try {
      await DeleteAllowlist(deleteTarget.slug);
      addToast('Allowlist excluída!', 'success');
      setDeleteOpen(false);
      setDeleteTarget(null);
      if (editingSlug === deleteTarget.slug) {
        setEditing(null);
        setEditingSlug(null);
      }
      await loadAllowlists();
    } catch (error: any) {
      addToast(error?.message || 'Erro ao excluir', 'error');
    }
  };

  const updateField = (field: keyof Allowlist, value: any) => {
    if (!editing) return;
    setEditing({ ...editing, [field]: value } as Allowlist);
  };

  const updateRules = (field: 'auto_approve' | 'always_deny', text: string) => {
    if (!editing) return;
    const rules = text.split('\n').map(s => s.trim()).filter(Boolean);
    setEditing({ ...editing, [field]: rules } as Allowlist);
  };

  const columns: DataGridColumn<AllowlistRow>[] = [
    { key: 'name', label: 'Nome', format: (_val, row) => row.name },
    { key: 'description', label: 'Descrição' },
    { key: 'ruleCount', label: 'Regras', format: (val) => `${val}` },
  ];

  return (
    <div className="allowlist-page">
      <Toolbar
        left={<h1 className="page-toolbar__title">Allowlists de Comandos</h1>}
        ariaLabel="Barra de ferramentas de allowlists"
        onFocusGrid={focusFirstCell}
        actions={[
          {
            key: 'new',
            label: 'Nova Allowlist',
            icon: '+',
            onClick: handleNew,
          },
        ]}
      />

      <div className="allowlist-page__content">
        <DataGrid
          columns={columns}
          items={rows}
          getItemId={(row) => row.id}
          onActivate={(row) => handleEdit(row)}
          onDelete={(row) => {
            setDeleteTarget(row);
            setDeleteOpen(true);
          }}
          label="Allowlists de Comandos"
          onGridReady={handleGridReady}
        />
      </div>

      <SimpleModal
        isOpen={!!editing}
        onClose={() => { setEditing(null); setEditingSlug(null); setIsNew(false); }}
        title={isNew ? 'Nova Allowlist' : `Editando: ${editing?.name || ''}`}
        size="lg"
      >
        {editing && (
          <>
            <div className="allowlist-page__fields">
              <div className="allowlist-page__field">
                <label className="allowlist-page__label">Nome</label>
                <input
                  type="text"
                  className="allowlist-page__input"
                  value={editing.name}
                  onChange={(e) => updateField('name', e.target.value)}
                />
              </div>

              <div className="allowlist-page__field">
                <label className="allowlist-page__label">Descrição</label>
                <input
                  type="text"
                  className="allowlist-page__input"
                  value={editing.description}
                  onChange={(e) => updateField('description', e.target.value)}
                />
              </div>

              <div className="allowlist-page__field">
                <label className="allowlist-page__label">Ação Padrão</label>
                <select
                  className="allowlist-page__input"
                  value={editing.default_action}
                  onChange={(e) => updateField('default_action', e.target.value)}
                >
                  <option value="confirm">Confirmar (pede aprovação ao usuário)</option>
                  <option value="deny">Negar (bloqueia comandos desconhecidos)</option>
                </select>
              </div>

              <div className="allowlist-page__field">
                <label className="allowlist-page__label">
                  Auto Approve (um pattern por linha)
                </label>
                <textarea
                  className="allowlist-page__textarea"
                  rows={10}
                  value={(editing.auto_approve || []).join('\n')}
                  onChange={(e) => updateRules('auto_approve', e.target.value)}
                  placeholder="ls&#10;git status&#10;git diff *&#10;go test *"
                />
                <span className="allowlist-page__hint">
                  Comandos aprovados sem confirmação. Use * no final para prefix match.
                </span>
              </div>

              <div className="allowlist-page__field">
                <label className="allowlist-page__label">
                  Always Deny (um pattern por linha)
                </label>
                <textarea
                  className="allowlist-page__textarea"
                  rows={6}
                  value={(editing.always_deny || []).join('\n')}
                  onChange={(e) => updateRules('always_deny', e.target.value)}
                  placeholder="rm -rf /&#10;shutdown&#10;reboot"
                />
                <span className="allowlist-page__hint">
                  Comandos sempre bloqueados, mesmo que estejam em Auto Approve.
                </span>
              </div>
            </div>

            <div className="allowlist-page__editor-footer">
              {!isNew && editingSlug && (
                <Button
                  variant="danger"
                  onClick={() => {
                    const row = rows.find(r => r.slug === editingSlug);
                    if (row) { setDeleteTarget(row); setDeleteOpen(true); }
                  }}
                >
                  Excluir
                </Button>
              )}
              <Button variant="ghost" onClick={() => { setEditing(null); setEditingSlug(null); setIsNew(false); }}>
                Cancelar
              </Button>
              <Button onClick={handleSave} loading={saving}>Salvar</Button>
            </div>
          </>
        )}
      </SimpleModal>

      <ConfirmDialog
        isOpen={deleteOpen}
        title="Excluir Allowlist"
        message={`Tem certeza que deseja excluir a allowlist "${deleteTarget?.name}"?`}
        confirmText="Excluir"
        cancelText="Cancelar"
        variant="danger"
        onConfirm={handleDelete}
        onCancel={() => { setDeleteOpen(false); setDeleteTarget(null); }}
      />
    </div>
  );
}
