import { Input, Checkbox, Button } from '../index';
import { ProfilePicker } from '../pickers/ProfilePicker';

interface TelegramForm {
  enabled: boolean;
  botToken: string;
  profile: string;
  maxHistory: number;
  maxContacts: number;
}

interface ChannelsTelegramSectionProps {
  form: TelegramForm;
  onChange: (form: TelegramForm) => void;
  onAnnounce: (message: string) => void;
  vaultEnabled: boolean;
  onToggleVault: (value: boolean) => void;
  credentialStored: boolean;
  credentialMasked: string;
  onRemoveCredential: () => void;
}

export function ChannelsTelegramSection({
  form,
  onChange,
  onAnnounce,
  vaultEnabled,
  onToggleVault,
  credentialStored,
  credentialMasked,
  onRemoveCredential,
}: ChannelsTelegramSectionProps) {
  return (
    <>
      <Checkbox
        label="Habilitado"
        checked={form.enabled}
        onChange={(e) => onChange({ ...form, enabled: e.target.checked })}
      />
      {form.enabled && (
        <>
          <Input
            label="Bot Token"
            type="password"
            value={form.botToken}
            onChange={(e) => onChange({ ...form, botToken: e.target.value })}
            placeholder="123456789:ABCDEFG_xyz"
            fullWidth
          />
          <Checkbox
            label="Salvar token no cofre de credenciais"
            checked={vaultEnabled}
            onChange={(e) => onToggleVault(e.target.checked)}
          />
          <p className="channels-page__hint">
            Quando habilitado, o token fica criptografado e não é salvo no arquivo do canal.
          </p>
          {credentialStored && (
            <div className="channels-page__vault-actions">
              <span className="channels-page__hint">
                Token salvo no cofre {credentialMasked ? `(${credentialMasked})` : ''}.
              </span>
              <Button variant="ghost" size="sm" onClick={onRemoveCredential}>
                Remover do cofre
              </Button>
            </div>
          )}
          <p className="channels-page__hint">
            Crie um bot no @BotFather ({' '}
            <a href="https://t.me/BotFather" target="_blank" rel="noopener noreferrer">
              https://t.me/BotFather
            </a>
            ) e use seu token.
          </p>
          <Input
            label="Máximo de Contatos"
            type="number"
            min="1"
            max="100"
            value={form.maxContacts}
            onChange={(e) =>
              onChange({ ...form, maxContacts: parseInt(e.target.value) || 1 })
            }
            fullWidth
          />
          <p className="channels-page__hint">
            Ao atingir o limite, novos contatos são ignorados silenciosamente.
          </p>
          <ProfilePicker
            value={form.profile}
            onChange={(slug) => onChange({ ...form, profile: slug })}
            label="Perfil do Canal"
            maxWidth="100%"
            onAnnounce={onAnnounce}
          />
          <p className="channels-page__hint">
            Perfil usado para conversas deste canal. Define modelo, voz, STT e
            comportamento. Vazio usa o perfil ativo global.
          </p>
          <Input
            label="Máximo de Histórico"
            type="number"
            min="1"
            max="200"
            value={form.maxHistory}
            onChange={(e) =>
              onChange({ ...form, maxHistory: parseInt(e.target.value) || 50 })
            }
            fullWidth
          />
        </>
      )}
    </>
  );
}
