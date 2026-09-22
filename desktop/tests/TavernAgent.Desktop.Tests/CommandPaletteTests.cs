using System.Linq;
using Avalonia.Headless.XUnit;
using Avalonia.Threading;
using TavernAgent.Core.Stores;
using TavernAgent.Desktop.ViewModels;
using TavernAgent.Desktop.ViewModels.Palette;
using Xunit;

namespace TavernAgent.Desktop.Tests;

public class CommandPaletteTests
{
    [AvaloniaFact]
    public void Palette_Opens_Filters_And_Runs()
    {
        var ws = new WorkspaceViewModel(new UiStore());
        var palette = ws.Palette;

        ws.Ui.SetPaletteOpen(true);
        Dispatcher.UIThread.RunJobs();
        Assert.True(palette.IsOpen);
        Assert.True(palette.Results.Count > 0);

        palette.Query = "设置";
        Dispatcher.UIThread.RunJobs();
        Assert.Contains(palette.Results, r => r.Title.Contains("设置"));

        var target = palette.Results.First(r => r.Title.Contains("设置"));
        palette.RunCommand.Execute(target);
        Dispatcher.UIThread.RunJobs();
        Assert.True(ws.Ui.SettingsOpen);
        Assert.False(ws.Ui.PaletteOpen);

        ws.Dispose();
    }

    [AvaloniaFact]
    public void Palette_Empty_Query_Lists_All_Groups()
    {
        var ws = new WorkspaceViewModel(new UiStore());
        ws.Ui.SetPaletteOpen(true);
        Dispatcher.UIThread.RunJobs();

        var groups = ws.Palette.Results.Select(r => r.Group).Distinct().ToList();
        Assert.Contains("navigation", groups);
        Assert.Contains("view", groups);
        Assert.Contains("panel", groups);

        // Navigation commands should be present even without a session.
        Assert.Contains(ws.Palette.Results, r => r.Id == "panel:settings");

        ws.Dispose();
    }
}
