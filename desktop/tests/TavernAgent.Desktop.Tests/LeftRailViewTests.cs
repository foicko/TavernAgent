using Avalonia.Controls;
using Avalonia.Headless.XUnit;
using Avalonia.Threading;
using TavernAgent.Core.Stores;
using TavernAgent.Desktop.ViewModels;
using TavernAgent.Desktop.Views.Left;
using Xunit;

namespace TavernAgent.Desktop.Tests;

public class LeftRailViewTests
{
    [AvaloniaFact]
    public void LeftRail_Lays_Out_And_Switches_Tabs()
    {
        var ws = new WorkspaceViewModel(new UiStore());
        var view = new LeftRailView { DataContext = ws.LeftRail };
        var window = new Window { Content = view, Width = 1200, Height = 800 };
        window.Show();
        Dispatcher.UIThread.RunJobs();

        Assert.True(view.Bounds.Width > 0);
        Assert.True(ws.LeftRail.IsTreeTab);

        ws.LeftRail.ShowSessionsCommand.Execute(null);
        Dispatcher.UIThread.RunJobs();
        Assert.True(ws.LeftRail.IsSessionsTab);

        window.Close();
        ws.Dispose();
    }

    [AvaloniaFact]
    public void StoryMap_Opens_Without_Session()
    {
        var ws = new WorkspaceViewModel(new UiStore());
        var view = new StoryMapModal { DataContext = ws.LeftRail.StoryMap };
        var window = new Window { Content = view, Width = 1200, Height = 800 };
        window.Show();
        ws.LeftRail.StoryMap.Open();
        Dispatcher.UIThread.RunJobs();
        Assert.True(ws.LeftRail.StoryMap.IsOpen);
        Assert.NotNull(ws.LeftRail.StoryMap.Error);
        window.Close();
        ws.Dispose();
    }
}
