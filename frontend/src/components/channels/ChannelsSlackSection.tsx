import { Input, Checkbox, Button } from '../index';
import { ProfilePicker } from '../pickers/ProfilePicker';

interface SlackForm {
  enabled: boolean;
  botToken: string;
  appToken: string;
  profile: string;
  maxHistory: number;
  maxContacts: number;
}

interface ChannelsSlackSectionProps {
  form: SlackForm;
  onChange: (form: SlackForm) => void;
  onAnnounce: (message: string) => void;
  vaultEnabled: boolean;
  onToggleVault: (value: boolean) => void;
  botTokenStored: boolean;
  botTokenMasked: string;
  appTokenStored: boolean;
  appTokenMasked: string;
  onRemoveBotToken: () => void;
  onRemoveAppToken: () => void;
}

export function ChannelsSlackSection({
  form,
  onChange,
  onAnnounce,
  vaultEnabled,
  onToggleVault,
  botTokenStored,
  botTokenMasked,
  appTokenStored,
  appTokenMasked,
  onRemoveBotToken,
  onRemoveAppToken,
}: ChannelsSlackSectionProps) {
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
            placeholder="xoxb-..."
            fullWidth
          />
          <Checkbox
            label="Salvar tokens no cofre de credenciais"
            checked={vaultEnabled}
            onChange={(e) => onToggleVault(e.target.checked)}
          />
          <p className="channels-page__hint">
            Quando habilitado, os tokens ficam criptografados e não são salvos no arquivo do canal.
          </p>
          {botTokenStored && (
            <div className="channels-page__vault-actions">
              <span className="channels-page__hint">
                Bot Token salvo no cofre {botTokenMasked ? `(${botTokenMasked})` : ''}.
              </span>
              <Button variant="ghost" size="sm" onClick={onRemoveBotToken}>
                Remover do cofre
              </Button>
            </div>
          )}
          <p className="channels-page__hint">Token do bot do Slack (xoxb-...).</p>
          <Input
            label="App Token (Socket Mode)"
            type="password"
            value={form.appToken}
            onChange={(e) => onChange({ ...form, appToken: e.target.value })}
            placeholder="xapp-..."
            fullWidth
          />
          {appTokenStored && (
            <div className="channels-page__vault-actions">
              <span className="channels-page__hint">
                App Token salvo no cofre {appTokenMasked ? `(${appTokenMasked})` : ''}.
              </span>
              <Button variant="ghost" size="sm" onClick={onRemoveAppToken}>
                Remover do cofre
              </Button>
            </div>
          )}
          <p className="channels-page__hint">
            Token do app do Slack para Socket Mode (xapp-...).
          </p>
          <Input
            label="Max. contatos autorizados"
            type="number"
            value={String(form.maxContacts)}
            onChange={(e) =>
              onChange({
                ...form,
                maxContacts: parseInt(e.target.value) || 1,
              })
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
              onChange({
                ...form,
                maxHistory: parseInt(e.target.value) || 50,
              })
            }
            fullWidth
          />
        </>
      )}
    </>
  );
}
