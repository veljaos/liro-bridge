#!/usr/bin/env node
/**
 * Builds `dist/` — ESM, CommonJS and types — and, with `--check`,
 * proves the committed `dist/` is what this source produces.
 *
 * The check exists because installing from GitHub means there is no
 * publish step to run a build: what is committed is what an integrator
 * gets. This project has already had one generated file drift from its
 * generator, and running the generator would have deleted a block five
 * screens depended on (decision D-183). A committed artefact nobody
 * regenerates is a committed artefact in name only; a check that
 * regenerates it and compares is what makes the header's "do not edit
 * by hand" true.
 */

import { execFileSync } from 'node:child_process';
import { existsSync, mkdirSync, readdirSync, readFileSync, rmSync, statSync, writeFileSync } from 'node:fs';
import { dirname, join, relative } from 'node:path';
import { fileURLToPath } from 'node:url';

const here = dirname(fileURLToPath(import.meta.url));
const check = process.argv.includes('--check');
const distDir = join(here, 'dist');
const workDir = check ? join(here, '.dist-check') : distDir;

const tsc = join('node_modules', 'typescript', 'bin', 'tsc');

function run(configFile, outDir, declarationDir) {
  const args = [
    join(here, tsc),
    '--project',
    join(here, configFile),
    '--outDir',
    outDir,
  ];
  if (declarationDir !== undefined) {
    args.push('--declarationDir', declarationDir);
  }
  execFileSync(process.execPath, args, { stdio: 'inherit', cwd: here });
}

rmSync(workDir, { recursive: true, force: true });
mkdirSync(workDir, { recursive: true });

run('tsconfig.esm.json', join(workDir, 'esm'), join(workDir, 'types'));
run('tsconfig.cjs.json', join(workDir, 'cjs'), undefined);

// Node decides whether a .js file is ESM or CommonJS from the nearest
// package.json's "type". The package itself is "module", so the CommonJS
// output needs its own marker or `require()` of it fails with
// ERR_REQUIRE_ESM — and the ESM output needs one too, in case the
// package's own type ever changes.
writeFileSync(join(workDir, 'esm', 'package.json'), `${JSON.stringify({ type: 'module' }, null, 2)}\n`, 'utf8');
writeFileSync(join(workDir, 'cjs', 'package.json'), `${JSON.stringify({ type: 'commonjs' }, null, 2)}\n`, 'utf8');

if (!check) {
  console.log(`built ${relative(here, distDir)}`);
  process.exit(0);
}

const differences = compare(distDir, workDir);
rmSync(workDir, { recursive: true, force: true });

if (differences.length > 0) {
  console.error('The committed dist/ is not what src/ produces:\n');
  for (const line of differences) {
    console.error(`  ${line}`);
  }
  console.error('\nRun `npm run build` in sdk/typescript and commit the result.');
  process.exit(1);
}
console.log('dist/ matches src/');

/** Every difference between two directory trees, as sentences. */
function compare(committed, fresh) {
  const out = [];
  const left = new Set(listFiles(committed));
  const right = new Set(listFiles(fresh));
  for (const name of [...new Set([...left, ...right])].sort()) {
    if (!left.has(name)) {
      out.push(`missing from dist/: ${name}`);
      continue;
    }
    if (!right.has(name)) {
      out.push(`in dist/ but not produced by the build: ${name}`);
      continue;
    }
    const a = readFileSync(join(committed, name));
    const b = readFileSync(join(fresh, name));
    if (!a.equals(b)) {
      out.push(`differs: ${name} (committed ${a.length} bytes, built ${b.length})`);
    }
  }
  return out;
}

function listFiles(root) {
  if (!existsSync(root)) {
    return [];
  }
  const out = [];
  const walk = (dir) => {
    for (const entry of readdirSync(dir).sort()) {
      const full = join(dir, entry);
      if (statSync(full).isDirectory()) {
        walk(full);
        continue;
      }
      out.push(relative(root, full).split('\\').join('/'));
    }
  };
  walk(root);
  return out;
}
