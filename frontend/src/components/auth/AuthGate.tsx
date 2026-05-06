import { useEffect, useState } from 'react';
import type { CSSProperties, FormEvent, ReactNode } from 'react';
import { useAuthStore } from '../../store/authStore';

interface AuthGateProps {
  children: ReactNode;
}

export function AuthGate({ children }: AuthGateProps) {
  const status = useAuthStore((s) => s.status);
  const isLoading = useAuthStore((s) => s.isLoading);
  const error = useAuthStore((s) => s.error);
  const isAuthenticated = useAuthStore((s) => s.isAuthenticated);
  const loadStatus = useAuthStore((s) => s.loadStatus);
  const setupVault = useAuthStore((s) => s.setupVault);
  const unlockVault = useAuthStore((s) => s.unlockVault);
  const createAdmin = useAuthStore((s) => s.createAdmin);
  const login = useAuthStore((s) => s.login);
  const [secret, setSecret] = useState('');
  const [confirmSecret, setConfirmSecret] = useState('');
  const [username, setUsername] = useState('');
  const [recoveryKey, setRecoveryKey] = useState('');

  useEffect(() => {
    void loadStatus();
  }, [loadStatus]);

  if (isAuthenticated) {
    return <>{children}</>;
  }

  const title = !status?.vaultConfigured
    ? 'Inicializar cofre'
    : !status.vaultUnlocked
      ? 'Desbloquear cofre'
      : !status.hasUsers
        ? 'Criar admin local'
        : 'Entrar';

  const submit = async (event: FormEvent) => {
    event.preventDefault();
    if (!status?.vaultConfigured) {
      if (secret !== confirmSecret) return;
      setRecoveryKey(await setupVault(secret));
      setSecret('');
      setConfirmSecret('');
      return;
    }
    if (!status.vaultUnlocked) {
      await unlockVault('master', secret);
      setSecret('');
      return;
    }
    if (!status.hasUsers) {
      if (secret !== confirmSecret) return;
      await createAdmin(username, secret);
      setSecret('');
      setConfirmSecret('');
      return;
    }
    await login(username, secret);
    setSecret('');
  };

  return (
    <main style={styles.shell} aria-busy={isLoading}>
      <form style={styles.card} onSubmit={(event) => void submit(event)}>
        <h1 style={styles.title}>{title}</h1>
        <p style={styles.description}>
          AEP-0052 adiciona contas locais. Configure o cofre e entre para continuar.
        </p>

        {recoveryKey && (
          <div style={styles.recovery}>
            <strong>Código de recuperação</strong>
            <code style={styles.code}>{recoveryKey}</code>
            <span>Guarde este código em local seguro antes de continuar.</span>
          </div>
        )}

        {(status?.hasUsers || status?.vaultUnlocked) && (
          <label style={styles.label}>
            Usuário
            <input
              value={username}
              onChange={(event) => setUsername(event.target.value)}
              required
              autoComplete="username"
              style={styles.input}
            />
          </label>
        )}

        <label style={styles.label}>
          {status?.hasUsers ? 'Senha' : 'Senha mestre'}
          <input
            value={secret}
            onChange={(event) => setSecret(event.target.value)}
            required
            type="password"
            autoComplete={status?.hasUsers ? 'current-password' : 'new-password'}
            style={styles.input}
          />
        </label>

        {(!status?.vaultConfigured || (status?.vaultUnlocked && !status?.hasUsers)) && (
          <label style={styles.label}>
            Confirmar senha
            <input
              value={confirmSecret}
              onChange={(event) => setConfirmSecret(event.target.value)}
              required
              type="password"
              autoComplete="new-password"
              style={styles.input}
            />
          </label>
        )}

        {error && <p style={styles.error}>{error}</p>}
        <button disabled={isLoading || (!!confirmSecret && secret !== confirmSecret)} style={styles.button}>
          {isLoading ? 'Aguarde...' : 'Continuar'}
        </button>
      </form>
    </main>
  );
}

const styles: Record<string, CSSProperties> = {
  shell: {
    alignItems: 'center',
    background: 'var(--color-bg-primary, #111827)',
    color: 'var(--color-text-primary, #fff)',
    display: 'flex',
    minHeight: '100vh',
    justifyContent: 'center',
    padding: 24,
  },
  card: {
    background: 'var(--color-bg-secondary, #1f2937)',
    border: '1px solid var(--color-border, #374151)',
    borderRadius: 16,
    display: 'grid',
    gap: 16,
    maxWidth: 420,
    padding: 24,
    width: '100%',
  },
  title: { fontSize: 24, margin: 0 },
  description: { margin: 0, opacity: 0.8 },
  label: { display: 'grid', gap: 6, fontWeight: 600 },
  input: { borderRadius: 8, border: '1px solid #4b5563', padding: '10px 12px' },
  button: { borderRadius: 8, cursor: 'pointer', fontWeight: 700, padding: '10px 12px' },
  error: { color: '#fca5a5', margin: 0 },
  recovery: { display: 'grid', gap: 8, padding: 12, border: '1px solid #4b5563', borderRadius: 8 },
  code: { whiteSpace: 'pre-wrap', wordBreak: 'break-word' },
};
