using Avalonia.Controls;
using Avalonia.Controls.Templates;
using TavernAgent.Desktop.ViewModels;

namespace TavernAgent.Desktop;

/// <summary>Maps ViewModel types to Views by naming convention (FooViewModel → FooView).</summary>
public class ViewLocator : IDataTemplate
{
    public Control? Build(object? data)
    {
        if (data is null) return null;
        var name = data.GetType().FullName!
            .Replace("ViewModels", "Views", StringComparison.Ordinal)
            .Replace("ViewModel", "View", StringComparison.Ordinal);
        var type = Type.GetType(name);
        if (type is not null && Activator.CreateInstance(type) is Control control)
        {
            control.DataContext = data;
            return control;
        }
        return new TextBlock { Text = "Not Found: " + name };
    }

    public bool Match(object? data) => data is ViewModelBase;
}
