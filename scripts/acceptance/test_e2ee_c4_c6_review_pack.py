import copy
import importlib.util
import pathlib
import sys
import unittest


HERE = pathlib.Path(__file__).resolve().parent
SPEC = importlib.util.spec_from_file_location(
    "e2ee_c4_c6_review_pack", HERE / "validate_e2ee_c4_c6_review_pack.py"
)
MODULE = importlib.util.module_from_spec(SPEC)
assert SPEC.loader
sys.modules[SPEC.name] = MODULE
SPEC.loader.exec_module(MODULE)


class E2EEC4C6ReviewPackTests(unittest.TestCase):
    def setUp(self):
        self.data = MODULE.load()

    def test_authoritative_packet_is_reproducible_and_fail_closed(self):
        MODULE.validate(self.data)

    def test_rejects_source_component_review_or_dependency_drift(self):
        mutations = (
            (lambda value: value["sourceCandidate"].__setitem__("tree", "0" * 40), "source candidate tree"),
            (lambda value: value["components"].pop(), "component packet inventory"),
            (lambda value: value["terminalIndependentReviews"].pop(), "terminal independent review"),
            (lambda value: value["dependencyInventory"]["manifests"][0].__setitem__("sha256", "0" * 64), "dependency manifest drifted"),
        )
        for mutate, expected in mutations:
            changed = copy.deepcopy(self.data)
            mutate(changed)
            with self.assertRaisesRegex(MODULE.E2EEReviewPackError, expected):
                MODULE.validate(changed)

    def test_rejects_false_c4_c5_c6_manual_or_external_claims(self):
        for key in ("c4Accepted", "c5Accepted", "c6Accepted", "externalImplementationReviewSatisfied", "manualEvidenceClaimed"):
            changed = copy.deepcopy(self.data)
            changed["decision"][key] = True
            with self.assertRaisesRegex(MODULE.E2EEReviewPackError, "fail-closed decision"):
                MODULE.validate(changed)
        changed = copy.deepcopy(self.data)
        changed["c4ThroughC6"]["C5"]["storageAndTrafficCapture"] = "pass"
        with self.assertRaisesRegex(MODULE.E2EEReviewPackError, "C5 capture evidence invented"):
            MODULE.validate(changed)
        changed = copy.deepcopy(self.data)
        changed["externalReviewHandoff"]["independenceSatisfied"] = True
        with self.assertRaisesRegex(MODULE.E2EEReviewPackError, "self-certified"):
            MODULE.validate(changed)

    def test_rejects_feature_activation_or_production_crypto_selection(self):
        changed = copy.deepcopy(self.data)
        changed["featureFlagHandoff"]["activationAllowed"] = True
        with self.assertRaisesRegex(MODULE.E2EEReviewPackError, "feature flag handoff"):
            MODULE.validate(changed)
        changed = copy.deepcopy(self.data)
        changed["dependencyInventory"]["productionCryptoProvider"] = "invented-provider"
        with self.assertRaisesRegex(MODULE.E2EEReviewPackError, "production cryptography"):
            MODULE.validate(changed)

    def test_rejects_hidden_manual_pairing_or_residual_risk(self):
        changed = copy.deepcopy(self.data)
        changed["manualHandoff"]["requiredPairings"].pop()
        with self.assertRaisesRegex(MODULE.E2EEReviewPackError, "manual handoff"):
            MODULE.validate(changed)
        changed = copy.deepcopy(self.data)
        changed["residualRisks"].pop()
        with self.assertRaisesRegex(MODULE.E2EEReviewPackError, "residual risk inventory"):
            MODULE.validate(changed)


if __name__ == "__main__":
    unittest.main()
