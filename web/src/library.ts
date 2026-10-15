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

export async function readTrack(file: File, relativePath: string): Promise<MusicTrack> {
  const fallbackTitle = file.name.replace(/\.[^.]+$/, '');
  try {
    const { parseBlob } = await import('music-metadata');
    const metadata = await parseBlob(file, { duration: true });
    const picture = metadata.common.picture?.[0];
    let artworkUrl: string | undefined;
    if (picture) {
      const artwork = new Uint8Array(picture.data.byteLength);
      artwork.set(picture.data);
      artworkUrl = URL.createObjectURL(new Blob([artwork.buffer], { type: picture.format }));
    }

    return {
      id: `${relativePath}:${file.size}:${file.lastModified}`,
      file,
      relativePath,
      title: metadata.common.title || fallbackTitle,
      artist: metadata.common.artist || metadata.common.albumartist || 'Unknown artist',
      album: metadata.common.album || 'Unknown album',
      albumArtist: metadata.common.albumartist || metadata.common.artist || 'Unknown artist',
      genre: metadata.common.genre?.[0],
      year: metadata.common.year,
      trackNumber: metadata.common.track.no ?? undefined,
      duration: metadata.format.duration,
      format: metadata.format.container || file.name.split('.').pop()?.toUpperCase() || 'Audio',
      size: file.size,
      artworkUrl,
    };
  } catch {
    return {
      id: `${relativePath}:${file.size}:${file.lastModified}`,
      file,
      relativePath,
      title: fallbackTitle,
      artist: 'Unknown artist',
      album: 'Unknown album',
      albumArtist: 'Unknown artist',
      format: file.name.split('.').pop()?.toUpperCase() || 'Audio',
      size: file.size,
      tagError: true,
    };
  }
}

export async function indexFiles(
  files: File[],
  onProgress: (complete: number, total: number) => void,
): Promise<MusicTrack[]> {
  const candidates = files.filter(isMusicFile);
  const tracks: MusicTrack[] = new Array(candidates.length);
  let next = 0;
  let complete = 0;

  const worker = async () => {
    while (true) {
      const index = next++;
      if (index >= candidates.length) return;
      const file = candidates[index];
      const relativePath = (file as File & { webkitRelativePath?: string }).webkitRelativePath || file.name;
      tracks[index] = await readTrack(file, relativePath);
      complete++;
      onProgress(complete, candidates.length);
    }
  };

  await Promise.all(Array.from({ length: Math.min(4, candidates.length) }, worker));
  return tracks.sort((a, b) =>
    a.artist.localeCompare(b.artist) || a.album.localeCompare(b.album) ||
    (a.trackNumber ?? Number.MAX_SAFE_INTEGER) - (b.trackNumber ?? Number.MAX_SAFE_INTEGER) ||
    a.title.localeCompare(b.title),
  );
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
