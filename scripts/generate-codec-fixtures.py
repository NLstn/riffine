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
for suffix, encoder, container in (("flac", "flac", "flac"), ("mp3", "libmp3lame", "mp3"),
                                    ("m4a", "aac", "mp4"), ("alac.m4a", "alac", "mp4"),
                                    ("opus", "libopus", "ogg")):
    path = root / f"generated-stereo.{suffix}"
    with av.open(str(path), mode="w", format=container) as output:
        output.metadata["title"] = "Riffine generated stereo fixture"
        stream = output.add_stream(encoder, rate=rate)
        stream.layout = "stereo"
        if encoder in ("libmp3lame", "aac", "libopus"):
            stream.bit_rate = 192000
        frame = av.AudioFrame.from_ndarray(samples, format="s16", layout="stereo")
        frame.sample_rate = rate
        frame.time_base = Fraction(1, rate)
        frame.pts = 0
        for packet in stream.encode(frame):
            output.mux(packet)
        for packet in stream.encode(None):
            output.mux(packet)
    record = {"name": path.name, "bytes": path.stat().st_size, "sha256": hashlib.sha256(path.read_bytes()).hexdigest()}
    if suffix in ("m4a", "alac.m4a", "opus"):
        # Independent FFmpeg reference samples; AAC's MP4 end clips its final
        # padded frame. Opus applies pre-skip and the final granule itself.
        with av.open(str(path)) as source:
            blocks = []
            for frame in source.decode(audio=0):
                data = frame.to_ndarray().astype(np.float32)
                if frame.format.name == "s16p":
                    data /= 32768
                elif frame.format.name == "s32p":
                    data /= 2147483648
                blocks.append(data)
            decoded = np.concatenate(blocks, axis=1)
            record["audio"] = {"encoding": "opus" if encoder == "libopus" else encoder, "sampleRate": rate, "channels": 2, "frames": frames}
            positions = [0, 1, 255, 256, 1023, 1024, 12345, 17999, 18000, 45055, 45056, 47999]
            record["referenceSamples"] = [{"frame": i, "channels": [float(decoded[c, i]) for c in range(2)]} for i in positions]
    manifest["files"].append(record)
(root / "generated-codecs.json").write_text(json.dumps(manifest, indent=2) + "\n")
