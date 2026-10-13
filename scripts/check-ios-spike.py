#!/usr/bin/env python3
"""Run actual simulator C/Go conformance; no SDK installs or audio claims."""
import json
import os
from pathlib import Path
import subprocess
import sys
import time

root = Path(__file__).resolve().parent.parent
output = Path(os.environ.get("RIFFINE_SPIKE_OUT", root / "build/host-spike/ios")).resolve()
portable = root / "build/host-spike"
fixture = root / "core/practice/testdata/session-v1.json"

def log(message):
    output.mkdir(parents=True, exist_ok=True)
    with (output / "simulator.log").open("a") as stream:
        stream.write(message + "\n")
    print(message, file=sys.stderr, flush=True)


def text(value):
    if isinstance(value, bytes):
        return value.decode("utf-8", errors="replace")
    return value or ""


def run(*args, timeout=30):
    log(f"Running {args!r} (timeout {timeout}s)")
    started = time.monotonic()
    try:
        result = subprocess.run(args, check=True, text=True, capture_output=True, timeout=timeout)
    except (subprocess.CalledProcessError, subprocess.TimeoutExpired) as error:
        log(f"Failed after {time.monotonic() - started:.3f}s: {error}")
        log(text(error.stdout))
        log(text(error.stderr))
        raise
    log(f"Completed after {time.monotonic() - started:.3f}s")
    log(result.stdout)
    log(result.stderr)
    return result


def wait_for_spawn(udid):
    # Booted/bootstatus do not guarantee that launchd accepts spawn requests on
    # a cold hosted runner. Retry only a harmless readiness probe, never fixtures.
    for attempt in range(3):
        try:
            run("xcrun", "simctl", "spawn", udid, "/bin/launchctl", "version", timeout=60)
            return
        except (subprocess.TimeoutExpired, subprocess.CalledProcessError):
            if attempt == 2:
                raise
            log(f"Simulator spawn not ready; retry {attempt + 1}/2")


def main():
    output.mkdir(parents=True, exist_ok=True)
    (output / "simulator.log").write_text("")
    # Avoid retaining successful reports from an earlier failed local run.
    for name in ("report.json", "metrics.json"):
        (output / name).unlink(missing_ok=True)
    # A cold CI CoreSimulator service can take more than 30 seconds to initialize.
    # Retry inventory only; actual fixture failures are never retried or skipped.
    for attempt in range(3):
        try:
            inventory = json.loads(run("xcrun", "simctl", "list", "devices", "available", "--json", timeout=60).stdout)
            break
        except (subprocess.TimeoutExpired, subprocess.CalledProcessError):
            if attempt == 2:
                raise
            log(f"CoreSimulator inventory not ready; retry {attempt + 1}/2")

    candidates = [(runtime, device) for runtime, devices in inventory["devices"].items()
                  if ".iOS-" in runtime for device in devices
                  if device.get("isAvailable") and "iPhone" in device["name"]]
    if not candidates:
        raise SystemExit("No installed available iPhone simulator; runtime conformance cannot be skipped")
    runtime, device = next(((r, d) for r, d in candidates if d["state"] == "Booted"), candidates[0])
    udid = device["udid"]
    booted_here = device["state"] != "Booted"
    try:
        if booted_here:
            run("xcrun", "simctl", "boot", udid)
        # Also wait for devices already marked Booted: that state can precede
        # completion of first-boot setup and launchd readiness.
        run("xcrun", "simctl", "bootstatus", udid, "-b", timeout=240)
        wait_for_spawn(udid)
        expected = subprocess.run(
            [str(portable / "host-spike")], input=fixture.read_text(), text=True,
            capture_output=True, check=True, timeout=30)
        started = time.perf_counter()
        result = run("xcrun", "simctl", "spawn", udid, str(output / "mobile-runner"), str(fixture), "20", timeout=60)
        actual = json.loads(result.stdout)
        if not actual.get("ok") or actual != json.loads(expected.stdout):
            raise SystemExit("iOS C/Go report differs from native Go")
        (output / "report.json").write_text(result.stdout)
        metrics = {"runtime": runtime, "device": device["name"], "iterations": 20,
                   "spawnAndFixturesMs": round((time.perf_counter() - started) * 1000, 3),
                   "archiveBytes": (output / "libhost-spike.a").stat().st_size,
                   "runnerBytes": (output / "mobile-runner").stat().st_size,
                   "fixture": json.loads(result.stderr.strip().splitlines()[-1])}
        (output / "metrics.json").write_text(json.dumps(metrics, indent=2) + "\n")
        print(json.dumps(metrics, indent=2))
    finally:
        if booted_here:
            # Hosted CoreSimulator shutdown can outlast the default control timeout.
            # Keep cleanup bounded and preserve a preceding conformance failure.
            prior_failure = sys.exc_info()[0] is not None
            try:
                run("xcrun", "simctl", "shutdown", udid, timeout=120)
            except subprocess.SubprocessError:
                if not prior_failure:
                    raise
                log("Simulator cleanup also failed; preserving the fixture failure")


if __name__ == "__main__":
    main()
