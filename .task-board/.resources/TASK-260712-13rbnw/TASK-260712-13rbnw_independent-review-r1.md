# TASK-260712-13rbnw — frozen independent review R1

Date: 2026-07-14  
Role: same-executor cold review required by strict inline workflow  
Reviewed implementation commit: `f5b73f06a9e06f71c6193d982e6138e5bec68247`

## Scope and method

Reviewed the complete `origin/main...f5b73f0` diff, every changed production/script/test file in full, the rendered manifest and downloaded signed artifact, the task guard, Rev16 packaging constraints, accepted lifecycle boundary, and official Microsoft MSIX signing/trust/identity contracts. Review focused on least privilege, certificate handling, untrusted package parsing, install rollback, artifact truthfulness, and separation of hosted checks from real hardware evidence.

## Findings closed during review

1. `PACKAGE_ID` initially used the wrong native field order, so PFN derivation failed. Corrected to the documented layout and AMD64 architecture; Windows CI now derives the accepted `q036g2bzd7ngc` publisher ID.
2. Code Signing EKU inspection initially treated PowerShell's adapted `ObjectId` as a nested OID object. Corrected to the actual adapter value; certificate validation now passes and rejects missing EKU.
3. Trust was initially changed before the archive manifest was checked. The installer now preflights the manifest from the MSIX and freezes root/application declarations, target family, executable, trust/runtime, and capabilities before importing public trust.
4. The first CI signer validity was shorter than artifact retention, and launch observation was a fixed two seconds. Validity is now 30 days for a 14-day artifact, and launch observation is bounded at 15 seconds.
5. Untrusted archive parsing initially used ordinary XML conversion without an explicit size/DTD boundary. It now caps the manifest at 128 KiB, prohibits DTDs and external resolution, and disposes archive/XML handles on every path.
6. An install failure after partial registration could leave the package behind, and build/install receipts did not bind the exact bytes. Failure now re-queries and removes every task-family registration, and SHA-256 is checked across preflight, signature validation, build metadata, and install receipt.

## Final review result

No unresolved correctness, security, capability, signing, documentation, or evidence-truthfulness finding remains in the reviewed scope. No certificate export or private material is present. The self-signed route is correctly limited to controlled testing; Store signing and real hardware remain explicitly external.

Final exact-hash CI run `29292631211` is green and the root audit found no later production drift.

Verdict: **PASS**.
