using Microsoft.UI.Xaml;

namespace Riffine.Windows;

public partial class App : Application
{
    private MainWindow? window;
    private readonly string? smokeReport;

    public App()
    {
        var arguments = Environment.GetCommandLineArgs();
        var smoke = Array.IndexOf(arguments, "--smoke-report");
        smokeReport = smoke >= 0 && smoke + 1 < arguments.Length ? arguments[smoke + 1] : null;
        UnhandledException += (_, e) => RecordStartupFailure(e.Exception);
        try { InitializeComponent(); }
        catch (Exception error) { RecordStartupFailure(error); throw; }
    }

    protected override void OnLaunched(LaunchActivatedEventArgs args)
    {
        try { window = new MainWindow(); }
        catch (Exception error) { RecordStartupFailure(error); throw; }
        // Windows CI uses an actual native window/resource load before it
        // accepts an installer. This path does not open audio or user files.
        if (smokeReport is not null)
        {
            ((FrameworkElement)window.Content).Loaded += (_, _) =>
            {
                File.WriteAllText(smokeReport, "{\"winui\":\"loaded\",\"framework\":\"WinUI 3\"}");
                window.Close();
            };
        }
        window.Activate();
    }

    private void RecordStartupFailure(Exception error)
    {
        if (smokeReport is null) return;
        try { File.AppendAllText(smokeReport + ".error", error + Environment.NewLine); }
        catch (IOException) { }
    }
}
