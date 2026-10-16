namespace Riffine.Windows;

// Keep the file dialog and recursive folder scan on the same format list.
internal static class MusicLibrary
{
    internal static readonly string[] Extensions = [".wav", ".wave", ".flac", ".mp3", ".m4a"];
    private static readonly HashSet<string> OtherAudioExtensions = new(StringComparer.OrdinalIgnoreCase)
    {
        ".aac", ".aif", ".aiff", ".ape", ".m4b", ".m4p", ".ogg", ".oga", ".opus", ".wma", ".wv"
    };

    internal sealed record ScanResult(List<LibrarySong> Songs, int UnsupportedAudio);

    internal static ScanResult Scan(string directory, CancellationToken token)
    {
        var songs = new List<LibrarySong>();
        var unsupported = 0;
        var options = new EnumerationOptions { RecurseSubdirectories = true, IgnoreInaccessible = true, AttributesToSkip = FileAttributes.ReparsePoint };
        foreach (var path in Directory.EnumerateFiles(directory, "*", options))
        {
            token.ThrowIfCancellationRequested();
            var extension = Path.GetExtension(path);
            if (!Extensions.Contains(extension, StringComparer.OrdinalIgnoreCase))
            {
                if (OtherAudioExtensions.Contains(extension)) unsupported++;
                continue;
            }
            if (songs.Count == 100000) throw new IOException("This library is too large. Choose a smaller music folder.");
            songs.Add(new LibrarySong(path));
        }
        songs.Sort((a, b) => StringComparer.CurrentCultureIgnoreCase.Compare(a.Title, b.Title));
        return new ScanResult(songs, unsupported);
    }

    internal static string Status(ScanResult result)
    {
        if (result.Songs.Count == 0)
            return "No supported music files found. Choose a folder with WAV, MP3, FLAC or M4A (AAC/Apple Lossless) files." +
                (result.UnsupportedAudio > 0 ? $" Skipped {result.UnsupportedAudio} audio file(s) in unsupported formats." : "");
        return $"{result.Songs.Count} music files found. Choose a song to open it." +
            (result.UnsupportedAudio > 0 ? $" Skipped {result.UnsupportedAudio} audio file(s) in unsupported formats; supported: WAV, MP3, FLAC and M4A." : "");
    }
}
