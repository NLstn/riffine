using Microsoft.UI;
using Microsoft.UI.Xaml;
using Microsoft.UI.Xaml.Controls;
using Microsoft.UI.Xaml.Media;
using Windows.Graphics;
using Microsoft.Windows.Storage.Pickers;

namespace Riffine.Windows;

public sealed class MainWindow : Window
{
    private static readonly SolidColorBrush BackgroundBrush = Brush(18, 20, 17);
    private static readonly SolidColorBrush PanelBrush = Brush(26, 29, 24);
    private static readonly SolidColorBrush MutedBrush = Brush(152, 159, 145);
    private static readonly SolidColorBrush AccentBrush = Brush(200, 238, 145);
    private readonly CoreClient core = new();
    private readonly Grid root = new() { Background = BackgroundBrush, RequestedTheme = ElementTheme.Dark };
    private readonly TextBlock status = new() { Text = "Choose a song or a music folder to begin.", TextWrapping = TextWrapping.Wrap, Foreground = MutedBrush };
    private readonly TextBlock songTitle = new() { Text = "Find your next phrase.", FontSize = 30, FontWeight = Microsoft.UI.Text.FontWeights.SemiBold, TextWrapping = TextWrapping.Wrap };
    private readonly TextBlock songInfo = new() { Text = "Open a local song to start practicing.", Foreground = MutedBrush };
    private readonly TextBlock viewTimes = new() { Foreground = MutedBrush };
    private readonly TextBlock folderLabel = new() { Text = "Your music", TextTrimming = TextTrimming.CharacterEllipsis, Foreground = MutedBrush };
    private readonly TextBox search = new() { PlaceholderText = "Search songs…" };
    private readonly ListView songs = new() { SelectionMode = ListViewSelectionMode.Single, IsItemClickEnabled = true, MinHeight = 100, MaxHeight = 440 };
    private readonly StackPanel recentProjects = new() { Spacing = 4 };
    private readonly StackPanel fields = new() { Spacing = 18 };
    private readonly WaveformView waveform = new() { IsEnabled = false };
    private readonly NumberBox speed = Number("Speed ×", .25, 2, 1, .05);
    private readonly NumberBox pitch = Number("Pitch · semitones", -12, 12, 0, 1);
    private readonly NumberBox loopA = Number("A · seconds", 0, 0, 0, .1);
    private readonly NumberBox loopB = Number("B · seconds", 0, 0, 0, .1);
    private readonly CheckBox loopEnabled = new() { Content = "Use selected phrase" };
    private readonly Button openSong, openFolder, openProject, save, preview, apply, zoom, full;
    private readonly Button cancel = new() { Content = "Stop", Visibility = Visibility.Collapsed };
    private readonly ProgressRing progress = new() { Width = 18, Height = 18, IsActive = false, Visibility = Visibility.Collapsed };
    private readonly List<LibrarySong> library = [];
    private readonly List<string> recent = [];
    private readonly string dataDirectory = Path.Combine(Environment.GetFolderPath(Environment.SpecialFolder.LocalApplicationData), "Riffine");
    private SessionSnapshot? snapshot;
    private string? projectPath;
    private CancellationTokenSource? operation;
    private TaskCompletionSource? operationFinished;
    private bool dirty, updating, busy, closeApproved, closing, dialogOpen;
    private bool helloComplete;

    public MainWindow()
    {
        Title = "Riffine";
        AppWindow.Resize(new SizeInt32(1320, 900));
        openSong = Action("Open song", async () =>
        {
            if (!await ConfirmChangesAsync()) return;
            var picker = new FileOpenPicker(AppWindow.Id);
            foreach (var extension in MusicLibrary.Extensions) picker.FileTypeFilter.Add(extension);
            if (await picker.PickSingleFileAsync() is { } file) await ImportAsync(file.Path);
        });
        openFolder = Action("Choose music folder", ChooseFolderAsync);
        openProject = Action("Open project", async () =>
        {
            if (!await ConfirmChangesAsync()) return;
            var picker = new FileOpenPicker(AppWindow.Id);
            picker.FileTypeFilter.Add(".json");
            if (await picker.PickSingleFileAsync() is { } file) await OpenProjectAsync(file.Path);
        });
        save = Action("Save project", () => RunAsync(async token => { await SaveProjectAsync(token); }, "Saving project…"));
        preview = Action("Preview once", () => RunAsync(async token =>
        {
            await CommitSettingsAsync(token);
            await core.SendAsync(new { version = 1, type = "preview" }, token);
            status.Text = "Preview finished.";
        }, "Previewing the selection…"));
        // Theme styles may not be materialized before the first window loads.
        // Local colors give the primary action its accent without that lookup.
        preview.Background = AccentBrush;
        preview.Foreground = BackgroundBrush;
        apply = Action("Apply settings", () => RunAsync(async token =>
        {
            await CommitSettingsAsync(token);
            status.Text = "Practice settings applied.";
        }, "Applying settings…"));
        zoom = Action("Zoom to phrase", () => RunAsync(token => LoadWaveformAsync(token, loopA.Value, loopB.Value), "Loading waveform…"));
        full = Action("Show full song", () => RunAsync(token => LoadWaveformAsync(token), "Loading waveform…"));
        cancel.Click += (_, _) => operation?.Cancel();
        search.TextChanged += (_, _) => RenderLibrary();
        songs.ItemClick += async (_, e) =>
        {
            if (e.ClickedItem is ListViewItem { Tag: LibrarySong song } && await ConfirmChangesAsync()) await ImportAsync(song.Path);
        };
        foreach (var field in new[] { speed, pitch, loopA, loopB }) field.ValueChanged += (_, _) => SettingsChanged();
        loopEnabled.Checked += (_, _) => SettingsChanged();
        loopEnabled.Unchecked += (_, _) => SettingsChanged();
        waveform.SelectionChanged += (a, b) => { loopA.Value = a; loopB.Value = b; loopEnabled.IsChecked = true; };
        BuildView();
        Content = root;
        ReadRecent();
        RenderRecent();
        UpdateEnabled();
        AppWindow.Closing += async (_, e) =>
        {
            if (closeApproved) return;
            if (!busy && !dirty && !dialogOpen) { closeApproved = true; return; }
            e.Cancel = true;
            if (closing || dialogOpen) return;
            closing = true;
            try
            {
                operation?.Cancel();
                if (operationFinished is { } pending) await pending.Task;
                if (await ConfirmChangesAsync())
                {
                    closeApproved = true;
                    // Finish the current Closing event before asking WinUI to
                    // close again after an asynchronous save/discard decision.
                    DispatcherQueue.TryEnqueue(() => Close());
                }
            }
            finally { closing = false; }
        };
        Closed += (_, _) => { operation?.Cancel(); core.Dispose(); };
    }

    private void BuildView()
    {
        root.RowDefinitions.Add(new RowDefinition { Height = GridLength.Auto });
        root.RowDefinitions.Add(new RowDefinition());
        root.RowDefinitions.Add(new RowDefinition { Height = GridLength.Auto });
        var header = new Grid { Padding = new Thickness(28, 20, 28, 20), Background = PanelBrush, ColumnSpacing = 24 };
        header.ColumnDefinitions.Add(new ColumnDefinition());
        header.ColumnDefinitions.Add(new ColumnDefinition { Width = GridLength.Auto });
        header.Children.Add(new TextBlock { Text = "riffine.", FontSize = 27, Foreground = AccentBrush, FontWeight = Microsoft.UI.Text.FontWeights.Bold });
        var local = new TextBlock { Text = "●  Audio stays on your device", Foreground = MutedBrush, VerticalAlignment = VerticalAlignment.Center };
        Grid.SetColumn(local, 1);
        header.Children.Add(local);
        root.Children.Add(header);

        var body = new Grid { ColumnSpacing = 24, Padding = new Thickness(24) };
        body.ColumnDefinitions.Add(new ColumnDefinition { Width = new GridLength(270) });
        body.ColumnDefinitions.Add(new ColumnDefinition());
        Grid.SetRow(body, 1);
        root.Children.Add(body);
        var sidebar = new StackPanel { Spacing = 14 };
        sidebar.Children.Add(Eyebrow("MUSIC LIBRARY"));
        sidebar.Children.Add(openFolder);
        sidebar.Children.Add(folderLabel);
        sidebar.Children.Add(search);
        sidebar.Children.Add(songs);
        sidebar.Children.Add(new Border { Height = 1, Background = Brush(48, 54, 43), Margin = new Thickness(0, 10, 0, 10) });
        sidebar.Children.Add(Eyebrow("TRAINING PROJECTS"));
        sidebar.Children.Add(openProject);
        sidebar.Children.Add(recentProjects);
        body.Children.Add(new ScrollViewer { Content = sidebar, HorizontalScrollBarVisibility = ScrollBarVisibility.Disabled });

        var workspace = new StackPanel { Spacing = 22 };
        workspace.Children.Add(Eyebrow("PRACTICE WORKSPACE"));
        workspace.Children.Add(songTitle);
        workspace.Children.Add(songInfo);
        var actions = Row(openSong, save);
        workspace.Children.Add(actions);
        workspace.Children.Add(new Border { Child = waveform, BorderBrush = Brush(48, 54, 43), BorderThickness = new Thickness(1), CornerRadius = new CornerRadius(8) });
        workspace.Children.Add(Row(viewTimes, zoom, full));

        var parameters = new Grid { ColumnSpacing = 24 };
        parameters.ColumnDefinitions.Add(new ColumnDefinition());
        parameters.ColumnDefinitions.Add(new ColumnDefinition());
        parameters.Children.Add(speed);
        Grid.SetColumn(pitch, 1);
        parameters.Children.Add(pitch);
        fields.Children.Add(parameters);
        fields.Children.Add(loopEnabled);
        var selection = new Grid { ColumnSpacing = 24 };
        selection.ColumnDefinitions.Add(new ColumnDefinition());
        selection.ColumnDefinitions.Add(new ColumnDefinition());
        selection.Children.Add(loopA);
        Grid.SetColumn(loopB, 1);
        selection.Children.Add(loopB);
        fields.Children.Add(selection);
        fields.Children.Add(apply);
        workspace.Children.Add(new Border { Child = fields, Padding = new Thickness(24), Background = PanelBrush, CornerRadius = new CornerRadius(8) });
        workspace.Children.Add(Row(preview));
        workspace.Children.Add(new TextBlock
        {
            Text = "Drag across the waveform to choose a phrase. Preview plays it once at the selected speed and pitch. Apply new settings between previews.",
            TextWrapping = TextWrapping.Wrap, Foreground = MutedBrush, MaxWidth = 700, HorizontalAlignment = HorizontalAlignment.Left
        });
        Grid.SetColumn(workspace, 1);
        var scroll = new ScrollViewer { Content = workspace, HorizontalScrollBarVisibility = ScrollBarVisibility.Disabled };
        Grid.SetColumn(scroll, 1);
        body.Children.Add(scroll);
        var footer = new Grid { Padding = new Thickness(28, 14, 28, 14), ColumnSpacing = 12, Background = PanelBrush };
        footer.ColumnDefinitions.Add(new ColumnDefinition { Width = GridLength.Auto });
        footer.ColumnDefinitions.Add(new ColumnDefinition());
        footer.ColumnDefinitions.Add(new ColumnDefinition { Width = GridLength.Auto });
        footer.Children.Add(progress);
        Grid.SetColumn(status, 1);
        footer.Children.Add(status);
        Grid.SetColumn(cancel, 2);
        footer.Children.Add(cancel);
        Grid.SetRow(footer, 2);
        root.Children.Add(footer);
    }

    private async Task EnsureHelloAsync(CancellationToken token)
    {
        if (helloComplete) return;
        var hello = await core.SendAsync(new { version = 1, type = "hello" }, token);
        if (hello.Formats is null || hello.Formats.Length == 0) throw new IOException("The practice engine did not report supported formats.");
        helloComplete = true;
    }

    private Task ImportAsync(string path) => RunAsync(async token =>
    {
        await EnsureHelloAsync(token);
        Directory.CreateDirectory(Path.Combine(dataDirectory, "Unsaved"));
        var response = await core.SendAsync(new
        {
            version = 1, type = "import", source = path,
            document = Path.Combine(dataDirectory, "Unsaved", Guid.NewGuid() + ".riffine.json")
        }, token);
        SetSnapshot(response.Snapshot!);
        projectPath = null;
        dirty = true;
        UpdateTitle();
        waveform.SetWaveform(null);
        await LoadWaveformAsync(token);
        status.Text = "Song ready. Select a phrase or preview the full song.";
    }, "Opening song…");

    private Task OpenProjectAsync(string path) => RunAsync(async token =>
    {
        await EnsureHelloAsync(token);
        var response = await core.SendAsync(new { version = 1, type = "open", document = path }, token);
        SetSnapshot(response.Snapshot!);
        projectPath = path;
        dirty = false;
        UpdateTitle();
        AddRecent(path);
        waveform.SetWaveform(null);
        await LoadWaveformAsync(token);
        status.Text = "Project opened.";
    }, "Opening project…");

    private async Task LoadWaveformAsync(CancellationToken token, double? start = null, double? end = null)
    {
        if (snapshot is null) return;
        var info = snapshot.Project.Track.Audio;
        var startFrame = start.HasValue ? (long)Math.Round(start.Value * info.SampleRate, MidpointRounding.AwayFromZero) : 0;
        var endFrame = end.HasValue ? (long)Math.Round(end.Value * info.SampleRate, MidpointRounding.AwayFromZero) : info.Frames;
        var result = await core.SendAsync(new { version = 1, type = "waveform", startFrame, endFrame, bins = 1400 }, token);
        waveform.SetWaveform(result.Window, info.SampleRate);
        waveform.SetSelection(loopA.Value, loopB.Value);
        viewTimes.Text = $"{Time(waveform.ViewStart)} — {Time(waveform.ViewEnd)}";
    }

    private async Task CommitSettingsAsync(CancellationToken token)
    {
        if (snapshot is null) return;
        // Capture all inputs before applying responses; one edit must not erase
        // an unsent edit in another field. Core alone validates product bounds.
        var newSpeed = speed.Value;
        var newPitch = pitch.Value;
        var loop = new PracticeLoop(loopEnabled.IsChecked == true, loopA.Value, loopB.Value);
        if (!double.IsFinite(newSpeed) || !double.IsFinite(newPitch) || !double.IsFinite(loop.Start) || !double.IsFinite(loop.End))
            throw new IOException("Enter a number in each practice setting.");
        if (newSpeed != snapshot.Project.Settings.Speed || newPitch != snapshot.Project.Settings.Pitch)
        {
            var response = await core.SendAsync(new { version = 1, type = "command", command = new
            {
                version = 1, sequence = snapshot.Sequence + 1, type = "set_parameters", speed = newSpeed, pitch = newPitch
            } }, token);
            snapshot = response.Snapshot!;
        }
        if (loop != snapshot.Project.Settings.Loop)
        {
            var response = await core.SendAsync(new { version = 1, type = "command", command = new
            {
                version = 1, sequence = snapshot.Sequence + 1, type = "set_loop", loop
            } }, token);
            snapshot = response.Snapshot!;
        }
        SetSnapshot(snapshot);
    }

    private async Task<bool> SaveProjectAsync(CancellationToken token)
    {
        if (snapshot is null) return false;
        await CommitSettingsAsync(token);
        if (projectPath is null)
        {
            var picker = new FileSavePicker(AppWindow.Id)
            {
                SuggestedFileName = Path.GetFileNameWithoutExtension(snapshot.Project.Name) + ".riffine",
                DefaultFileExtension = ".json"
            };
            picker.FileTypeChoices.Add("Riffine project", new List<string> { ".json" });
            var file = await picker.PickSaveFileAsync();
            if (file is null) { status.Text = "Save cancelled."; return false; }
            // The Windows App SDK picker returns a path without creating a
            // placeholder. Core publishes exclusively and protects existing data.
            var path = file.Path;
            if (File.Exists(path)) throw new IOException("Choose a new project filename. Riffine protects existing files from replacement.");
            await core.SendAsync(new { version = 1, type = "save_as", document = path }, token);
            AddRecent(path);
            var opened = await core.SendAsync(new { version = 1, type = "open", document = path }, token);
            SetSnapshot(opened.Snapshot!);
            projectPath = path;
        }
        else
        {
            var saved = await core.SendAsync(new { version = 1, type = "save" }, token);
            SetSnapshot(saved.Snapshot!);
        }
        dirty = false;
        UpdateTitle();
        status.Text = "Project saved.";
        return true;
    }

    private async Task ChooseFolderAsync()
    {
        var picker = new FolderPicker(AppWindow.Id);
        if (await picker.PickSingleFolderAsync() is not { } folder) return;
        await RunAsync(async token =>
        {
            var found = await Task.Run(() => MusicLibrary.Scan(folder.Path, token), token);
            library.Clear();
            library.AddRange(found.Songs);
            folderLabel.Text = Path.GetFileName(folder.Path);
            RenderLibrary();
            status.Text = MusicLibrary.Status(found);
        }, "Reading music folder…");
    }

    private void RenderLibrary()
    {
        songs.Items.Clear();
        var query = search.Text.Trim();
        // Keep native item creation bounded even for very large directories.
        var matches = library.Where(s => s.Path.Contains(query, StringComparison.CurrentCultureIgnoreCase)).Take(500);
        foreach (var song in matches)
        {
            var labels = new StackPanel { Spacing = 3 };
            labels.Children.Add(new TextBlock { Text = song.Title, TextTrimming = TextTrimming.CharacterEllipsis });
            labels.Children.Add(new TextBlock { Text = song.Detail, FontSize = 11, Foreground = MutedBrush, TextTrimming = TextTrimming.CharacterEllipsis });
            var item = new ListViewItem { Content = labels, Tag = song, HorizontalContentAlignment = HorizontalAlignment.Stretch };
            ToolTipService.SetToolTip(item, song.Path);
            songs.Items.Add(item);
        }
        search.PlaceholderText = library.Count > 500 ? "Search all songs · first 500 shown" : "Search songs…";
    }

    private void SetSnapshot(SessionSnapshot value)
    {
        snapshot = value;
        updating = true;
        try
        {
            var project = value.Project;
            songTitle.Text = Path.GetFileNameWithoutExtension(project.Name);
            songInfo.Text = $"{Time(project.Track.Audio.Duration)}  ·  {project.Track.Audio.SampleRate / 1000.0:0.#} kHz  ·  {project.Track.Audio.Channels} channel(s)  ·  {project.Track.Audio.Encoding.ToUpperInvariant()}";
            speed.Value = project.Settings.Speed;
            pitch.Value = project.Settings.Pitch;
            loopA.Maximum = loopB.Maximum = project.Track.Audio.Duration;
            loopA.Value = project.Settings.Loop.Start;
            loopB.Value = project.Settings.Loop.End;
            loopEnabled.IsChecked = project.Settings.Loop.Enabled;
            waveform.SetSelection(loopA.Value, loopB.Value);
        }
        finally { updating = false; }
        UpdateEnabled();
    }

    private void SettingsChanged()
    {
        if (updating || snapshot is null) return;
        dirty = true;
        waveform.SetSelection(loopA.Value, loopB.Value);
        UpdateTitle();
    }

    private async Task RunAsync(Func<CancellationToken, Task> work, string message)
    {
        if (busy) return;
        busy = true;
        operation = new CancellationTokenSource();
        operationFinished = new TaskCompletionSource(TaskCreationOptions.RunContinuationsAsynchronously);
        status.Text = message;
        UpdateEnabled();
        try { await work(operation.Token); }
        catch (OperationCanceledException) { status.Text = "Stopped."; }
        catch (Exception error) { status.Text = error.Message; }
        finally
        {
            busy = false;
            operation.Dispose();
            operation = null;
            UpdateEnabled();
            operationFinished.TrySetResult();
            operationFinished = null;
        }
    }

    private async Task<bool> ConfirmChangesAsync()
    {
        if (busy || dialogOpen) return false;
        if (!dirty) return true;
        var dialog = new ContentDialog
        {
            XamlRoot = root.XamlRoot, RequestedTheme = ElementTheme.Dark,
            Title = "Save this practice project?",
            Content = "Keep your speed, pitch and phrase selection before opening something else.",
            PrimaryButtonText = "Save project", SecondaryButtonText = "Discard changes", CloseButtonText = "Cancel",
            DefaultButton = ContentDialogButton.Primary
        };
        ContentDialogResult result;
        dialogOpen = true;
        try { result = await dialog.ShowAsync(); }
        finally { dialogOpen = false; }
        if (result == ContentDialogResult.Secondary) return true;
        if (result != ContentDialogResult.Primary) return false;
        var saved = false;
        await RunAsync(async token => { saved = await SaveProjectAsync(token); }, "Saving project…");
        return saved;
    }

    private void UpdateEnabled()
    {
        openSong.IsEnabled = openFolder.IsEnabled = openProject.IsEnabled = !busy;
        songs.IsEnabled = search.IsEnabled = !busy;
        foreach (var child in recentProjects.Children.OfType<Control>()) child.IsEnabled = !busy;
        waveform.IsEnabled = save.IsEnabled = snapshot is not null && !busy;
        foreach (var control in new Control[] { speed, pitch, loopA, loopB, loopEnabled, apply })
            control.IsEnabled = snapshot is not null && !busy;
        preview.IsEnabled = snapshot?.Capabilities.StreamPreview == true && !busy;
        zoom.IsEnabled = full.IsEnabled = snapshot is not null && !busy;
        progress.IsActive = busy;
        progress.Visibility = cancel.Visibility = busy ? Visibility.Visible : Visibility.Collapsed;
    }

    private void UpdateTitle() => Title = snapshot is null ? "Riffine" : $"{(dirty ? "* " : "")}{snapshot.Project.Name} — Riffine";
    private static string Time(double seconds) => $"{(int)seconds / 60}:{seconds % 60:00.00}";
    private static SolidColorBrush Brush(byte r, byte g, byte b) => new(ColorHelper.FromArgb(255, r, g, b));
    private static TextBlock Eyebrow(string text) => new() { Text = text, FontSize = 11, Foreground = MutedBrush, CharacterSpacing = 150 };
    private static StackPanel Row(params UIElement[] children)
    {
        var panel = new StackPanel { Orientation = Orientation.Horizontal, Spacing = 12 };
        foreach (var child in children) panel.Children.Add(child);
        return panel;
    }
    private static NumberBox Number(string header, double min, double max, double value, double step) => new()
    {
        Header = header, Minimum = min, Maximum = max, Value = value,
        SmallChange = step, LargeChange = step * 5, SpinButtonPlacementMode = NumberBoxSpinButtonPlacementMode.Compact
    };
    private Button Action(string text, Func<Task> action)
    {
        var button = new Button { Content = text };
        button.Click += async (_, _) =>
        {
            try { await action(); }
            catch (Exception error) { status.Text = error.Message; }
        };
        return button;
    }

    private void ReadRecent()
    {
        try
        {
            var path = Path.Combine(dataDirectory, "recent-projects.json");
            if (new FileInfo(path) is { Exists: true, Length: <= 65536 })
                recent.AddRange((System.Text.Json.JsonSerializer.Deserialize<string[]>(File.ReadAllText(path)) ?? []).Where(File.Exists).Take(12));
        }
        catch (Exception) { /* Recent paths are optional; project files are authoritative. */ }
    }
    private void AddRecent(string path)
    {
        recent.RemoveAll(p => string.Equals(p, path, StringComparison.OrdinalIgnoreCase));
        recent.Insert(0, path);
        if (recent.Count > 12) recent.RemoveRange(12, recent.Count - 12);
        try
        {
            Directory.CreateDirectory(dataDirectory);
            var temp = Path.Combine(dataDirectory, Guid.NewGuid() + ".tmp");
            try
            {
                File.WriteAllText(temp, System.Text.Json.JsonSerializer.Serialize(recent));
                File.Move(temp, Path.Combine(dataDirectory, "recent-projects.json"), overwrite: true);
            }
            finally { if (File.Exists(temp)) File.Delete(temp); }
        }
        catch (Exception) { /* A recent-list failure must not fail a project save. */ }
        RenderRecent();
    }
    private void RenderRecent()
    {
        recentProjects.Children.Clear();
        foreach (var path in recent)
        {
            var button = Action(Path.GetFileNameWithoutExtension(path), async () =>
            {
                if (await ConfirmChangesAsync()) await OpenProjectAsync(path);
            });
            button.HorizontalAlignment = HorizontalAlignment.Stretch;
            ToolTipService.SetToolTip(button, path);
            recentProjects.Children.Add(button);
        }
    }
}
