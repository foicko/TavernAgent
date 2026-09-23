using Avalonia.Controls;
using Avalonia.Input.Platform;
using Avalonia.Interactivity;
using TavernAgent.Desktop.Services;
using TavernAgent.Desktop.ViewModels.Stage;

namespace TavernAgent.Desktop.Views.Stage;

public partial class MessageView : UserControl
{
    public MessageView()
    {
        InitializeComponent();
    }

    private async void OnCopyClick(object? sender, RoutedEventArgs e)
    {
        try
        {
            if (DataContext is not MessageViewModel vm) return;
            IClipboard? clipboard = TopLevel.GetTopLevel(this)?.Clipboard;
            if (clipboard is not null)
            {
                await clipboard.SetTextAsync(vm.PlainText);
                vm.NotifyCopied();
            }
        }
        catch (Exception ex)
        {
            // Clipboard access can throw COMException / access denied; never fatal.
            CrashLog.Write(ex, "MessageView.Copy");
        }
    }
}
