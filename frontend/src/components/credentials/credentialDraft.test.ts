import { describe, expect, it } from 'vitest';
import { credentialInput, newCredential, validateCredential } from './credentialDraft';
const t = (key: string) => key;
describe('shared credential draft', () => {
  it.each(['env', 'keyring', 'command'])('never sends static material for %s', (source) => {
    const draft = {
      ...newCredential('example.com', 'basic'),
      source,
      token: 'TOKEN',
      password: 'PASSWORD',
      headerValue: 'HEADER',
      command: 'tool',
    };
    const input = credentialInput(draft);
    expect(input.token).toBeUndefined();
    expect(input.password).toBeUndefined();
    expect(input.headerValue).toBeUndefined();
  });
  it('requires one explicit keyring locator and an integer timeout', () => {
    const draft = {
      ...newCredential('example.com'),
      source: 'keyring',
      token: 'target',
      keyringService: 'service',
      keyringUser: 'user',
    };
    expect(validateCredential(draft, t)).toBe('credentials.sourceFields.keyringChoice');
    expect(validateCredential({ ...draft, token: '' }, t)).toBeNull();
    expect(
      validateCredential(
        {
          ...newCredential('example.com'),
          source: 'command',
          command: 'tool',
          timeoutSeconds: 1.5,
        },
        t
      )
    ).toBe('credentials.sourceFields.invalidTimeout');
  });
});
