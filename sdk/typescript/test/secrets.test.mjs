/**
 * The device secret goes into a store and comes back, and goes nowhere
 * else at all.
 */

import assert from 'node:assert/strict';
import { execFileSync } from 'node:child_process';
import { readFile, stat, writeFile } from 'node:fs/promises';
import { inspect } from 'node:util';
import test from 'node:test';

import { FileSecretStore, LiroError, REDACTED_SECRET, StoredPairing } from '../dist/esm/index.js';
import { scratch } from './helpers.mjs';

const SECRET = Buffer.from('0123456789abcdef0123456789abcdef', 'utf8');
const SECRET_BASE64 = SECRET.toString('base64');

function samplePairing() {
  return new StoredPairing('e1a263168556ef06d1490dd26b247ec0', 'Moj ERP', 'https://erp.example.com', SECRET);
}

test('the secret is not an enumerable property', () => {
  const pairing = samplePairing();
  assert.deepEqual(Object.keys(pairing).sort(), ['appId', 'applicationName', 'origin']);
  assert.deepEqual(Object.getOwnPropertyNames(pairing).sort(), ['appId', 'applicationName', 'origin']);
  const spread = { ...pairing };
  assert.ok(!JSON.stringify(spread).includes(SECRET_BASE64));
  assert.ok(!JSON.stringify(spread).includes(SECRET.toString('utf8')));
});

test('JSON.stringify of a pairing does not print the secret', () => {
  const pairing = samplePairing();
  const text = JSON.stringify(pairing);
  assert.ok(!text.includes(SECRET_BASE64), 'JSON.stringify printed the base64 secret');
  assert.ok(!text.includes(SECRET.toString('utf8')), 'JSON.stringify printed the raw secret');
  assert.ok(text.includes(REDACTED_SECRET));
  // Nested, which is how it actually reaches a log.
  const nested = JSON.stringify({ context: { pairing, when: 'now' } });
  assert.ok(!nested.includes(SECRET_BASE64));
});

test('console.log of a pairing does not print the secret', () => {
  const pairing = samplePairing();
  for (const depth of [null, 0, 2, 10]) {
    for (const showHidden of [false, true]) {
      const text = inspect({ pairing }, { depth, showHidden, getters: true });
      assert.ok(!text.includes(SECRET_BASE64), `inspect(depth=${depth}, showHidden=${showHidden}) printed the secret`);
      assert.ok(!text.includes(SECRET.toString('utf8')), 'inspect printed the raw secret');
    }
  }
  assert.ok(!`${pairing}`.includes(SECRET_BASE64), 'string interpolation printed the secret');
  assert.ok(!String(pairing).includes(SECRET_BASE64));
});

test('serialise is the one way out, and deserialise is the way back', () => {
  const pairing = samplePairing();
  const record = pairing.serialise();
  assert.equal(record.deviceSecret, SECRET_BASE64);
  const back = StoredPairing.deserialise(record);
  assert.equal(back.appId, pairing.appId);
  assert.equal(back.applicationName, pairing.applicationName);
  assert.equal(back.origin, pairing.origin);
  assert.deepEqual([...back.deviceSecretBytes()], [...SECRET]);
});

test('the bytes handed out are a copy: mutating them does not change the pairing', () => {
  const pairing = samplePairing();
  const bytes = pairing.deviceSecretBytes();
  bytes.fill(0);
  assert.deepEqual([...pairing.deviceSecretBytes()], [...SECRET]);
});

test('a store that persisted JSON.stringify is told exactly what to do instead', () => {
  const pairing = samplePairing();
  const roundTripped = JSON.parse(JSON.stringify(pairing));
  assert.throws(
    () => StoredPairing.deserialise(roundTripped),
    (err) => {
      assert.ok(err instanceof LiroError);
      assert.equal(err.code, 'SECRET_STORE_INVALID');
      assert.match(err.message, /serialise\(\)/);
      assert.match(err.message, /deserialise\(\)/);
      return true;
    },
  );
});

test('deserialise refuses a record that is missing something, or the wrong length', () => {
  const good = samplePairing().serialise();
  for (const field of ['appId', 'applicationName', 'origin', 'deviceSecret']) {
    const broken = { ...good };
    delete broken[field];
    assert.throws(() => StoredPairing.deserialise(broken), new RegExp(field));
  }
  assert.throws(
    () => StoredPairing.deserialise({ ...good, deviceSecret: Buffer.alloc(16).toString('base64') }),
    /16 bytes/,
  );
  assert.throws(() => StoredPairing.deserialise(null), /not an object/);
});

test('FileSecretStore round-trips a pairing and forgets it', async () => {
  const dir = await scratch();
  try {
    const store = new FileSecretStore(dir.path('secrets.json'));
    assert.equal(await store.get('Moj ERP'), null);

    await store.set('Moj ERP', samplePairing());
    const back = await store.get('Moj ERP');
    assert.ok(back instanceof StoredPairing);
    assert.deepEqual([...back.deviceSecretBytes()], [...SECRET]);

    // Two applications, one file.
    await store.set('Drugi ERP', new StoredPairing('other', 'Drugi ERP', 'local', Buffer.alloc(32, 7)));
    assert.equal((await store.get('Moj ERP')).appId, 'e1a263168556ef06d1490dd26b247ec0');
    assert.equal((await store.get('Drugi ERP')).appId, 'other');

    await store.clear('Moj ERP');
    assert.equal(await store.get('Moj ERP'), null);
    assert.notEqual(await store.get('Drugi ERP'), null);
  } finally {
    await dir.dispose();
  }
});

/** The principals `icacls` lists for a file, as it spells them locally. */
function aclPrincipals(path) {
  const acl = execFileSync('icacls', [path], { encoding: 'utf8', windowsHide: true });
  return acl
    .split(/\r?\n/)
    .map((line) => line.replace(path, '').trim())
    .filter((line) => line.includes(':('))
    .map((line) => line.slice(0, line.indexOf(':(')));
}

test('FileSecretStore takes the file away from every other ordinary account', async () => {
  const dir = await scratch();
  try {
    const path = dir.path('secrets.json');

    if (process.platform === 'win32') {
      // Grant the Users group on the directory first, so the file it
      // holds actually inherits access for other ordinary accounts.
      // Without that this test would pass against a store that does
      // nothing at all, on a directory that had nothing to take away —
      // the right property against the wrong fixture (decision D-161).
      execFileSync('icacls', [dir.dir, '/grant', '*S-1-5-32-545:(OI)(CI)(F)'], {
        encoding: 'utf8',
        windowsHide: true,
      });
      // The localised name of that group, learnt from the machine rather
      // than assumed: it is "BUILTIN\\Users" here and "VGRAĐENO\\Korisnici"
      // on a Serbian Windows, and this product's users are on Serbian
      // Windows. Asked for by granting the SID on a file of its own and
      // reading back the one name that comes out.
      const namePath = dir.path('name-probe.txt');
      await writeFile(namePath, 'x', 'utf8');
      execFileSync('icacls', [namePath, '/inheritance:r', '/grant:r', '*S-1-5-32-545:(F)'], {
        encoding: 'utf8',
        windowsHide: true,
      });
      const probed = aclPrincipals(namePath);
      assert.equal(probed.length, 1, `expected one principal on the probe file, got ${probed.join(', ')}`);
      const usersGroup = probed[0];

      const store = new FileSecretStore(path);
      await store.set('Moj ERP', samplePairing());

      const after = aclPrincipals(path);
      assert.ok(
        !after.includes(usersGroup),
        `${usersGroup} still has access to the secret store: ${after.join(', ')}`,
      );
      const user = process.env.USERDOMAIN ? `${process.env.USERDOMAIN}\\${process.env.USERNAME}` : process.env.USERNAME;
      assert.ok(
        after.some((p) => p.toUpperCase() === user.toUpperCase()),
        `this user has no entry on the secret store: ${after.join(', ')}`,
      );
    } else {
      const store = new FileSecretStore(path);
      await store.set('Moj ERP', samplePairing());
      const mode = (await stat(path)).mode & 0o777;
      assert.equal(mode, 0o600, `expected 0600, got 0${mode.toString(8)}`);
    }
  } finally {
    await dir.dispose();
  }
});

test('FileSecretStore needs a path and says so', () => {
  assert.throws(() => new FileSecretStore(''), /needs a path/);
  assert.throws(() => new FileSecretStore('   '), /deliberately no default/);
});

test('FileSecretStore reads a file somebody saved with a byte-order mark', async () => {
  const dir = await scratch();
  try {
    const path = dir.path('secrets.json');
    const store = new FileSecretStore(path);
    await store.set('Moj ERP', samplePairing());
    const text = await readFile(path, 'utf8');
    await writeFile(path, `﻿${text}`, 'utf8');

    // Every ordinary way of editing this file on Windows writes one, and
    // JSON.parse refuses it at offset 1 — the agent's own configuration
    // loader had exactly this defect (decision D-157).
    const back = await store.get('Moj ERP');
    assert.deepEqual([...back.deviceSecretBytes()], [...SECRET]);
  } finally {
    await dir.dispose();
  }
});

test('FileSecretStore says what is wrong with a file it cannot use', async () => {
  const dir = await scratch();
  try {
    const path = dir.path('secrets.json');
    await writeFile(path, 'this is not JSON', 'utf8');
    const store = new FileSecretStore(path);
    await assert.rejects(store.get('Moj ERP'), /not valid JSON/);

    await writeFile(path, '[]', 'utf8');
    await assert.rejects(store.get('Moj ERP'), /pairing map/);
  } finally {
    await dir.dispose();
  }
});
