import { useEffect, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { GetCredentialForURL } from '@wailsjs/go/wailsapi/Credentials';
import { Button } from '../ui/Button';
import { useAnnouncer } from '../../hooks/useAnnouncer';
import { CredentialFields } from './CredentialFields';
import { credentialFromSummary, newCredential, type CredentialDraft } from './credentialDraft';

// Keyed by resource/type/consumer by its caller: edits never move to another destination.
export function McpCredentialEditor({
  url,
  type,
  onChange,
}: {
  url: string;
  type: string;
  onChange: (draft: CredentialDraft | null) => void;
}) {
  const { t } = useTranslation();
  const { announce } = useAnnouncer();
  let hostname = '';
  try {
    const parsed = new URL(url);
    if (['https:', 'http:'].includes(parsed.protocol))
      hostname = parsed.hostname.replace(/^\[|\]$/g, '');
  } catch {
    /* Incomplete URL. */
  }
  const [draft, setDraft] = useState(() => newCredential(hostname, type));
  const [editing, setEditing] = useState(false);
  const sourceRef = useRef<HTMLSelectElement>(null);
  const configureRef = useRef<HTMLButtonElement>(null);
  const wasEditing = useRef(false);
  useEffect(() => {
    if (editing) sourceRef.current?.focus();
    else if (wasEditing.current) configureRef.current?.focus();
    wasEditing.current = editing;
  }, [editing]);
  const [loading, setLoading] = useState(true);
  const [failed, setFailed] = useState(false);
  useEffect(() => {
    let active = true;
    void GetCredentialForURL(url)
      .then((existing) => {
        if (!active) return;
        if (existing && !existing.managed) setDraft({ ...credentialFromSummary(existing), type });
      })
      .catch(() => {
        if (active) setFailed(true);
      })
      .finally(() => {
        if (active) setLoading(false);
      });
    return () => {
      active = false;
    };
  }, [url, type]);
  useEffect(() => {
    if (failed) announce(t('credentials.sourceFields.loadError'), 'assertive');
  }, [failed, announce, t]);
  useEffect(() => {
    onChange(editing ? draft : null);
  }, [draft, editing, onChange]);
  return (
    <div className="credential-fields">
      <p>{t('credentials.mcp.destination', { hostname })}</p>
      <p>{t('credentials.mcp.sharedHint')}</p>
      <p>{t('credentials.mcp.binding', { pattern: draft.pattern })}</p>
      {failed && <p>{t('credentials.sourceFields.loadError')}</p>}
      {!editing ? (
        <Button
          ref={configureRef}
          disabled={loading || failed || !hostname}
          onClick={() => {
            setEditing(true);
          }}
        >
          {t('credentials.mcp.configure')}
        </Button>
      ) : (
        <>
          <CredentialFields
            value={draft}
            sourceRef={sourceRef}
            fixedType
            allowOAuth={false}
            onChange={(field, value) => {
              setDraft((current) => ({ ...current, [field]: value }));
            }}
          />
          <Button
            variant="ghost"
            onClick={() => {
              setEditing(false);
            }}
          >
            {t('credentials.mcp.keepExisting')}
          </Button>
        </>
      )}
    </div>
  );
}
