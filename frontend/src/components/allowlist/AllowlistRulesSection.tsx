import { allowlist } from '../../../wailsjs/go/models';

interface AllowlistRulesSectionProps {
  item: allowlist.Allowlist;
  onRulesChange: (field: 'auto_approve' | 'always_deny', rules: string[]) => void;
}

export function AllowlistRulesSection({
  item,
  onRulesChange,
}: AllowlistRulesSectionProps) {
  const handleTextChange = (field: 'auto_approve' | 'always_deny', text: string) => {
    const rules = text.split('\n').map((s) => s.trim()).filter(Boolean);
    onRulesChange(field, rules);
  };

  return (
    <section className="allowlist-section" data-testid="allowlist-rules-section">
      <h3 className="allowlist-section__title">Regras</h3>
      <div className="allowlist-fields">
        <div className="allowlist-field">
          <label htmlFor="al-auto-approve" className="allowlist-field__label">
            Auto Approve (um pattern por linha)
          </label>
          <textarea
            id="al-auto-approve"
            className="allowlist-field__textarea"
            rows={10}
            value={(item.auto_approve || []).join('\n')}
            onChange={(e) => handleTextChange('auto_approve', e.target.value)}
            placeholder="ls&#10;git status&#10;git diff *&#10;go test *"
          />
          <span className="allowlist-field__hint">
            Comandos aprovados sem confirmação. Use * no final para prefix match.
          </span>
        </div>

        <div className="allowlist-field">
          <label htmlFor="al-always-deny" className="allowlist-field__label">
            Always Deny (um pattern por linha)
          </label>
          <textarea
            id="al-always-deny"
            className="allowlist-field__textarea"
            rows={6}
            value={(item.always_deny || []).join('\n')}
            onChange={(e) => handleTextChange('always_deny', e.target.value)}
            placeholder="rm -rf /&#10;shutdown&#10;reboot"
          />
          <span className="allowlist-field__hint">
            Comandos sempre bloqueados, mesmo que estejam em Auto Approve.
          </span>
        </div>
      </div>
    </section>
  );
}
