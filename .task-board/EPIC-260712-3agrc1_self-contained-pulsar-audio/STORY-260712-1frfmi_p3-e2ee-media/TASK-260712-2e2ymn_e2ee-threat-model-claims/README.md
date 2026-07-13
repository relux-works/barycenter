# Freeze the E2EE threat model scope and honest claims

## Description
Define exactly what end-to-end encryption protects and against whom before any primitive or library is selected.

## Scope
Model an honest-but-curious versus malicious coordinator explicitly, storage and traffic capture, backups, moderators, Telegram, compromised or cloned devices, malicious group members, lost devices, cached plaintext or keys and local OS compromise. Cover clips, tracks, saved cues and live PTT separately; identify Spotify and Telegram plaintext boundaries; enumerate metadata such as actors, targets, media type, size, duration and timing. State that deletion or membership revoke cannot erase keys or plaintext already obtained, that recovery without a surviving device or user-held recovery capability may be impossible, and that a volunteered report copy intentionally leaves the E2EE boundary. Freeze device-verification, silent-downgrade and product-claim rules.

## Acceptance Criteria
A reviewable threat model maps assets, attackers, trust assumptions, exclusions, metadata leakage and every C4-C6 case to requirements. It explicitly states whether coordinator equivocation is in scope, which media paths are encrypted, the limits of revoke and deletion, Telegram limitations and key-loss outcomes. No UI or Store copy may say E2EE outside the proven paths or before the separate flag and reviews pass.
