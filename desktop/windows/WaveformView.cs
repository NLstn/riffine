using Microsoft.UI;
using Microsoft.UI.Input;
using Microsoft.UI.Xaml;
using Microsoft.UI.Xaml.Automation;
using Microsoft.UI.Xaml.Controls;
using Microsoft.UI.Xaml.Input;
using Microsoft.UI.Xaml.Media;
using Microsoft.UI.Xaml.Shapes;
using Windows.Foundation;

namespace Riffine.Windows;

// Draws only Go-produced source-coordinate extrema. Selection is available
// through the adjacent native number fields as well as pointer dragging.
public sealed class WaveformView : UserControl
{
    private readonly Canvas canvas = new() { Background = new SolidColorBrush(ColorHelper.FromArgb(255, 18, 20, 17)) };
    private readonly Microsoft.UI.Xaml.Shapes.Path signal = new() { Stroke = new SolidColorBrush(ColorHelper.FromArgb(255, 200, 238, 145)), StrokeThickness = 1 };
    private readonly Rectangle selection = new()
    {
        Fill = new SolidColorBrush(ColorHelper.FromArgb(42, 200, 238, 145)),
        Stroke = new SolidColorBrush(ColorHelper.FromArgb(255, 200, 238, 145)),
        StrokeThickness = 1,
        IsHitTestVisible = false
    };
    private WaveformWindow? waveform;
    private int sampleRate;
    private double loopStart, loopEnd, anchor, originalStart, originalEnd;
    private uint? pointer;
    public event Action<double, double>? SelectionChanged;
    public double ViewStart { get; private set; }
    public double ViewEnd { get; private set; }

    public WaveformView()
    {
        Height = 250;
        HorizontalAlignment = HorizontalAlignment.Stretch;
        Content = canvas;
        canvas.Children.Add(selection);
        canvas.Children.Add(signal);
        signal.IsHitTestVisible = false;
        SizeChanged += (_, _) => Draw();
        canvas.PointerPressed += Pressed;
        canvas.PointerMoved += Moved;
        canvas.PointerReleased += Released;
        canvas.PointerCaptureLost += (_, _) =>
        {
            if (pointer is not null) { loopStart = originalStart; loopEnd = originalEnd; }
            pointer = null;
            DrawSelection();
        };
        AutomationProperties.SetName(this, "Song waveform. Drag to select a phrase, or enter A and B below.");
    }

    public void SetWaveform(WaveformWindow? value, int rate = 0)
    {
        waveform = value;
        sampleRate = rate;
        ViewStart = rate > 0 && value is not null ? (double)value.RequestedStart / rate : 0;
        ViewEnd = rate > 0 && value is not null ? (double)value.RequestedEnd / rate : 0;
        Draw();
    }

    public void SetSelection(double start, double end)
    {
        loopStart = start;
        loopEnd = end;
        DrawSelection();
    }

    private double TimeAt(double x) => ViewStart + Math.Clamp(x / Math.Max(1, ActualWidth), 0, 1) * (ViewEnd - ViewStart);
    private void Pressed(object sender, PointerRoutedEventArgs e)
    {
        if (waveform is null || !e.GetCurrentPoint(canvas).Properties.IsLeftButtonPressed) return;
        pointer = e.Pointer.PointerId;
        originalStart = loopStart;
        originalEnd = loopEnd;
        anchor = TimeAt(e.GetCurrentPoint(canvas).Position.X);
        canvas.CapturePointer(e.Pointer);
        e.Handled = true;
    }
    private void Moved(object sender, PointerRoutedEventArgs e)
    {
        if (pointer != e.Pointer.PointerId) return;
        var time = TimeAt(e.GetCurrentPoint(canvas).Position.X);
        loopStart = Math.Min(anchor, time);
        loopEnd = Math.Max(anchor, time);
        DrawSelection();
    }
    private void Released(object sender, PointerRoutedEventArgs e)
    {
        if (pointer != e.Pointer.PointerId) return;
        Moved(sender, e);
        pointer = null;
        canvas.ReleasePointerCapture(e.Pointer);
        if ((loopEnd - loopStart) * sampleRate >= 1) SelectionChanged?.Invoke(loopStart, loopEnd);
        else { loopStart = originalStart; loopEnd = originalEnd; DrawSelection(); }
    }

    private void Draw()
    {
        if (waveform is null || waveform.Cells.Length == 0 || ActualWidth <= 0) { signal.Data = null; DrawSelection(); return; }
        var geometry = new PathGeometry();
        var range = waveform.RequestedEnd - waveform.RequestedStart;
        var center = Height / 2;
        foreach (var cell in waveform.Cells)
        {
            if (cell.Channels.Length == 0) continue;
            var min = cell.Channels.Min(c => c.Min);
            var max = cell.Channels.Max(c => c.Max);
            var x = ((cell.StartFrame + (cell.EndFrame - cell.StartFrame) / 2.0 - waveform.RequestedStart) / range) * ActualWidth;
            var figure = new PathFigure { StartPoint = new Point(x, center - max * (center - 16)), IsClosed = false, IsFilled = false };
            figure.Segments.Add(new LineSegment { Point = new Point(x, center - min * (center - 16)) });
            geometry.Figures.Add(figure);
        }
        signal.Data = geometry;
        DrawSelection();
    }

    private void DrawSelection()
    {
        var span = ViewEnd - ViewStart;
        if (span <= 0 || loopEnd <= ViewStart || loopStart >= ViewEnd) { selection.Visibility = Visibility.Collapsed; return; }
        selection.Visibility = Visibility.Visible;
        var x = Math.Clamp((loopStart - ViewStart) / span, 0, 1) * ActualWidth;
        var right = Math.Clamp((loopEnd - ViewStart) / span, 0, 1) * ActualWidth;
        Canvas.SetLeft(selection, x);
        Canvas.SetTop(selection, 8);
        selection.Width = Math.Max(0, right - x);
        selection.Height = Math.Max(0, Height - 16);
    }
}
