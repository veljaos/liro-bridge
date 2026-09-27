#!/bin/sh
# The soft token signs a PDF on this machine's own libraries, and two
# verifiers that share no code with the signer accept it — F12's exit
# condition "the soft token signs on both, in CI" (open-items F2, D-375).
#
# Run on a clean image after the package is installed, so the libraries the
# binary loads are the distribution's and not a build machine's. The soft
# token is never in a package (SPEC §16.6); the binary this runs is a
# softtoken build made beside the package, with the same toolchain.
#
#   check-softtoken-sign.sh <dir>
#
# <dir> holds liro-bridge-softtoken, gentestkeys, verifypdf and blank.pdf.
# Needs jq and pdfsig (poppler-utils). Touches nothing outside a temporary
# directory: every path the program writes is pointed there first.
set -eu

dir=$1
work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT

export HOME="$work/home"
export XDG_CONFIG_HOME="$HOME/.config" XDG_DATA_HOME="$HOME/.local/share"
export XDG_STATE_HOME="$HOME/.local/state" XDG_CACHE_HOME="$HOME/.cache"
mkdir -p "$HOME"

# The key is made here and dies with $work: a test key is never carried
# between machines, even in CI (F2 §3.3).
"$dir/gentestkeys" "$work/softtoken" >/dev/null
export LIRO_SOFTTOKEN_P12="$work/softtoken/test.p12"
export LIRO_SOFTTOKEN_PASSWORD=liro-softtoken-test

tp=$("$dir/liro-bridge-softtoken" certs --json | jq -r '.certificates[] | select(.isTestKey==true) | .thumbprint')
[ -n "$tp" ] || { echo "FAIL: the soft token's certificate is not listed"; exit 1; }

# B-B with no timestamp authority, said explicitly: no TSA is configured
# here, and asking for more without one is a failure (SPEC §18.11).
"$dir/liro-bridge-softtoken" sign-no-consent --in "$dir/blank.pdf" --out "$work/signed.pdf" \
    --thumbprint "$tp" --force --on-tsa-failure b-b

# pdfsig exits 0 whatever it finds — "Digest Mismatch" included, measured —
# so its verdict is read from its output, never from its status.
pdfsig_valid() {
    pdfsig "$1" 2>/dev/null | grep -q 'Signature Validation: Signature is Valid\.'
}

"$dir/verifypdf" "$work/signed.pdf"
pdfsig "$work/signed.pdf" 2>/dev/null | sed -n 's/^  - //p'
pdfsig_valid "$work/signed.pdf" || { echo "FAIL: pdfsig does not accept the signature"; exit 1; }

# The control, on every run: one byte changed inside the signed range must
# fail both verifiers, or neither of them is checking anything.
cp "$work/signed.pdf" "$work/tampered.pdf"
at=$(grep -abo '/MediaBox' "$work/tampered.pdf" | head -n 1 | cut -d: -f1)
[ -n "$at" ] || { echo "FAIL: nothing to tamper with"; exit 1; }
printf 'm' | dd of="$work/tampered.pdf" bs=1 seek=$((at + 1)) conv=notrunc 2>/dev/null
if cmp -s "$work/signed.pdf" "$work/tampered.pdf"; then
    echo "FAIL: the control did not change the file"
    exit 1
fi
if "$dir/verifypdf" "$work/tampered.pdf" >/dev/null 2>&1; then
    echo "FAIL: verifypdf accepts a tampered document"
    exit 1
fi
if pdfsig_valid "$work/tampered.pdf"; then
    echo "FAIL: pdfsig accepts a tampered document"
    exit 1
fi
echo "OK: signed with the soft token; verifypdf and pdfsig accept it and reject one changed byte"
