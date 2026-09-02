# TSA client-certificate credential (not committed)

This directory is for **Pošta Srbije's real test TSA client certificate**
(F3 §6.2's second development endpoint,
`https://test-tsa.ca.posta.rs/timestamp2`), not for anything this
project generates itself.

**This directory is deliberately ignored by git** (see `.gitignore`).
Even a test credential is still a credential; it is not this project's
to redistribute, and — mirroring `testdata/certs/local/` and
`testdata/pdfs/local/` — nothing placed here is ever pushed.

## What to place here

```
testdata/tsa/local/
└── posta-client.p12   # password: 1234 (F3 §6.2)
```

`internal/pades/tsa.TestIntegrationPostaClientCertTSA` reads this file
and calls the real endpoint with it. If the file is absent, that test
skips itself with a clear message — `go test ./...` on a machine without
this credential still passes normally, exercising only the HTTP Basic
auth endpoint (`timestamp1`), which needs no local file at all since its
credentials (`Test.Korisnik` / `123456`) are published directly in
SPEC §12.7.
