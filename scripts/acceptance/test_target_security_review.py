import copy
import importlib.util
import pathlib
import sys
import unittest

HERE = pathlib.Path(__file__).resolve().parent
spec = importlib.util.spec_from_file_location("target_security_review", HERE / "validate_target_security_review.py")
review = importlib.util.module_from_spec(spec)
assert spec.loader
sys.modules[spec.name] = review
spec.loader.exec_module(review)


class TargetSecurityReviewTests(unittest.TestCase):
    def setUp(self):
        self.data = review.load()

    def test_current_review_is_fail_closed(self):
        review.validate(self.data)
        self.assertFalse(self.data["decision"]["productionTargetsAllowed"])
        self.assertFalse(self.data["reviewer"]["independenceSatisfied"])

    def test_rejects_false_independence_or_production(self):
        changed = copy.deepcopy(self.data)
        changed["reviewer"]["independenceSatisfied"] = True
        with self.assertRaisesRegex(review.ReviewError, "cannot claim independence"):
            review.validate(changed)
        changed = copy.deepcopy(self.data)
        changed["decision"]["productionTargetsAllowed"] = True
        with self.assertRaisesRegex(review.ReviewError, "production targets"):
            review.validate(changed)

    def test_rejects_silent_high_closure(self):
        changed = copy.deepcopy(self.data)
        changed["findings"][2]["status"] = "fixed-re-reviewed"
        with self.assertRaisesRegex(review.ReviewError, "silently closed"):
            review.validate(changed)

    def test_rejects_source_drift(self):
        changed = copy.deepcopy(self.data)
        changed["sourceAnchors"][0]["sha256"] = "0" * 64
        with self.assertRaisesRegex(review.ReviewError, "digest mismatch"):
            review.validate(changed)


if __name__ == "__main__":
    unittest.main()
