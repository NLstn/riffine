import { useMemo, useRef, useState } from "react";
import type { ChangeEvent } from "react";
import { chooseFolder, formatDuration, indexFiles } from "./library";
import type { MusicTrack } from "./library";
import PracticeWorkspace from "./PracticeWorkspace";
import OfflineStatus from "./OfflineStatus";

const supportedSource = (file: File) =>
  /\.(wav|wave|mp3|flac|m4a|opus|ogg|oga)$/i.test(file.name);

export default function App() {
  const folderInput = useRef<HTMLInputElement>(null);
  const [practiceSource, setPracticeSource] = useState<File>();
  const [practiceKey, setPracticeKey] = useState(0);
  const [tracks, setTracks] = useState<MusicTrack[]>([]);
  const [libraryOpen, setLibraryOpen] = useState(false);
  const [folderName, setFolderName] = useState("");
  const [query, setQuery] = useState("");
  const [indexing, setIndexing] = useState(false);
  const [progress, setProgress] = useState({ complete: 0, total: 0 });
  const [notice, setNotice] = useState("");
  const librarySources = useMemo(
    () =>
      tracks
        .filter((track) => supportedSource(track.file))
        .map((track) => track.file),
    [tracks],
  );
  const visibleTracks = useMemo(
    () =>
      tracks.filter((track) =>
        [track.title, track.artist, track.album, track.relativePath].some(
          (value) => value.toLowerCase().includes(query.toLowerCase()),
        ),
      ),
    [query, tracks],
  );
  const openTrack = (file: File) => {
    setPracticeSource(file);
    setPracticeKey((value) => value + 1);
    setLibraryOpen(false);
  };
  const addFolder = async (name: string, files: File[]) => {
    setFolderName(name);
    setIndexing(true);
    setNotice("");
    try {
      const result = await indexFiles(files, (complete, total) =>
        setProgress({ complete, total }),
      );
      setTracks(result);
      setNotice(
        result.length
          ? `${result.length} songs ready to practice.`
          : "No supported music files found in this folder.",
      );
    } catch {
      setNotice(
        "The selected folder could not be read. Try choosing it again.",
      );
    } finally {
      setIndexing(false);
    }
  };
  const openFolder = () => {
    if (
      "showDirectoryPicker" in window &&
      typeof (window as Window & { showDirectoryPicker?: unknown })
        .showDirectoryPicker === "function"
    ) {
      void chooseFolder()
        .then(
          (selection) =>
            selection && addFolder(selection.name, selection.files),
        )
        .catch((failure) => {
          if (
            !(failure instanceof DOMException && failure.name === "AbortError")
          )
            setNotice("Could not open that folder. Try choosing it again.");
        });
    } else folderInput.current?.click();
  };
  const handleFolder = (event: ChangeEvent<HTMLInputElement>) => {
    const files = Array.from(event.target.files ?? []);
    if (files.length)
      void addFolder(
        files[0].webkitRelativePath?.split("/")[0] || "Music folder",
        files,
      );
    event.target.value = "";
  };
  return (
    <div className="studio-shell">
      <header className="studio-topbar">
        <a className="brand" href="#top" aria-label="Riffine home">
          <span className="brand-mark">
            <span />
            <span />
            <span />
            <span />
          </span>
          <span>
            riffine<span className="brand-period">.</span>
          </span>
        </a>
        <nav aria-label="Main navigation">
          <button
            type="button"
            aria-pressed={!libraryOpen}
            onClick={() => setLibraryOpen(false)}
          >
            Practice
          </button>
          <button
            type="button"
            aria-pressed={libraryOpen}
            onClick={() => setLibraryOpen((value) => !value)}
          >
            Music library {tracks.length > 0 && <span>{tracks.length}</span>}
          </button>
          <a className="mobile-projects-link" href="#training-projects">
            Projects
          </a>
        </nav>
        <span className="studio-local">
          <span className="local-dot" /> Audio stays on your device
        </span>
      </header>
      {libraryOpen && (
        <section className="studio-library" aria-label="Music library">
          <div className="library-drawer-heading">
            <div>
              <div className="eyebrow">YOUR MUSIC</div>
              <h2>{folderName || "Find your next phrase."}</h2>
              <p>Choose a song to open it in the waveform player.</p>
            </div>
            <div className="practice-actions">
              <button
                type="button"
                className="primary-button"
                disabled={indexing}
                onClick={openFolder}
              >
                {folderName ? "Change folder" : "Choose music folder"}
              </button>
              <button
                type="button"
                onClick={() => setLibraryOpen(false)}
                aria-label="Close music library"
              >
                ×
              </button>
            </div>
          </div>
          <input
            ref={folderInput}
            className="visually-hidden"
            type="file"
            {...({
              webkitdirectory: "",
              directory: "",
              multiple: true,
            } as Record<string, string | boolean>)}
            onChange={handleFolder}
            aria-label="Choose a music folder"
          />
          {indexing && (
            <p role="status">
              Reading music tags · {progress.complete} of {progress.total}
            </p>
          )}
          {notice && <p role="status">{notice}</p>}
          {tracks.length > 0 && (
            <>
              <label className="search-box">
                <span aria-hidden="true">⌕</span>
                <input
                  aria-label="Search music library"
                  value={query}
                  onChange={(event) => setQuery(event.target.value)}
                  placeholder="Search songs, artists, albums…"
                />
              </label>
              <div className="library-song-list">
                {visibleTracks.map((track) => (
                  <button
                    className="library-song"
                    key={track.id}
                    type="button"
                    disabled={!supportedSource(track.file)}
                    onClick={() => openTrack(track.file)}
                    aria-label={`Open ${track.title} in player`}
                  >
                    <span className="song-open" aria-hidden="true">
                      ▶
                    </span>
                    {track.artworkUrl ? (
                      <img src={track.artworkUrl} alt="" />
                    ) : (
                      <span className="song-art" aria-hidden="true">
                        ♫
                      </span>
                    )}
                    <span className="song-title">
                      <strong>{track.title}</strong>
                      <small>{track.artist}</small>
                    </span>
                    <span className="song-album">{track.album}</span>
                    <span className="format-badge">
                      {supportedSource(track.file)
                        ? track.format
                        : "Unsupported"}
                    </span>
                    <span className="song-duration">
                      {formatDuration(track.duration)}
                    </span>
                  </button>
                ))}
                {!visibleTracks.length && <p>No songs match your search.</p>}
              </div>
            </>
          )}
          {!tracks.length && !indexing && (
            <p className="library-help">
              WAV, MP3, FLAC, M4A and Opus. Pick a folder to browse, or open a
              song directly in Practice.
            </p>
          )}
        </section>
      )}
      <main id="top">
        <PracticeWorkspace
          key={practiceKey}
          source={practiceSource}
          librarySources={librarySources}
          onClose={() => {
            setPracticeSource(undefined);
            setPracticeKey((value) => value + 1);
          }}
        />
      </main>
      <OfflineStatus />
    </div>
  );
}
