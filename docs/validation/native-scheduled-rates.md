# Native scheduled device-rate negotiation (partial #20)

Native scheduled workers now configure a device output rate before their first
generation. Source metadata and decoder seeks remain in original-source frames;
output bounds, fades and produced PCM use the negotiated device rate. Resets
retain that rate, and attempts to change it after a generation reject without
altering pending state. Device changes will require a fresh verified worker.

Every slot uses the shared rate-aware selection port. Equal rates bypass rate
conversion and preserve existing PCM; differing rates use the tested shared
converter. Expected output follows the Go transport formula, including its
minimum one-device-frame result. C ABI setters/getters expose the rate without
changing the original source-info struct or accepting hardware acknowledgements.

Persistent control protocol v2 reports output rate separately from source rate
and supports negotiation before reset. Requests/responses remain 128/640 bytes,
with exact uint64 IDs, bounded metadata/errors and no PCM. The helper advertises
`scheduledControl: 2`; adapters reject incompatible workers with a rebuild message
while retaining their valid decoder/preview/render capabilities. Go exposes
OpenScheduledWorkerRate, with the original opening API defaulting to source rate.

## Evidence

The actual native scheduled-worker fixture compares output sample-for-sample with
the standalone shared converter for WAV, FLAC and MP3 at 8 kHz, 44.1 kHz and
192 kHz outputs, across 25%/-12, 75%/+3 and 200%/+12 profiles. It checks exact
schedule generation/sequence/block coordinates, original caller cursor ownership,
rate retention through reset and rejection of changes after activation. Existing
equal-rate two-slot, 100-reset and borrowed-source regressions remain.

Mandatory Go/native tests exercise v2 handshake/rate negotiation, device-frame
schedules, post-generation rejection and reset retention. Existing malformed
handshake/cancellation/Close fixtures remain, with the separate rate checked in
every successful response. Full native/Go tests/vet/gofmt, native race tests,
Windows compilation and local ASan+UBSan cover the changed worker and actual IPC.
Required native/sanitizer CI retains these tests.

This worker still reports processing metadata and cannot acknowledge device
presentation. Integrating the producer, CoreAudio queue/clock and Go live factory
is the next device slice; WASAPI and the desktop shell remain outstanding.
