using System.Linq;
using Avalonia.Controls;
using Avalonia.Headless.XUnit;
using Avalonia.Media;
using Avalonia.Threading;
using Avalonia.VisualTree;
using TavernAgent.Desktop.Controls;
using Xunit;

namespace TavernAgent.Desktop.Tests;

/// <summary>Guards the flat "ink &amp; hairline" surface system against regressions.</summary>
public class DesignSystemTests
{
    [AvaloniaFact]
    public void Modal_Uses_Flat_Scrim_Without_Acrylic()
    {
        var modal = new Modal();
        var window = new Window { Content = modal };
        window.Show();
        Dispatcher.UIThread.RunJobs();

        Assert.NotNull(modal.FindControl<Border>("Scrim"));
        Assert.Empty(window.GetVisualDescendants().OfType<ExperimentalAcrylicBorder>());

        window.Close();
    }

    [AvaloniaFact]
    public void EmptyState_Renders_IconGeometry_As_Path()
    {
        var empty = new EmptyState
        {
            IconGeometry = Geometry.Parse("M0 0 L10 10"),
            Title = "空态",
        };
        var window = new Window { Content = empty };
        window.Show();
        Dispatcher.UIThread.RunJobs();

        var paths = window.GetVisualDescendants().OfType<Avalonia.Controls.Shapes.Path>().ToList();
        Assert.Contains(paths, path => path.Data is not null);

        window.Close();
    }
}
