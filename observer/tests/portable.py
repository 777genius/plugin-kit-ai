"""Observer tests supported by an ordinary unprivileged CI runner."""

from __future__ import annotations

import unittest

from observer.tests.classification import load_partition


def load_tests(
    loader: unittest.TestLoader,
    tests: unittest.TestSuite,
    pattern: str | None,
) -> unittest.TestSuite:
    del tests, pattern
    return load_partition(privileged=False, loader=loader)
