import { logger } from '../utils/logger';
import { useState, useEffect, useCallback } from 'react';
import { useTranslation } from 'react-i18next';
import { DeleteOutlined, ReloadOutlined } from '@ant-design/icons';
import { GetAuthorizedContacts, RemoveAuthorizedContact } from '@wailsjs/go/app/App';
import { useUIStore } from '../store/uiStore';
import { useAnnouncer } from '../hooks/useAnnouncer';
import { useGridFocus } from '../hooks/useGridFocus';
import { useGridPageLandmarks } from '../hooks/useGridPageLandmarks';
import { Toolbar } from '../components/ui/Toolbar';
import { DataGrid, DataGridColumn } from '../components/ui/DataGrid';
import { MenuButton } from '../components/layout/MenuButton';
import { useConfirm } from '../hooks/useConfirm';
import './ContactsPage.css';

interface ContactRow {
  id: string;
  channel: string;
  contactId: string;
  displayName: string;
  username: string;
}

interface AuthorizedContact {
  id?: string;
  display_name?: string;
  username?: string;
}

type AuthorizedContactsResponse = Record<string, AuthorizedContact[]>;

export default function ContactsPage() {
  const { t } = useTranslation();
  const addToast = useUIStore((s) => s.addToast);
  const { announce } = useAnnouncer();
  const { handleGridReady } = useGridFocus();
  useGridPageLandmarks({ pageClass: 'contacts-page' });
  const requestConfirm = useConfirm();
  const getErrorMessage = (error: unknown) =>
    error instanceof Error ? error.message : String(error ?? '');

  const [loading, setLoading] = useState(true);
  const [contactRows, setContactRows] = useState<ContactRow[]>([]);
  const [focusedContact, setFocusedContact] = useState<ContactRow | null>(null);

  const loadContacts = useCallback(async () => {
    setLoading(true);
    try {
      const allContacts = await GetAuthorizedContacts() as AuthorizedContactsResponse | null;
      const rows: ContactRow[] = [];
      if (allContacts) {
        for (const [ch, contacts] of Object.entries(allContacts)) {
          if (Array.isArray(contacts)) {
            for (const c of contacts) {
              rows.push({
                id: `${ch}:${c.id}`,
                channel: ch,
                contactId: c.id || '',
                displayName: c.display_name || '',
                username: c.username || '',
              });
            }
          }
        }
      }
      setContactRows(rows);
    } catch (error) {
      logger.error('Erro ao carregar contatos:', error);
      addToast(t('contacts.error.loadFailed', 'Erro ao carregar contatos'), 'error');
      setContactRows([]);
    } finally {
      setLoading(false);
    }
  }, [addToast, t]);

  useEffect(() => {
    loadContacts();
  }, [loadContacts]);

  const handleDeleteContact = useCallback(async (row: ContactRow) => {
    const name = row.displayName || row.contactId;
    const shouldRemove = await requestConfirm({
      title: t('channels.confirm.removeContactTitle'),
      message: t('channels.confirm.removeContactMessage', { name, channel: row.channel }),
      confirmText: t('common.remove'),
      cancelText: t('common.cancel'),
      variant: 'danger',
    });

    if (!shouldRemove) return;

    try {
      await RemoveAuthorizedContact(row.channel, row.contactId);
      addToast(t('channels.toast.contactRemoved'), 'success', undefined, undefined, { suppressAnnounce: true });
      announce(t('channels.announce.contactRemoved'));
      await loadContacts();
    } catch (error: unknown) {
      addToast(getErrorMessage(error) || t('contacts.error.removeFailed'), 'error');
    }
  }, [addToast, announce, loadContacts, requestConfirm, t]);

  function getContactRowActions(row: ContactRow) {
    return [
      {
        id: 'remove',
        label: t('channels.actions.removeContact', 'Remover'),
        icon: <DeleteOutlined aria-hidden="true" />,
        onClick: () => handleDeleteContact(row),
        danger: true,
      },
    ];
  }

  const contactColumns: DataGridColumn<ContactRow>[] = [
    { key: 'channel', label: t('channels.columns.channel'), width: '100px' },
    { key: 'displayName', label: t('channels.columns.name'), width: '200px', truncate: true },
    { key: 'username', label: t('channels.columns.username'), width: '150px', truncate: true },
    { key: 'contactId', label: t('channels.columns.id'), width: '200px', truncate: true },
    {
      key: 'actions', label: t('common.actions'), width: '80px',
      format: (_val, row) => (
        <MenuButton
          items={getContactRowActions(row)}
          buttonLabel={t('channels.actions.actions', 'Ações')}
        />
      ),
    },
  ];

  const getContactRowId = useCallback((item: ContactRow) => item.id, []);
  const handleContactFocusChange = useCallback((item: ContactRow | null) => setFocusedContact(item), []);

  const toolbarActions = [
    {
      key: 'remove-contact',
      label: t('channels.actions.removeContact', 'Remover'),
      icon: <DeleteOutlined aria-hidden="true" />,
      onClick: () => focusedContact && handleDeleteContact(focusedContact),
      disabled: !focusedContact,
      variant: 'danger' as const,
    },
    {
      key: 'reload',
      label: t('channels.buttons.reload', 'Recarregar'),
      icon: <ReloadOutlined aria-hidden="true" />,
      variant: 'secondary' as const,
      onClick: loadContacts,
      disabled: false,
    },
  ];

  if (loading) {
    return (
      <div className="contacts-page">
        <div className="contacts-page__loading" role="status">
          {t('contacts.loading', 'Carregando contatos...')}
        </div>
      </div>
    );
  }

  return (
    <div className="contacts-page">
      <Toolbar
        left={<h1 className="page-toolbar__title">{t('contacts.title', 'Contatos Autorizados')}</h1>}
        actions={toolbarActions}
        ariaLabel={t('contacts.toolbarLabel', 'Barra de ferramentas de contatos')}
      />
      <div className="contacts-page__content">
        <p className="contacts-page__description">
          {t('contacts.description', 'Contatos que podem se comunicar com o assistente por cada canal. Remova um contato para liberar uma vaga para novas autorizações.')}
        </p>
        {contactRows.length > 0 ? (
          <DataGrid
            items={contactRows}
            columns={contactColumns}
            label={t('contacts.gridLabel', 'Contatos autorizados')}
            autoFocusOnMount={false}
            getItemId={getContactRowId}
            onGridReady={handleGridReady}
            getRowActions={getContactRowActions}
            onFocusChange={handleContactFocusChange}
          />
        ) : (
          <p className="contacts-page__empty" role="status">
            {t('contacts.empty', 'Nenhum contato autorizado.')}
          </p>
        )}
      </div>
    </div>
  );
}
