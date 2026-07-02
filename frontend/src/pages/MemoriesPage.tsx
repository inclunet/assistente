import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { PlusOutlined, DeleteOutlined, InboxOutlined, RollbackOutlined, FilterOutlined } from '@ant-design/icons';
import { useTranslation } from 'react-i18next';
import {
  ArchiveMemoryRecord,
  CreateMemoryRecord,
  DeleteMemoryRecord,
  GetMemoryRecord,
  ListMemoryRecords,
  UnarchiveMemoryRecord,
  UpdateMemoryRecord,
} from '@wailsjs/go/app/App';
import { database, memory } from '../../wailsjs/go/models';
import { DataGrid, DataGridColumn } from '../components/ui/DataGrid';
import { Toolbar } from '../components/ui/Toolbar';
import { Combobox, type ComboboxItem } from '../components/pickers/Combobox';
import { Button } from '../components/ui/Button';
import { Modal } from '../components/ui/Modal';
import { useConfirm } from '../hooks/useConfirm';
import { useAnnouncer } from '../hooks/useAnnouncer';
import { useGridFocus } from '../hooks/useGridFocus';
import { useGridPageLandmarks } from '../hooks/useGridPageLandmarks';
import { useResourceEditRequest } from '../hooks/useResourceEditRequest';
import { useUIStore } from '../store/uiStore';
import { useWorkspaceStore, type WorkspaceData } from '../store/workspaceStore';
import './MemoriesPage.css';

type MemoryRecord = database.MemoryRecord;

const LOAD_POLICIES = ['core', 'pinned', 'auto', 'retrievable', 'archived'] as const;
const KINDS = ['user_preference', 'identity', 'project_fact', 'decision', 'convention', 'historical_note', 'resolved_issue'] as const;
const SCOPES = ['global', 'user', 'workspace', 'project', 'conversation'] as const;
const PAGE_SIZE = 250;

interface FormState {
  content: string;
  summary: string;
  loadPolicy: string;
  kind: string;
  scope: string;
  scopeRef: string;
  tags: string;
  importance: number;
  confidence: number;
}

const emptyForm: FormState = {
  content: '',
  summary: '',
  loadPolicy: 'retrievable',
  kind: 'historical_note',
  scope: 'user',
  scopeRef: '',
  tags: '',
  importance: 3,
  confidence: 80,
};

export default function MemoriesPage() {
  const { t } = useTranslation();
  const confirm = useConfirm();
  const { announce } = useAnnouncer();
  const addToast = useUIStore((s) => s.addToast);
  const workspace = useWorkspaceStore((s) => s.workspace);
  const { handleGridReady } = useGridFocus();
  useGridPageLandmarks({ pageClass: 'memories-page' });

  const [records, setRecords] = useState<MemoryRecord[]>([]);
  const [totalRecords, setTotalRecords] = useState(0);
  const [searchTerm, setSearchTerm] = useState('');
  const [debouncedSearchTerm, setDebouncedSearchTerm] = useState('');
  const [policyFilter, setPolicyFilter] = useState('');
  const [includeArchived, setIncludeArchived] = useState(false);
  const [loading, setLoading] = useState(false);
  const [loadingMore, setLoadingMore] = useState(false);
  const [saving, setSaving] = useState(false);
  const [editing, setEditing] = useState<MemoryRecord | null>(null);
  const [modalOpen, setModalOpen] = useState(false);
  const [form, setForm] = useState<FormState>(emptyForm);
  const loadRequestRef = useRef(0);
  const recordsRef = useRef<MemoryRecord[]>([]);
  const totalRecordsRef = useRef(0);
  const hasLoadedRecordsRef = useRef(false);
  const loadingRecordsRef = useRef(false);
  const savingRef = useRef(false);

  const policyOptions = useMemo(
    () => includeArchived ? LOAD_POLICIES : LOAD_POLICIES.filter((policy) => policy !== 'archived'),
    [includeArchived],
  );
  const policyFilterItems = useMemo<ComboboxItem[]>(
    () => [
      { value: '', label: t('memories.filters.allPolicies') },
      ...policyOptions.map((policy) => ({
        value: policy,
        label: t(`memories.loadPolicies.${policy}`),
      })),
    ],
    [policyOptions, t],
  );
  const includeArchivedDisabled = policyFilter !== '' && policyFilter !== 'archived';

  const loadRecords = useCallback(async (options?: { reset?: boolean; announceProgress?: boolean }) => {
    const reset = options?.reset ?? false;
    if (loadingRecordsRef.current && !reset) return;
    const offset = reset ? 0 : recordsRef.current.length;
    if (!reset && hasLoadedRecordsRef.current && offset >= totalRecordsRef.current) {
      return;
    }
    const requestId = loadRequestRef.current + 1;
    loadRequestRef.current = requestId;
    loadingRecordsRef.current = true;
    if (reset) {
      setLoading(true);
    } else {
      setLoadingMore(true);
    }
    if (!reset && options?.announceProgress) {
      announce(t('memories.announcements.loadingMore'));
    }
    try {
      const filter = memory.Filter.createFrom({
        query: debouncedSearchTerm.trim(),
        loadPolicies: policyFilter ? [policyFilter] : undefined,
        includeArchived,
        limit: PAGE_SIZE,
        offset,
      });
      const result = await ListMemoryRecords(filter);
      if (loadRequestRef.current !== requestId) {
        return;
      }
      const total = result.total || 0;
      const nextRecords = result.records || [];
      setTotalRecords(total);
      totalRecordsRef.current = total;
      hasLoadedRecordsRef.current = true;
      setRecords((previous) => {
        const next = reset ? nextRecords : mergeMemoryRecords(previous, nextRecords);
        recordsRef.current = next;
        return next;
      });
      if (!reset && options?.announceProgress && nextRecords.length > 0) {
        announce(t('memories.announcements.loadedMore', { count: nextRecords.length }));
      }
    } catch {
      if (loadRequestRef.current === requestId) {
        if (reset) {
          setRecords([]);
          recordsRef.current = [];
          setTotalRecords(0);
          totalRecordsRef.current = 0;
        }
        addToast(t('memories.errors.loadFailed'), 'error');
        if (!reset && options?.announceProgress) {
          announce(t('memories.announcements.loadMoreFailed'), 'assertive');
        }
      }
    } finally {
      if (loadRequestRef.current === requestId) {
        loadingRecordsRef.current = false;
        setLoading(false);
        setLoadingMore(false);
      }
    }
  }, [addToast, announce, debouncedSearchTerm, includeArchived, policyFilter, t]);

  useEffect(() => {
    const timeout = window.setTimeout(() => setDebouncedSearchTerm(searchTerm), 250);
    return () => window.clearTimeout(timeout);
  }, [searchTerm]);

  useEffect(() => {
    void loadRecords({ reset: true });
  }, [loadRecords]);

  const handleSearchChange = useCallback((value: string) => {
    setSearchTerm(value);
  }, []);

  const handlePolicyFilterChange = useCallback((value: string) => {
    setPolicyFilter(value);
    if (value === 'archived') {
      setIncludeArchived(true);
    } else if (value !== '') {
      if (includeArchived) {
        announce(t('memories.filters.includeArchivedAutoDisabled'));
      }
      setIncludeArchived(false);
    }
  }, [announce, includeArchived, t]);

  const handleIncludeArchivedChange = useCallback((checked: boolean) => {
    setIncludeArchived(checked);
    if (!checked && policyFilter === 'archived') {
      setPolicyFilter('');
    }
  }, [policyFilter]);

  const openCreate = useCallback(() => {
    setEditing(null);
    setForm({ ...emptyForm, scopeRef: scopeRefForScope(emptyForm.scope, workspace) });
    setModalOpen(true);
  }, [workspace]);

  const openEdit = useCallback((record: MemoryRecord) => {
    setEditing(record);
    setForm({
      content: record.content || '',
      summary: record.summary || '',
      loadPolicy: record.loadPolicy || 'retrievable',
      kind: record.kind || 'historical_note',
      scope: record.scope || 'user',
      scopeRef: record.scopeRef || '',
      tags: parseTags(record.tags).join(', '),
      importance: record.importance || 3,
      confidence: record.confidence || 80,
    });
    setModalOpen(true);
  }, []);

  const openEditById = useCallback(async (id: string) => {
    const found = records.find((record) => record.id === id);
    if (found) {
      openEdit(found);
      return;
    }
    try {
      const record = await GetMemoryRecord(id);
      openEdit(record);
    } catch {
      addToast(t('memories.errors.loadFailed'), 'error');
    }
  }, [addToast, openEdit, records, t]);

  useResourceEditRequest('memories', {
    onEdit: (id) => { void openEditById(id); },
    onNew: () => openCreate(),
  });

  const closeModal = useCallback(() => {
    setModalOpen(false);
    setEditing(null);
    setForm(emptyForm);
  }, []);

  const saveRecord = useCallback(async () => {
    if (savingRef.current) return;
    if (scopeRequiresRef(form.scope) && form.scopeRef.trim() === '') {
      addToast(t('memories.errors.scopeRefRequired', { scope: t(`memories.scopes.${form.scope}`, form.scope) }), 'error');
      return;
    }
    savingRef.current = true;
    setSaving(true);
    const input = memory.RecordInput.createFrom({
      content: form.content.trim(),
      summary: form.summary.trim(),
      loadPolicy: form.loadPolicy,
      kind: form.kind,
      scope: form.scope,
      scopeRef: form.scopeRef.trim(),
      tags: form.tags.split(',').map((tag) => tag.trim()).filter(Boolean),
      importance: Number(form.importance),
      confidence: Number(form.confidence),
      sourceType: editing?.sourceType || '',
      sourceId: editing?.sourceId || '',
      expiresAt: editing?.expiresAt,
    });
    try {
      if (editing) {
        await UpdateMemoryRecord(editing.id, input);
        announce(t('memories.announcements.updated'));
      } else {
        await CreateMemoryRecord(input);
        announce(t('memories.announcements.created'));
      }
      closeModal();
      if (form.loadPolicy === 'archived') {
        let changedFilter = false;
        if (!includeArchived) {
          setIncludeArchived(true);
          changedFilter = true;
        }
        if (policyFilter && policyFilter !== 'archived') {
          setPolicyFilter('archived');
          changedFilter = true;
        }
        if (!changedFilter) {
          await loadRecords({ reset: true });
        }
      } else {
        await loadRecords({ reset: true });
      }
    } catch {
      addToast(t('memories.errors.saveFailed'), 'error');
    } finally {
      savingRef.current = false;
      setSaving(false);
    }
  }, [addToast, announce, closeModal, editing, form, includeArchived, loadRecords, policyFilter, t]);

  const archiveRecord = useCallback(async (record: MemoryRecord) => {
    try {
      if (record.loadPolicy === 'archived') {
        await UnarchiveMemoryRecord(record.id, record.archivedFromPolicy || '');
        announce(t('memories.announcements.unarchived'));
      } else {
        await ArchiveMemoryRecord(record.id);
        announce(t('memories.announcements.archived'));
      }
      await loadRecords({ reset: true });
    } catch {
      addToast(t('memories.errors.archiveFailed'), 'error');
    }
  }, [addToast, announce, loadRecords, t]);

  const deleteRecord = useCallback(async (record: MemoryRecord) => {
    const ok = await confirm({
      title: t('memories.deleteConfirm.title'),
      message: t('memories.deleteConfirm.message'),
      confirmText: t('memories.deleteConfirm.confirm'),
      variant: 'danger',
    });
    if (!ok) return;
    try {
      await DeleteMemoryRecord(record.id);
      announce(t('memories.announcements.deleted'));
      await loadRecords({ reset: true });
    } catch {
      addToast(t('memories.errors.deleteFailed'), 'error');
    }
  }, [addToast, announce, confirm, loadRecords, t]);

  const columns = useMemo<DataGridColumn<MemoryRecord>[]>(() => [
    {
      key: 'content',
      label: t('memories.columns.content'),
      width: '2fr',
      truncate: true,
      format: (_value, record) => record.summary || record.content,
    },
    {
      key: 'loadPolicy',
      label: t('memories.columns.loadPolicy'),
      width: '140px',
      format: (_value, record) => t(`memories.loadPolicies.${record.loadPolicy}`, record.loadPolicy),
    },
    {
      key: 'kind',
      label: t('memories.columns.kind'),
      width: '160px',
      format: (_value, record) => t(`memories.kinds.${record.kind}`, record.kind),
    },
    {
      key: 'scope',
      label: t('memories.columns.scope'),
      width: '120px',
      format: (_value, record) => t(`memories.scopes.${record.scope}`, record.scope),
    },
    { key: 'importance', label: t('memories.columns.importance'), width: '110px' },
  ], [t]);
  const hasMoreRecords = records.length < totalRecords;
  const handleNearEnd = useCallback(() => {
    void loadRecords({ announceProgress: true });
  }, [loadRecords]);

  return (
    <div className="memories-page">
      <Toolbar
        ariaLabel={t('memories.toolbarLabel')}
        left={<h1 className="page-toolbar__title">{t('memories.title')}</h1>}
        searchValue={searchTerm}
        onSearchChange={handleSearchChange}
        searchPlaceholder={t('memories.searchPlaceholder')}
        isLoading={loading || loadingMore}
        right={(
          <div className="memories-page__filters">
            <Combobox
              items={policyFilterItems}
              selected={policyFilter}
              onSelect={(value) => handlePolicyFilterChange(value)}
              label={t('memories.filters.policy')}
              icon={<FilterOutlined aria-hidden="true" />}
              maxWidth="180px"
            />
            <button
              type="button"
              className={`memories-page__filter-toggle${includeArchived ? ' memories-page__filter-toggle--active' : ''}`}
              onClick={() => handleIncludeArchivedChange(!includeArchived)}
              disabled={includeArchivedDisabled}
              aria-pressed={includeArchived}
            >
              {t('memories.filters.includeArchived')}
            </button>
          </div>
        )}
        actions={[{
          key: 'new',
          label: t('memories.actions.new'),
          icon: <PlusOutlined />,
          variant: 'primary',
          onClick: openCreate,
        }]}
      />

      <DataGrid
        items={records}
        columns={columns}
        label={t('memories.gridLabel')}
        getItemId={(record) => record.id}
        onActivate={openEdit}
        onGridReady={handleGridReady}
        onNearEnd={handleNearEnd}
        getRowActions={(record) => [
          {
            id: 'archive',
            label: record.loadPolicy === 'archived' ? t('memories.actions.unarchive') : t('memories.actions.archive'),
            icon: record.loadPolicy === 'archived' ? <RollbackOutlined /> : <InboxOutlined />,
            action: () => { void archiveRecord(record); },
          },
          {
            id: 'delete',
            label: t('memories.actions.delete'),
            icon: <DeleteOutlined />,
            danger: true,
            action: () => { void deleteRecord(record); },
          },
        ]}
      />

      {records.length === 0 && !loading && (
        <p className="memories-page__empty">{t('memories.empty')}</p>
      )}

      {loadingMore && records.length > 0 && hasMoreRecords && (
        <p className="memories-page__load-status">
          {t('memories.loadingMore')}
        </p>
      )}

      <Modal
        isOpen={modalOpen}
        onClose={closeModal}
        title={editing ? t('memories.editTitle') : t('memories.createTitle')}
        size="lg"
      >
        <form className="memories-page__form" onSubmit={(event) => { event.preventDefault(); void saveRecord(); }}>
          <label>
            <span>{t('memories.fields.content')}</span>
            <textarea value={form.content} onChange={(event) => setForm({ ...form, content: event.target.value })} required rows={5} />
          </label>
          <label>
            <span>{t('memories.fields.summary')}</span>
            <input value={form.summary} onChange={(event) => setForm({ ...form, summary: event.target.value })} />
          </label>
          <div className="memories-page__form-grid">
            <label>
              <span>{t('memories.fields.loadPolicy')}</span>
              <select value={form.loadPolicy} onChange={(event) => setForm({ ...form, loadPolicy: event.target.value })}>
                {LOAD_POLICIES.map((policy) => (
                  <option key={policy} value={policy}>{t(`memories.loadPolicies.${policy}`)}</option>
                ))}
              </select>
            </label>
            <label>
              <span>{t('memories.fields.kind')}</span>
              <select value={form.kind} onChange={(event) => setForm({ ...form, kind: event.target.value })}>
                {KINDS.map((kind) => (
                  <option key={kind} value={kind}>{t(`memories.kinds.${kind}`)}</option>
                ))}
              </select>
            </label>
            <label>
              <span>{t('memories.fields.scope')}</span>
              <select
                value={form.scope}
                onChange={(event) => {
                  const scope = event.target.value;
                  const derivedScopeRef = scopeRefForScope(scope, workspace);
                  setForm({
                    ...form,
                    scope,
                    scopeRef: derivedScopeRef || scopeRefFallbackForScope(scope, form.scopeRef),
                  });
                }}
              >
                {SCOPES.map((scope) => (
                  <option key={scope} value={scope}>{t(`memories.scopes.${scope}`)}</option>
                ))}
              </select>
            </label>
            <label>
              <span>{t('memories.fields.scopeRef')}</span>
              <input value={form.scopeRef} onChange={(event) => setForm({ ...form, scopeRef: event.target.value })} />
            </label>
            <label>
              <span>{t('memories.fields.importance')}</span>
              <input type="number" min={1} max={5} value={form.importance} onChange={(event) => setForm({ ...form, importance: Number(event.target.value) })} />
            </label>
            <label>
              <span>{t('memories.fields.confidence')}</span>
              <input type="number" min={0} max={100} value={form.confidence} onChange={(event) => setForm({ ...form, confidence: Number(event.target.value) })} />
            </label>
          </div>
          <label>
            <span>{t('memories.fields.tags')}</span>
            <input value={form.tags} onChange={(event) => setForm({ ...form, tags: event.target.value })} placeholder={t('memories.fields.tagsPlaceholder')} />
          </label>
          <p className="memories-page__policy-help">{t(`memories.policyHelp.${form.loadPolicy}`)}</p>
          <div className="memories-page__modal-actions">
            <Button type="button" variant="secondary" onClick={closeModal} disabled={saving}>{t('common.cancel')}</Button>
            <Button type="submit" variant="primary" disabled={saving}>{t('common.save')}</Button>
          </div>
        </form>
      </Modal>
    </div>
  );
}

function parseTags(raw?: string): string[] {
  if (!raw) return [];
  try {
    const parsed = JSON.parse(raw);
    return Array.isArray(parsed) ? parsed.map(String) : [];
  } catch {
    return [];
  }
}

function scopeRefForScope(scope: string, workspace: WorkspaceData | null): string {
  if (!workspace) return '';
  if (scope === 'workspace') return workspace.id;
  if (scope === 'conversation') {
    const activeTab = workspace.tabs.find((tab) => tab.id === workspace.activeTabId);
    return activeTab?.conversationId || '';
  }
  return '';
}

function scopeRequiresRef(scope: string): boolean {
  return scope === 'workspace' || scope === 'conversation' || scope === 'project' || scope === 'surface';
}

function scopeRefFallbackForScope(scope: string, currentScopeRef: string): string {
  return scope === 'project' ? currentScopeRef : '';
}

function mergeMemoryRecords(previous: MemoryRecord[], nextPage: MemoryRecord[]): MemoryRecord[] {
  if (nextPage.length === 0) return previous;
  const seen = new Set(previous.map((record) => record.id));
  const merged = [...previous];
  for (const record of nextPage) {
    if (!seen.has(record.id)) {
      seen.add(record.id);
      merged.push(record);
    }
  }
  return merged;
}
