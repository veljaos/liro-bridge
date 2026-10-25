#!/bin/sh
# F13 §4: a SoftHSM token on CI's macOS runner, so the PKCS#11 layer —
# Linux's own code, now built for macOS — talks to a real module there:
# one RSA key and its self-signed certificate under one CKA_ID, the shape
# docs/f12-linux-session-5.md gave the Linux token.
#
#   build/macos/ci-softhsm.sh <dir> <pin>
#
# Prints the module's path. Needs Homebrew's softhsm and opensc.
set -eu
dir=$1
pin=$2
mkdir -p "$dir/tokens"
cat > "$dir/softhsm2.conf" <<CONF
directories.tokendir = $dir/tokens
objectstore.backend = file
log.level = INFO
CONF
export SOFTHSM2_CONF="$dir/softhsm2.conf"
softhsm2-util --init-token --free --label liro-ci --pin "$pin" --so-pin 87654321 >&2
module=$(brew --prefix softhsm)/lib/softhsm/libsofthsm2.so
openssl req -x509 -newkey rsa:2048 -nodes -keyout "$dir/key.pem" -out "$dir/cert.pem" \
	-subj "/CN=Liro F13 Test Signer" -days 30 >&2 2>/dev/null
openssl pkcs8 -topk8 -nocrypt -in "$dir/key.pem" -outform DER -out "$dir/key.der"
openssl x509 -in "$dir/cert.pem" -outform DER -out "$dir/cert.der"
pkcs11-tool --module "$module" --login --pin "$pin" --write-object "$dir/key.der" --type privkey --id 01 --label signer >&2
pkcs11-tool --module "$module" --login --pin "$pin" --write-object "$dir/cert.der" --type cert --id 01 --label signer >&2
rm -f "$dir/key.pem" "$dir/key.der"
echo "$module"
