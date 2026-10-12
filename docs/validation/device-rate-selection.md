# Shared device-rate selection processing (partial #20/#26)

The reusable C++ worker now composes the existing latency-trimmed stretch/pitch
selection with the pinned miniaudio linear/low-pass resampler. Original-source
frames enter in blocks up to 512; device-rate PCM leaves in blocks up to 8192.
Storage stays fixed independently of selection duration. This same implementation
builds natively and in the browser's fixed 32 MiB WASM processing worker; clients
supply rate parameters and contain no resampling algorithm.

The ABI reports both source and output rates and the resampler's reported integer
output latency, which is trimmed before emitting selected PCM. Output duration is
max(1, round(sourceFrames * outputRate / sourceRate / speed)), matching the shared
transport's device-rate formula. A fixed finite zero tail drains converter latency
and rounding; source samples never repeat and no reads exceed the original range.
Scheduled edge fades apply to device-rate output after conversion. Equal rates
bypass resampling and preserve the existing shared selection PCM exactly.

The browser adapter keeps its original equal-rate API behavior and chooses the
new shared ABI when an output rate differs. It checks rate arguments and safe
worker coordinates before returning results. Device/host negotiation and live
native/browser capability wiring remain separate integration work; this processing
slice does not advertise new live capabilities.

## Evidence

Native and actual WASM worker fixtures cover 48k→44.1k, 44.1k→48k, 8k→192k,
192k→8k and equal rates at 25%/-12, 100%/+3 and 200%/+12 settings. All 15 cases
have exact output/source counts, finite PCM, zero edge values and bounded tone
frequency/RMS differences. WASM repeats them twice without heap growth; native
processing performs no C++ allocations after construction. A downsampled 10 kHz
fixture checks low-pass attenuation below .01 RMS rather than unfiltered aliasing.

Tiny one-source-frame cases exercise conversion/rounding at both rate extremes.
Equal-rate output compares sample-for-sample with the original selection worker.
A huge coordinate range creates/destroys within the fixed WASM heap rather than
allocating duration-sized storage. C-compiled ABI checks reject invalid creation
with cleared ownership. Required native CTests, Linux ASan+UBSan and the native/WASM
workflow include the new fixtures and retain conformance metrics as artifacts.
The WASM package copies the existing pinned miniaudio license with its notices.

The converter uses the already pinned miniaudio revision; no dependency version
changes. Tone/attenuation tests do not establish musical quality. Resampler filter
latency may be fractional at the output rate; the worker uses the library's
reported integer latency and returns the transport's exact requested duration.
Persistent device scheduling/rate negotiation, WASAPI and desktop shell work remain.
