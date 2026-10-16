using Riffine.Windows;

// Actual C# <-> Go process integration. No WinUI or audio device is needed.
var directory = Path.Combine(Path.GetTempPath(), "riffine-bridge-" + Guid.NewGuid());
Directory.CreateDirectory(directory);
try
{
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
    using var core = new CoreClient(args.Length == 1 ? args[0] : null);
    var hello = await core.SendAsync(new { version = 1, type = "hello" });
    Require(hello.Formats?.Contains("wav-pcm") == true, "handshake format capabilities");
    var imported = await core.SendAsync(new { version = 1, type = "import", source, document });
    Require(imported.Snapshot is { Sequence: 0 } && imported.Snapshot.Project.Track.Audio.Frames == 4000, "imported metadata");
    var changed = await core.SendAsync(new
    {
        version = 1, type = "command",
        command = new { version = 1, sequence = 1, type = "set_parameters", speed = .75, pitch = -2 }
    });
    Require(changed.Snapshot?.Project.Settings.Speed == .75, "parameter edit");
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
    Require(retained.Snapshot is { Sequence: 1 } && retained.Snapshot.Project.Settings.Pitch == -2, "failed replacement retained session");
    var window = await core.SendAsync(new { version = 1, type = "waveform", startFrame = 0, endFrame = 4000, bins = 40 });
    Require(window.Window is { Cells.Length: > 0 }, "native waveform response");
    await core.SendAsync(new { version = 1, type = "save_as", document });
    var reopened = await core.SendAsync(new { version = 1, type = "open", document });
    Require(reopened.Snapshot is { Sequence: 0 } && reopened.Snapshot.Project.Settings.Speed == .75, "saved project reopened");
    Console.WriteLine("C# / Go bridge passed: handshake, Unicode import, edits, cancellation, failed replacement, waveform, save and reopen.");
}
finally { Directory.Delete(directory, recursive: true); }

static void Require(bool condition, string operation)
{
    if (!condition) throw new Exception("Failed: " + operation);
}
