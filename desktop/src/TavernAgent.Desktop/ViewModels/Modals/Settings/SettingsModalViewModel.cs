using System.Collections.ObjectModel;
using System.ComponentModel;
using CommunityToolkit.Mvvm.ComponentModel;
using CommunityToolkit.Mvvm.Input;
using TavernAgent.Core.Stores;

namespace TavernAgent.Desktop.ViewModels.Modals.Settings;

/// <summary>A settings tab page that can (re)load its data when shown.</summary>
public interface ISettingsPage
{
    Task LoadAsync();
}

public sealed partial class SettingsTabItem : ObservableObject
{
    public SettingsTabItem(string key, string title, string description)
    {
        Key = key;
        Title = title;
        Description = description;
    }

    public string Key { get; }
    public string Title { get; }
    public string Description { get; }
    public System.Windows.Input.ICommand? SelectCommand { get; set; }
    [ObservableProperty] private bool _isActive;
}

/// <summary>Settings modal host with four tabs (web: SettingsModal).</summary>
public sealed partial class SettingsModalViewModel : WorkspaceChildViewModel
{
    private bool _disposed;

    public SettingsModalViewModel(WorkspaceViewModel workspace) : base(workspace)
    {
        Models = new ModelSettingsPageViewModel(workspace);
        Tts = new TtsSettingsPageViewModel(workspace);
        Images = new ImageSettingsPageViewModel(workspace);
        Embedding = new EmbeddingSettingsPageViewModel(workspace);

        Tabs = new ObservableCollection<SettingsTabItem>
        {
            new("models", "模型与连接", "配置对话生成、导演协商与后台记忆使用的模型，以及连接凭据。"),
            new("tts", "语音设置", "选择朗读引擎与声音，试听并微调语速与演播方式。"),
            new("images", "图像生成", "为回合生成场景插画：配置生图引擎、画幅与画风。"),
            new("embedding", "记忆向量", "配置语义检索向量模型，提升记忆召回质量。"),
        };
        SelectTab(Tabs[0]);
        foreach (var tab in Tabs) tab.SelectCommand = SelectTabCommand;
        Workspace.Ui.PropertyChanged += OnUiChanged;
    }

    public ModelSettingsPageViewModel Models { get; }
    public TtsSettingsPageViewModel Tts { get; }
    public ImageSettingsPageViewModel Images { get; }
    public EmbeddingSettingsPageViewModel Embedding { get; }

    public ObservableCollection<SettingsTabItem> Tabs { get; }

    [ObservableProperty] private SettingsTabItem? _activeTab;
    [ObservableProperty] private ViewModelBase? _page;

    public bool IsOpen => Workspace.Ui.SettingsOpen;
    public string ActiveDescription => ActiveTab?.Description ?? "";

    [RelayCommand]
    private void Close() => Workspace.Ui.SetSettingsOpen(false);

    [RelayCommand]
    private void SelectTab(SettingsTabItem? tab)
    {
        if (tab is null) return;
        foreach (var t in Tabs) t.IsActive = ReferenceEquals(t, tab);
        ActiveTab = tab;
        Page = tab.Key switch
        {
            "tts" => Tts,
            "images" => Images,
            "embedding" => Embedding,
            _ => Models,
        };
        OnPropertyChanged(nameof(ActiveDescription));
        _ = LoadAsync(Page);
    }

    private static async Task LoadAsync(ViewModelBase? page)
    {
        try
        {
            if (page is ISettingsPage settingsPage) await settingsPage.LoadAsync().ConfigureAwait(true);
        }
        catch
        {
            // page-level errors are surfaced inside each page
        }
    }

    private void OnUiChanged(object? sender, PropertyChangedEventArgs e)
    {
        if (e.PropertyName != nameof(UiStore.SettingsOpen)) return;
        OnPropertyChanged(nameof(IsOpen));
        if (IsOpen) _ = OnOpenedAsync();
    }

    private async Task OnOpenedAsync()
    {
        try { await Workspace.Settings.LoadAsync().ConfigureAwait(true); } catch { /* ignore */ }
        await LoadAsync(Page).ConfigureAwait(true);
    }

    public override void Dispose()
    {
        if (_disposed) return;
        _disposed = true;
        Workspace.Ui.PropertyChanged -= OnUiChanged;
        Models.Dispose();
    }
}
