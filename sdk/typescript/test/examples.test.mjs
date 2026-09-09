/**
 * The README's other-language fragments are the files they say they come
 * from, and each example is careful about the four things writing a
 * client by hand actually gets wrong.
 *
 * A README fragment is a generated artefact in every sense that matters:
 * nobody keeps one in step by hand reliably, and a fragment that has
 * drifted from the file beside it is worse than no fragment — the reader
 * copies it, it does not work, and they have lost more time than writing
 * it themselves would have cost. This project has already had one
 * generated file drift from its generator (decision D-183).
 */

import assert from 'node:assert/strict';
import { readFile, readdir } from 'node:fs/promises';
import { dirname, join } from 'node:path';
import { fileURLToPath } from 'node:url';
import test from 'node:test';

const here = dirname(fileURLToPath(import.meta.url));
const examplesDir = join(here, '..', '..', 'examples');
const readmePath = join(here, '..', 'README.md');

const EVERY_EXAMPLE = ['sign.py', 'sign.go', 'Sign.cs', 'Sign.java', 'sign.php', 'liro-test-client.ps1'];

const QUOTED = [
  ['Sign.cs', 'csharp'],
  ['sign.py', 'python'],
  ['Sign.java', 'java'],
  ['sign.php', 'php'],
  ['sign.go', 'go'],
];

/** The part of an example the README quotes, dedented to column zero. */
function quotedRegion(source, file) {
  const start = source.indexOf('--- README ---');
  const end = source.indexOf('--- /README ---');
  assert.notEqual(start, -1, `${file} has no --- README --- marker`);
  assert.notEqual(end, -1, `${file} has no --- /README --- marker`);

  const body = source.slice(source.indexOf('\n', start) + 1, source.lastIndexOf('\n', end));
  const lines = body.split('\n');
  const indents = lines.filter((l) => l.trim() !== '').map((l) => l.length - l.trimStart().length);
  const indent = indents.length === 0 ? 0 : Math.min(...indents);
  return lines
    .map((l) => l.slice(indent))
    .join('\n')
    .replace(/\n+$/, '');
}

/** One fenced block of a given language from the README. */
function fencedBlock(readme, language) {
  const open = '```' + language + '\n';
  const start = readme.indexOf(open);
  assert.notEqual(start, -1, `the README has no ${language} block`);
  const from = start + open.length;
  const end = readme.indexOf('\n```', from);
  assert.notEqual(end, -1, `the README's ${language} block is not closed`);
  return readme.slice(from, end).replace(/\n+$/, '');
}

/** A file with its comment lines taken out, so a comment cannot satisfy a check. */
function withoutComments(source) {
  return source
    .replace(/\/\*[\s\S]*?\*\//g, '')
    .replace(/^\s*(\/\/|#).*$/gm, '')
    .replace(/^\s*\*.*$/gm, '');
}

for (const [file, language] of QUOTED) {
  test(`the README's ${language} fragment is what ${file} contains`, async () => {
    const source = await readFile(join(examplesDir, file), 'utf8');
    const readme = await readFile(readmePath, 'utf8');
    assert.equal(
      fencedBlock(readme, language),
      quotedRegion(source, file),
      `the README's ${language} block and ${file} have drifted apart — copy the region between the ` +
        'markers in the file into the README, or the other way round',
    );
  });
}

test('the README’s first example is five lines or fewer', async () => {
  // SPEC §20: "If the README example needs more than five lines, the SDK
  // is wrong — fix the SDK, not the README." So this is a check on the
  // SDK's shape, not on the prose: it fails when connect-then-sign stops
  // fitting, and the fix is upstream of here.
  const readme = await readFile(readmePath, 'utf8');
  const first = fencedBlock(readme, 'ts');
  const lines = first.split('\n');
  assert.ok(lines.length <= 5, `the first example is ${lines.length} lines:\n${first}`);
  assert.match(first, /LiroBridge\.connect\(/, 'the first example does not connect');
  assert.match(first, /\.signPdf\(/, 'the first example does not sign');
  assert.match(first, /secretStore/, 'the first example hides where the device secret goes');
});

test('every example is linked from the README and from the examples index', async () => {
  const readme = await readFile(readmePath, 'utf8');
  const index = await readFile(join(examplesDir, 'README.md'), 'utf8');
  const files = (await readdir(examplesDir)).filter((n) => n !== 'README.md');

  assert.ok(files.length >= 7, `expected the examples to still be there, found ${files.join(', ')}`);
  for (const file of files) {
    assert.ok(index.includes(file), `sdk/examples/README.md does not mention ${file}`);
  }
  for (const [file] of QUOTED) {
    assert.ok(readme.includes(`../examples/${file}`), `the SDK README does not link to ${file}`);
  }
  assert.ok(
    readme.includes('../examples/liro-test-client.ps1'),
    'the SDK README does not link to the PowerShell client',
  );
});

test('the PowerShell client is where F8 asked for it, and not at the repository root', async () => {
  const root = join(here, '..', '..', '..');
  const rootFiles = await readdir(root);
  assert.ok(!rootFiles.includes('liro-test-client.ps1'), 'liro-test-client.ps1 is still at the repository root');
  const moved = await readFile(join(examplesDir, 'liro-test-client.ps1'), 'utf8');
  assert.match(moved, /pair\/confirm/, 'the moved client is not the one that speaks this protocol');
});

test('no example sends a Content-Type on a request with no body', async () => {
  // The mistake that made every request from a real .NET client fail.
  // Checked by looking at what encloses the line that sets the header,
  // rather than by pattern-matching each language's way of spelling "if
  // the body is not empty".
  for (const file of ['sign.py', 'sign.go', 'Sign.cs', 'Sign.java', 'sign.php']) {
    const source = await readFile(join(examplesDir, file), 'utf8');
    // Only the region the README quotes: that is where the request is
    // built, and every one of these files also *names* Content-Type in
    // its header comment, which is the whole point of the comment.
    const lines = quotedRegion(source, file).split('\n');
    // C# spells the header ContentType, with no hyphen.
    const at = lines.findIndex((l) => /Content-?Type/i.test(l) && !/^\s*(\/\/|#|\*)/.test(l));
    assert.notEqual(at, -1, `${file} never sets a Content-Type at all`);
    // The line itself and the four before it: some of these put the
    // condition on the same line as the header.
    const window = lines
      .slice(Math.max(0, at - 4), at + 1)
      .filter((l) => l.trim() !== '' && !/^\s*(\/\/|#|\*)/.test(l))
      .join(' ');
    assert.match(window, /\bif\b/, `${file} sets Content-Type unconditionally; a request with no body must not send one`);
  }
});

test('every example sends the same origin in both pairing calls', async () => {
  for (const file of EVERY_EXAMPLE) {
    const source = await readFile(join(examplesDir, file), 'utf8');
    const request = source.indexOf('/v2/pair/request');
    const confirm = source.indexOf('/v2/pair/confirm');
    assert.notEqual(request, -1, `${file} does not call /v2/pair/request`);
    assert.notEqual(confirm, -1, `${file} does not call /v2/pair/confirm`);
    // Look either side of the call: some of these build the body on the
    // line before it.
    const around = source.slice(Math.max(0, confirm - 500), confirm + 600);
    assert.match(around, /origin/, `${file} does not send origin with pair/confirm — the field that costs an afternoon`);
  }
});

test('every example draws its nonce from a cryptographic source', async () => {
  // Asserted the positive way round rather than by banning the
  // pseudo-random calls. A ban would scan comments too, and one of these
  // files says "never mt_rand()" in a comment — which is exactly the
  // right thing to say and exactly what a ban would fail on.
  const cryptographic = {
    'sign.py': /uuid4\(\)/,
    'sign.go': /randomBytes\(/,
    'Sign.cs': /Guid\.NewGuid\(\)/,
    'Sign.java': /UUID\.randomUUID\(\)/,
    'sign.php': /random_bytes\(/,
    'liro-test-client.ps1': /\[Guid\]::NewGuid\(\)/,
  };
  for (const [file, pattern] of Object.entries(cryptographic)) {
    const source = await readFile(join(examplesDir, file), 'utf8');
    assert.match(source, pattern, `${file} does not draw its nonce from a cryptographic source`);
  }
  // And the one indirection: sign.go's randomBytes must itself be
  // crypto/rand and not math/rand.
  const go = await readFile(join(examplesDir, 'sign.go'), 'utf8');
  assert.match(withoutComments(go), /"crypto\/rand"/, 'sign.go does not import crypto/rand');
  assert.ok(!withoutComments(go).includes('"math/rand"'), 'sign.go imports math/rand');
});

test('every example reads the port from the discovery file and never scans', async () => {
  for (const file of EVERY_EXAMPLE) {
    const code = withoutComments(await readFile(join(examplesDir, file), 'utf8'));
    assert.match(code, /bridge\.json/, `${file} does not read the discovery file`);
    for (let port = 17580; port <= 17590; port++) {
      assert.ok(!code.includes(String(port)), `${file} names ${port}, a port from the agent's range`);
    }
  }
});

test('every example resolves LOCALAPPDATA the way the agent writes it', async () => {
  // The agent writes to the LOCALAPPDATA *environment variable*. A
  // client that reads the shell's known folder instead can look
  // somewhere else — .NET's Environment.GetFolderPath does, and the C#
  // example did, and it looked in the wrong place. Measured.
  const readsTheVariable = {
    'sign.py': /environ\[.LOCALAPPDATA.\]/,
    'sign.go': /Getenv\("LOCALAPPDATA"\)/,
    'Sign.cs': /GetEnvironmentVariable\("LOCALAPPDATA"\)/,
    'Sign.java': /getenv\("LOCALAPPDATA"\)/,
    'sign.php': /getenv\('LOCALAPPDATA'\)/,
    'liro-test-client.ps1': /\$env:LOCALAPPDATA/,
  };
  for (const [file, pattern] of Object.entries(readsTheVariable)) {
    const source = await readFile(join(examplesDir, file), 'utf8');
    assert.match(source, pattern, `${file} does not read the LOCALAPPDATA environment variable`);
  }
});
