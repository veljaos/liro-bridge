#!/bin/sh
# build/linux/sign.sh <dir>
#
# Signs the Linux packages build.sh left in <dir> with the package
# signing key (session 6 §E; D-356), then checks the result with
# verify.sh against the committed public key before anything is handed
# on.
#
#   - the .rpm gets an embedded signature (rpmsign --addsign), which
#     `rpm -K` checks and dnf can;
#   - SHA256SUMS lists the .deb and the signed .rpm, and SHA256SUMS.asc
#     is a detached, armoured signature over it.
#
# The .deb carries no embedded signature, deliberately: `apt install
# ./file.deb` does not check one and debsig-verify needs a policy nobody
# has installed. The signed checksum file is the check.
#
# The key arrives in the environment, never on a command line:
#   LIRO_PACKAGE_SIGNING_KEY         base64 of the armoured secret key
#   LIRO_PACKAGE_SIGNING_PASSPHRASE  its passphrase
# Both unset refuses, and writes nothing — there is no unsigned path.
# The passphrase reaches gpg-agent through a pipe (PRESET_PASSPHRASE),
# so rpmsign's own gpg call needs no argument carrying it.
#
# A key whose fingerprint is not the committed public key's is refused
# before anything is signed. LIRO_PACKAGE_PUBLIC_KEY names a different
# public key; only CI's throwaway run sets it.
set -eu

if [ $# -ne 1 ]; then
    echo "usage: $0 <dir>" >&2
    exit 2
fi
here=$(cd "$(dirname "$0")" && pwd)
root=$(cd "$here/../.." && pwd)
dir=$(cd "$1" && pwd)
pub=${LIRO_PACKAGE_PUBLIC_KEY:-$root/build/linux/liro-bridge-packages.asc}

fail() {
    echo "sign: $*" >&2
    exit 1
}

[ -n "${LIRO_PACKAGE_SIGNING_KEY:-}" ] || fail "LIRO_PACKAGE_SIGNING_KEY is not set"
[ -n "${LIRO_PACKAGE_SIGNING_PASSPHRASE:-}" ] || fail "LIRO_PACKAGE_SIGNING_PASSPHRASE is not set"

set -- "$dir"/*.deb
[ $# -eq 1 ] && [ -f "$1" ] || fail "want exactly one .deb in $dir"
deb=$(basename "$1")
set -- "$dir"/*.rpm
[ $# -eq 1 ] && [ -f "$1" ] || fail "want exactly one .rpm in $dir"
rpm=$(basename "$1")

work=$(mktemp -d)
export GNUPGHOME="$work/gnupg"
trap 'gpgconf --kill gpg-agent 2>/dev/null || true; rm -rf "$work"' EXIT
mkdir -m 700 "$GNUPGHOME"
echo allow-preset-passphrase > "$GNUPGHOME/gpg-agent.conf"
# --pinentry-mode error: a missing or wrong passphrase fails here rather
# than opening a passphrase window on whatever display there is.
g() { gpg --batch --no-tty --pinentry-mode error "$@"; }

want=$(gpg --homedir "$work" --batch --with-colons --show-keys "$pub" 2>/dev/null | awk -F: '/^fpr/{print $10; exit}')
[ -n "$want" ] || fail "cannot read a key from $pub"

printf '%s' "$LIRO_PACKAGE_SIGNING_KEY" | base64 -d 2>/dev/null | g --quiet --import 2>/dev/null ||
    fail "LIRO_PACKAGE_SIGNING_KEY is not base64 of an armoured OpenPGP secret key"
have=$(g --with-colons --list-secret-keys | awk -F: '/^fpr/{print $10; exit}')
[ -n "$have" ] || fail "LIRO_PACKAGE_SIGNING_KEY holds no secret key"
[ "$have" = "$want" ] || fail "LIRO_PACKAGE_SIGNING_KEY is key $have, and the committed public key is $want"
echo "signing with $have"

# One PRESET_PASSPHRASE per keygrip. printf is a shell builtin, so the
# passphrase is in no process's argv.
hex=$(printf '%s' "$LIRO_PACKAGE_SIGNING_PASSPHRASE" | od -An -v -tx1 | tr -d ' \n' | tr a-f A-F)
for grip in $(g --with-colons --with-keygrip --list-secret-keys | awk -F: '/^grp/{print $10}'); do
    printf 'PRESET_PASSPHRASE %s -1 %s\n' "$grip" "$hex" | gpg-connect-agent >/dev/null ||
        fail "gpg-agent refused the passphrase preset"
done
hex=

# Ubuntu's rpm names /usr/bin/gpg2, which 24.04 does not ship; --define
# is what makes rpmsign find gpg rather than failing on a missing file.
rpmsign --define "__gpg $(command -v gpg)" --define "_gpg_name $have" \
    --define "_gpg_sign_cmd_extra_args --batch --pinentry-mode error" \
    --addsign "$dir/$rpm" || fail "rpmsign failed on $rpm"

(cd "$dir" && sha256sum "$deb" "$rpm" > SHA256SUMS)
rm -f "$dir/SHA256SUMS.asc"
g --local-user "$have" --armor --detach-sign --output "$dir/SHA256SUMS.asc" "$dir/SHA256SUMS" ||
    fail "gpg could not sign SHA256SUMS (a wrong passphrase ends here)"

"$here/verify.sh" "$dir"
