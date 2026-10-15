import { useEffect, useRef, useState } from "react";
import { appPath } from './app-url';
import type { KeyboardEvent } from "react";
import {
  loadShortcuts,
  saveShortcuts,
  shortcutKeys,
  shortcutActions,
  matchShortcut,
} from "./practice-shortcuts";
import PracticePlans from "./PracticePlans";
import PracticeWaveform from "./PracticeWaveform";
import { deviceProjects } from "./practice-storage";
import type { ProjectStorage, SavedProject } from "./practice-storage";
import {
  BrowserPractice,
  presentationSnapshot,
  practiceActionLabels,
} from "./practice";
import type {
  EditCommand,
  LiveCommand,
  PracticeFacade,
  PracticeResponse,
  Snapshot,
  WaveWindow,
} from "./practice";

export default function PracticeWorkspace({
  source,
  onClose,
  librarySources = [],
  createFacade = () => new BrowserPractice(),
  createStorage = deviceProjects,
}: {
  source?: File;
  onClose: () => void;
  librarySources?: File[];
  createFacade?: () => PracticeFacade;
  createStorage?: () => Promise<ProjectStorage>;
}) {
  const facade = useRef<PracticeFacade | null>(null);
  const operation = useRef(false);
  const accepted = useRef<Snapshot | undefined>(undefined);
  const pendingEdits = useRef(new Map<EditCommand["type"], EditCommand>());
  const drainEdits = useRef<() => void>(() => {});
  const editTimer = useRef<ReturnType<typeof setTimeout> | undefined>(
    undefined,
  );
  const [liveUpdating, setLiveUpdating] = useState(false);
  const [projectQuery, setProjectQuery] = useState("");
  const [loadedShortcuts] = useState(loadShortcuts);
  const [shortcuts, setShortcuts] = useState(loadedShortcuts.settings);
  const [shortcutDraft, setShortcutDraft] = useState(loadedShortcuts.settings);
  const [shortcutWarning, setShortcutWarning] = useState(
    loadedShortcuts.warning ?? "",
  );
  const sourceInput = useRef<HTMLInputElement>(null);
  const projectInput = useRef<HTMLInputElement>(null);
  const pendingProjectFile = useRef<File | undefined>(undefined);
  const [selectedSource, setSelectedSource] = useState(source);
  const [localProject, setLocalProject] = useState<SavedProject>();
  const [savedProjects, setSavedProjects] = useState<SavedProject[]>([]);
  const [storageWarning, setStorageWarning] = useState("");
  const localSourceInput = useRef<HTMLInputElement>(null);
  const pendingLocalProject = useRef<SavedProject | undefined>(undefined);
  const [sourceSession, setSourceSession] = useState(0);
  const [snapshot, setSnapshot] = useState<Snapshot>();
  const [wave, setWave] = useState<WaveWindow>();
  const [busy, setBusy] = useState(false);
  const [previewControls, setPreviewControls] = useState(false);
  const [previewPaused, setPreviewPaused] = useState(false);
  const [previewControlBusy, setPreviewControlBusy] = useState(false);
  const [busyLabel, setBusyLabel] = useState("Working locally…");
  const [error, setError] = useState("");
  const [status, setStatus] = useState(
    "WAV, MP3, FLAC, M4A and Opus · Everything stays on this device.",
  );
  const [start, setStart] = useState("0");
  const [end, setEnd] = useState("0");
  const [speed, setSpeed] = useState("100");
  const [pitch, setPitch] = useState("0");
  const [seekSeconds, setSeekSeconds] = useState("0");
  const [label, setLabel] = useState("Phrase");
  const [selectedRegion, setSelectedRegion] = useState<string>();
  const [notes, setNotes] = useState("");
  const [phraseNotes, setPhraseNotes] = useState("");
  const [markerLabel, setMarkerLabel] = useState("Section");
  const [markerKind, setMarkerKind] = useState<"section" | "measure" | "beat">(
    "section",
  );
  const [markerMeasure, setMarkerMeasure] = useState("1");
  const [markerBeat, setMarkerBeat] = useState("1");
  const [detailView, setDetailView] = useState(false);
  const install = (response: PracticeResponse, syncForms = false) => {
    if (response.snapshot) {
      const current = response.snapshot;
      const previous = accepted.current;
      accepted.current = presentationSnapshot(
        accepted.current,
        current,
        syncForms,
      );
      setSnapshot(accepted.current);
      if (syncForms) {
        setSourceSession((previous) => previous + 1);
        setSpeed(
          String(
            Number((current.project.settings.speed * 100).toPrecision(15)),
          ),
        );
        setPitch(String(current.project.settings.pitch));
        setStart(
          String(Number(current.project.settings.loop.start.toFixed(6))),
        );
        setEnd(String(Number(current.project.settings.loop.end.toFixed(6))));
        setNotes(current.project.annotations.notes ?? "");
      } else if (previous) {
        const settings = accepted.current.project.settings;
        if (
          !pendingEdits.current.has("set_parameters") &&
          (previous.project.settings.speed !== settings.speed ||
            previous.project.settings.pitch !== settings.pitch)
        ) {
          setSpeed(String(Number((settings.speed * 100).toPrecision(15))));
          setPitch(String(settings.pitch));
        }
        if (
          !pendingEdits.current.has("set_loop") &&
          (previous.project.settings.loop.start !== settings.loop.start ||
            previous.project.settings.loop.end !== settings.loop.end)
        ) {
          setStart(String(Number(settings.loop.start.toFixed(6))));
          setEnd(String(Number(settings.loop.end.toFixed(6))));
        }
      }
    }
    if (response.error)
      throw new Error(`${response.error.code}: ${response.error.message}`);
    if (response.window) setWave(response.window);
    return response;
  };
  const run = async (
    action: () => Promise<void>,
    message = "Working locally…",
    interactive = false,
  ) => {
    const client = facade.current;
    if (!client || operation.current) return;
    operation.current = true;
    if (interactive) setLiveUpdating(true);
    else setBusy(true);
    setBusyLabel(message);
    setError("");
    try {
      await action();
    } catch (failure) {
      // Effect replay may have disposed this client and started a new import.
      // Its late rejection must not fail or unlock the new client's operation.
      if (facade.current !== client) return;
      console.error(
        "[riffine:practice] workspace operation failed",
        {
          operation: message,
          encoding: snapshot?.project.track.audio.encoding,
        },
        failure,
      );
      const errorMessage = String(
        failure instanceof Error ? failure.message : failure,
      );
      setError(errorMessage);
      setStatus(
        errorMessage.startsWith("cancelled:")
          ? "Operation cancelled."
          : "Operation could not finish.",
      );
    } finally {
      if (facade.current === client) {
        operation.current = false;
        setBusy(false);
        setLiveUpdating(false);
        drainEdits.current();
      }
    }
  };
  const open = async (original: File, saved?: File, local?: SavedProject) => {
    const client = facade.current;
    if (!client) return;
    if (local && !client.openLocal)
      throw new Error(
        "Device projects are unavailable in this host. Reopen a downloaded project.",
      );
    pendingEdits.current.clear();
    clearTimeout(editTimer.current);
    const opened = local
      ? await client.openLocal!(original, local.id)
      : await client.open(original, saved);
    if (facade.current !== client) return;
    const result = install(opened, true);
    setWave(undefined);
    setDetailView(false);
    setSelectedRegion(undefined);
    setLabel("Phrase");
    setPhraseNotes("");
    setSelectedSource(original);
    setLocalProject(local);
    if (result.snapshot) {
      const waveform = await client.waveform(
        0,
        result.snapshot.project.track.audio.frames,
      );
      if (facade.current !== client) return;
      install(waveform, false);
      document.getElementById("top")?.scrollIntoView?.({ block: "start" });
      setStatus(
        saved || local
          ? "Project reopened. Ready to pick up where you left off."
          : "Ready to practice. Select a phrase or press Play.",
      );
    }
  };
  useEffect(() => {
    let active = true;
    void createStorage()
      .then((store) => store.list())
      .then((items) => {
        if (active) setSavedProjects(items);
      })
      .catch((failure) => {
        if (active)
          setStorageWarning(
            String(failure instanceof Error ? failure.message : failure),
          );
      });
    return () => {
      active = false;
    };
  }, []);
  useEffect(() => {
    let client: PracticeFacade;
    try {
      client = createFacade();
    } catch (failure) {
      console.error(
        "[riffine:practice] workspace initialization failed",
        failure,
      );
      setError(String(failure instanceof Error ? failure.message : failure));
      setStatus("Practice could not start.");
      return;
    }
    facade.current = client;
    if (source) void run(() => open(source), "Opening library audio…");
    return () => {
      facade.current = null;
      operation.current = false;
      pendingEdits.current.clear();
      clearTimeout(editTimer.current);
      client.dispose();
    };
    // One worker per mounted workspace. Injected facades are fixed for its lifetime.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);
  // The timer only requests shared-core snapshots. Device acknowledgements own
  // position and completed-loop counts; React never advances either value.
  useEffect(() => {
    const client = facade.current;
    if (
      busy ||
      !snapshot?.capabilities.liveTransport ||
      snapshot.transport?.state === "error" ||
      !client?.pollTransport
    )
      return;
    let active = true;
    let timer: ReturnType<typeof setTimeout>;
    const poll = async () => {
      try {
        if (operation.current) {
          timer = setTimeout(() => void poll(), 50);
          return;
        }
        const response = await client.pollTransport!();
        if (!active) return;
        if (response.error) {
          if (response.snapshot) {
            accepted.current = presentationSnapshot(
              accepted.current,
              response.snapshot,
            );
            setSnapshot(accepted.current);
          }
          setError("Playback was interrupted. Recover playback to try again.");
          setStatus("Playback stopped.");
          return;
        }
        install(response);
      } catch {
        if (active) {
          setError("Playback was interrupted. Recover playback to try again.");
          setStatus("Playback stopped.");
        }
        return;
      }
      if (active) timer = setTimeout(() => void poll(), 50);
    };
    void poll();
    return () => {
      active = false;
      clearTimeout(timer);
    };
    // Polling does not edit the project or consume a command sequence.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [busy, snapshot?.capabilities.liveTransport, snapshot?.transport?.state]);
  const transport = (command: LiveCommand) =>
    run(async () => {
      if (!facade.current?.transport || !accepted.current) return;
      const response = install(
        await facade.current.transport(accepted.current, command),
      );
      if (command.type in practiceActionLabels && response.snapshot) {
        const settings = response.snapshot.project.settings;
        if (!pendingEdits.current.has("set_parameters")) {
          setSpeed(String(Number((settings.speed * 100).toPrecision(15))));
          setPitch(String(settings.pitch));
        }
        if (!pendingEdits.current.has("set_loop")) {
          setStart(String(Number(settings.loop.start.toFixed(6))));
          setEnd(String(Number(settings.loop.end.toFixed(6))));
        }
      }
      setStatus(
        command.type === "recover"
          ? "Playback recovered. Press Play to start."
          : command.type === "select_practice_plan"
            ? "Practice plan selected. Press Play to start."
            : "Playback updated.",
      );
    });
  const canPlay =
    !!snapshot &&
    (snapshot.capabilities.liveTransport ||
      (facade.current?.liveSupported && snapshot.capabilities.streamPreview)) &&
    snapshot.transport?.state !== "error" &&
    snapshot.transport?.plan?.state !== "completed";
  const togglePlayback = () =>
    transport({
      type: snapshot?.transport?.state === "playing" ? "pause" : "play",
    });
  const keydown = (event: KeyboardEvent<HTMLElement>) => {
    const action = matchShortcut(event.nativeEvent, shortcuts);
    if (!action || !snapshot) return;
    const available =
      action === "play_pause"
        ? canPlay
        : action === "restart" || action === "stop"
          ? snapshot.capabilities.liveTransport &&
            snapshot.transport?.state !== "error"
          : !!(
              (snapshot.capabilities.liveTransport ||
                (facade.current?.liveSupported &&
                  snapshot.capabilities.streamPreview)) &&
              snapshot.transport?.state !== "error"
            );
    if (!available) return;
    event.preventDefault();
    // Consume repeats without delivering a second command; the operation latch
    // also covers separate quick presses before React has rendered busy state.
    if (event.repeat || operation.current) return;
    if (action === "play_pause") void togglePlayback();
    else void transport({ type: action });
  };
  const shortcutAttribute = (action: keyof typeof shortcuts.bindings) =>
    (shortcuts.enabled && shortcuts.bindings[action]) || undefined;
  const applyShortcuts = () => {
    try {
      const accepted = saveShortcuts(shortcutDraft);
      setShortcuts(accepted);
      setShortcutDraft(accepted);
      setShortcutWarning("");
      setStatus("Keyboard shortcuts saved on this device.");
    } catch (failure) {
      setShortcutWarning(
        String(failure instanceof Error ? failure.message : failure),
      );
    }
  };
  const releasePlayback = () =>
    run(async () => {
      if (!facade.current?.detachTransport) return;
      install(await facade.current.detachTransport());
      setStatus("Playback closed. Selection preview is available.");
    });
  // Coalesce continuous gestures, then send each command with the latest core
  // sequence. A final slider value is retained even while another edit is in flight.
  drainEdits.current = () => {
    if (
      operation.current ||
      !facade.current ||
      !accepted.current ||
      !pendingEdits.current.size
    )
      return;
    void run(
      async () => {
        const client = facade.current!;
        while (
          pendingEdits.current.size &&
          facade.current === client &&
          accepted.current
        ) {
          const [type, command] = pendingEdits.current.entries().next().value!;
          pendingEdits.current.delete(type);
          try {
            const response = await client.command(accepted.current, command);
            if (facade.current !== client) return;
            install(response);
          } catch (failure) {
            if (facade.current !== client) return;
            if (!pendingEdits.current.has(type) && accepted.current) {
              const settings = accepted.current.project.settings;
              if (type === "set_parameters") {
                setSpeed(String(settings.speed * 100));
                setPitch(String(settings.pitch));
              }
              if (type === "set_loop") {
                setStart(String(Number(settings.loop.start.toFixed(6))));
                setEnd(String(Number(settings.loop.end.toFixed(6))));
              }
            }
            throw failure;
          }
        }
        setStatus(
          "Playback settings updated. Save your project to keep your changes.",
        );
      },
      "Updating playback…",
      true,
    );
  };
  const queueEdit = (command: EditCommand) => {
    setLiveUpdating(true);
    pendingEdits.current.set(command.type, command);
    clearTimeout(editTimer.current);
    editTimer.current = setTimeout(() => drainEdits.current(), command.type === "set_parameters" ? 0 : 60);
  };
  const edit = (command: EditCommand) =>
    run(async () => {
      if (!facade.current || !accepted.current) return;
      install(await facade.current.command(accepted.current, command));
      setStatus(
        "Changes saved in this session. Save your project to keep them.",
      );
    });
  const changeParameters = (nextSpeed: string, nextPitch: string) => {
    setSpeed(nextSpeed);
    setPitch(nextPitch);
    if (!nextSpeed.trim() || !nextPitch.trim()) return;
    const value = Number(nextSpeed),
      tuning = Number(nextPitch);
    if (
      !Number.isFinite(value) ||
      value < 25 ||
      value > 200 ||
      !Number.isFinite(tuning) ||
      tuning < -12 ||
      tuning > 12
    )
      return;
    queueEdit({ type: "set_parameters", speed: value / 100, pitch: tuning });
  };
  const selectLoop = (nextStart: number, nextEnd: number, enabled = true) => {
    const current = accepted.current;
    if (!current) return;
    const rate = current.project.track.audio.sampleRate;
    const duration = current.project.track.audio.frames / rate;
    if (!Number.isFinite(nextStart) || !Number.isFinite(nextEnd)) return;
    const a = Math.max(
      0,
      Math.min(duration - 1 / rate, Math.round(nextStart * rate) / rate),
    );
    const b = Math.max(
      a + 1 / rate,
      Math.min(duration, Math.round(nextEnd * rate) / rate),
    );
    setStart(String(Number(a.toFixed(6))));
    setEnd(String(Number(b.toFixed(6))));
    queueEdit({ type: "set_loop", loop: { enabled, start: a, end: b } });
  };
  const selectBoundary = (boundary: "start" | "end", seconds: number) => {
    const rate = accepted.current!.project.track.audio.sampleRate;
    if (boundary === "start")
      selectLoop(seconds, Math.max(Number(end), seconds + 1 / rate));
    else selectLoop(Math.min(Number(start), seconds - 1 / rate), seconds);
  };
  const annotations = snapshot?.project.annotations ?? {};
  // Region and named-loop references are built together; the core validates them.
  const savePhrase = () => {
    const regionId = `region-${crypto.randomUUID()}`;
    void edit({
      type: "set_annotations",
      annotations: {
        ...annotations,
        regions: [
          ...(annotations.regions ?? []),
          {
            id: regionId,
            label,
            start: Number(start),
            end: Number(end),
            notes: phraseNotes,
          },
        ],
        loops: [
          ...(annotations.loops ?? []),
          { id: `loop-${crypto.randomUUID()}`, label, regionId },
        ],
      },
    });
  };
  const updatePhrase = () => {
    if (!selectedRegion) return;
    void edit({
      type: "set_annotations",
      annotations: {
        ...annotations,
        regions: annotations.regions?.map((region) =>
          region.id === selectedRegion
            ? {
                ...region,
                label,
                start: Number(start),
                end: Number(end),
                notes: phraseNotes,
              }
            : region,
        ),
        loops: annotations.loops?.map((loop) =>
          loop.regionId === selectedRegion ? { ...loop, label } : loop,
        ),
      },
    });
  };
  const deletePhrase = () => {
    if (!selectedRegion) return;
    void edit({
      type: "set_annotations",
      annotations: {
        ...annotations,
        regions: annotations.regions?.filter(
          (region) => region.id !== selectedRegion,
        ),
        loops: annotations.loops?.filter(
          (loop) => loop.regionId !== selectedRegion,
        ),
      },
    });
  };
  const loadProject = (file?: File) => {
    if (!file) return;
    pendingProjectFile.current = file;
    if (selectedSource)
      void run(async () => {
        await open(selectedSource, file);
        pendingProjectFile.current = undefined;
      });
    else {
      pendingProjectFile.current = file;
      setStatus("Choose the original audio for this project.");
      sourceInput.current?.click();
    }
  };
  const chooseSource = (file?: File) => {
    if (!file) return;
    const project = pendingProjectFile.current;
    void run(async () => {
      await open(file, project);
      pendingProjectFile.current = undefined;
    }, "Opening audio…");
  };
  const refreshSaved = async () => {
    setSavedProjects(await (await createStorage()).list());
    setStorageWarning("");
  };
  const saveDevice = (copy = false) =>
    run(async () => {
      if (!facade.current?.saveLocal) return;
      const result = install(await facade.current.saveLocal(copy));
      setStatus(
        result.saved
          ? "Project saved. Its previous version is available in project options."
          : "Project saved in Training projects. Reopen it there to update this saved copy.",
      );
      try {
        await refreshSaved();
      } catch (failure) {
        setStorageWarning(
          String(failure instanceof Error ? failure.message : failure),
        );
      }
    }, "Saving project on this device…");
  const chooseLocalOriginal = (item: SavedProject) => {
    pendingLocalProject.current = item;
    localSourceInput.current?.click();
  };
  const reopenDevice = (item: SavedProject) => {
    const original = [selectedSource, ...librarySources].find(
      (file) => file?.name === item.sourceName,
    );
    if (original)
      void run(() => open(original, undefined, item), "Opening saved project…");
    else chooseLocalOriginal(item);
  };
  const removeDevice = (item: SavedProject) =>
    run(async () => {
      await (await createStorage()).remove(item.id);
      await refreshSaved();
      setStatus("Saved project removed from this device.");
    }, "Removing device project…");
  const exportProject = () =>
    run(async () => {
      if (!facade.current) return;
      const result = install(await facade.current.exportProject());
      if (result.document) {
        const url = URL.createObjectURL(
          new Blob([result.document], { type: "application/json" }),
        );
        const anchor = document.createElement("a");
        anchor.href = url;
        anchor.download = "practice.riffine.json";
        anchor.click();
        setTimeout(() => URL.revokeObjectURL(url), 1000);
        setStatus(
          "Project downloaded. Reopen it with the matching original audio.",
        );
      }
    });
  const zoom = () =>
    run(async () => {
      if (!facade.current || !snapshot) return;
      const rate = snapshot.project.track.audio.sampleRate;
      install(
        await facade.current.waveform(
          Math.round(Number(start) * rate),
          Math.round(Number(end) * rate),
          // Exact PCM detail is bounded; longer selections use the pyramid.
          Math.round((Number(end) - Number(start)) * rate) <= 1 << 20,
        ),
        false,
      );
      setDetailView(true);
    });
  const overview = () =>
    run(async () => {
      if (!facade.current || !snapshot) return;
      install(
        await facade.current.waveform(0, snapshot.project.track.audio.frames),
        false,
      );
      setDetailView(false);
    });
  const preview = () =>
    run(async () => {
      if (!facade.current?.preview || !snapshot) return;
      setStatus("Previewing the saved selection once…");
      setPreviewPaused(false);
      setPreviewControls(false);
      let result: PracticeResponse;
      try {
        result = install(
          await facade.current.preview(snapshot, () =>
            setPreviewControls(true),
          ),
        );
      } finally {
        setPreviewControls(false);
        setPreviewControlBusy(false);
      }
      setStatus(
        result.preview?.underrunFrames
          ? "Selection preview finished with an audio interruption. You can try it again."
          : "Selection preview finished.",
      );
    }, "Previewing the saved selection once…");
  const togglePreview = async () => {
    const client = facade.current;
    if (!client) return;
    setPreviewControlBusy(true);
    try {
      if (previewPaused) {
        await client.resumePreview?.();
        setPreviewPaused(false);
        setBusyLabel("Previewing the saved selection once…");
      } else if (await client.pausePreview?.()) {
        setPreviewPaused(true);
        setBusyLabel("Selection preview paused.");
      }
    } catch (failure) {
      setError(String(failure instanceof Error ? failure.message : failure));
      client.cancel();
    } finally {
      setPreviewControlBusy(false);
    }
  };
  const duration = snapshot
    ? snapshot.project.track.audio.frames /
      snapshot.project.track.audio.sampleRate
    : 0;
  const controlsBusy = busy || liveUpdating;
  const position = snapshot?.transport
    ? snapshot.transport.sourceFrame / snapshot.project.track.audio.sampleRate
    : 0;
  const loopEnabled =
    (
      pendingEdits.current.get("set_loop") as
        | Extract<EditCommand, { type: "set_loop" }>
        | undefined
    )?.loop.enabled ??
    snapshot?.project.settings.loop.enabled ??
    false;
  const time = (seconds: number) =>
    `${Math.floor(seconds / 60)}:${(seconds % 60).toFixed(2).padStart(5, "0")}`;
  const projects = savedProjects.filter(
    (item) =>
      !item.recoveryOf &&
      `${item.name} ${item.sourceName}`
        .toLowerCase()
        .includes(projectQuery.toLowerCase()),
  );
  const playableAction =
    !!snapshot &&
    snapshot.transport?.state !== "error" &&
    !!(
      (facade.current?.liveSupported && snapshot.capabilities.streamPreview) ||
      snapshot.capabilities.liveTransport
    );
  return (
    <section
      className="practice-workspace"
      aria-label="Practice workspace"
      onKeyDown={keydown}
    >
      <aside
        id="training-projects"
        className="project-sidebar"
        aria-label="Training projects"
      >
        <div className="project-sidebar-heading">
          <h2>Training projects</h2>
          <span>{savedProjects.filter((item) => !item.recoveryOf).length}</span>
        </div>
        <button
          className="new-project-button"
          type="button"
          disabled={controlsBusy}
          onClick={() => sourceInput.current?.click()}
        >
          ＋ Open a song
        </button>
        <label className="project-search">
          <span className="visually-hidden">Search training projects</span>
          <input
            value={projectQuery}
            onChange={(event) => setProjectQuery(event.target.value)}
            placeholder="Find a project…"
          />
        </label>
        {storageWarning && (
          <p role="alert" className="practice-error">
            {storageWarning}
          </p>
        )}
        <ul className="saved-project-list">
          {projects.map((item) => (
            <li key={item.id}>
              <button
                className="saved-project-card"
                type="button"
                disabled={controlsBusy || !facade.current?.openLocal}
                aria-pressed={localProject?.id === item.id}
                onClick={() => reopenDevice(item)}
              >
                <span className="project-glyph" aria-hidden="true">
                  ≋
                </span>
                <span>
                  <strong>{item.name}</strong>
                  <small>{item.sourceName}</small>
                  <small>
                    {new Date(item.savedAt).toLocaleDateString(undefined, {
                      month: "short",
                      day: "numeric",
                    })}
                  </small>
                </span>
              </button>
              <details className="project-options">
                <summary aria-label={`Options for ${item.name}`}>•••</summary>
                <button
                  type="button"
                  disabled={controlsBusy || !facade.current?.openLocal}
                  onClick={() => chooseLocalOriginal(item)}
                >
                  Choose original and reopen
                </button>
                {savedProjects
                  .filter((recovery) => recovery.recoveryOf === item.id)
                  .map((recovery) => (
                    <button
                      key={recovery.id}
                      type="button"
                      disabled={controlsBusy}
                      onClick={() => reopenDevice(recovery)}
                    >
                      Reopen previous version
                    </button>
                  ))}
                <button
                  type="button"
                  disabled={
                    controlsBusy ||
                    item.id === localProject?.id ||
                    item.id === localProject?.recoveryOf
                  }
                  onClick={() => void removeDevice(item)}
                >
                  Remove device copy
                </button>
              </details>
            </li>
          ))}
        </ul>
        {!projects.length && (
          <div className="projects-empty">
            <span aria-hidden="true">≋</span>
            <strong>
              {projectQuery
                ? "No matching projects"
                : "Your practice, ready to return to."}
            </strong>
            <p>
              {projectQuery
                ? "Try another name."
                : "Open a song, find a phrase, and save your project here."}
            </p>
          </div>
        )}
        <div className="project-sidebar-footer">
          <button
            type="button"
            disabled={controlsBusy}
            onClick={() => projectInput.current?.click()}
          >
            Import project
          </button>
          <button
            type="button"
            disabled={controlsBusy}
            onClick={() => void run(refreshSaved)}
          >
            Refresh projects
          </button>
          <p>Saved on this device. Reopen with the original audio.</p>
        </div>
      </aside>
      <div className="practice-main">
        <header className="practice-heading">
          <div>
            <div className="eyebrow">PRACTICE SESSION</div>
            <h1>{snapshot?.project.name ?? "Make every replay count."}</h1>
            <p>
              {selectedSource?.name ??
                "Open a song. Slow it down. Find your phrase."}
            </p>
          </div>
          <div className="practice-actions">
            <button
              type="button"
              disabled={controlsBusy}
              onClick={() => sourceInput.current?.click()}
            >
              Open song
            </button>
            <button
              className="save-project-button"
              type="button"
              disabled={controlsBusy || !snapshot || !facade.current?.saveLocal}
              onClick={() => void saveDevice()}
            >
              Save project
            </button>
            {snapshot && (
              <details className="session-menu">
                <summary aria-label="Session options">•••</summary>
                <button
                  type="button"
                  disabled={controlsBusy}
                  onClick={() => void exportProject()}
                >
                  Download project
                </button>
                {facade.current?.saveLocal && (
                  <button
                    type="button"
                    disabled={controlsBusy}
                    onClick={() => void saveDevice(true)}
                  >
                    Save a new device copy
                  </button>
                )}
                <button type="button" disabled={controlsBusy} onClick={onClose}>
                  Close song
                </button>
              </details>
            )}
          </div>
        </header>
        <input
          className="visually-hidden"
          ref={sourceInput}
          type="file"
          accept=".wav,.wave,.mp3,.flac,.m4a,.opus,.ogg,.oga,audio/wav,audio/mpeg,audio/flac,audio/mp4,audio/ogg,audio/opus"
          onChange={(event) => {
            chooseSource(event.target.files?.[0]);
            event.target.value = "";
          }}
        />
        <input
          className="visually-hidden"
          ref={projectInput}
          type="file"
          accept=".json,application/json"
          onChange={(event) => {
            loadProject(event.target.files?.[0]);
            event.target.value = "";
          }}
        />
        <input
          className="visually-hidden"
          ref={localSourceInput}
          type="file"
          accept=".wav,.wave,.mp3,.flac,.m4a,.opus,.ogg,.oga"
          onChange={(event) => {
            const file = event.target.files?.[0];
            const local = pendingLocalProject.current;
            pendingLocalProject.current = undefined;
            if (file && local) void run(() => open(file, undefined, local));
            event.target.value = "";
          }}
        />
        {error && (
          <p role="alert" className="practice-error">
            {error}
          </p>
        )}
        <div className="session-status">
          <p role="status">{busy ? busyLabel : status}</p>
          {busy && (
            <button type="button" onClick={() => facade.current?.cancel()}>
              Cancel operation
            </button>
          )}
          {busy && previewControls && (
            <button
              type="button"
              disabled={previewControlBusy}
              onClick={() => void togglePreview()}
            >
              {previewPaused ? "Resume preview" : "Pause preview"}
            </button>
          )}
        </div>
        <section className="waveform-player" aria-label="Song player">
          <div className="waveform-toolbar">
            <span className="waveform-caption">
              {detailView ? "SELECTION DETAIL" : "TRACK WAVEFORM"}
            </span>
            <div className="practice-actions">
              <button
                type="button"
                disabled={controlsBusy || !snapshot}
                aria-pressed={!detailView}
                onClick={() => void overview()}
              >
                Full track
              </button>
              <button
                type="button"
                disabled={controlsBusy || !snapshot}
                aria-pressed={detailView}
                onClick={() => void zoom()}
              >
                Zoom to loop
              </button>
            </div>
          </div>
          {wave && snapshot ? (
            <PracticeWaveform
              wave={wave}
              start={Number(start)}
              end={Number(end)}
              detail={detailView}
              disabled={busy}
              position={position}
              markers={annotations.markers}
              onSelect={selectBoundary}
              onRange={(a, b) => selectLoop(a, b)}
            />
          ) : (
            <div className="waveform-empty">
              <div className="empty-wave" aria-hidden="true">
                {Array.from({ length: 72 }, (_, i) => (
                  <i
                    key={i}
                    style={{
                      height: `${12 + Math.sin(i * 0.7) ** 2 * 46 + Math.sin(i * 0.21) ** 2 * 34}%`,
                    }}
                  />
                ))}
              </div>
              <strong>
                {busy
                  ? "Preparing your waveform…"
                  : "A little slower. A lot clearer."}
              </strong>
              <p>
                {busy
                  ? "Your audio is being opened on this device."
                  : "Your song’s waveform will appear here."}
              </p>
              {!busy && (
                <button
                  type="button"
                  onClick={() =>
                    void run(async () => {
                      const response = await fetch(appPath('practice/demo.mp3'));
                      if (!response.ok)
                        throw new Error("The demo could not be loaded.");
                      const file = new File(
                        [await response.blob()],
                        "demo.mp3",
                        { type: "audio/mpeg" },
                      );
                      await open(file);
                    }, "Opening demo…")
                  }
                >
                  Try a demo song
                </button>
              )}
            </div>
          )}
          <div className="player-transport">
            <div className="transport-buttons">
              <button
                type="button"
                title="Restart"
                aria-label="Restart"
                disabled={
                  controlsBusy ||
                  !snapshot?.capabilities.liveTransport ||
                  snapshot.transport?.state === "error"
                }
                aria-keyshortcuts={shortcutAttribute("restart")}
                onClick={() => void transport({ type: "restart" })}
              >
                |◀
              </button>
              <button
                className="transport-play"
                type="button"
                aria-label={
                  snapshot?.transport?.state === "playing"
                    ? "Pause playback"
                    : "Play"
                }
                aria-keyshortcuts={shortcutAttribute("play_pause")}
                disabled={controlsBusy || !canPlay}
                onClick={() => void togglePlayback()}
              >
                {snapshot?.transport?.state === "playing" ? "Ⅱ" : "▶"}
              </button>
              <button
                type="button"
                aria-label="Stop"
                aria-keyshortcuts={shortcutAttribute("stop")}
                disabled={
                  controlsBusy ||
                  !snapshot?.capabilities.liveTransport ||
                  snapshot.transport?.state === "error"
                }
                onClick={() => void transport({ type: "stop" })}
              >
                ■
              </button>
            </div>
            <div className="player-time" aria-label="Playback position">
              <strong>{time(position)}</strong>
              <span>/ {time(duration)}</span>
            </div>
            <div className="transport-secondary">
              <button
                type="button"
                disabled={controlsBusy || !playableAction}
                onClick={() => void transport({ type: "seek_backward" })}
                aria-label="Seek back one source second"
              >
                −1s
              </button>
              <button
                type="button"
                disabled={controlsBusy || !playableAction}
                onClick={() => void transport({ type: "seek_forward" })}
                aria-label="Seek forward one source second"
              >
                +1s
              </button>
              <button
                type="button"
                disabled={busy || !snapshot}
                aria-pressed={loopEnabled}
                onClick={() =>
                  selectLoop(Number(start), Number(end), !loopEnabled)
                }
              >
                ↻ Loop
              </button>
              <span className="loop-count">
                {snapshot?.transport?.completedLoops ?? 0} repeats
              </span>
            </div>
          </div>
          {snapshot && (
            <input
              className="track-seek"
              type="range"
              min="0"
              max={duration}
              step={1 / snapshot.project.track.audio.sampleRate}
              value={Math.min(position, duration)}
              aria-label="Seek through track"
              disabled={busy || !playableAction}
              onChange={(event) =>
                void transport({
                  type: "seek",
                  position: Math.round(
                    Number(event.target.value) *
                      snapshot.project.track.audio.sampleRate,
                  ),
                })
              }
            />
          )}
        </section>
        <div className="live-controls" aria-label="Playback settings">
          <section className="control-panel speed-panel">
            <div className="control-heading">
              <h2>Speed</h2>
              <span className="live-indicator">LIVE</span>
            </div>
            <div className="control-value">
              <input
                aria-label="Speed (%)"
                type="number"
                min="25"
                max="200"
                step="0.1"
                value={speed}
                onBlur={(event) => {
                  if (!event.target.validity.valid || !event.target.value)
                    setSpeed(
                      String(
                        Number(
                          (
                            (accepted.current?.project.settings.speed ?? 1) *
                            100
                          ).toPrecision(15),
                        ),
                      ),
                    );
                }}
                disabled={busy || !snapshot}
                onChange={(event) =>
                  changeParameters(event.target.value, pitch)
                }
              />
              <span>%</span>
            </div>
            <input
              aria-label="Speed slider"
              type="range"
              min="25"
              max="200"
              step="1"
              value={Number(speed) || 100}
              disabled={busy || !snapshot}
              onChange={(event) => changeParameters(event.target.value, pitch)}
            />
            <div className="speed-presets">
              {[25, 50, 75, 100].map((value) => (
                <button
                  key={value}
                  type="button"
                  disabled={busy || !snapshot}
                  aria-pressed={Number(speed) === value}
                  onClick={() => changeParameters(String(value), pitch)}
                >
                  {value}%
                </button>
              ))}
            </div>
          </section>
          <section className="control-panel pitch-panel">
            <div className="control-heading">
              <h2>Pitch</h2>
              <span className="control-unit">SEMITONES</span>
            </div>
            <div className="control-value">
              <button
                type="button"
                aria-label="Decrease pitch"
                disabled={busy || !snapshot || Number(pitch) <= -12}
                onClick={() =>
                  changeParameters(
                    speed,
                    String(Math.max(-12, Number(pitch) - 1)),
                  )
                }
              >
                −
              </button>
              <input
                aria-label="Pitch (semitones)"
                type="number"
                min="-12"
                max="12"
                step="0.1"
                value={pitch}
                onBlur={(event) => {
                  if (!event.target.validity.valid || !event.target.value)
                    setPitch(
                      String(accepted.current?.project.settings.pitch ?? 0),
                    );
                }}
                disabled={busy || !snapshot}
                onChange={(event) =>
                  changeParameters(speed, event.target.value)
                }
              />
              <button
                type="button"
                aria-label="Increase pitch"
                disabled={busy || !snapshot || Number(pitch) >= 12}
                onClick={() =>
                  changeParameters(
                    speed,
                    String(Math.min(12, Number(pitch) + 1)),
                  )
                }
              >
                ＋
              </button>
            </div>
            <input
              aria-label="Pitch slider"
              type="range"
              min="-12"
              max="12"
              step="0.1"
              value={Number(pitch) || 0}
              disabled={busy || !snapshot}
              onChange={(event) => changeParameters(speed, event.target.value)}
            />
            <button
              className="pitch-reset"
              type="button"
              disabled={busy || !snapshot}
              onClick={() => changeParameters(speed, "0")}
            >
              Original pitch
            </button>
          </section>
          <section className="control-panel loop-panel">
            <div className="control-heading">
              <h2>A / B loop</h2>
              <span className="control-unit">
                {loopEnabled ? "REPEATING" : "OFF"}
              </span>
            </div>
            <div className="loop-boundaries">
              <label>
                <span>A · Start</span>
                <input
                  aria-label="Selection start (source seconds)"
                  type="number"
                  min="0"
                  max={duration}
                  step={
                    snapshot
                      ? 1 / snapshot.project.track.audio.sampleRate
                      : 0.01
                  }
                  value={start}
                  disabled={busy || !snapshot}
                  onChange={(event) => {
                    setStart(event.target.value);
                    if (event.target.value !== "")
                      selectBoundary("start", Number(event.target.value));
                  }}
                />
              </label>
              <label>
                <span>B · End</span>
                <input
                  aria-label="Selection end (source seconds)"
                  type="number"
                  min="0"
                  max={duration}
                  step={
                    snapshot
                      ? 1 / snapshot.project.track.audio.sampleRate
                      : 0.01
                  }
                  value={end}
                  disabled={busy || !snapshot}
                  onChange={(event) => {
                    setEnd(event.target.value);
                    if (event.target.value !== "")
                      selectBoundary("end", Number(event.target.value));
                  }}
                />
              </label>
            </div>
            <div className="practice-actions">
              <button
                type="button"
                disabled={controlsBusy || !playableAction}
                onClick={() => void transport({ type: "set_a" })}
              >
                Set A here
              </button>
              <button
                type="button"
                disabled={controlsBusy || !playableAction}
                onClick={() => void transport({ type: "set_b" })}
              >
                Set B here
              </button>
            </div>
            <p>Drag across the waveform to loop a phrase.</p>
          </section>
        </div>
        {snapshot && (
          <>
            {!canPlay && (
              <p className="practice-capability">
                {snapshot.transport?.state === "error"
                  ? "Playback was interrupted."
                  : "Live playback requires a browser with audio processing support."}
                {snapshot.transport?.state === "error" && (
                  <button
                    type="button"
                    disabled={controlsBusy}
                    onClick={() => void transport({ type: "recover" })}
                  >
                    Recover playback
                  </button>
                )}
                {snapshot.capabilities.streamPreview &&
                  !snapshot.capabilities.liveTransport && (
                    <button
                      type="button"
                      disabled={controlsBusy || !facade.current?.preview}
                      onClick={() => void preview()}
                    >
                      Preview selection
                    </button>
                  )}
              </p>
            )}
            <section className="phrases-panel" aria-label="Saved phrases">
              <div className="control-heading">
                <h2>Saved phrases</h2>
                <span>{annotations.regions?.length ?? 0}</span>
              </div>
              <ul className="practice-regions">
                {annotations.regions?.map((region) => (
                  <li key={region.id}>
                    <button
                      type="button"
                      disabled={busy}
                      aria-pressed={selectedRegion === region.id}
                      onClick={() => {
                        setSelectedRegion(region.id);
                        selectLoop(region.start, region.end);
                        setLabel(region.label);
                        setPhraseNotes(region.notes ?? "");
                      }}
                    >
                      <strong>{region.label}</strong>
                      <span>
                        {time(region.start)} — {time(region.end)}
                      </span>
                    </button>
                  </li>
                ))}
              </ul>
              <div className="practice-form">
                <label>
                  Phrase name
                  <input
                    value={label}
                    onChange={(event) => setLabel(event.target.value)}
                    disabled={controlsBusy}
                  />
                </label>
                <button
                  type="button"
                  disabled={controlsBusy}
                  onClick={savePhrase}
                >
                  Save named phrase / loop
                </button>
                {annotations.regions?.some(
                  (region) => region.id === selectedRegion,
                ) && (
                  <>
                    <button
                      type="button"
                      disabled={controlsBusy}
                      onClick={updatePhrase}
                    >
                      Update selected phrase
                    </button>
                    <button
                      type="button"
                      disabled={controlsBusy}
                      onClick={deletePhrase}
                    >
                      Remove selected phrase
                    </button>
                  </>
                )}
              </div>
            </section>
            <details className="session-detail">
              <summary>Notes &amp; markers</summary>
              <label className="practice-notes">
                Phrase notes
                <textarea
                  value={phraseNotes}
                  onChange={(event) => setPhraseNotes(event.target.value)}
                  disabled={controlsBusy}
                />
              </label>
              <label className="practice-notes">
                Project notes
                <textarea
                  value={notes}
                  onChange={(event) => setNotes(event.target.value)}
                  disabled={controlsBusy}
                />
              </label>
              <button
                type="button"
                disabled={controlsBusy}
                onClick={() =>
                  void edit({
                    type: "set_annotations",
                    annotations: { ...annotations, notes },
                  })
                }
              >
                Save project notes
              </button>
              <div className="practice-form">
                <label>
                  Marker name
                  <input
                    value={markerLabel}
                    onChange={(event) => setMarkerLabel(event.target.value)}
                    disabled={controlsBusy}
                  />
                </label>
                <label>
                  Marker kind
                  <select
                    value={markerKind}
                    disabled={controlsBusy}
                    onChange={(event) =>
                      setMarkerKind(
                        event.target.value as "section" | "measure" | "beat",
                      )
                    }
                  >
                    <option value="section">Section</option>
                    <option value="measure">Measure</option>
                    <option value="beat">Beat</option>
                  </select>
                </label>
                {markerKind !== "section" && (
                  <label>
                    Marker measure
                    <input
                      type="number"
                      step="1"
                      value={markerMeasure}
                      disabled={controlsBusy}
                      onChange={(event) => setMarkerMeasure(event.target.value)}
                    />
                  </label>
                )}
                {markerKind === "beat" && (
                  <label>
                    Marker beat
                    <input
                      type="number"
                      step="1"
                      value={markerBeat}
                      disabled={controlsBusy}
                      onChange={(event) => setMarkerBeat(event.target.value)}
                    />
                  </label>
                )}
                <button
                  type="button"
                  disabled={controlsBusy}
                  onClick={() =>
                    void edit({
                      type: "set_annotations",
                      annotations: {
                        ...annotations,
                        markers: [
                          ...(annotations.markers ?? []),
                          {
                            id: `marker-${crypto.randomUUID()}`,
                            kind: markerKind,
                            label: markerLabel,
                            position: position,
                            ...(markerKind !== "section"
                              ? { measure: Number(markerMeasure) }
                              : {}),
                            ...(markerKind === "beat"
                              ? { beat: Number(markerBeat) }
                              : {}),
                          },
                        ],
                      },
                    })
                  }
                >
                  Add marker here
                </button>
              </div>
              <ul className="practice-regions" aria-label="Saved markers">
                {annotations.markers?.map((marker) => (
                  <li key={marker.id}>
                    <span>
                      {marker.label} · {time(marker.position)}
                    </span>
                    <button
                      type="button"
                      disabled={controlsBusy}
                      aria-label={`Remove marker ${marker.label}`}
                      onClick={() =>
                        void edit({
                          type: "set_annotations",
                          annotations: {
                            ...annotations,
                            markers: annotations.markers?.filter(
                              (item) => item.id !== marker.id,
                            ),
                          },
                        })
                      }
                    >
                      Remove marker
                    </button>
                  </li>
                ))}
              </ul>
            </details>
            <details className="session-detail">
              <summary>Training plans &amp; timing</summary>
              <PracticePlans
                key={sourceSession}
                snapshot={snapshot}
                busy={controlsBusy}
                liveAvailable={playableAction}
                edit={edit}
                transport={transport}
              />
            </details>
            <details className="session-detail">
              <summary>More playback controls</summary>
              <div className="practice-actions" aria-label="Practice actions">
                {(
                  Object.keys(
                    practiceActionLabels,
                  ) as (keyof typeof practiceActionLabels)[]
                ).map((action) => (
                  <button
                    type="button"
                    key={action}
                    aria-keyshortcuts={shortcutAttribute(action)}
                    disabled={controlsBusy || !playableAction}
                    onClick={() => void transport({ type: action })}
                  >
                    {practiceActionLabels[action]}
                  </button>
                ))}
                {snapshot.capabilities.liveTransport && (
                  <button
                    type="button"
                    disabled={controlsBusy}
                    onClick={() => void releasePlayback()}
                  >
                    Close playback
                  </button>
                )}
              </div>
              <div className="practice-form">
                <label>
                  Seek (source seconds)
                  <input
                    type="number"
                    value={seekSeconds}
                    disabled={controlsBusy}
                    onChange={(event) => setSeekSeconds(event.target.value)}
                  />
                </label>
                <button
                  type="button"
                  disabled={controlsBusy || !playableAction}
                  onClick={() =>
                    void transport({
                      type: "seek",
                      position: Math.round(
                        Number(seekSeconds) *
                          snapshot.project.track.audio.sampleRate,
                      ),
                    })
                  }
                >
                  Seek
                </button>
              </div>
            </details>
          </>
        )}
        <details className="session-detail practice-shortcuts">
          <summary>
            Keyboard shortcuts <span>Space to play / pause</span>
          </summary>
          <label>
            <input
              type="checkbox"
              checked={shortcutDraft.enabled}
              onChange={(event) =>
                setShortcutDraft({
                  ...shortcutDraft,
                  enabled: event.target.checked,
                })
              }
            />{" "}
            Enable practice shortcuts
          </label>
          <div className="practice-form">
            {shortcutActions.map((action) => (
              <label key={action}>
                {action === "play_pause"
                  ? "Play / pause shortcut"
                  : action === "restart"
                    ? "Restart shortcut"
                    : action === "stop"
                      ? "Stop shortcut"
                      : `${practiceActionLabels[action]} shortcut`}
                <select
                  value={shortcutDraft.bindings[action]}
                  onChange={(event) =>
                    setShortcutDraft({
                      ...shortcutDraft,
                      bindings: {
                        ...shortcutDraft.bindings,
                        [action]: event.target.value,
                      },
                    })
                  }
                >
                  {shortcutKeys.map((key) => (
                    <option key={key} value={key}>
                      {key || "No shortcut"}
                    </option>
                  ))}
                </select>
              </label>
            ))}
          </div>
          <button type="button" onClick={applyShortcuts}>
            Save shortcuts
          </button>
          {shortcutWarning && (
            <p role="alert" className="practice-error">
              {shortcutWarning}
            </p>
          )}
        </details>
      </div>
    </section>
  );
}
