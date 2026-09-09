// Sign a PDF with Liro Bridge, through the TypeScript SDK.
//
//   node sign-with-sdk.mjs ugovor.pdf
//
// This is the same program the other five examples in this directory
// are: read the discovery file, pair, list the certificates, submit,
// watch, collect. The difference is that none of that is written here.
// Compare it against sign.py or sign.go beside it — that is the whole
// argument for the SDK.
//
// Environment, all optional:
//   LIRO_SECRET_STORE   where the pairing is kept (default ./liro-pairing.json)
//   LIRO_ORIGIN         the origin bound at pairing (default "local")
//   LIRO_PAIRING_CODE   the six digits, for an unattended run
//   LIRO_LEVEL          b-b | b-t | b-lt
//   LIRO_THUMBPRINT     which certificate to sign with

import { createInterface } from 'node:readline/promises';
import { readFile, writeFile } from 'node:fs/promises';
import { basename, extname, join } from 'node:path';
import { stdin, stdout } from 'node:process';

import { FileSecretStore, LiroBridge, LiroError } from '@liro/bridge';

const [, , input] = process.argv;
if (!input) {
  console.error('usage: node sign-with-sdk.mjs <document.pdf>');
  process.exit(2);
}

/** Asks the person for the six digits the agent is showing. */
async function askForTheCode({ attempt, attemptsRemaining }) {
  const preset = process.env.LIRO_PAIRING_CODE;
  if (preset) {
    return preset;
  }
  const rl = createInterface({ input: stdin, output: stdout });
  try {
    const suffix = attemptsRemaining === null ? '' : ` (${attemptsRemaining} attempts left)`;
    return await rl.question(`Liro Bridge is showing six digits. Code${suffix}: `);
  } finally {
    rl.close();
  }
}

try {
  const bridge = await LiroBridge.connect({
    applicationName: 'Liro SDK primer',
    origin: process.env.LIRO_ORIGIN ?? 'local',
    secretStore: new FileSecretStore(process.env.LIRO_SECRET_STORE ?? join(process.cwd(), 'liro-pairing.json')),
    onPairingCode: askForTheCode,
  });

  console.log(`agent ${bridge.agentInfo.agentVersion}, protocol ${bridge.agentInfo.protocolVersion}`);

  for (const certificate of await bridge.certificates()) {
    const mark = certificate.isTestKey ? ' [TEST KEY]' : '';
    const state = certificate.usable ? 'usable' : (certificate.notUsableReason ?? 'not usable');
    console.log(`  ${certificate.thumbprint.slice(-8)}  ${certificate.displayName}${mark}  ${state}`);
  }

  bridge.onProgress((p) => {
    if (p.state === 'awaiting_consent') {
      console.log(`  waiting for approval — ${Math.round((p.consentRemainingMs ?? 0) / 1000)} s left`);
    } else if (p.state === 'preparing_card') {
      // Expected, not a stall: about 4 s on a MUP card and 12.7 s on a
      // Pošta one, measured. It is card initialisation.
      console.log('  preparing the card…');
    } else if (p.state === 'signing') {
      console.log(`  signing ${p.completed}/${p.total}${p.etaMs === null ? '' : ` (~${Math.round(p.etaMs / 1000)} s)`}`);
    } else {
      console.log(`  ${p.state}`);
    }
  });

  const [signed] = await bridge.signPdf(
    { name: basename(input), content: await readFile(input) },
    {
      ...(process.env.LIRO_LEVEL ? { level: process.env.LIRO_LEVEL } : {}),
      ...(process.env.LIRO_THUMBPRINT ? { certificateThumbprint: process.env.LIRO_THUMBPRINT } : {}),
    },
  );

  if (signed.content === null) {
    console.error(`not signed: ${signed.failure?.agentCode}`);
    process.exit(1);
  }
  const out = `${input.slice(0, input.length - extname(input).length)}-signed.pdf`;
  await writeFile(out, signed.content);
  console.log(`saved ${out} (${signed.content.length} bytes) at ${signed.achievedLevel}`);
} catch (error) {
  if (error instanceof LiroError) {
    // The code is what to branch on; the message is English and for a
    // developer's log. Show the person something in their own language.
    console.error(`${error.code}: ${error.message}`);
    process.exit(1);
  }
  throw error;
}
