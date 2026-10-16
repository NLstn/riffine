namespace Riffine.Windows;

// These are presentation DTOs for the versioned Go bridge, not product rules.
public sealed record CoreError(int Version, string Code, string Message);
public sealed record CoreEnvelope(int Version, ulong Id, CoreReply Response);
public sealed record CoreReply(
    int Version,
    SessionSnapshot? Snapshot = null,
    WaveformWindow? Window = null,
    CoreError? Error = null,
    string[]? Formats = null,
    bool StreamPreview = false);
public sealed record SessionSnapshot(int Version, ulong Sequence, string State, PracticeProject Project, Capabilities Capabilities);
public sealed record Capabilities(string[] Formats, bool OfflineRender, bool LiveTransport, bool StreamPreview);
public sealed record PracticeProject(int Version, string Name, ProjectTrack Track, PracticeSettings Settings);
public sealed record ProjectTrack(string Path, AudioInfo Audio);
public sealed record AudioInfo(string Encoding, int SampleRate, int Channels, long Frames, string SHA256)
{
    public double Duration => (double)Frames / SampleRate;
}
public sealed record PracticeSettings(double Speed, double Pitch, PracticeLoop Loop);
public sealed record PracticeLoop(bool Enabled, double Start, double End);
public sealed record WaveformWindow(int Version, long RequestedStart, long RequestedEnd, WaveformCell[] Cells);
public sealed record WaveformCell(long StartFrame, long EndFrame, WaveformPeak[] Channels);
public sealed record WaveformPeak(double Min, double Max);
public sealed record LibrarySong(string Path)
{
    public string Title => System.IO.Path.GetFileNameWithoutExtension(Path);
    public string Detail => System.IO.Path.GetFileName(System.IO.Path.GetDirectoryName(Path)) + " · " + System.IO.Path.GetExtension(Path).TrimStart('.').ToUpperInvariant();
}
