import { Button } from '../index';

interface SignalAccountManagementProps {
  accounts: string[];
  unregistering: string | null;
  onUnregister: (account: string) => Promise<void>;
}

export function SignalAccountManagement({
  accounts,
  unregistering,
  onUnregister,
}: SignalAccountManagementProps) {
  if (accounts.length === 0) {
    return null;
  }

  return (
    <div className="channels-page__subsection">
      <h4>Conta Conectada</h4>
      <div className="channels-page__account-row">
        <span className="channels-page__account-number">{accounts[0]}</span>
        <Button
          variant="danger"
          size="sm"
          onClick={() => onUnregister(accounts[0])}
          loading={unregistering === accounts[0]}
          disabled={unregistering !== null}
        >
          Desconectar
        </Button>
      </div>
    </div>
  );
}
