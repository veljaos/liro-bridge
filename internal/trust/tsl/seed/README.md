# Bundled Trusted List seed

`TSL-RS.xml` is the Republic of Serbia's ETSI TS 119 612 Trusted List,
fetched from `https://www.mit.gov.rs/TrustedList/TSL-RS.xml` (the
Ministry of Information and Telecommunications' own publication
endpoint) on 2026-08-31.

It is sequence 36, issued 2026-05-20T01:00:00Z. Its SHA-256, after
stripping the UTF-8 byte-order mark the server sends before `<?xml`, is:

```
3f5744843bcfaba0698c6b5f7121ee9f133151249808c9de0ebb8e351d9ddb8b
```

This matches the digest specified in F1 §4.7 exactly, confirming this is
the same publication F1 was written against.

Do not hand-edit this file: `internal/trust/tsl.TestSeedDigestMatchesExpected`
checks its digest at build time, and `internal/trust/tsl/c14n` /
`internal/trust/tsl.Verify` are tested against its real XAdES-BES
signature (RSA-SHA512 over exclusive-C14N-canonicalised content). A
hand-edit would break both.

To refresh this seed for a later phase, fetch the URL above, strip the
leading BOM if present, and update `seedSHA256` in `seed.go` together
with this file.
