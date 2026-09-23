using System.Collections.ObjectModel;
using System.ComponentModel;
using System.Runtime.CompilerServices;

namespace TavernAgent.Core.Stores;

/// <summary>
/// Minimal observable store for Core (no Avalonia dependency). Desktop ViewModels can wrap these.
/// </summary>
public abstract class ObservableStore : INotifyPropertyChanged
{
    public event PropertyChangedEventHandler? PropertyChanged;

    protected bool SetProperty<T>(ref T field, T value, [CallerMemberName] string? propertyName = null)
    {
        if (EqualityComparer<T>.Default.Equals(field, value)) return false;
        field = value;
        OnPropertyChanged(propertyName);
        return true;
    }

    protected virtual void OnPropertyChanged([CallerMemberName] string? propertyName = null)
        => StoreDispatcher.Dispatch(() => PropertyChanged?.Invoke(this, new PropertyChangedEventArgs(propertyName)));
}

public sealed class ObservableList<T> : ObservableCollection<T>
{
    public void ReplaceAll(IEnumerable<T> items)
    {
        Clear();
        foreach (var item in items) Add(item);
    }
}
