#!/bin/bash
# Creates a stable self-signed code-signing identity "duet-nodeapp" in a
# dedicated keychain (goal DoD-2): every NodeApp.app build signed with it has
# the same designated requirement, so the TCC Automation grant (NodeApp ->
# Airfoil) survives updates. Idempotent; run once per build machine.
#
# The keychain password below is not a secret: it protects only this local
# throwaway signing key, never account credentials.
set -euo pipefail

KEYCHAIN="$HOME/Library/Keychains/duet-signing.keychain-db"
KC_PASS="duet-signing-local"
CN="duet-nodeapp"

if security find-certificate -c "$CN" >/dev/null 2>&1; then
    echo "identity '$CN' already present"
    security find-certificate -c "$CN" -Z | grep "SHA-1" || true
    exit 0
fi

TMP=$(mktemp -d)
trap 'rm -rf "$TMP"' EXIT

cat > "$TMP/ext.cnf" <<EOF
[req]
distinguished_name=dn
x509_extensions=ext
prompt=no
[dn]
CN=$CN
[ext]
keyUsage=critical,digitalSignature
extendedKeyUsage=critical,codeSigning
basicConstraints=critical,CA:false
EOF

openssl req -x509 -newkey rsa:2048 -sha256 -days 3650 -nodes \
    -keyout "$TMP/key.pem" -out "$TMP/cert.pem" -config "$TMP/ext.cnf" 2>/dev/null
# OpenSSL 3.x defaults produce PKCS12 that SecKeychainItemImport rejects;
# -legacy restores the compatible encoding (LibreSSL lacks and needs no flag).
P12_FLAGS=()
if openssl version | grep -q "^OpenSSL 3"; then
    P12_FLAGS=(-legacy)
fi
openssl pkcs12 -export "${P12_FLAGS[@]}" -out "$TMP/identity.p12" \
    -inkey "$TMP/key.pem" -in "$TMP/cert.pem" -passout pass:p12 2>/dev/null

if [[ ! -f "$KEYCHAIN" ]]; then
    security create-keychain -p "$KC_PASS" "$KEYCHAIN"
fi
security set-keychain-settings "$KEYCHAIN"          # never auto-lock
security unlock-keychain -p "$KC_PASS" "$KEYCHAIN"
security import "$TMP/identity.p12" -k "$KEYCHAIN" -P p12 -T /usr/bin/codesign
security set-key-partition-list -S apple-tool:,apple:,codesign: -s -k "$KC_PASS" "$KEYCHAIN" >/dev/null

# Append to the user keychain search list (keeps existing entries).
EXISTING=$(security list-keychains -d user | tr -d '" ' | tr '\n' ' ')
if [[ "$EXISTING" != *duet-signing* ]]; then
    # shellcheck disable=SC2086
    security list-keychains -d user -s $EXISTING "$KEYCHAIN"
fi

echo "identity '$CN' created:"
security find-certificate -c "$CN" -Z | grep "SHA-1"
