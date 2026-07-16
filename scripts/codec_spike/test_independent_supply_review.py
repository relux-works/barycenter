import copy
import importlib.util
import pathlib
import sys
import unittest


HERE = pathlib.Path(__file__).resolve().parent


def load_module():
    spec = importlib.util.spec_from_file_location(
        "codec_independent_supply_review", HERE / "validate_independent_supply_review.py"
    )
    module = importlib.util.module_from_spec(spec)
    assert spec.loader
    sys.modules[spec.name] = module
    spec.loader.exec_module(module)
    return module


review_contract = load_module()


class IndependentSupplyReviewTests(unittest.TestCase):
    def setUp(self):
        self.review = review_contract.load()

    def test_current_review_is_a_valid_block_not_an_acceptance(self):
        review_contract.validate(self.review)
        self.assertEqual(self.review["decision"]["result"], "block-phase2")
        self.assertIsNone(self.review["decision"]["acceptedCombination"])
        self.assertFalse(self.review["reviewer"]["independenceSatisfied"])
        self.assertTrue(all(item["status"] == "open-blocking" for item in self.review["findings"]))

    def test_rejects_false_independence_and_approval(self):
        changed = copy.deepcopy(self.review)
        changed["reviewer"]["independenceSatisfied"] = True
        with self.assertRaisesRegex(review_contract.ReviewError, "cannot claim independence"):
            review_contract.validate(changed)

        changed = copy.deepcopy(self.review)
        changed["reviewer"]["approvalStatus"] = "approved"
        with self.assertRaisesRegex(review_contract.ReviewError, "falsely completed"):
            review_contract.validate(changed)

    def test_rejects_false_winner_or_phase2_escape(self):
        changed = copy.deepcopy(self.review)
        changed["decision"]["acceptedCombination"] = "bundled-ffmpeg-both-platforms"
        with self.assertRaisesRegex(review_contract.ReviewError, "cannot select"):
            review_contract.validate(changed)

        changed = copy.deepcopy(self.review)
        changed["nextTaskMayStart"] = True
        with self.assertRaisesRegex(review_contract.ReviewError, "strict sequence"):
            review_contract.validate(changed)

    def test_rejects_silent_high_finding_closure(self):
        changed = copy.deepcopy(self.review)
        changed["findings"][0]["status"] = "closed"
        with self.assertRaisesRegex(review_contract.ReviewError, "unreviewed high finding"):
            review_contract.validate(changed)

    def test_rejects_input_drift(self):
        changed = copy.deepcopy(self.review)
        changed["inputs"][0]["sha256"] = "0" * 64
        with self.assertRaisesRegex(review_contract.ReviewError, "digest mismatch"):
            review_contract.validate(changed)


if __name__ == "__main__":
    unittest.main()
