using Microsoft.UI.Xaml;

namespace Riffine.Windows;

public partial class App : Application
{
    private MainWindow? window;

    public App() => InitializeComponent();

    protected override void OnLaunched(LaunchActivatedEventArgs args)
    {
        window = new MainWindow();
        // Windows CI uses an actual native window/resource load before it
        // accepts an installer. This path does not open audio or user files.
        var arguments = Environment.GetCommandLineArgs();
        var smoke = Array.IndexOf(arguments, "--smoke-report");
        if (smoke >= 0 && smoke + 1 < arguments.Length)
        {
            ((FrameworkElement)window.Content).Loaded += (_, _) =>
            {
                File.WriteAllText(arguments[smoke + 1], "{\"winui\":\"loaded\",\"framework\":\"WinUI 3\"}");
                Exit();
            };
        }
        window.Activate();
    }
}
