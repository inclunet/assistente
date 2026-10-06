import { useEffect, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { GetCredentialForURL, ListCredentials } from '@wailsjs/go/wailsapi/Credentials';
import { Button } from '../ui/Button';
import { useAnnouncer } from '../../hooks/useAnnouncer';
import { CredentialFields } from './CredentialFields';
import { credentialFromSummary, newCredential, type CredentialDraft } from './credentialDraft';

// Keyed by resource/type/consumer by its caller: edits never move to another destination.
export function ResourceCredentialEditor({
  url,
  type,
  pattern,
  allowedTypes,
  onChange,
}: {
  url: string;
  type: string;
  pattern?: string;
  allowedTypes?: string[];
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
    void Promise.resolve().then(async () => {
      if (!pattern) return GetCredentialForURL(url);
      return (await ListCredentials()).find(item => item.pattern === pattern);
    })
      .then((existing) => {
        if (!active) return;
        if (existing?.managed) { setFailed(true); return; }
        if (existing) setDraft({ ...credentialFromSummary(existing),
          type: allowedTypes?.includes(existing.type) ? existing.type : type });
        else if (pattern) setDraft(newCredential(pattern, type));
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
  }, [url, type, pattern, allowedTypes]);
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
          type="button"
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
            fixedType={!allowedTypes}
            allowedTypes={allowedTypes}
            allowOAuth={false}
            onChange={(field, value) => {
              setDraft((current) => ({ ...current, [field]: value }));
            }}
          />
          <Button
          type="button"
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
