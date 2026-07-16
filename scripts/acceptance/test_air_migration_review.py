import copy
import importlib.util
import pathlib
import sys
import unittest


HERE = pathlib.Path(__file__).resolve().parent


def load_module():
    spec = importlib.util.spec_from_file_location(
        "air_migration_review", HERE / "validate_air_migration_review.py"
    )
    module = importlib.util.module_from_spec(spec)
    assert spec.loader
    sys.modules[spec.name] = module
    spec.loader.exec_module(module)
    return module


review_contract = load_module()


class AirMigrationReviewTests(unittest.TestCase):
    def setUp(self):
        self.review = review_contract.load()

    def test_current_review_is_valid_and_fail_closed(self):
        review_contract.validate(self.review)
        self.assertEqual(
            self.review["decision"]["result"],
            "engineering-review-complete-production-blocked",
        )
        self.assertFalse(self.review["decision"]["productionAirAllowed"])
        self.assertFalse(self.review["reviewer"]["independenceSatisfied"])
        self.assertEqual(self.review["findings"][0]["status"], "fixed-re-reviewed")

    def test_rejects_false_independence_or_external_approval(self):
        changed = copy.deepcopy(self.review)
        changed["reviewer"]["independenceSatisfied"] = True
        with self.assertRaisesRegex(review_contract.ReviewError, "cannot claim independence"):
            review_contract.validate(changed)

        changed = copy.deepcopy(self.review)
        changed["reviewer"]["approvalStatus"] = "approved"
        with self.assertRaisesRegex(review_contract.ReviewError, "falsely completed"):
            review_contract.validate(changed)

    def test_rejects_production_or_manual_claim_escape(self):
        changed = copy.deepcopy(self.review)
        changed["decision"]["productionAirAllowed"] = True
        with self.assertRaisesRegex(review_contract.ReviewError, "production Air"):
            review_contract.validate(changed)

        changed = copy.deepcopy(self.review)
        changed["decision"]["manualMigrationClaim"] = True
        with self.assertRaisesRegex(review_contract.ReviewError, "claimed manual migration"):
            review_contract.validate(changed)

    def test_rejects_authority_or_capacity_drift(self):
        changed = copy.deepcopy(self.review)
        changed["frozenContract"]["authorityModes"] = ["airs_authoritative"]
        with self.assertRaisesRegex(review_contract.ReviewError, "authority modes"):
            review_contract.validate(changed)

        changed = copy.deepcopy(self.review)
        changed["frozenContract"]["barycentersPerAir"] = 9
        with self.assertRaisesRegex(review_contract.ReviewError, "capacity"):
            review_contract.validate(changed)

    def test_rejects_silent_high_finding_closure(self):
        changed = copy.deepcopy(self.review)
        changed["findings"][2]["status"] = "fixed-re-reviewed"
        with self.assertRaisesRegex(review_contract.ReviewError, "silently closed"):
            review_contract.validate(changed)

    def test_rejects_source_hash_drift(self):
        changed = copy.deepcopy(self.review)
        changed["sourceAnchors"][0]["sha256"] = "0" * 64
        with self.assertRaisesRegex(review_contract.ReviewError, "digest mismatch"):
            review_contract.validate(changed)


if __name__ == "__main__":
    unittest.main()
