# Audit exact codec licenses, patents, supply chain and packaging

## Description
Produce a source-cited shipping decision for every exact candidate and version, not a generic library-family opinion.

## Scope
For Media Foundation and native macOS frameworks plus every pure-Go and bundled candidate, record exact source, commit or version, license text, transitive dependencies, redistribution and source or notice obligations, dynamic-link constraints, patent or commercial exposure, supported architectures, update and CVE policy, SBOM entries, Store/AppContainer, codesign and notarization impact and any first-run download or sandbox requirement. Use current authoritative vendor and license sources and identify where qualified legal advice is still required.

## Acceptance Criteria
Each exact candidate is classified shippable, shippable with enumerated obligations or rejected, with URLs, retrieval date, packaged binary shape, notices and unresolved legal decision. No unknown transitive license, downloaded executable, GPL-incompatible distribution assumption, unpatched binary or vague patent claim reaches the ADR.
