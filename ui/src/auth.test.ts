import assert from 'node:assert/strict';
import test from 'node:test';
import { activateSession, getToken, listSignedInUsers, rememberSession, removeSession, setToken } from './auth.ts';

class MemoryStorage {
  private values = new Map<string, string>();

  getItem(key: string) { return this.values.get(key) ?? null; }
  setItem(key: string, value: string) { this.values.set(key, value); }
  removeItem(key: string) { this.values.delete(key); }
}

test('signed-in accounts can be remembered, switched, and removed', () => {
  Object.defineProperty(globalThis, 'localStorage', { value: new MemoryStorage(), configurable: true });
  const first = { google_id: 'one', email: 'one@example.com', is_admin: false };
  const second = { google_id: 'two', email: 'two@example.com', is_admin: false };

  setToken('token-one');
  rememberSession(first);
  setToken('token-two');
  rememberSession(second);
  assert.deepEqual(listSignedInUsers().map(user => user.google_id), ['two', 'one']);

  assert.equal(activateSession('one'), true);
  assert.equal(getToken(), 'token-one');
  assert.deepEqual(removeSession('one').map(user => user.google_id), ['two']);
  assert.equal(getToken(), 'token-two');
});
