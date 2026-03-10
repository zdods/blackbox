// In-memory state for the registration flow. Never persisted to storage.
let pending = null;

export function setRegisterCredentials(username, password) {
  pending = { username, password };
}

export function getRegisterCredentials() {
  return pending;
}

export function clearRegisterCredentials() {
  pending = null;
}
