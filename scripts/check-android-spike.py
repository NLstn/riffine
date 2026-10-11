#!/usr/bin/env python3
"""Execute the C-to-Go control fixture on a connected Android target."""
import json
import os
from pathlib import Path
import subprocess
import sys
import time
import uuid

root = Path(__file__).resolve().parent.parent
output = Path(os.environ.get("RIFFINE_SPIKE_OUT", root / "build/host-spike/android-amd64")).resolve()
fixture = root / "core/practice/testdata/session-v1.json"
sdk = os.environ.get("ANDROID_HOME") or os.environ.get("ANDROID_SDK_ROOT")
adb = os.environ.get("RIFFINE_ADB") or (str(Path(sdk) / "platform-tools/adb") if sdk else "adb")
serial = os.environ.get("RIFFINE_ANDROID_SERIAL", "emulator-5554")
remote = "/data/local/tmp/riffine-host-spike-" + uuid.uuid4().hex

def command(*args, timeout=30):
    try:
        return subprocess.run(args, text=True, capture_output=True, check=True, timeout=timeout)
    except subprocess.CalledProcessError as error:
        print(error.stdout[-4096:], file=sys.stderr)
        print(error.stderr[-4096:], file=sys.stderr)
        raise

def target(*args, timeout=30):
    return command(adb, "-s", serial, *args, timeout=timeout)

target("wait-for-device", timeout=120)
deadline = time.monotonic() + 300
while target("shell", "getprop", "sys.boot_completed").stdout.strip() != "1":
    if time.monotonic() >= deadline:
        raise SystemExit("Android runtime did not finish booting")
    time.sleep(2)
expected = subprocess.run([str(root / "build/host-spike/host-spike")],
                          input=fixture.read_text(), text=True,
                          capture_output=True, check=True, timeout=30)
target("shell", "mkdir", remote)
try:
    target("push", str(output / "libhost-spike.so"), str(output / "mobile-runner"), str(fixture), remote + "/")
    target("shell", "chmod", "700", remote + "/mobile-runner")
    started = time.perf_counter()
    result = target("shell", "LD_LIBRARY_PATH=" + remote, remote + "/mobile-runner",
                    remote + "/session-v1.json", "20", timeout=60)
    actual = json.loads(result.stdout)
    if not actual.get("ok") or actual != json.loads(expected.stdout):
        raise SystemExit("Android C/Go report differs from native Go")
    (output / "report.json").write_text(result.stdout)
    metrics = {"abi": target("shell", "getprop", "ro.product.cpu.abi").stdout.strip(),
               "api": target("shell", "getprop", "ro.build.version.sdk").stdout.strip(),
               "iterations": 20, "spawnAndFixturesMs": round((time.perf_counter() - started) * 1000, 3),
               "libraryBytes": (output / "libhost-spike.so").stat().st_size,
               "runnerBytes": (output / "mobile-runner").stat().st_size,
               "fixture": json.loads(result.stderr.strip().splitlines()[-1])}
    (output / "metrics.json").write_text(json.dumps(metrics, indent=2) + "\n")
    print(json.dumps(metrics, indent=2))
finally:
    target("shell", "rm", "-rf", remote)
