using Avalonia.Controls;
using Avalonia.Interactivity;
using TavernAgent.Desktop.ViewModels.Left;

namespace TavernAgent.Desktop.Views.Left;

public partial class SessionsPanel : UserControl
{
    public SessionsPanel()
    {
        InitializeComponent();
    }

    private async void OnExportClick(object? sender, RoutedEventArgs e)
    {
        if (DataContext is SessionsPanelViewModel vm) await vm.ExportAsync(this);
    }

    private async void OnImportClick(object? sender, RoutedEventArgs e)
    {
        if (DataContext is SessionsPanelViewModel vm) await vm.ImportAsync(this);
    }
}
