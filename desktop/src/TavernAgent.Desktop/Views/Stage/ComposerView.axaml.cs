using System;
using Avalonia.Controls;
using Avalonia.Input;
using Avalonia.Interactivity;
using Avalonia.Threading;
using TavernAgent.Desktop.ViewModels.Stage;

namespace TavernAgent.Desktop.Views.Stage;

public partial class ComposerView : UserControl
{
    public ComposerView()
    {
        InitializeComponent();
    }

    private void OnInputKeyDown(object? sender, KeyEventArgs e)
    {
        if (e.Key != Key.Enter || e.KeyModifiers.HasFlag(KeyModifiers.Shift)) return;
        if (DataContext is not ComposerViewModel vm) return;
        if (vm.SendCommand.CanExecute(null))
        {
            vm.SendCommand.Execute(null);
            e.Handled = true;
        }
    }

    private void OnActionItemClick(object? sender, RoutedEventArgs e)
    {
        // Defer the hide until after this Click dispatch returns: hiding the flyout
        // synchronously detaches its subtree, which clears the inherited DataContext
        // before Button.OnClick runs the bound Command (so the command would never fire).
        var flyout = ActionMenuButton?.Flyout;
        if (flyout is not null) Dispatcher.UIThread.Post(() => flyout.Hide());
    }

    private void OnMacroClick(object? sender, RoutedEventArgs e)
    {
        if (sender is Button { DataContext: MacroPill macro })
        {
            ApplyMacroSmart(macro);
            ActionMenuButton?.Flyout?.Hide();
            e.Handled = true;
        }
    }

    private void ApplyMacroSmart(MacroPill macro)
    {
        if (InputBox is null) return;
        var selStart = InputBox.SelectionStart;
        var selEnd = InputBox.SelectionEnd;
        var text = InputBox.Text ?? "";

        if (selEnd > selStart && selStart >= 0 && selEnd <= text.Length)
        {
            var selected = text.Substring(selStart, selEnd - selStart);
            var wrapped = macro.Prefix + selected + macro.Suffix;
            InputBox.Text = text.Substring(0, selStart) + wrapped + text.Substring(selEnd);
            InputBox.CaretIndex = selStart + wrapped.Length;
            InputBox.SelectionStart = InputBox.CaretIndex;
            InputBox.SelectionEnd = InputBox.CaretIndex;
        }
        else
        {
            var insertPos = Math.Clamp(InputBox.CaretIndex, 0, text.Length);
            var insertText = macro.Prefix + macro.Suffix;
            InputBox.Text = text.Insert(insertPos, insertText);
            InputBox.CaretIndex = insertPos + macro.Prefix.Length;
            InputBox.SelectionStart = InputBox.CaretIndex;
            InputBox.SelectionEnd = InputBox.CaretIndex;
        }
        if (DataContext is ComposerViewModel vm)
        {
            vm.Text = InputBox.Text ?? "";
        }
        InputBox.Focus();
    }
}
