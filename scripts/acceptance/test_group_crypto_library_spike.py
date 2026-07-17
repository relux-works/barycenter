import copy
import importlib.util
import pathlib
import sys
import unittest


HERE = pathlib.Path(__file__).resolve().parent
spec = importlib.util.spec_from_file_location(
    "group_crypto_library_spike",
    HERE / "validate_group_crypto_library_spike.py",
)
contract = importlib.util.module_from_spec(spec)
assert spec.loader
sys.modules[spec.name] = contract
spec.loader.exec_module(contract)


class GroupCryptoLibrarySpikeTests(unittest.TestCase):
    def setUp(self):
        self.data = contract.load()

    def test_frozen_library_no_go(self):
        contract.validate(self.data)

    def test_rejects_library_suite_binding_or_review_selection(self):
        for field in (
            "productionLibrarySelected",
            "productionCipherSuiteSelected",
            "platformBindingsSelected",
            "canonicalSerializationSelected",
            "implementationAuthorized",
            "e2eeFeatureEnabled",
            "productClaimAllowed",
        ):
            changed = copy.deepcopy(self.data)
            changed["decision"][field] = True
            with self.assertRaisesRegex(
                contract.GroupCryptoLibrarySpikeError,
                "unsafe library selection",
            ):
                contract.validate(changed)
        changed = copy.deepcopy(self.data)
        changed["decision"]["independentReview"] = "pass"
        with self.assertRaisesRegex(
            contract.GroupCryptoLibrarySpikeError,
            "invented review",
        ):
            contract.validate(changed)

    def test_rejects_custom_crypto_or_false_product_interop(self):
        changed = copy.deepcopy(self.data)
        changed["protocolAssessment"][1]["status"] = "selected"
        with self.assertRaisesRegex(
            contract.GroupCryptoLibrarySpikeError,
            "custom or sender-key",
        ):
            contract.validate(changed)

        changed = copy.deepcopy(self.data)
        changed["evidenceMatrix"]["windowsToMacOS"] = "pass"
        with self.assertRaisesRegex(
            contract.GroupCryptoLibrarySpikeError,
            "evidence invented",
        ):
            contract.validate(changed)

    def test_rejects_hidden_openmls_or_mls_rs_security_gap(self):
        changed = copy.deepcopy(self.data)
        changed["libraryCandidates"][0]["security"]["remainingAuditFinding"] = "none"
        with self.assertRaisesRegex(
            contract.GroupCryptoLibrarySpikeError,
            "audit status invented",
        ):
            contract.validate(changed)

        changed = copy.deepcopy(self.data)
        changed["libraryCandidates"][1]["security"]["maintainerStatement"] = "fully-audited"
        with self.assertRaisesRegex(
            contract.GroupCryptoLibrarySpikeError,
            "audit gap hidden",
        ):
            contract.validate(changed)

    def test_rejects_self_interop_as_independent_or_swift_test_claim(self):
        changed = copy.deepcopy(self.data)
        changed["libraryCandidates"][1]["platform"]["independentImplementationInterop"] = True
        with self.assertRaisesRegex(
            contract.GroupCryptoLibrarySpikeError,
            "self interop represented",
        ):
            contract.validate(changed)

        changed = copy.deepcopy(self.data)
        changed["libraryCandidates"][1]["bindingSnapshot"]["officialSwiftTests"] = True
        with self.assertRaisesRegex(
            contract.GroupCryptoLibrarySpikeError,
            "binding evidence overstated",
        ):
            contract.validate(changed)

    def test_rejects_closed_blocker_or_e2ee_unblock(self):
        changed = copy.deepcopy(self.data)
        changed["blockingFindings"][0]["status"] = "closed"
        with self.assertRaisesRegex(
            contract.GroupCryptoLibrarySpikeError,
            "severity or status drifted",
        ):
            contract.validate(changed)

        changed = copy.deepcopy(self.data)
        changed["exit"]["e2eeMediaBlocked"] = False
        with self.assertRaisesRegex(
            contract.GroupCryptoLibrarySpikeError,
            "unsafe downstream exit",
        ):
            contract.validate(changed)


if __name__ == "__main__":
    unittest.main()
