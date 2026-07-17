import copy
import importlib.util
import pathlib
import sys
import unittest


HERE = pathlib.Path(__file__).resolve().parent
spec = importlib.util.spec_from_file_location(
    "e2ee_threat_model", HERE / "validate_e2ee_threat_model.py"
)
contract = importlib.util.module_from_spec(spec)
assert spec.loader
sys.modules[spec.name] = contract
spec.loader.exec_module(contract)


class E2EEThreatModelTests(unittest.TestCase):
    def setUp(self):
        self.data = contract.load()

    def test_frozen_complete_contract(self):
        contract.validate(self.data)

    def test_rejects_implementation_claim_or_invented_review(self):
        for key in ("implementationAuthorized", "e2eeFeatureEnabled", "productClaimAllowed"):
            changed = copy.deepcopy(self.data)
            changed["decision"][key] = True
            with self.assertRaisesRegex(contract.E2EEThreatModelError, "unsafe E2EE decision"):
                contract.validate(changed)
        changed = copy.deepcopy(self.data)
        changed["decision"]["independentReview"] = "pass"
        with self.assertRaisesRegex(contract.E2EEThreatModelError, "invented review"):
            contract.validate(changed)

    def test_rejects_trusted_coordinator_or_missing_identity_equivocation(self):
        changed = copy.deepcopy(self.data)
        next(
            role for role in changed["trustRoles"]
            if role["id"] == "coordinator-delivery-service"
        )["trust"] = "trusted"
        with self.assertRaisesRegex(contract.E2EEThreatModelError, "trusted with content"):
            contract.validate(changed)

        changed = copy.deepcopy(self.data)
        changed["attackerClasses"] = [
            item for item in changed["attackerClasses"]
            if item["id"] != "malicious-identity-coordinator"
        ]
        with self.assertRaisesRegex(contract.E2EEThreatModelError, "attacker inventory"):
            contract.validate(changed)

    def test_rejects_silent_downgrade_or_false_current_path(self):
        changed = copy.deepcopy(self.data)
        next(
            path for path in changed["mediaPaths"]
            if path["id"] == "legacy-or-mixed-version-target"
        )["targetState"] = "plaintext-fallback"
        with self.assertRaisesRegex(contract.E2EEThreatModelError, "media path state drifted"):
            contract.validate(changed)

        changed = copy.deepcopy(self.data)
        next(path for path in changed["mediaPaths"] if path["id"] == "clip")[
            "currentState"
        ] = "protected"
        with self.assertRaisesRegex(contract.E2EEThreatModelError, "media path state drifted"):
            contract.validate(changed)

    def test_rejects_hidden_metadata_or_missing_c4_c6(self):
        changed = copy.deepcopy(self.data)
        changed["metadataDisclosure"]["coordinatorVisible"].remove(
            "ciphertext-size-chunk-count-and-declared-duration"
        )
        with self.assertRaisesRegex(contract.E2EEThreatModelError, "metadata inventory"):
            contract.validate(changed)

        changed = copy.deepcopy(self.data)
        del changed["acceptanceScenarios"]["C6"]
        with self.assertRaisesRegex(contract.E2EEThreatModelError, "C4-C6"):
            contract.validate(changed)

    def test_rejects_secondary_or_misrepresented_source(self):
        changed = copy.deepcopy(self.data)
        changed["sources"][0]["url"] = "https://example.com/mls-summary"
        with self.assertRaisesRegex(contract.E2EEThreatModelError, "non-primary source"):
            contract.validate(changed)

        changed = copy.deepcopy(self.data)
        next(
            source for source in changed["sources"]
            if source["id"] == "NIST-SP-800-154-IPD"
        )["status"] = "NIST final standard"
        with self.assertRaisesRegex(contract.E2EEThreatModelError, "represented as final"):
            contract.validate(changed)


if __name__ == "__main__":
    unittest.main()
