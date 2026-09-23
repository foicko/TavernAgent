using Avalonia.Headless.XUnit;
using TavernAgent.Core.Stores;
using TavernAgent.Desktop.ViewModels;
using Xunit;

namespace TavernAgent.Desktop.Tests;

/// <summary>
/// Responsive rail behaviour: the window width drives an auto-collapse flag that
/// is OR'd with the user's manual preference, so growing the window re-expands a
/// rail instead of leaving it collapsed forever.
/// </summary>
public class ResponsiveLayoutTests
{
    private static (WorkspaceViewModel Ws, UiStore Ui) Fresh()
    {
        var ui = new UiStore();
        return (new WorkspaceViewModel(ui), ui);
    }

    [AvaloniaFact]
    public void AutoCollapse_HidesThenReExpandsRails()
    {
        var (ws, ui) = Fresh();

        Assert.True(ws.ShowRightRail);
        Assert.True(ws.ShowLeftPanel);

        ui.SetAutoRightCollapsed(true);
        ui.SetAutoLeftCollapsed(true);
        Assert.False(ws.ShowRightRail);
        Assert.Equal(0d, ws.RightRailWidth);
        Assert.True(ws.ShowIconDock);
        Assert.False(ws.ShowLeftPanel);
        Assert.Equal(52d, ws.LeftRailWidth);

        ui.SetAutoRightCollapsed(false);
        ui.SetAutoLeftCollapsed(false);
        Assert.True(ws.ShowRightRail);
        Assert.Equal(304d, ws.RightRailWidth);
        Assert.True(ws.ShowLeftPanel);
        Assert.Equal(224d, ws.LeftRailWidth);

        ws.Dispose();
    }

    [AvaloniaFact]
    public void ManualCollapse_OutranksAutoExpand()
    {
        var (ws, ui) = Fresh();

        ui.SetRightRailCollapsed(true);
        ui.SetAutoRightCollapsed(false);

        Assert.False(ws.ShowRightRail);   // manual preference wins
        ws.Dispose();
    }

    [AvaloniaFact]
    public void TogglingWhileAutoCollapsed_ExpandsAndClearsAuto()
    {
        var (ws, ui) = Fresh();

        ui.SetAutoLeftCollapsed(true);
        Assert.False(ws.ShowLeftPanel);

        ws.ToggleLeftRailCommand.Execute(null);

        Assert.True(ws.ShowLeftPanel);
        Assert.False(ui.AutoLeftCollapsed);
        ws.Dispose();
    }
}
