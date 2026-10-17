import type { TrackTags } from './music-cache';

export interface MusicTrack {
  id: string;
  file: File;
  relativePath: string;
  title: string;
  artist: string;
  album: string;
  albumArtist: string;
  genre?: string;
  year?: number;
  trackNumber?: number;
  duration?: number;
  format: string;
  size: number;
  artworkUrl?: string;
  tagError?: boolean;
}

export interface FolderSelection {
  name: string;
  files: File[];
}

const audioExtensions = new Set([
  'aac', 'aif', 'aiff', 'ape', 'flac', 'm4a', 'm4b', 'm4p', 'mp3', 'mpc',
  'oga', 'ogg', 'opus', 'wav', 'wave', 'wma', 'wv',
]);

export function isMusicFile(file: File): boolean {
  const extension = file.name.split('.').pop()?.toLowerCase() ?? '';
  return audioExtensions.has(extension);
}

export function pendingTrack(file: File): MusicTrack {
  const relativePath = file.webkitRelativePath || file.name;
  return {
    id: `${relativePath}:${file.size}:${file.lastModified}`,
    file, relativePath,
    title: file.name.replace(/\.[^.]+$/, ''),
    artist: 'Reading tags…',
    album: '—',
    albumArtist: 'Unknown artist',
    format: file.name.split('.').pop()?.toUpperCase() || 'Audio',
    size: file.size,
  };
}

function applyTags(track: MusicTrack, tags?: TrackTags): MusicTrack {
  return {
    ...track,
    title: tags?.title || track.title,
    artist: tags?.artist || tags?.albumArtist || 'Unknown artist',
    album: tags?.album || 'Unknown album',
    albumArtist: tags?.albumArtist || tags?.artist || 'Unknown artist',
    genre: tags?.genre,
    year: tags?.year,
    trackNumber: tags?.trackNumber,
    duration: tags?.duration,
    format: tags?.format || track.format,
    artworkUrl: tags?.artwork ? URL.createObjectURL(tags.artwork) : undefined,
    tagError: !tags,
  };
}

// Rows are available before any IO. Two dedicated workers enrich them without
// blocking search, opening a song, or the practice player's control worker.
export async function indexFiles(
  files: File[],
  onProgress: (complete: number, total: number) => void,
  onTrack: (track: MusicTrack, index: number) => void = () => {},
  signal?: AbortSignal,
): Promise<MusicTrack[]> {
  const tracks = files.filter(isMusicFile).map(pendingTrack);
  let next = 0;
  let complete = 0;
  onProgress(0, tracks.length);
  const worker = async () => {
    let reader: Worker | undefined;
    const abort = () => reader?.terminate();
    signal?.addEventListener('abort', abort, { once: true });
    try {
      while (!signal?.aborted) {
        const index = next++;
        if (index >= tracks.length) return;
        let tags: TrackTags | undefined;
        try {
          reader ??= new Worker(new URL('./library-worker.ts', import.meta.url), { type: 'module' });
          tags = await new Promise<TrackTags | undefined>((resolve) => {
            const cancelled = () => finish();
            const finish = (value?: TrackTags) => {
              signal?.removeEventListener('abort', cancelled);
              resolve(value);
            };
            signal?.addEventListener('abort', cancelled, { once: true });
            reader!.onmessage = event => finish(event.data.tags);
            reader!.onerror = event => { event.preventDefault(); reader?.terminate(); reader = undefined; finish(); };
            try { reader!.postMessage(tracks[index].file); }
            catch { reader?.terminate(); reader = undefined; finish(); }
          });
        } catch { reader?.terminate(); reader = undefined; }
        if (signal?.aborted) return;
        tracks[index] = applyTags(tracks[index], tags);
        onTrack(tracks[index], index);
        onProgress(++complete, tracks.length);
      }
    } finally {
      reader?.terminate();
      signal?.removeEventListener('abort', abort);
    }
  };
  await Promise.all(Array.from({ length: Math.min(2, tracks.length) }, worker));
  return tracks;
}

export async function chooseFolder(): Promise<FolderSelection | null> {
  if (!('showDirectoryPicker' in window)) return null;
  const picker = window as Window & {
    showDirectoryPicker: () => Promise<{
      name: string;
      values: () => AsyncIterable<{
        kind: 'file' | 'directory';
        name: string;
        getFile?: () => Promise<File>;
        values?: () => AsyncIterable<unknown>;
      }>;
    }>;
  };
  const root = await picker.showDirectoryPicker();
  const files: File[] = [];

  const visit = async (directory: typeof root, prefix = ''): Promise<void> => {
    for await (const entry of directory.values()) {
      const relativePath = prefix ? `${prefix}/${entry.name}` : entry.name;
      if (entry.kind === 'file' && entry.getFile) {
        const file = await entry.getFile();
        Object.defineProperty(file, 'webkitRelativePath', { value: relativePath });
        files.push(file);
      } else if (entry.kind === 'directory' && entry.values) {
        await visit(entry as typeof root, relativePath);
      }
    }
  };

  await visit(root);
  return { name: root.name, files };
}

export function formatDuration(seconds?: number): string {
  if (seconds === undefined || !Number.isFinite(seconds)) return '—';
  const total = Math.floor(seconds);
  const hours = Math.floor(total / 3600);
  const minutes = Math.floor((total % 3600) / 60);
  const remainder = total % 60;
  return hours ? `${hours}:${String(minutes).padStart(2, '0')}:${String(remainder).padStart(2, '0')}`
    : `${minutes}:${String(remainder).padStart(2, '0')}`;
}

export function formatSize(bytes: number): string {
  if (bytes < 1024 * 1024) return `${Math.round(bytes / 1024)} KB`;
  return `${(bytes / (1024 * 1024)).toFixed(1)} MB`;
}
