using CommunityToolkit.Mvvm.ComponentModel;

namespace TavernAgent.Desktop.ViewModels;

public abstract class ViewModelBase : ObservableObject, IDisposable
{
    public virtual void Dispose()
    {
    }
}
