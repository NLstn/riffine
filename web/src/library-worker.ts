import { parseBlob } from 'music-metadata';
import { tagCache } from './music-cache';
import type { TrackTags } from './music-cache';

// Bound fingerprint memory per worker. Larger sources still get tags; their
// canonical streaming hash and waveform cache are handled by the practice core.
const MAX_TAG_HASH_BYTES = 64 << 20;
self.onmessage = async (event: MessageEvent<File>) => {
  const file = event.data;
  try {
    let hash: string | undefined;
    if (file.size <= MAX_TAG_HASH_BYTES) {
      try {
        const digest = await crypto.subtle.digest('SHA-256', await file.arrayBuffer());
        hash = Array.from(new Uint8Array(digest), byte => byte.toString(16).padStart(2, '0')).join('');
        const cached = await tagCache(hash);
        if (cached) { self.postMessage({ tags: cached }); return; }
      } catch { /* Storage and fingerprinting are optional for library tags. */ }
    }
    const metadata = await parseBlob(file, { duration: true });
    const picture = metadata.common.picture?.[0];
    const tags: TrackTags = {
      title: metadata.common.title,
      artist: metadata.common.artist,
      album: metadata.common.album,
      albumArtist: metadata.common.albumartist,
      genre: metadata.common.genre?.[0],
      year: metadata.common.year,
      trackNumber: metadata.common.track.no ?? undefined,
      duration: metadata.format.duration,
      format: metadata.format.container,
      artwork: picture && picture.data.byteLength <= (1 << 20) ? new Blob([new Uint8Array(picture.data)], { type: picture.format }) : undefined,
    };
    if (hash) await tagCache(hash, tags).catch(() => {});
    self.postMessage({ tags });
  } catch { self.postMessage({ tagError: true }); }
};
