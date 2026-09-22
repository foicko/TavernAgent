using Avalonia;
using Avalonia.Controls;
using Avalonia.Headless;
using Avalonia.Headless.XUnit;
using TavernAgent.Core.Stores;
using TavernAgent.Desktop;
using TavernAgent.Desktop.Services;
using TavernAgent.Desktop.ViewModels;
using TavernAgent.Desktop.Views;
using Xunit;

[assembly: AvaloniaTestApplication(typeof(TavernAgent.Desktop.Tests.TestAppBuilder))]

namespace TavernAgent.Desktop.Tests;

public class TestAppBuilder
{
    public static AppBuilder BuildAvaloniaApp() => AppBuilder.Configure<App>()
        .UseHeadless(new AvaloniaHeadlessPlatformOptions());
}

public class MainWindowTests
{
    [AvaloniaFact]
    public void Window_Constructs_With_Boot_Overlay()
    {
        var shell = new ShellViewModel();
        var window = new MainWindow { DataContext = shell };

        Assert.Equal("TavernAgent", window.Title);
        Assert.NotNull(window.FindControl<Border>("BootOverlay"));
        Assert.NotNull(window.FindControl<ContentControl>("MainContent"));
        shell.Dispose();
    }

    [AvaloniaFact]
    public void Shell_Initializes_In_Starting_Phase()
    {
        var shell = new ShellViewModel();
        Assert.Equal(BootPhase.StartingSidecar, shell.Phase);
        Assert.False(string.IsNullOrEmpty(shell.StatusText));
        shell.Dispose();
    }

    [AvaloniaFact]
    public void Window_Reveals_Workspace_When_Ready()
    {
        var shell = new ShellViewModel { Workspace = new WorkspaceViewModel(new UiStore()) };
        var window = new MainWindow { DataContext = shell };
        window.Show();
        Avalonia.Threading.Dispatcher.UIThread.RunJobs();

        var main = window.FindControl<Control>("MainContent");
        var overlay = window.FindControl<Border>("BootOverlay");

        Assert.NotNull(main);
        Assert.NotNull(overlay);
        Assert.True(main!.IsVisible);
        Assert.False(overlay!.IsVisible);

        window.Close();
        shell.Dispose();
    }
}

public class WorkspaceViewTests
{
    private static WorkspaceViewModel Fresh() => new(new UiStore());

    [AvaloniaFact]
    public void Workspace_Renders_With_Default_Three_Column_Geometry()
    {
        var ws = Fresh();
        var view = new WorkspaceView { DataContext = ws };

        Assert.NotNull(view);
        Assert.Equal(224d, ws.LeftRailWidth);
        Assert.Equal(304d, ws.RightRailWidth);
        Assert.True(ws.IsStudioMode);
        Assert.True(ws.ShowLeftRail);
        Assert.True(ws.ShowRightRail);
        Assert.False(ws.ShowRightRailRecall);
        Assert.True(ws.ShowLeftPanel);
        Assert.False(ws.ShowIconDock);
        ws.Dispose();
    }

    [AvaloniaFact]
    public void Panel_Toggles_And_Right_Rail_Collapses()
    {
        var ws = Fresh();

        // Panel starts expanded (default); activating the same tab collapses to the icon dock.
        Assert.True(ws.ShowLeftPanel);
        ws.LeftRail.ShowTreeCommand.Execute(null);
        Assert.False(ws.ShowLeftPanel);
        Assert.True(ws.ShowIconDock);

        ws.LeftRail.ShowTreeCommand.Execute(null);
        Assert.True(ws.ShowLeftPanel);

        ws.ToggleRightRailCommand.Execute(null);
        Assert.False(ws.ShowRightRail);
        Assert.Equal(0d, ws.RightRailWidth);
        Assert.True(ws.ShowRightRailRecall);
        ws.Dispose();
    }

    [AvaloniaFact]
    public void Reading_Mode_Hides_Rails_But_Keeps_Header()
    {
        var ws = Fresh();
        ws.EnterReadingCommand.Execute(null);
        Assert.True(ws.IsReadingMode);
        Assert.Equal(0d, ws.LeftRailWidth);
        Assert.Equal(0d, ws.RightRailWidth);
        Assert.False(ws.ShowLeftRail);
        Assert.False(ws.ShowRightRail);
        Assert.False(ws.ShowLeftPanel);

        ws.EnterStudioCommand.Execute(null);
        Assert.True(ws.IsStudioMode);
        Assert.Equal(224d, ws.LeftRailWidth);
        Assert.Equal(304d, ws.RightRailWidth);
        ws.Dispose();
    }

    [AvaloniaFact]
    public void Rail_Marks_The_Remembered_Tab_While_Closed()
    {
        var ws = Fresh();

        ws.LeftRail.ShowLoreCommand.Execute(null);
        Assert.True(ws.LeftRail.IsLoreTabActive);
        Assert.False(ws.LeftRail.IsLoreTabVisited);

        ws.LeftRail.CollapseCommand.Execute(null);
        Assert.False(ws.LeftRail.IsLoreTabActive);
        Assert.True(ws.LeftRail.IsLoreTabVisited);
        Assert.False(ws.LeftRail.IsTreeTabVisited);
        ws.Dispose();
    }

    [AvaloniaFact]
    public void Theme_Toggle_Flips_Store_And_Label()
    {
        var ws = Fresh();
        Assert.True(ws.IsDarkTheme);
        ws.ToggleThemeCommand.Execute(null);
        Assert.False(ws.IsDarkTheme);
        ws.ToggleThemeCommand.Execute(null);
        Assert.True(ws.IsDarkTheme);
        ws.Dispose();
    }
}
