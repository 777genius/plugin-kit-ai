"""Deterministic observer test capability classification."""

from __future__ import annotations

import unittest
from collections.abc import Callable, Iterator
from importlib import import_module
from pathlib import Path
from typing import TypeVar


_DISPOSABLE_HOST_ATTRIBUTE = "_requires_disposable_observer_host"
_T = TypeVar("_T", bound=Callable[..., object] | type)


def requires_disposable_observer_host(test: _T) -> _T:
    """Mark a test or test case as requiring the privileged observer host gate."""
    setattr(test, _DISPOSABLE_HOST_ATTRIBUTE, True)
    return test


def _test_cases(suite: unittest.TestSuite) -> Iterator[unittest.TestCase]:
    for test in suite:
        if isinstance(test, unittest.TestSuite):
            yield from _test_cases(test)
        else:
            assert isinstance(test, unittest.TestCase)
            yield test


def _requires_disposable_host(test: unittest.TestCase) -> bool:
    method = getattr(test, test._testMethodName)
    return bool(
        getattr(type(test), _DISPOSABLE_HOST_ATTRIBUTE, False)
        or getattr(method, _DISPOSABLE_HOST_ATTRIBUTE, False)
    )


def observer_test_module_names() -> tuple[str, ...]:
    """Return every concrete observer test module in deterministic order."""
    tests_directory = Path(__file__).parent
    return tuple(
        f"observer.tests.{path.stem}"
        for path in sorted(tests_directory.glob("test_*.py"))
        if path.is_file()
    )


def _load_all_tests(loader: unittest.TestLoader) -> unittest.TestSuite:
    suites = (
        loader.loadTestsFromModule(import_module(module_name))
        for module_name in observer_test_module_names()
    )
    return unittest.TestSuite(suites)


def load_partition(
    *,
    privileged: bool,
    loader: unittest.TestLoader | None = None,
) -> unittest.TestSuite:
    """Load exactly one side of the portable/disposable-host partition."""
    discovered = _load_all_tests(loader or unittest.TestLoader())
    selected = (
        test for test in _test_cases(discovered)
        if _requires_disposable_host(test) is privileged
    )
    return unittest.TestSuite(selected)
