using Avalonia.Controls;
using TavernAgent.Desktop.Services;

namespace TavernAgent.Desktop.Views.Stage;

public partial class SceneBarView : UserControl
{
    public SceneBarView()
    {
        InitializeComponent();
        // 卡片主题表面：常驻场景标签同样跟随卡片主题。
        CardThemeSync.Attach(this);
    }
}
