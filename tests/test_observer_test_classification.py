from __future__ import annotations

import importlib
import unittest
from collections import Counter
from pathlib import Path

from observer.tests import classification


def test_cases(suite: unittest.TestSuite) -> list[unittest.TestCase]:
    cases: list[unittest.TestCase] = []
    for test in suite:
        if isinstance(test, unittest.TestSuite):
            cases.extend(test_cases(test))
        else:
            if not isinstance(test, unittest.TestCase):
                raise TypeError(f"unexpected test object: {test!r}")
            cases.append(test)
    return cases


class ObserverTestClassificationTests(unittest.TestCase):
    def test_module_discovery_is_sorted_and_exhaustive(self) -> None:
        tests_directory = Path(classification.__file__).parent
        expected = tuple(
            f"observer.tests.{path.stem}"
            for path in sorted(tests_directory.glob("test_*.py"))
            if path.is_file()
        )

        self.assertEqual(classification.observer_test_module_names(), expected)
        self.assertEqual(expected, tuple(sorted(expected)))

    def test_partitions_are_unique_disjoint_and_exhaustive(self) -> None:
        loader = unittest.TestLoader()
        directly_discovered = unittest.TestSuite(
            loader.loadTestsFromModule(importlib.import_module(module_name))
            for module_name in classification.observer_test_module_names()
        )
        all_ids = [test.id() for test in test_cases(directly_discovered)]
        portable_ids = [
            test.id()
            for test in test_cases(classification.load_partition(privileged=False))
        ]
        privileged_ids = [
            test.id()
            for test in test_cases(classification.load_partition(privileged=True))
        ]

        self.assertEqual(len(all_ids), len(set(all_ids)))
        self.assertEqual(len(portable_ids), len(set(portable_ids)))
        self.assertEqual(len(privileged_ids), len(set(privileged_ids)))
        self.assertTrue(set(portable_ids).isdisjoint(privileged_ids))
        self.assertEqual(Counter(portable_ids + privileged_ids), Counter(all_ids))
        self.assertTrue(any(
            test_id.startswith("observer.tests.test_egress_proxy.")
            for test_id in portable_ids
        ))


if __name__ == "__main__":
    unittest.main()
