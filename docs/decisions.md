# Decision log

Short records of non-obvious engineering decisions. Newest first.

## 2026-07-07 — Pairing credentials in the Data Protection keychain (F2)

**Problem.** After a Sparkle update the app silently fell back to the
onboarding window: the pre-F2 login-keychain item's access control was bound
to the app on disk, and the freshly-updated binary was denied without a
prompt. A user who had paired lost their pairing on every update.

**Decision.** Store credentials in the **Data Protection keychain**
(`kSecUseDataProtectionKeychain = true`), `kSecAttrAccessibleAfterFirstUnlock`,
in the app's **default** access group — no explicit `kSecAttrAccessGroup`,
therefore no `keychain-access-groups` entitlement.

**Why no explicit group.** DP items are keyed by the app's code signature,
which is stable across updates of the same identity (Developer ID). An
explicit team access group would require an entitlement that self-signed dev
builds cannot carry; the default group needs none and works on every build.
Access still survives updates because the Developer ID signature is stable.

**Migration.** On first load after upgrade, `CredentialsStore.load` reads the
old login-keychain item (and, older still, the JSON file), re-saves into the
DP keychain, and deletes the source. One-way, idempotent.

**Related.** A dev-era `~/duet/node.yml` pointing at `127.0.0.1` used to
hijack a paired app onto a dead local coordinator; the app now retires it to
`.retired` on launch (only the default path, only when it points local).

## 2026-07-08 — Windows code signing: EV cert, not Azure Trusted Signing (F6)

**Context.** The direct-download Windows channel (.exe/.msix that runs without
a SmartScreen warning) needs a code-signing certificate. Azure Trusted /
Artifact Signing was the cheapest option ($10/mo) but is **available only to
organizations in the USA, Canada, EU & UK**. Relux Works, LLC is Armenian
(C=AM) — ineligible; individual signing is US/Canada only too.

**Decision.** Buy an **EV code-signing certificate with cloud signing**
(SSL.com eSigner, or Certum SimplySign as the budget alternative). EV gives
instant SmartScreen trust from the first download (OV would show a warning
until reputation accrues — unacceptable for a first-time user). Cloud signing
is mandatory post-2023 (EV keys must live on a FIPS HSM; no token in CI).

**Wiring.** CI scaffold is in release.yml (commented sslcom/esigner-codesign
steps for the inner exe and the MSIX). Activation = set the eSigner secrets,
set AppxManifest Publisher to the cert's exact subject DN (MSIX requires
Publisher == signer subject), uncomment the two steps.

**Microsoft Store (Partner Center)** stays the preferred long-term channel
(Microsoft signs, no cert, broader country support) — pursued separately.
EV direct-download is the channel that does not depend on Store review timing.

## 2026-07-08 (update) — Windows: Microsoft Store, not a paid cert

Relux Works, LLC passed Microsoft Store developer verification (email +
business + employment, Armenian entity accepted). The Store signs submitted
MSIX packages itself — no code-signing certificate needed, best UX for
end users (one-click install, no SmartScreen, Store auto-updates). This
SUPERSEDES the EV-cert plan above: SSL.com eSigner (~$1250/yr) is dropped;
the EV scaffold in release.yml stays commented as a fallback only if a
direct-download channel is ever needed. Next: reserve the "Pulsar" app name,
match AppxManifest Publisher/Identity to the Store-assigned values, submit.

## 2026-07-08 — Store submission automation (msstore CLI)

Azure AD app `pulsar-store-ci` (client 30b823e6-…, tenant 19420c32-…) drives
the Microsoft Store submission API from CI. Secrets MSSTORE_TENANT_ID /
MSSTORE_CLIENT_ID / MSSTORE_CLIENT_SECRET set. `.github/workflows/store-submit.yml`
is workflow_dispatch (never auto — beta tags must not publish to the Store).
Remaining before it works: (1) link the app in Partner Center → Account
settings → User management → Azure AD applications as Manager; (2) repo var
STORE_SELLER_ID; (3) account verification complete. The exact msstore submit
subcommand is confirmed on the first real run (blind until the account is live).
