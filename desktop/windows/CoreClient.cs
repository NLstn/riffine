using System.Diagnostics;
using System.Text;
using System.Text.Json;

namespace Riffine.Windows;

// One private Go process owns the session. Cancellation travels on its input
// pipe, and the final reply is drained before the next operation is allowed.
public sealed class CoreClient : IDisposable
{
    private const int MaxResponseChars = 8 * 1024 * 1024;
    private static readonly JsonSerializerOptions Json = new(JsonSerializerDefaults.Web);
    private readonly SemaphoreSlim operations = new(1, 1);
    private readonly SemaphoreSlim writes = new(1, 1);
    private readonly StringBuilder diagnostics = new();
    private readonly string executableDirectory;
    private readonly char[] responseBuffer = new char[8192];
    private int readOffset, readCount;
    private Process? process;
    private ulong nextId;
    private bool disposed;

    public CoreClient(string? executableDirectory = null) => this.executableDirectory = executableDirectory ?? AppContext.BaseDirectory;

    public async Task<CoreReply> SendAsync(object request, CancellationToken cancellation = default)
    {
        await operations.WaitAsync(cancellation);
        try
        {
            ObjectDisposedException.ThrowIf(disposed, this);
            EnsureProcess();
            cancellation.ThrowIfCancellationRequested();
            var id = checked(++nextId);
            await WriteAsync(new { version = 1, id, request });
            // Read the cancelled operation's reply rather than abandoning the
            // stream and accidentally consuming it as the next operation's reply.
            using var registration = cancellation.Register(() => _ = CancelAsync(id));
            string line;
            try { line = await ReadBoundedLineAsync(process!.StandardOutput); }
            catch
            {
                Terminate();
                throw;
            }
            CoreEnvelope reply;
            try
            {
                reply = JsonSerializer.Deserialize<CoreEnvelope>(line, Json)
                    ?? throw new IOException("The practice engine returned an empty response.");
            }
            catch { Terminate(); throw; }
            if (reply.Version != 1 || reply.Id != id || reply.Response.Version != 1)
            {
                Terminate();
                throw new IOException("The practice engine returned an incompatible response.");
            }
            if (reply.Response.Error is { } error)
            {
                if (error.Code == "cancelled") throw new OperationCanceledException(error.Message, cancellation);
                throw new IOException(error.Message);
            }
            return reply.Response;
        }
        finally { operations.Release(); }
    }

    private void EnsureProcess()
    {
        if (process is not null)
        {
            if (process.HasExited) throw new IOException("The practice engine stopped. Restart Riffine to reconnect.");
            return;
        }
        var executable = Path.Combine(executableDirectory, "riffine-desktop.exe");
        if (!File.Exists(executable)) throw new FileNotFoundException("The practice engine is missing. Reinstall Riffine or run scripts/build-windows.ps1.", executable);
        var start = new ProcessStartInfo(executable)
        {
            UseShellExecute = false,
            CreateNoWindow = true,
            RedirectStandardInput = true,
            RedirectStandardOutput = true,
            RedirectStandardError = true,
            StandardInputEncoding = new UTF8Encoding(false),
            StandardOutputEncoding = Encoding.UTF8,
            StandardErrorEncoding = Encoding.UTF8,
            WorkingDirectory = executableDirectory
        };
        // Only the helper shipped with this app may be launched by the UI host.
        start.Environment["RIFFINE_AUDIO_HELPER"] = Path.Combine(executableDirectory, "riffine-audio.exe");
        process = new Process { StartInfo = start };
        process.ErrorDataReceived += (_, e) =>
        {
            if (e.Data is null) return;
            lock (diagnostics)
            {
                diagnostics.AppendLine(e.Data);
                if (diagnostics.Length > 4096) diagnostics.Remove(0, diagnostics.Length - 4096);
            }
        };
        process.Start();
        process.BeginErrorReadLine();
    }

    private async Task WriteAsync(object message)
    {
        await writes.WaitAsync();
        try
        {
            await process!.StandardInput.WriteLineAsync(JsonSerializer.Serialize(message, Json));
            await process.StandardInput.FlushAsync();
        }
        finally { writes.Release(); }
    }

    private async Task CancelAsync(ulong id)
    {
        try { await WriteAsync(new { version = 1, id, cancel = true }); }
        catch (Exception) { Terminate(); }
    }

    private async Task<string> ReadBoundedLineAsync(StreamReader reader)
    {
        // StreamReader.ReadLineAsync has no length bound. Read chunks while
        // retaining at most the protocol limit, rejecting trailing partial lines.
        var line = new StringBuilder();
        while (true)
        {
            if (readOffset == readCount)
            {
                readCount = await reader.ReadAsync(responseBuffer.AsMemory());
                readOffset = 0;
                if (readCount == 0) throw new IOException("The practice engine disconnected. Restart Riffine to reconnect.");
            }
            var end = Array.IndexOf(responseBuffer, '\n', readOffset, readCount - readOffset);
            var length = (end < 0 ? readCount : end) - readOffset;
            if (line.Length + length > MaxResponseChars) throw new IOException("The practice engine response exceeded its limit.");
            line.Append(responseBuffer, readOffset, length);
            readOffset += length;
            if (end >= 0) { readOffset++; return line.ToString(); }
        }
    }

    private void Terminate()
    {
        try { if (process is { HasExited: false }) process.Kill(entireProcessTree: true); }
        catch (InvalidOperationException) { }
        catch (System.ComponentModel.Win32Exception) { }
    }

    public void Dispose()
    {
        if (disposed) return;
        disposed = true;
        if (process is null) return;
        try
        {
            // EOF asks Go to cancel/join the owned audio worker before exiting.
            process.StandardInput.Close();
            if (!process.WaitForExit(2000)) Terminate();
        }
        catch (InvalidOperationException) { }
        catch (IOException) { Terminate(); }
        finally { process.Dispose(); }
    }
}
