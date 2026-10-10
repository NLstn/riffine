import { useMemo, useRef, useState } from 'react';
import type { ChangeEvent } from 'react';
import { chooseFolder, formatDuration, formatSize, indexFiles } from './library';
import type { MusicTrack } from './library';

type Phase = 'idle' | 'indexing' | 'ready';

function App() {
  const folderInput = useRef<HTMLInputElement>(null);
  const [tracks, setTracks] = useState<MusicTrack[]>([]);
  const [folderName, setFolderName] = useState('');
  const [query, setQuery] = useState('');
  const [phase, setPhase] = useState<Phase>('idle');
  const [progress, setProgress] = useState({ complete: 0, total: 0 });
  const [notice, setNotice] = useState('');

  const visibleTracks = useMemo(() => {
    const term = query.trim().toLowerCase();
    if (!term) return tracks;
    return tracks.filter((track) =>
      [track.title, track.artist, track.album, track.albumArtist, track.relativePath]
        .some((value) => value.toLowerCase().includes(term)),
    );
  }, [query, tracks]);

  const addFolder = async (name: string, files: File[]) => {
    const audioFiles = files.filter((file) => /\.(aac|aif|aiff|ape|flac|m4a|m4b|m4p|mp3|mpc|oga|ogg|opus|wav|wave|wma|wv)$/i.test(file.name));
    setFolderName(name);
    setNotice('');
    setPhase('indexing');
    setProgress({ complete: 0, total: audioFiles.length });
    const result = await indexFiles(files, (complete, total) => setProgress({ complete, total }));
    setTracks(result);
    setPhase('ready');
    setNotice(audioFiles.length === 0
      ? 'No supported music files found in that folder.'
      : `${result.length.toLocaleString()} tracks ready`);
  };

  const openFolder = () => {
    setNotice('');
    if ('showDirectoryPicker' in window && typeof (window as Window & { showDirectoryPicker?: unknown }).showDirectoryPicker === 'function') {
      // Call the native picker directly in the click handler so it retains
      // the browser's transient user activation.
      void chooseFolder()
        .then((selection) => selection && addFolder(selection.name, selection.files))
        .catch((error: unknown) => {
          if (error instanceof DOMException && error.name === 'AbortError') return;
          setNotice('Could not open that folder. Try choosing it again.');
          setPhase(tracks.length ? 'ready' : 'idle');
        });
      return;
    }

    // The directory input is the cross-browser fallback. Trigger it before
    // returning from this click handler; deferring it loses user activation.
    folderInput.current?.click();
  };

  const handleFallback = async (event: ChangeEvent<HTMLInputElement>) => {
    const files = Array.from(event.target.files ?? []);
    if (files.length) {
      const first = files[0] as File & { webkitRelativePath?: string };
      const name = first.webkitRelativePath?.split('/')[0] ?? 'Music folder';
      try {
        await addFolder(name, files);
      } catch {
        setNotice('The selected folder could not be indexed. Try choosing it again.');
        setPhase(tracks.length ? 'ready' : 'idle');
      }
    }
    event.target.value = '';
  };

  const totalDuration = tracks.reduce((sum, track) => sum + (track.duration ?? 0), 0);
  const durationLabel = totalDuration > 0
    ? `${Math.floor(totalDuration / 3600)}h ${Math.floor((totalDuration % 3600) / 60)}m`
    : '—';

  return (
    <div className="app-shell">
      <aside className="sidebar">
        <a className="brand" href="#top" aria-label="Riffine home">
          <span className="brand-mark"><span /><span /><span /><span /></span>
          <span>riffine<span className="brand-period">.</span></span>
        </a>

        <div className="side-label">LIBRARY</div>
        <button className="side-link active" type="button">
          <span className="side-icon">♫</span><span>All music</span>
          {tracks.length > 0 && <span className="side-count">{tracks.length}</span>}
        </button>
        <button className="side-link disabled" type="button" title="Coming soon">
          <span className="side-icon">◷</span><span>Recently added</span><span className="coming-soon">SOON</span>
        </button>

        <div className="sidebar-bottom">
          <div className="local-mark"><span className="local-dot" /> Stored on this device</div>
          <div className="sidebar-footnote">Your files stay right where they are.</div>
          <div className="sidebar-version">RIFFINE <span>·</span> PRE-ALPHA</div>
        </div>
      </aside>

      <main id="top" className="main-content">
        <header className="topbar">
          <div className="breadcrumb"><span>Library</span><span className="crumb-separator">/</span><strong>All music</strong></div>
          <div className="topbar-right">
            <span className="local-pill"><span className="local-dot" /> LOCAL ONLY</span>
            <button className="avatar" aria-label="Settings">N</button>
          </div>
        </header>

        <section className="library-heading">
          <div>
            <div className="eyebrow">YOUR SPACE, YOUR SOUND</div>
            <h1>Your music library<span className="heading-period">.</span></h1>
            <p className="heading-subtitle">Pick a folder. Find your next riff.</p>
          </div>
          <button className="primary-button folder-label" type="button" onClick={openFolder}>
            <span className="button-plus">＋</span>{folderName ? 'Change folder' : 'Choose music folder'}
          </button>
          <input
            ref={folderInput}
            id="music-folder-input"
            className="visually-hidden"
            type="file"
            // Directory selection fallback for browsers without showDirectoryPicker.
            {...({ webkitdirectory: '', directory: '', multiple: true } as Record<string, string | boolean>)}
            onChange={handleFallback}
            aria-label="Choose a music folder"
          />
        </section>
        {notice && phase === 'idle' && <div className="notice page-notice" role="status">{notice}</div>}

        {phase === 'idle' ? (
          <section className="welcome-card">
            <div className="welcome-art" aria-hidden="true">
              <div className="art-disc"><div className="art-groove groove-one" /><div className="art-groove groove-two" /><div className="art-center">r<span>.</span></div></div>
              <div className="art-spark spark-one">✳</div><div className="art-spark spark-two">✳</div>
              <div className="art-note note-one">♪</div><div className="art-note note-two">♫</div>
            </div>
            <div className="welcome-copy">
              <div className="welcome-kicker">A good place to start</div>
              <h2>Bring your music in.</h2>
              <p>Choose a folder on your device and we’ll organize the tracks using the tags already in your files.</p>
              <button className="primary-button welcome-button folder-label" type="button" onClick={openFolder}>
                Choose a folder <span aria-hidden="true">→</span>
              </button>
              <div className="privacy-note"><span className="shield-icon">◈</span> Nothing is uploaded. Your music stays on your device.</div>
            </div>
          </section>
        ) : (
          <>
            <section className="stats-row" aria-label="Library summary">
              <div className="stat-card"><span className="stat-label">TRACKS</span><strong>{tracks.length.toLocaleString()}</strong></div>
              <div className="stat-card"><span className="stat-label">TOTAL PLAYTIME</span><strong>{durationLabel}</strong></div>
              <div className="stat-card folder-stat"><span className="stat-label">FOLDER</span><strong title={folderName}>{folderName || '—'}</strong></div>
              <div className="stat-note"><span className="pulse-dot" /> Files are read from your selected folder</div>
            </section>

            <section className="tracks-section">
              <div className="tracks-toolbar">
                <div className="tracks-title-wrap">
                  <h2>All tracks</h2>
                  <span className="track-count">{visibleTracks.length.toLocaleString()}</span>
                </div>
                <label className="search-box">
                  <span aria-hidden="true">⌕</span>
                  <input value={query} onChange={(event) => setQuery(event.target.value)} placeholder="Search tracks, artists…" />
                </label>
              </div>

              {phase === 'indexing' && (
                <div className="progress-banner">
                  <div className="spinner" />
                  <div className="progress-copy"><strong>Reading your music tags</strong><span>{progress.complete} of {progress.total} files</span></div>
                  <div className="progress-track"><span style={{ width: `${progress.total ? (progress.complete / progress.total) * 100 : 100}%` }} /></div>
                </div>
              )}

              {notice && <div className="notice" role="status">{notice}</div>}

              <div className="table-wrap">
                <table>
                <thead><tr><th className="number-col">#</th><th>TITLE</th><th>ALBUM</th><th>FILE</th><th className="duration-col">TIME</th><th className="more-col" aria-label="More" /></tr></thead>
                  <tbody>
                    {visibleTracks.map((track, index) => (
                      <tr key={track.id}>
                        <td className="track-number">{String(track.trackNumber ?? index + 1).padStart(2, '0')}</td>
                        <td>
                          <div className="track-cell">
                            {track.artworkUrl
                              ? <img className="cover-art" src={track.artworkUrl} alt="" />
                              : <div className="cover-placeholder"><span>♫</span></div>}
                            <div className="track-text"><strong title={track.title}>{track.title}</strong><span title={track.artist}>{track.artist}</span></div>
                          </div>
                        </td>
                        <td><div className="album-cell"><strong title={track.album}>{track.album}</strong><span>{[track.genre, track.year].filter(Boolean).join(' · ') || '—'}</span></div></td>
                        <td><span className="format-badge">{track.format}</span><span className="file-size">{formatSize(track.size)}</span></td>
                        <td className="duration-cell">{formatDuration(track.duration)}</td>
                        <td className="more-cell" title={track.tagError ? 'Tags could not be read; showing filename.' : track.relativePath}>•••</td>
                      </tr>
                    ))}
                    {phase === 'ready' && visibleTracks.length === 0 && (
                      <tr><td colSpan={6} className="empty-table">{tracks.length ? 'No tracks match your search.' : 'No supported music files found in this folder.'}</td></tr>
                    )}
                  </tbody>
                </table>
              </div>
              <div className="table-footer"><span>{tracks.length ? `Showing ${visibleTracks.length} of ${tracks.length} tracks` : 'Select a folder to get started'}</span><span>Tags read locally <span className="footer-lock">◆</span></span></div>
            </section>
          </>
        )}

        <footer className="page-footer"><span>Made for the moments between play and replay.</span><span>RIFFINE <span className="footer-accent">●</span> PRE-ALPHA</span></footer>
      </main>
    </div>
  );
}

export default App;
