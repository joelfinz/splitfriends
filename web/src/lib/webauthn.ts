// Uses native JSON helpers when present, falls back to @github/webauthn-json.
import { create as ponyCreate, get as ponyGet } from '@github/webauthn-json/browser-ponyfill';
import {
  parseCreationOptionsFromJSON as ponyParseCreation,
  parseRequestOptionsFromJSON as ponyParseRequest,
} from '@github/webauthn-json/browser-ponyfill';

type AnyPKC = typeof PublicKeyCredential & {
  parseCreationOptionsFromJSON?: (json: any) => CredentialCreationOptions['publicKey'];
  parseRequestOptionsFromJSON?: (json: any) => CredentialRequestOptions['publicKey'];
};

export function passkeysSupported(): boolean {
  return typeof window !== 'undefined' && 'PublicKeyCredential' in window && !!navigator.credentials;
}

function pkc(): AnyPKC | undefined {
  return typeof PublicKeyCredential !== 'undefined' ? (PublicKeyCredential as AnyPKC) : undefined;
}

/** `options` is the server response: `{ publicKey: {...} }` in standard JSON encoding. */
export async function createCredential(options: any): Promise<unknown> {
  const P = pkc();
  const pub = options?.publicKey ?? options;
  if (P?.parseCreationOptionsFromJSON) {
    const parsed = P.parseCreationOptionsFromJSON(pub);
    const cred = (await navigator.credentials.create({ publicKey: parsed })) as any;
    if (!cred) throw new Error('No credential returned');
    if (typeof cred.toJSON === 'function') return cred.toJSON();
  }
  const cred = await ponyCreate(ponyParseCreation({ publicKey: pub }));
  return cred.toJSON();
}

export async function getAssertion(options: any): Promise<unknown> {
  const P = pkc();
  const pub = options?.publicKey ?? options;
  if (P?.parseRequestOptionsFromJSON) {
    const parsed = P.parseRequestOptionsFromJSON(pub);
    const cred = (await navigator.credentials.get({ publicKey: parsed })) as any;
    if (!cred) throw new Error('No assertion returned');
    if (typeof cred.toJSON === 'function') return cred.toJSON();
  }
  const cred = await ponyGet(ponyParseRequest({ publicKey: pub }));
  return cred.toJSON();
}

export function describeWebAuthnError(e: unknown): string {
  const err = e as { name?: string; message?: string };
  switch (err?.name) {
    case 'NotAllowedError':
      return 'The passkey prompt was cancelled or timed out.';
    case 'InvalidStateError':
      return 'This device already has a passkey for this account.';
    case 'SecurityError':
      return 'Passkeys need a secure (https) origin.';
    case 'AbortError':
      return 'The passkey request was aborted.';
    default:
      return err?.message || 'Passkey operation failed.';
  }
}
