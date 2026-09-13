// Demo A — the application decides everything.
//
//   node sdk/examples/demo-a-application-decides.mjs
//   node sdk/examples/demo-a-application-decides.mjs --fresh
//   node sdk/examples/demo-a-application-decides.mjs a.pdf b.pdf
//
// This is the shape an ERP asks for when it already knows the answer to
// every question the agent could put to a person: which documents, which
// certificate, which level, and how the signature should look. All four
// go in the request.
//
// What that buys, and the only thing it buys: the person sees the
// approval and nothing else. It does NOT buy signing without a person.
// There is no flag in this SDK or in the protocol that skips the consent
// window, and the request below is the most fully-specified one it is
// possible to send.
//
// Run demo-b-person-places-stamp.mjs after this one. It is this same
// file with `stamp` removed, and nothing else changed.

import {
  LEVEL,
  chooseCertificate,
  connectAsDemo,
  heading,
  loadDocuments,
  narrateProgress,
  note,
  parseArgs,
  reportFailure,
  saveResults,
} from './demo-protocol-common.mjs';

const { fresh, paths } = parseArgs(process.argv);

try {
  heading('Demo A — the application decides everything');
  note('Three documents, one named certificate, one named level, and the');
  note('stamp position named too. You will be asked exactly one question.');

  const bridge = await connectAsDemo({
    applicationName: 'Liro Business (demo A)',
    storeFile: 'pairing-a.json',
    fresh,
  });
  note(`\n  agent ${bridge.agentInfo.agentVersion}, protocol ${bridge.agentInfo.protocolVersion}`);

  const certificate = await chooseCertificate(bridge);
  const documents = await loadDocuments(paths, 3);

  heading('Submitting.');
  note(`${documents.length} documents: ${documents.map((d) => d.name).join(', ')}`);
  note('The approval window is about to open. There will be no step header,');
  note('because with the method question answered there is only one step.');

  narrateProgress(bridge);

  const signed = await bridge.signPdf(documents, {
    // Every one of these four is the application answering a question
    // the person would otherwise be asked.
    certificateThumbprint: certificate.thumbprint,
    level: LEVEL,
    stamp: { visible: true, position: 'bottom-right' },
  });

  await saveResults(signed, 'demo-a');
} catch (error) {
  reportFailure(error);
}
