#!/usr/bin/env python3
"""Original generated signals only. Development recipe: PyAV 16.1.0 + NumPy.
Encoders are test tools, not Riffine runtime dependencies. The committed fixtures
are original stereo 440/660 Hz tones with gated attacks, not licensed recordings.
"""
from fractions import Fraction
from pathlib import Path
import hashlib
import json
import math
import av
import numpy as np

root = Path(__file__).resolve().parents[1] / "native/testdata"
root.mkdir(parents=True, exist_ok=True)
rate, frames = 48000, 48000
samples = np.empty((1, frames * 2), dtype=np.int16)
for i in range(frames):
    gain = .2 * min(1, i / 240, (frames - 1 - i) / 240)
    for c, hz in enumerate((440, 660)):
        samples[0, i * 2 + c] = round(gain * 32767 * math.sin(2 * math.pi * hz * i / rate))
manifest = {"generator": "scripts/generate-codec-fixtures.py", "pyav": av.__version__, "sourceFrames": frames, "sampleRate": rate, "channels": 2, "files": []}
for suffix, encoder in (("flac", "flac"), ("mp3", "libmp3lame")):
    path = root / f"generated-stereo.{suffix}"
    with av.open(str(path), mode="w", format=suffix) as output:
        output.metadata["title"] = "Riffine generated stereo fixture"
        stream = output.add_stream(encoder, rate=rate)
        stream.layout = "stereo"
        if suffix == "mp3":
            stream.bit_rate = 192000
        frame = av.AudioFrame.from_ndarray(samples, format="s16", layout="stereo")
        frame.sample_rate = rate
        frame.time_base = Fraction(1, rate)
        frame.pts = 0
        for packet in stream.encode(frame):
            output.mux(packet)
        for packet in stream.encode(None):
            output.mux(packet)
    manifest["files"].append({"name": path.name, "bytes": path.stat().st_size, "sha256": hashlib.sha256(path.read_bytes()).hexdigest()})
(root / "generated-codecs.json").write_text(json.dumps(manifest, indent=2) + "\n")
