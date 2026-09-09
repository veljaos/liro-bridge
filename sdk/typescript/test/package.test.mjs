/**
 * The package an integrator actually installs.
 *
 * Installing from GitHub means there is no publish step to run a build:
 * what is committed is what they get. This project has already had a
 * generated file drift from its generator once, and running the
 * generator would have deleted a block five screens depended on
 * (decision D-183). So the committed output being what the source
 * produces is a check, not a convention.
 */

import assert from 'node:assert/strict';
import { execFileSync } from 'node:child_process';
import { existsSync } from 'node:fs';
import { readFile, readdir, writeFile } from 'node:fs/promises';
import { createRequire } from 'node:module';
import { dirname, join } from 'node:path';
import { fileURLToPath } from 'node:url';
import test from 'node:test';

/**
 * npm, run as a script through this Node rather than through a shell.
 *
 * `execFileSync('npm', args, { shell: true })` works and concatenates
 * its arguments into a command line without escaping them, which is a
 * habit not worth having in a repository about signing.
 *
 * Where `npm-cli.js` sits depends on the platform. On Windows it is
 * beside the node binary, in `<prefix>/node_modules/npm/bin`. On POSIX —
 * `actions/setup-node` on ubuntu-latest included — node is in
 * `<prefix>/bin` and npm is in `<prefix>/lib/node_modules/npm/bin`, one
 * directory up and across. Assuming the Windows layout is what made this
 * test pass on the windows runner and fail on ubuntu-latest (D-221). So
 * ask npm first — it sets `npm_execpath` for the scripts it runs, and CI
 * runs this suite through `npm test` — and only then guess.
 */
const npmCliCandidates = () => {
  const fromEnv = process.env.npm_execpath;
  const prefix = dirname(process.execPath);
  return [
    ...(fromEnv && fromEnv.endsWith('.js') ? [fromEnv] : []),
    join(prefix, 'node_modules', 'npm', 'bin', 'npm-cli.js'),
    join(dirname(prefix), 'lib', 'node_modules', 'npm', 'bin', 'npm-cli.js'),
  ];
};

const resolveNpmCli = () => {
  const tried = npmCliCandidates();
  const found = tried.find((path) => existsSync(path));
  if (!found) {
    // A resolution failure has to say where it looked. The alternative
    // is the MODULE_NOT_FOUND this replaced, which named a path that
    // appeared nowhere in the test's output.
    throw new Error(`npm-cli.js was not found. Tried:\n${tried.map((p) => `  ${p}`).join('\n')}`);
  }
  return found;
};

const NPM_CLI = resolveNpmCli();
const npm = (args, options) => execFileSync(process.execPath, [NPM_CLI, ...args], options);

import { SDK_VERSION } from '../dist/esm/index.js';
import { scratch } from './helpers.mjs';

const here = dirname(fileURLToPath(import.meta.url));
const pkgDir = join(here, '..');
const repoRoot = join(pkgDir, '..', '..');

const readJSON = async (path) => JSON.parse(await readFile(path, 'utf8'));

test('the committed dist/ is what src/ produces', () => {
  // The build is run again into a scratch directory and compared file by
  // file. A failure prints which file and by how much.
  const output = execFileSync(process.execPath, [join(pkgDir, 'build.mjs'), '--check'], {
    cwd: pkgDir,
    encoding: 'utf8',
  });
  assert.match(output, /dist\/ matches src\//);
});

test('the package has no runtime dependencies', async () => {
  const pkg = await readJSON(join(pkgDir, 'package.json'));
  assert.deepEqual(pkg.dependencies ?? {}, {}, 'a runtime dependency here is a dependency in every integrator’s app');
  assert.deepEqual(
    pkg.peerDependencies ?? {},
    {},
    'a peer dependency is a dependency the integrator has to install themselves',
  );
  assert.deepEqual(pkg.optionalDependencies ?? {}, {});
  // The two development ones are pinned exactly, so the build is the
  // same build on every machine that runs it.
  for (const [name, range] of Object.entries(pkg.devDependencies ?? {})) {
    assert.match(range, /^\d+\.\d+\.\d+$/, `${name} is not pinned to an exact version: ${range}`);
  }
});

test('the package requires Node 18 or later, and says so where npm will read it', async () => {
  const pkg = await readJSON(join(pkgDir, 'package.json'));
  assert.equal(pkg.engines?.node, '>=18.0.0');
});

test('SDK_VERSION is the version package.json states', async () => {
  const pkg = await readJSON(join(pkgDir, 'package.json'));
  assert.equal(SDK_VERSION, pkg.version);
});

test('both module formats load, and export the same names', async () => {
  const esm = await import('../dist/esm/index.js');
  const require = createRequire(import.meta.url);
  const cjs = require('../dist/cjs/index.js');

  const esmNames = Object.keys(esm).filter((n) => n !== 'default').sort();
  const cjsNames = Object.keys(cjs).sort();
  assert.deepEqual(cjsNames, esmNames, 'require() and import see different packages');
  assert.ok(esmNames.includes('LiroBridge'));
  assert.ok(esmNames.includes('FileSecretStore'));
  assert.ok(esmNames.includes('LiroError'));
});

test('the CommonJS half is marked as CommonJS, and the ESM half as ESM', async () => {
  assert.equal((await readJSON(join(pkgDir, 'dist', 'cjs', 'package.json'))).type, 'commonjs');
  assert.equal((await readJSON(join(pkgDir, 'dist', 'esm', 'package.json'))).type, 'module');
});

test('the shipped types need no @types/node to compile against', async () => {
  const types = join(pkgDir, 'dist', 'types');
  for (const name of await readdir(types)) {
    const text = await readFile(join(types, name), 'utf8');
    // Declarations only. A doc comment may name `Buffer` — one of them
    // says a `Buffer` is a `Uint8Array` and costs nothing to pass — but
    // no declared type may be one.
    const declarations = text.replace(/\/\*[\s\S]*?\*\//g, '').replace(/^\s*\/\/.*$/gm, '');
    assert.ok(!/\bNodeJS\./.test(declarations), `${name} refers to a NodeJS type in the public surface`);
    assert.ok(!/from ['"]node:/.test(declarations), `${name} imports a node: builtin in the public surface`);
    assert.ok(!/\bBuffer\b/.test(declarations), `${name} names Buffer in the public surface; use Uint8Array`);
  }
});

test('the repository root is installable as @liro/bridge', async () => {
  // `npm install github:veljaos/liro-bridge#main` installs the
  // repository's own root, so that is where the package manifest has to
  // be. This checks the root manifest points at the SDK's real output
  // and that every path it names exists.
  const root = await readJSON(join(repoRoot, 'package.json'));
  assert.equal(root.name, '@liro/bridge');
  assert.equal(root.version, SDK_VERSION, 'the root manifest and the SDK disagree about the version');

  const exportsEntry = root.exports['.'];
  for (const [condition, path] of Object.entries(exportsEntry)) {
    assert.match(path, /^\.\/sdk\/typescript\/dist\//, `exports.${condition} does not point into the SDK's output`);
    await readFile(join(repoRoot, path), 'utf8');
  }
  await readFile(join(repoRoot, root.types), 'utf8');
  await readFile(join(repoRoot, root.main), 'utf8');
  assert.equal(root.engines?.node, '>=18.0.0');
  assert.deepEqual(root.dependencies ?? {}, {});
});

test('installing the repository the way the README says produces a working import', async (t) => {
  // The end of the whole argument: pack the repository root exactly as
  // npm would for a GitHub install, install the tarball into an empty
  // project, and import from '@liro/bridge'. Both module formats.
  const dir = await scratch();
  t.after(() => dir.dispose());

  const packed = npm(['pack', '--silent', '--pack-destination', dir.dir, repoRoot], {
    cwd: dir.dir,
    encoding: 'utf8',
  })
    .trim()
    .split(/\r?\n/)
    .at(-1);
  assert.ok(packed, 'npm pack produced no tarball');

  await writeFile(
    join(dir.dir, 'package.json'),
    JSON.stringify({ name: 'consumer', version: '1.0.0', type: 'module', private: true }, null, 2),
    'utf8',
  );
  npm(['install', '--no-audit', '--no-fund', '--silent', join(dir.dir, packed)], {
    cwd: dir.dir,
    encoding: 'utf8',
  });

  await writeFile(
    join(dir.dir, 'use-esm.mjs'),
    "import { LiroBridge, FileSecretStore, LiroError } from '@liro/bridge';\n" +
      "console.log([typeof LiroBridge.connect, typeof FileSecretStore, typeof LiroError].join(','));\n",
    'utf8',
  );
  const esm = execFileSync(process.execPath, ['use-esm.mjs'], { cwd: dir.dir, encoding: 'utf8' }).trim();
  assert.equal(esm, 'function,function,function');

  await writeFile(
    join(dir.dir, 'use-cjs.cjs'),
    "const { LiroBridge, FileSecretStore, LiroError } = require('@liro/bridge');\n" +
      "console.log([typeof LiroBridge.connect, typeof FileSecretStore, typeof LiroError].join(','));\n",
    'utf8',
  );
  const cjs = execFileSync(process.execPath, ['use-cjs.cjs'], { cwd: dir.dir, encoding: 'utf8' }).trim();
  assert.equal(cjs, 'function,function,function', 'require() of the installed package did not work');

  // And no dependencies came with it.
  const installed = await readdir(join(dir.dir, 'node_modules'));
  assert.deepEqual(
    installed.filter((n) => !n.startsWith('.')).sort(),
    ['@liro'],
    `installing this package brought other packages with it: ${installed.join(', ')}`,
  );
});
