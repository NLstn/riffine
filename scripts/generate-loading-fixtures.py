#!/usr/bin/env python3
"""Original stereo signals for loading profiles; requires PyAV and NumPy.
Outputs stay in ignored build/import-profile, never in production web assets.
"""
from fractions import Fraction
from pathlib import Path
import argparse
import json
import av
import numpy as np

parser = argparse.ArgumentParser()
parser.add_argument('--seconds', nargs='+', type=int, default=[30, 180])
args = parser.parse_args()
root = Path(__file__).resolve().parents[1] / 'build' / 'import-profile'
root.mkdir(parents=True, exist_ok=True)
rate = 48000
manifest = []
formats = [('wav', 'pcm_s16le', 'wav'), ('mp3', 'libmp3lame', 'mp3'),
           ('flac', 'flac', 'flac'), ('m4a', 'aac', 'mp4'), ('opus', 'libopus', 'ogg')]
for seconds in args.seconds:
    for suffix, encoder, container in formats:
        path = root / f'tone-{seconds}s.{suffix}'
        with av.open(str(path), mode='w', format=container) as output:
            output.metadata['title'] = f'Riffine generated loading fixture ({seconds}s)'
            stream = output.add_stream(encoder, rate=rate)
            stream.layout = 'stereo'
            if suffix in ('mp3', 'm4a', 'opus'):
                stream.bit_rate = 192000
            for offset in range(0, seconds * rate, 8192):
                count = min(8192, seconds * rate - offset)
                t = (np.arange(count) + offset) / rate
                gain = .16 * (.6 + .4 * np.sin(2 * np.pi * .7 * t) ** 2)
                samples = np.empty((1, count * 2), dtype=np.int16)
                samples[0, 0::2] = np.rint(32767 * gain * np.sin(2 * np.pi * 440 * t)).astype(np.int16)
                samples[0, 1::2] = np.rint(32767 * gain * np.sin(2 * np.pi * 660 * t)).astype(np.int16)
                frame = av.AudioFrame.from_ndarray(samples, format='s16', layout='stereo')
                frame.sample_rate = rate
                frame.time_base = Fraction(1, rate)
                frame.pts = offset
                for packet in stream.encode(frame):
                    output.mux(packet)
            for packet in stream.encode(None):
                output.mux(packet)
        manifest.append({'name': path.name, 'seconds': seconds, 'format': suffix,
                         'bytes': path.stat().st_size, 'sampleRate': rate, 'channels': 2})
        print(f'{path.name}: {path.stat().st_size} bytes', flush=True)
(root / 'manifest.json').write_text(json.dumps({'pyav': av.__version__, 'fixtures': manifest}, indent=2) + '\n')
