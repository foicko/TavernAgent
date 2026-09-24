using System.Collections.Specialized;
using Avalonia.Controls;
using Avalonia.Threading;
using TavernAgent.Desktop.Services;
using TavernAgent.Desktop.ViewModels.Stage;

namespace TavernAgent.Desktop.Views.Stage;

public partial class StageView : UserControl
{
    private StageViewModel? _vm;

    public StageView()
    {
        InitializeComponent();
        DataContextChanged += OnDataContextChanged;
    }

    private void OnDataContextChanged(object? sender, EventArgs e)
    {
        if (_vm is not null)
        {
            _vm.Messages.CollectionChanged -= OnCollectionChanged;
            _vm.DraftBlocks.CollectionChanged -= OnCollectionChanged;
            _vm.PropertyChanged -= OnVmPropertyChanged;
        }

        _vm = DataContext as StageViewModel;
        if (_vm is not null)
        {
            _vm.Messages.CollectionChanged += OnCollectionChanged;
            _vm.DraftBlocks.CollectionChanged += OnCollectionChanged;
            _vm.PropertyChanged += OnVmPropertyChanged;
            CardThemeSync.SetTokens(_vm.CardThemeTokens);
            ScheduleScrollToEnd();
        }
    }

    private void OnCollectionChanged(object? sender, NotifyCollectionChangedEventArgs e) => ScheduleScrollToEnd();

    private void OnVmPropertyChanged(object? sender, System.ComponentModel.PropertyChangedEventArgs e)
    {
        if (e.PropertyName is nameof(StageViewModel.CardThemeTokens))
        {
            CardThemeSync.SetTokens(_vm?.CardThemeTokens);
        }
        if (e.PropertyName is nameof(StageViewModel.IsStreaming)
            or nameof(StageViewModel.HasDraftBlocks)
            or nameof(StageViewModel.ThinkingText)
            or nameof(StageViewModel.HasMessages)
            or nameof(StageViewModel.ShowSkeleton)
            or nameof(StageViewModel.IsIdle))
        {
            ScheduleScrollToEnd();
        }
    }

    private void ScheduleScrollToEnd()
    {
        Dispatcher.UIThread.Post(() =>
        {
            var max = Stream.Extent.Height - Stream.Viewport.Height;
            if (max > 0) Stream.Offset = new Avalonia.Vector(Stream.Offset.X, max);
        }, DispatcherPriority.Background);
    }
}
