using Riffine.Windows;

// Actual C# <-> Go process integration. No WinUI or audio device is needed.
var directory = Path.Combine(Path.GetTempPath(), "riffine-bridge-" + Guid.NewGuid());
Directory.CreateDirectory(directory);
try
{
    var libraryFolder = Path.Combine(directory, "library");
    Directory.CreateDirectory(Path.Combine(libraryFolder, "nested"));
    foreach (var name in new[] { "song.M4A", "song.mp3", "nested/lossless.m4a", "unsupported.opus", "notes.txt" })
        File.WriteAllText(Path.Combine(libraryFolder, name), "fixture");
    var scanned = MusicLibrary.Scan(libraryFolder, CancellationToken.None);
    Require(scanned.Songs.Count == 3 && scanned.UnsupportedAudio == 1, "recursive M4A library scan and unsupported count");
    Require(MusicLibrary.Status(scanned).Contains("Skipped 1"), "mixed-folder unsupported message");
    foreach (var path in Directory.EnumerateFiles(libraryFolder, "*", SearchOption.AllDirectories))
        if (MusicLibrary.Extensions.Contains(Path.GetExtension(path), StringComparer.OrdinalIgnoreCase)) File.Delete(path);
    scanned = MusicLibrary.Scan(libraryFolder, CancellationToken.None);
    Require(scanned.Songs.Count == 0 && MusicLibrary.Status(scanned).Contains("M4A") && MusicLibrary.Status(scanned).Contains("Skipped 1"), "unsupported-only folder guidance");
    using (var cancelledScan = new CancellationTokenSource())
    {
        cancelledScan.Cancel();
        try { MusicLibrary.Scan(libraryFolder, cancelledScan.Token); throw new Exception("accepted cancelled folder scan"); }
        catch (OperationCanceledException) { }
    }
    var source = Path.Combine(directory, "音楽 original.wav");
    var document = Path.Combine(directory, "practice.json");
    using (var writer = new BinaryWriter(File.Create(source)))
    {
        writer.Write("RIFF"u8); writer.Write(36 + 8000); writer.Write("WAVEfmt "u8);
        writer.Write(16); writer.Write((short)1); writer.Write((short)1);
        writer.Write(8000); writer.Write(16000); writer.Write((short)2); writer.Write((short)16);
        writer.Write("data"u8); writer.Write(8000);
        for (var frame = 0; frame < 4000; frame++) writer.Write((short)(12000 * Math.Sin(frame * 440 * 2 * Math.PI / 8000)));
    }
    using var core = new CoreClient(args.Length >= 1 ? args[0] : null);
    var hello = await core.SendAsync(new { version = 1, type = "hello" });
    Require(hello.Formats?.Contains("wav-pcm") == true, "handshake format capabilities");
    if (args.Length >= 1)
        Require(hello.StreamPreview && hello.Formats!.Contains("flac") && hello.Formats.Contains("mp3") && hello.Formats.Contains("aac") && hello.Formats.Contains("alac"), "installed native audio helper capabilities");
    var imported = await core.SendAsync(new { version = 1, type = "import", source, document });
    Require(imported.Snapshot is { Sequence: 0 } && imported.Snapshot.Project.Track.Audio.Frames == 4000, "imported metadata");
    var changed = await core.SendAsync(new
    {
        version = 1, type = "command",
        command = new { version = 1, sequence = 1, type = "set_parameters", speed = .75, pitch = -2 }
    });
    Require(changed.Snapshot?.Project.Settings.Speed == .75, "parameter edit");
    var phrase = new PracticeLoop(true, .1, .4);
    var looped = await core.SendAsync(new
    {
        version = 1, type = "command",
        command = new { version = 1, sequence = 2, type = "set_loop", loop = phrase }
    });
    Require(looped.Snapshot?.Project.Settings.Loop == phrase, "source-time phrase edit");
    using (var cancelled = new CancellationTokenSource())
    {
        cancelled.Cancel();
        try { await core.SendAsync(new { version = 1, type = "snapshot" }, cancelled.Token); throw new Exception("accepted cancelled operation"); }
        catch (OperationCanceledException) { }
    }
    try
    {
        await core.SendAsync(new { version = 1, type = "open", document = Path.Combine(directory, "missing.json") });
        throw new Exception("accepted missing replacement");
    }
    catch (IOException) { }
    var retained = await core.SendAsync(new { version = 1, type = "snapshot" });
    Require(retained.Snapshot is { Sequence: 2 } && retained.Snapshot.Project.Settings.Pitch == -2, "failed replacement retained session");
    var window = await core.SendAsync(new { version = 1, type = "waveform", startFrame = 0, endFrame = 4000, bins = 40 });
    Require(window.Window is { Cells.Length: > 0 }, "native waveform response");
    await core.SendAsync(new { version = 1, type = "save_as", document });
    var reopened = await core.SendAsync(new { version = 1, type = "open", document });
    Require(reopened.Snapshot is { Sequence: 0 } && reopened.Snapshot.Project.Settings.Speed == .75, "saved project reopened");
    Require(reopened.Snapshot?.Project.Settings.Loop == phrase, "saved phrase reopened");
    if (args.Length >= 2)
    {
        foreach (var suffix in new[] { "m4a", "alac.m4a" })
        {
            var m4aSource = Path.Combine(directory, "音楽." + suffix);
            File.Copy(Path.Combine(args[1], "generated-stereo." + suffix), m4aSource);
            var m4aDocument = Path.Combine(directory, suffix + ".riffine.json");
            var m4a = await core.SendAsync(new { version = 1, type = "import", source = m4aSource, document = m4aDocument });
            var audio = m4a.Snapshot!.Project.Track.Audio;
            Require(audio.Encoding == (suffix == "m4a" ? "aac" : "alac") && audio.Channels == 2 && audio.SampleRate == 48000 && audio.Frames >= 48000, "M4A imported metadata");
            var peaks = await core.SendAsync(new { version = 1, type = "waveform", startFrame = 0, endFrame = audio.Frames, bins = 40 });
            Require(peaks.Window is { Cells.Length: > 0 }, "M4A waveform");
            await core.SendAsync(new { version = 1, type = "save_as", document = m4aDocument });
            var restored = await core.SendAsync(new { version = 1, type = "open", document = m4aDocument });
            Require(restored.Snapshot!.Project.Track.Audio.Encoding == audio.Encoding && restored.Snapshot.Project.Track.Audio.Frames == audio.Frames, "M4A saved project reopened");
        }
    }
    Console.WriteLine("C# / Go bridge passed: handshake, Unicode import, edits, cancellation, failed replacement, waveform, save and reopen.");
}
finally { Directory.Delete(directory, recursive: true); }

static void Require(bool condition, string operation)
{
    if (!condition) throw new Exception("Failed: " + operation);
}
