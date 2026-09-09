/**
 * Every code the agent can send is typed, has a sentence, and arrives as
 * the code it was sent as.
 *
 * The completeness check reads `internal/errs/errs.go`'s own syntax
 * rather than a list somebody keeps in step by hand — decision D-158's
 * method, applied across the language boundary this time. A list kept in
 * step by hand is a check that quietly stops checking, and here it would
 * stop checking in the direction that hurts: an agent code this SDK does
 * not know reaches an integrator as `UNKNOWN` with no sentence.
 */

import assert from 'node:assert/strict';
import { readFile } from 'node:fs/promises';
import { fileURLToPath } from 'node:url';
import { dirname, join } from 'node:path';
import test from 'node:test';

import { LiroError, allKnownAgentCodes, isKnownAgentCode, messageForCode } from '../dist/esm/index.js';

const here = dirname(fileURLToPath(import.meta.url));
const repoRoot = join(here, '..', '..', '..');

/** Every `X Code = "VALUE"` constant declared in internal/errs. */
async function agentCodes() {
  const source = await readFile(join(repoRoot, 'internal', 'errs', 'errs.go'), 'utf8');
  const codes = [...source.matchAll(/^\s*Code[A-Za-z0-9]+\s+Code\s*=\s*"([A-Z0-9_]+)"/gm)].map((m) => m[1]);
  assert.ok(codes.length > 30, `expected the agent to declare many codes, found ${codes.length}`);
  return codes;
}

test('this SDK knows every code the agent declares', async () => {
  const declared = new Set(await agentCodes());
  const known = new Set(allKnownAgentCodes());

  const missing = [...declared].filter((code) => !known.has(code)).sort();
  assert.deepEqual(missing, [], 'codes the agent can send that this SDK does not name');

  const extra = [...known].filter((code) => !declared.has(code)).sort();
  assert.deepEqual(extra, [], 'codes this SDK names that the agent does not declare');
});

test('every code has a sentence, and none of them is empty or a placeholder', () => {
  for (const code of allKnownAgentCodes()) {
    const message = messageForCode(code);
    assert.equal(typeof message, 'string', `${code} has no message`);
    assert.ok(message.length > 20, `${code}'s message is too short to be one: ${JSON.stringify(message)}`);
    assert.ok(!message.includes(code), `${code}'s message reads like its own key`);
  }
});

test('AUTH_FAILED names the agent’s log and /v2/echo', () => {
  const message = messageForCode('AUTH_FAILED');
  assert.match(message, /log/i);
  assert.match(message, /\/v2\/echo/);
  for (const reason of ['signature_mismatch', 'clock_skew', 'nonce_reused', 'origin_mismatch']) {
    assert.ok(message.includes(reason), `AUTH_FAILED's message does not name ${reason}`);
  }
});

test('PIN_INCORRECT says never to retry, and says why', () => {
  const message = messageForCode('PIN_INCORRECT');
  assert.match(message, /NEVER retry/);
  assert.match(message, /block/i);
  assert.match(message, /Ministry/);
});

test('CERTIFICATE_LISTING_DISABLED reads as a setting, not a caller error', () => {
  const message = messageForCode('CERTIFICATE_LISTING_DISABLED');
  assert.match(message, /setting/i);
  assert.match(message, /not a fault in your request/i);
  assert.match(message, /Signing is unaffected/i);
});

test('DOCUMENT_SIGNING_DISABLED reads as a setting too', () => {
  const message = messageForCode('DOCUMENT_SIGNING_DISABLED');
  assert.match(message, /setting/i);
  assert.match(message, /not a fault in your request/i);
});

test('an agent code arrives as itself, with its details and status', () => {
  const error = LiroError.fromAgent('PAIRING_CODE_INCORRECT', { attemptsRemaining: 4 }, 401, 'POST /v2/pair/confirm');
  assert.ok(error instanceof LiroError);
  assert.equal(error.code, 'PAIRING_CODE_INCORRECT');
  assert.equal(error.agentCode, 'PAIRING_CODE_INCORRECT');
  assert.equal(error.httpStatus, 401);
  assert.equal(error.attemptsRemaining, 4);
  assert.deepEqual(error.details, { attemptsRemaining: 4 });
});

test('RATE_LIMITED surfaces retryAfterSeconds', () => {
  const error = LiroError.fromAgent('RATE_LIMITED', { retryAfterSeconds: 42 }, 429);
  assert.equal(error.retryAfterSeconds, 42);
  assert.equal(error.attemptsRemaining, null);
});

test('a code newer than this SDK is UNKNOWN and keeps what the agent sent', () => {
  const error = LiroError.fromAgent('SOMETHING_NOBODY_HAS_WRITTEN_YET', undefined, 422);
  assert.equal(error.code, 'UNKNOWN');
  assert.equal(error.agentCode, 'SOMETHING_NOBODY_HAS_WRITTEN_YET');
  assert.ok(!isKnownAgentCode('SOMETHING_NOBODY_HAS_WRITTEN_YET'));
  assert.match(error.message, /does not know/);
});

test('a switch over code is exhaustive — every known code has a branch', () => {
  // The compiler checks this for a TypeScript caller; this checks the
  // runtime half, which is that the union and the message table are the
  // same set of keys and neither has grown a member the other lacks.
  const codes = allKnownAgentCodes();
  const clientCodes = [
    'AGENT_NOT_RUNNING',
    'PROTOCOL_VIOLATION',
    'UNSUPPORTED_ENVIRONMENT',
    'CONFIGURATION_INVALID',
    'SECRET_STORE_INVALID',
    'TIMEOUT',
    'NETWORK',
    'CANCELLED',
    'UNKNOWN',
  ];
  for (const code of [...codes, ...clientCodes]) {
    assert.equal(typeof messageForCode(code), 'string', `${code} has no message`);
    const error = new LiroError(code);
    assert.equal(error.code, code);
    assert.equal(error.name, 'LiroError');
    assert.ok(error instanceof Error);
  }
});

test('the sentences say nothing about which authentication check failed', () => {
  // PROTOCOL.md §3.3 and decision D-175: the response never says, and
  // the SDK must not appear to know either. AUTH_FAILED lists the
  // reasons the agent's *log* records — which is the point of naming
  // them — but no other code may claim one.
  for (const code of allKnownAgentCodes()) {
    if (code === 'AUTH_FAILED') {
      continue;
    }
    const message = messageForCode(code);
    for (const reason of ['signature_mismatch', 'clock_skew', 'nonce_reused', 'nonce_too_long']) {
      assert.ok(!message.includes(reason), `${code} claims to know that ${reason} was the reason`);
    }
  }
});
