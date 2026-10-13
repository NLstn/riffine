"""Check simulator startup boundaries without an installed iOS runtime."""
import contextlib
import importlib.util
import io
import json
from pathlib import Path
import subprocess
import tempfile
import unittest
from unittest.mock import patch

spec = importlib.util.spec_from_file_location(
    "check_ios_spike", Path(__file__).with_name("check-ios-spike.py"))
checker = importlib.util.module_from_spec(spec)
spec.loader.exec_module(checker)


class SimulatorCheckTests(unittest.TestCase):
    def setUp(self):
        self.directory = tempfile.TemporaryDirectory()
        self.addCleanup(self.directory.cleanup)
        self.output = Path(self.directory.name)
        self.fixture = self.output / "fixture.json"
        self.fixture.write_text("{}")
        for name in ("mobile-runner", "libhost-spike.a"):
            (self.output / name).write_bytes(b"fixture")
        self.calls = []
        self.state = "Shutdown"
        self.probe_failures = []
        self.fixture_error = None
        self.cleanup_error = None
        self.report = {"ok": True, "version": 1}
        self.actual_report = self.report
        for name, value in (("output", self.output), ("fixture", self.fixture)):
            replacement = patch.object(checker, name, value)
            replacement.start()
            self.addCleanup(replacement.stop)
        for name in ("stdout", "stderr"):
            redirect = getattr(contextlib, "redirect_" + name)(io.StringIO())
            redirect.__enter__()
            self.addCleanup(redirect.__exit__, None, None, None)
        replacement = patch.object(checker.subprocess, "run", side_effect=self.command)
        replacement.start()
        self.addCleanup(replacement.stop)

    def command(self, args, **kwargs):
        self.calls.append(tuple(args))
        stdout, stderr = "", ""
        if "list" in args:
            stdout = json.dumps({"devices": {"com.apple.CoreSimulator.SimRuntime.iOS-26-4": [
                {"isAvailable": True, "name": "iPhone test", "udid": "test-udid", "state": self.state}
            ]}})
        elif "/bin/launchctl" in args:
            if self.probe_failures:
                raise self.probe_failures.pop(0)
        elif "spawn" in args:
            if self.fixture_error:
                raise self.fixture_error
            stdout = json.dumps(self.actual_report)
            stderr = '{"iterations":20}\n'
        elif "shutdown" in args:
            if self.cleanup_error:
                raise self.cleanup_error
        elif len(args) == 1:
            stdout = json.dumps(self.report)
        return subprocess.CompletedProcess(args, 0, stdout, stderr)

    def spawns(self):
        return [args for args in self.calls if "spawn" in args]

    def test_cold_device_waits_for_boot_and_spawn_before_fixture(self):
        checker.main()
        actions = [args[2] for args in self.calls if args[0] == "xcrun"]
        self.assertEqual(actions, ["list", "boot", "bootstatus", "spawn", "spawn", "shutdown"])
        self.assertIn("/bin/launchctl", self.spawns()[0])
        self.assertEqual(self.spawns()[1][-1], "20")
        self.assertEqual(json.loads((self.output / "report.json").read_text()), self.report)

    def test_already_booted_device_still_waits_and_is_not_shut_down(self):
        self.state = "Booted"
        checker.main()
        actions = [args[2] for args in self.calls if args[0] == "xcrun"]
        self.assertEqual(actions, ["list", "bootstatus", "spawn", "spawn"])

    def test_only_readiness_probe_is_retried(self):
        self.probe_failures = [subprocess.TimeoutExpired("probe", 60),
                               subprocess.CalledProcessError(1, "probe", stderr="not ready")]
        checker.main()
        self.assertEqual(len(self.spawns()), 4)
        self.assertTrue(all("/bin/launchctl" in args for args in self.spawns()[:3]))

    def test_exhausted_readiness_fails_without_running_fixture(self):
        self.probe_failures = [subprocess.TimeoutExpired("probe", 60) for _ in range(3)]
        with self.assertRaises(subprocess.TimeoutExpired):
            checker.main()
        self.assertEqual(len(self.spawns()), 3)
        self.assertTrue(all("/bin/launchctl" in args for args in self.spawns()))
        self.assertFalse((self.output / "report.json").exists())
        self.assertIn("shutdown", self.calls[-1])

    def test_fixture_timeout_is_not_retried_and_preserves_partial_logs(self):
        self.fixture_error = subprocess.TimeoutExpired(
            "fixture", 60, output=b"partial report", stderr=b"partial diagnostics")
        self.cleanup_error = subprocess.TimeoutExpired("cleanup", 120)
        (self.output / "report.json").write_text("stale report")
        (self.output / "metrics.json").write_text("stale metrics")
        with self.assertRaises(subprocess.TimeoutExpired) as failure:
            checker.main()
        self.assertIs(failure.exception, self.fixture_error)
        self.assertEqual(len(self.spawns()), 2)
        log = (self.output / "simulator.log").read_text()
        self.assertIn("partial report", log)
        self.assertIn("partial diagnostics", log)
        self.assertFalse((self.output / "report.json").exists())
        self.assertFalse((self.output / "metrics.json").exists())

    def test_fixture_nonzero_exit_is_not_retried(self):
        self.fixture_error = subprocess.CalledProcessError(2, "fixture", stderr="fixture failed")
        with self.assertRaises(subprocess.CalledProcessError):
            checker.main()
        self.assertEqual(len(self.spawns()), 2)
        self.assertFalse((self.output / "metrics.json").exists())

    def test_report_mismatch_still_fails(self):
        self.actual_report = {"ok": True, "version": 2}
        with self.assertRaisesRegex(SystemExit, "differs from native Go"):
            checker.main()
        self.assertEqual(len(self.spawns()), 2)
        self.assertFalse((self.output / "report.json").exists())


if __name__ == "__main__":
    unittest.main()
