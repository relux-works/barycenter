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
