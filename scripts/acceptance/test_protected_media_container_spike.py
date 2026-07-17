import copy
import importlib.util
import pathlib
import sys
import unittest


HERE = pathlib.Path(__file__).resolve().parent
spec = importlib.util.spec_from_file_location(
    "protected_media_container_spike",
    HERE / "validate_protected_media_container_spike.py",
)
contract = importlib.util.module_from_spec(spec)
assert spec.loader
sys.modules[spec.name] = contract
spec.loader.exec_module(contract)


class ProtectedMediaContainerSpikeTests(unittest.TestCase):
    def setUp(self):
        self.data = contract.load()

    def test_frozen_no_go_contract(self):
        contract.validate(self.data)

    def test_rejects_production_selection_or_invented_review(self):
        for field in (
            "productionContainerSelected",
            "productionCodecSelected",
            "localPreparationToolchainSelected",
            "implementationAuthorized",
            "e2eeFeatureEnabled",
            "productClaimAllowed",
        ):
            changed = copy.deepcopy(self.data)
            changed["decision"][field] = True
            with self.assertRaisesRegex(
                contract.ProtectedMediaContainerSpikeError,
                "unsafe production decision",
            ):
                contract.validate(changed)
        changed = copy.deepcopy(self.data)
        changed["decision"]["independentReview"] = "pass"
        with self.assertRaisesRegex(
            contract.ProtectedMediaContainerSpikeError,
            "invented review",
        ):
            contract.validate(changed)

    def test_rejects_hidden_manual_or_upstream_no_go(self):
        changed = copy.deepcopy(self.data)
        changed["manualAndIndependentEvidence"]["signedMacOSPackage"] = "pass"
        with self.assertRaisesRegex(
            contract.ProtectedMediaContainerSpikeError,
            "invented",
        ):
            contract.validate(changed)

        changed = copy.deepcopy(self.data)
        changed["upstream"]["codecPlayerHandoff"]["production"] = "go"
        with self.assertRaisesRegex(
            contract.ProtectedMediaContainerSpikeError,
            "hides upstream",
        ):
            contract.validate(changed)

    def test_rejects_weakened_nonce_or_overhead_boundary(self):
        changed = copy.deepcopy(self.data)
        changed["format"]["nonceRule"] = "counter"
        with self.assertRaisesRegex(
            contract.ProtectedMediaContainerSpikeError,
            "nonce uniqueness",
        ):
            contract.validate(changed)

        changed = copy.deepcopy(self.data)
        changed["format"]["maximumFourChunkOverheadBytes"] = 4096
        with self.assertRaisesRegex(
            contract.ProtectedMediaContainerSpikeError,
            "overhead bound",
        ):
            contract.validate(changed)

    def test_rejects_closed_blocker_or_accepted_primitive(self):
        changed = copy.deepcopy(self.data)
        changed["blockingFindings"][0]["status"] = "closed"
        with self.assertRaisesRegex(
            contract.ProtectedMediaContainerSpikeError,
            "closed without evidence",
        ):
            contract.validate(changed)

        changed = copy.deepcopy(self.data)
        changed["probe"]["primitiveApprovedForProduction"] = True
        with self.assertRaisesRegex(
            contract.ProtectedMediaContainerSpikeError,
            "unreviewed production",
        ):
            contract.validate(changed)


if __name__ == "__main__":
    unittest.main()
