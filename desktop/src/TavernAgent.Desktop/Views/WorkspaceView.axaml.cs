using Avalonia.Controls;
using Avalonia.Input;
using TavernAgent.Desktop.ViewModels;

namespace TavernAgent.Desktop.Views;

public partial class WorkspaceView : UserControl
{
    public WorkspaceView()
    {
        InitializeComponent();
    }

    private void OnFlyoutScrimPressed(object? sender, PointerPressedEventArgs e)
    {
        if (DataContext is WorkspaceViewModel vm) vm.CloseFlyoutCommand.Execute(null);
        e.Handled = true;
    }
}
