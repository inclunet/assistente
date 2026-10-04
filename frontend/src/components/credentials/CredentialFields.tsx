import { useCallback, useEffect, useId, useRef, useState, type Ref } from 'react';
import { useTranslation } from 'react-i18next';
import { ListExternalSources } from '@wailsjs/go/wailsapi/Credentials';
import { Input } from '../ui/Input';
import { Select } from '../ui/Select';
import { useAnnouncer } from '../../hooks/useAnnouncer';
import type { CredentialDraft } from './credentialDraft';
import './CredentialFields.css';

export function CredentialFields({
  value,
  onChange,
  fixedType = false,
  allowOAuth = true,
  sourceRef,
}: {
  value: CredentialDraft;
  onChange: (field: keyof CredentialDraft, value: unknown) => void;
  fixedType?: boolean;
  allowOAuth?: boolean;
  sourceRef?: Ref<HTMLSelectElement>;
}) {
  const { t } = useTranslation();
  const { announce } = useAnnouncer();
  const uid = useId();
  const updateField = onChange;
  const [suggestions, setSuggestions] = useState<Array<{ value: string; label: string }>>([]);
  const [showSuggestions, setShowSuggestions] = useState(false);
  const [activeIndex, setActiveIndex] = useState(-1);
  const allSuggestionsRef = useRef<Array<{ value: string; label: string }>>([]);
  const loadedPrefixRef = useRef<string | null>(null);
  const sourcesPromiseRef = useRef<{
    prefix: string;
    epoch: number;
    promise: Promise<Array<{ value: string; label: string }>>;
  } | null>(null);
  const cacheEpochRef = useRef(0);
  const listboxRef = useRef<HTMLUListElement>(null);
  const latestTokenRef = useRef<string>('');
  const prevShowSuggestionsRef = useRef(false);

  const typeOptions = [
    { value: 'bearer', label: t('credentials.types.bearer') },
    { value: 'basic', label: t('credentials.types.basic') },
    { value: 'custom', label: t('credentials.types.custom') },
    { value: 'secret', label: t('credentials.types.secret') },
  ];

  const invalidateSuggestionCache = useCallback(() => {
    cacheEpochRef.current += 1;
    sourcesPromiseRef.current = null;
    allSuggestionsRef.current = [];
    loadedPrefixRef.current = null;
  }, []);

  const loadExternalSources = useCallback((prefix: string) => {
    if (loadedPrefixRef.current === prefix) return Promise.resolve(allSuggestionsRef.current);

    const epoch = cacheEpochRef.current;
    const pending = sourcesPromiseRef.current;
    if (pending && pending.prefix === prefix && pending.epoch === epoch) return pending.promise;

    const promise = (async () => {
      let items: Array<{ value: string; label: string }> = [];
      try {
        const results = await ListExternalSources(prefix);
        items = (results || []).map((r) => ({ value: r.value, label: r.label }));
      } catch {
        items = [];
      }
      // Descarta resultado tardio se o cache foi invalidado (reset/close/limpeza) durante a requisição.
      if (cacheEpochRef.current === epoch) {
        allSuggestionsRef.current = items;
        loadedPrefixRef.current = prefix;
        if (
          sourcesPromiseRef.current?.epoch === epoch &&
          sourcesPromiseRef.current?.prefix === prefix
        ) {
          sourcesPromiseRef.current = null;
        }
      }
      return items;
    })();

    sourcesPromiseRef.current = { prefix, epoch, promise };
    return promise;
  }, []);

  const handleTokenChange = async (inputValue: string) => {
    updateField('token', inputValue);
    setActiveIndex(-1);
    latestTokenRef.current = inputValue;

    const prefix = value.source === 'keyring' || value.source === 'env' ? value.source : null;

    if (!prefix) {
      // Só limpa/invalida se o autocomplete chegou a ser ativado (evita renders e
      // incremento de epoch a cada tecla quando o token nunca foi uma referência).
      if (loadedPrefixRef.current !== null || showSuggestions) {
        setShowSuggestions(false);
        setSuggestions([]);
        invalidateSuggestionCache();
      }
      return;
    }

    const epoch = cacheEpochRef.current;
    const items = await loadExternalSources(prefix);
    if (cacheEpochRef.current !== epoch || latestTokenRef.current !== inputValue) return;

    const search = inputValue.toLowerCase();
    const filtered =
      search === '' ? items : items.filter((s) => s.label.toLowerCase().includes(search));
    setSuggestions(filtered);
    setShowSuggestions(filtered.length > 0);
  };

  const handleTokenKeyDown = useCallback(
    (e: React.KeyboardEvent) => {
      if (!showSuggestions || suggestions.length === 0) return;

      switch (e.key) {
        case 'ArrowDown':
          e.preventDefault();
          setActiveIndex((prev) => {
            const next = prev < suggestions.length - 1 ? prev + 1 : 0;
            requestAnimationFrame(() => {
              listboxRef.current
                ?.querySelector(`#${CSS.escape(uid)}-suggestion-${next}`)
                ?.scrollIntoView({ block: 'nearest' });
            });
            return next;
          });
          break;
        case 'ArrowUp':
          e.preventDefault();
          setActiveIndex((prev) => {
            const next = prev > 0 ? prev - 1 : suggestions.length - 1;
            requestAnimationFrame(() => {
              listboxRef.current
                ?.querySelector(`#${CSS.escape(uid)}-suggestion-${next}`)
                ?.scrollIntoView({ block: 'nearest' });
            });
            return next;
          });
          break;
        case 'Enter':
          if (activeIndex >= 0 && activeIndex < suggestions.length) {
            e.preventDefault();
            const selected = suggestions[activeIndex].value;
            latestTokenRef.current = selected;
            updateField('token', selected);
            setShowSuggestions(false);
            setActiveIndex(-1);
          }
          break;
        case 'Escape':
          e.stopPropagation();
          latestTokenRef.current = '';
          setShowSuggestions(false);
          setActiveIndex(-1);
          break;
      }
    },
    [showSuggestions, suggestions, activeIndex, updateField, uid]
  );

  const resetSuggestions = useCallback(() => {
    latestTokenRef.current = '';
    setSuggestions([]);
    setShowSuggestions(false);
    setActiveIndex(-1);
    invalidateSuggestionCache();
  }, [invalidateSuggestionCache]);

  useEffect(() => {
    resetSuggestions();
    return invalidateSuggestionCache;
  }, [value.source, resetSuggestions, invalidateSuggestionCache]);

  // Anuncia as sugestões apenas na TRANSIÇÃO de visibilidade do dropdown
  // (fechado→aberto), não a cada tecla — evita "spam" de announcements no leitor
  // de tela enquanto o usuário digita e a lista é apenas refiltrada. O ref é
  // mantido em sincronia por este efeito independentemente de onde showSuggestions
  // muda (seleção, Escape, blur, reset).
  useEffect(() => {
    if (showSuggestions === prevShowSuggestionsRef.current) return;
    prevShowSuggestionsRef.current = showSuggestions;
    if (showSuggestions) {
      announce(t('credentials.aria.suggestionsAvailable', { count: suggestions.length }));
    }
  }, [showSuggestions, suggestions.length, announce, t]);

  return (
    <div className="credential-fields">
      <Select
        ref={sourceRef}
        label={t('credentials.sourceFields.source')}
        value={value.source}
        options={(allowOAuth
          ? ['static', 'env', 'keyring', 'command', 'oauth']
          : ['static', 'env', 'keyring', 'command']
        ).map((value) => ({ value, label: t(`credentials.sourceFields.${value}`) }))}
        onChange={(e) => {
          resetSuggestions();
          updateField('source', e.target.value);
          updateField('token', '');
          if (e.target.value === 'oauth') announce(t('credentials.sourceFields.oauthUnavailable'));
        }}
        fullWidth
      />
      {!fixedType && (
        <Select
          label={t('credentials.labels.type')}
          value={value.type}
          options={typeOptions}
          onChange={(e) => updateField('type', e.target.value)}
          fullWidth
        />
      )}

      {(value.source === 'env' ||
        value.source === 'keyring' ||
        (value.source === 'static' && (value.type === 'bearer' || value.type === 'secret'))) &&
        (() => {
          const tokenIsRef = value.source === 'env' || value.source === 'keyring';
          const hasSuggestions = showSuggestions && suggestions.length > 0;
          return (
            <div className="credentials-page__token-field">
              <Input
                label={
                  tokenIsRef
                    ? t(`credentials.sourceFields.${value.source}Name`)
                    : t('credentials.labels.token')
                }
                type={tokenIsRef ? 'text' : 'password'}
                value={value.token || ''}
                onChange={(e) => handleTokenChange(e.target.value)}
                onKeyDown={handleTokenKeyDown}
                onBlur={() => {
                  latestTokenRef.current = '';
                  setShowSuggestions(false);
                  setActiveIndex(-1);
                }}
                onFocus={() => {
                  if (tokenIsRef) void handleTokenChange(value.token || '');
                }}
                fullWidth
                autoComplete="off"
                role={tokenIsRef ? 'combobox' : undefined}
                aria-haspopup={tokenIsRef ? 'listbox' : undefined}
                aria-expanded={tokenIsRef ? hasSuggestions : undefined}
                aria-controls={tokenIsRef && hasSuggestions ? `${uid}-suggestions` : undefined}
                aria-activedescendant={
                  tokenIsRef && activeIndex >= 0 ? `${uid}-suggestion-${activeIndex}` : undefined
                }
                aria-autocomplete={tokenIsRef ? 'list' : undefined}
              />
              {tokenIsRef && hasSuggestions && (
                <ul
                  id={`${uid}-suggestions`}
                  ref={listboxRef}
                  className="credentials-page__suggestions"
                  role="listbox"
                  aria-label={t('credentials.aria.suggestions')}
                >
                  {suggestions.map((s, index) => (
                    <li
                      key={s.value}
                      id={`${uid}-suggestion-${index}`}
                      role="option"
                      aria-selected={index === activeIndex}
                      className={`credentials-page__suggestion${index === activeIndex ? ' credentials-page__suggestion--active' : ''}`}
                      onMouseDown={(e) => {
                        e.preventDefault();
                        latestTokenRef.current = s.value;
                        updateField('token', s.value);
                        setShowSuggestions(false);
                        setActiveIndex(-1);
                      }}
                    >
                      {s.label}
                    </li>
                  ))}
                </ul>
              )}
            </div>
          );
        })()}

      {value.source === 'keyring' && (
        <>
          <Input
            label={t('credentials.sourceFields.keyringService')}
            value={value.keyringService || ''}
            onChange={(e) => updateField('keyringService', e.target.value)}
            fullWidth
          />
          <Input
            label={t('credentials.sourceFields.keyringUser')}
            value={value.keyringUser || ''}
            onChange={(e) => updateField('keyringUser', e.target.value)}
            fullWidth
          />
        </>
      )}
      {value.source === 'command' && (
        <>
          <p>{t('credentials.sourceFields.cacheHint')}</p>
          <Input
            label={t('credentials.sourceFields.commandName')}
            value={value.command || ''}
            onChange={(e) => updateField('command', e.target.value)}
            fullWidth
          />
          <Input
            label={t('credentials.sourceFields.args')}
            value={value.argsText ?? '[]'}
            onChange={(e) => updateField('argsText', e.target.value)}
            fullWidth
          />
          <Input
            label={t('credentials.sourceFields.timeout')}
            type="number"
            min={1}
            max={300}
            value={value.timeoutSeconds ?? 30}
            onChange={(e) => updateField('timeoutSeconds', Number(e.target.value))}
            fullWidth
          />
        </>
      )}
      {value.source === 'oauth' && <p>{t('credentials.sourceFields.oauthUnavailable')}</p>}
      {value.type === 'basic' && (
        <>
          <Input
            label={t('credentials.labels.username')}
            value={value.username || ''}
            onChange={(e) => updateField('username', e.target.value)}
            fullWidth
          />
          {value.source === 'static' && (
            <Input
              label={t('credentials.labels.password')}
              type="password"
              value={value.password || ''}
              onChange={(e) => updateField('password', e.target.value)}
              fullWidth
            />
          )}
        </>
      )}
      {value.type === 'custom' && (
        <div className="credentials-page__row">
          <Input
            label={t('credentials.labels.header')}
            value={value.headerName || ''}
            onChange={(e) => updateField('headerName', e.target.value)}
            fullWidth
          />
          {value.source === 'static' && (
            <Input
              label={t('credentials.labels.value')}
              type="password"
              value={value.headerValue || ''}
              onChange={(e) => updateField('headerValue', e.target.value)}
              fullWidth
            />
          )}
        </div>
      )}

      <p className="credentials-page__hint">{t('credentials.hint.sensitive')}</p>
    </div>
  );
}
