#!/bin/sh
# build/linux/verify.sh <dir>
#
# Checks the signed Linux packages in <dir> exactly as a person is told
# to in the README, against the committed public key and nothing else
# (session 6 §E; D-356). It is the Linux form of scripts/verifyrelease:
# sign.sh runs it before anything is handed on, and CI runs it with a
# key that must be refused.
#
#   - SHA256SUMS.asc is a good signature over SHA256SUMS by the key in
#     build/linux/liro-bridge-packages.asc, in a keyring holding only
#     that key;
#   - SHA256SUMS names exactly the .deb and .rpm in <dir>, and each one
#     matches;
#   - the .rpm carries an embedded signature that rpm accepts, in an rpm
#     database holding only that key. "digests OK" without "signatures"
#     is an unsigned package and is refused.
#
# LIRO_PACKAGE_PUBLIC_KEY names a different public key. CI uses it to
# run the whole path with a throwaway key; a release never sets it.
set -eu

if [ $# -ne 1 ]; then
    echo "usage: $0 <dir>" >&2
    exit 2
fi
root=$(cd "$(dirname "$0")/../.." && pwd)
dir=$(cd "$1" && pwd)
pub=${LIRO_PACKAGE_PUBLIC_KEY:-$root/build/linux/liro-bridge-packages.asc}

fail() {
    echo "verify: $*" >&2
    exit 1
}

set -- "$dir"/*.deb
[ $# -eq 1 ] && [ -f "$1" ] || fail "want exactly one .deb in $dir"
deb=$(basename "$1")
set -- "$dir"/*.rpm
[ $# -eq 1 ] && [ -f "$1" ] || fail "want exactly one .rpm in $dir"
rpm=$(basename "$1")
[ -f "$dir/SHA256SUMS" ] || fail "no SHA256SUMS in $dir"
[ -f "$dir/SHA256SUMS.asc" ] || fail "no SHA256SUMS.asc in $dir"

work=$(mktemp -d)
trap 'gpgconf --homedir "$work/gnupg" --kill gpg-agent 2>/dev/null || true; rm -rf "$work"' EXIT
mkdir -m 700 "$work/gnupg"
g() { gpg --homedir "$work/gnupg" --batch --no-tty "$@"; }

g --quiet --import "$pub" 2>/dev/null || fail "cannot import $pub"
fpr=$(g --with-colons --list-keys | awk -F: '/^fpr/{print $10}')
[ "$(printf '%s\n' "$fpr" | wc -l)" -eq 1 ] || fail "$pub holds more than one key"
[ -z "$(g --with-colons --list-secret-keys)" ] || fail "$pub holds secret key material"
echo "key: $fpr"

# VALIDSIG's last field is the primary key's fingerprint, whichever
# subkey signed.
status=$(g --status-fd 1 --verify "$dir/SHA256SUMS.asc" "$dir/SHA256SUMS" 2>/dev/null) ||
    fail "SHA256SUMS.asc is not a good signature over SHA256SUMS by $fpr"
signer=$(printf '%s\n' "$status" | awk '$2 == "VALIDSIG" {print $NF}')
[ "$signer" = "$fpr" ] || fail "SHA256SUMS is signed by '$signer', not $fpr"
echo "SHA256SUMS: good signature by $fpr"

listed=$(awk '{sub(/^\*/, "", $2); print $2}' "$dir/SHA256SUMS" | sort)
[ "$listed" = "$(printf '%s\n%s\n' "$deb" "$rpm" | sort)" ] ||
    fail "SHA256SUMS lists [$(echo $listed)], not exactly $deb and $rpm"
(cd "$dir" && sha256sum --check --strict SHA256SUMS) || fail "a package does not match SHA256SUMS"

mkdir "$work/rpmdb"
rpmkeys --dbpath "$work/rpmdb" --import "$pub" || fail "rpm cannot import $pub"
k=$(rpmkeys --dbpath "$work/rpmdb" --checksig "$dir/$rpm") || fail "rpm refuses $rpm: $k"
echo "$k"
case "$k" in
    *": digests signatures OK") ;;
    *) fail "$rpm carries no signature rpm accepts" ;;
esac
echo "verified"
