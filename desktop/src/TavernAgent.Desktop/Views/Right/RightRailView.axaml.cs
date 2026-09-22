using Avalonia.Controls;
using Avalonia.Interactivity;
using TavernAgent.Desktop.ViewModels.Right;

namespace TavernAgent.Desktop.Views.Right;

public partial class RightRailView : UserControl
{
    public RightRailView()
    {
        InitializeComponent();
    }

    private void OnActClick(object? sender, RoutedEventArgs e)
    {
        if (DataContext is not RightRailViewModel vm) return;
        if (sender is Button { DataContext: InventoryActViewModel act })
            vm.ActivateInventoryCommand.Execute(act);
    }
}
