using System.ComponentModel;
using Avalonia.Controls;
using Avalonia.Input;
using Avalonia.Threading;
using TavernAgent.Desktop.ViewModels.Palette;

namespace TavernAgent.Desktop.Views;

public partial class CommandPaletteView : UserControl
{
    private CommandPaletteViewModel? _vm;

    public CommandPaletteView()
    {
        InitializeComponent();
        DataContextChanged += OnDataContextChanged;
    }

    private void OnDataContextChanged(object? sender, EventArgs e)
    {
        if (_vm is not null) _vm.PropertyChanged -= OnVmChanged;
        _vm = DataContext as CommandPaletteViewModel;
        if (_vm is not null) _vm.PropertyChanged += OnVmChanged;
    }

    private void OnVmChanged(object? sender, PropertyChangedEventArgs e)
    {
        if (e.PropertyName != nameof(CommandPaletteViewModel.IsOpen)) return;
        if (_vm?.IsOpen == true)
            Dispatcher.UIThread.Post(() => SearchBox.Focus(), DispatcherPriority.Input);
    }

    private async void OnSearchKeyDown(object? sender, KeyEventArgs e)
    {
        if (_vm is null) return;
        switch (e.Key)
        {
            case Key.Escape:
                _vm.CloseCommand.Execute(null);
                e.Handled = true;
                break;
            case Key.Down:
                _vm.MoveActive(1);
                e.Handled = true;
                break;
            case Key.Up:
                _vm.MoveActive(-1);
                e.Handled = true;
                break;
            case Key.Home:
                _vm.MoveToEdge(false);
                e.Handled = true;
                break;
            case Key.End:
                _vm.MoveToEdge(true);
                e.Handled = true;
                break;
            case Key.Enter:
                e.Handled = true;
                try { await _vm.RunActiveAsync(); }
                catch (Exception ex) { TavernAgent.Desktop.Services.CrashLog.Write(ex, "CommandPalette.Run"); }
                break;
        }
    }
}
