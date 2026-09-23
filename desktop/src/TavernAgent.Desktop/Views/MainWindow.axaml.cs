using System;
using System.Runtime.InteropServices;
using Avalonia;
using Avalonia.Controls;
using Avalonia.Input;
using Avalonia.Interactivity;
using Avalonia.Media;
using Avalonia.VisualTree;
using TavernAgent.Desktop.Services;
using TavernAgent.Desktop.ViewModels;

namespace TavernAgent.Desktop.Views;

public partial class MainWindow : Window
{
    // Web-aligned breakpoints (web/src/styles/tokens.css): the right rail yields first.
    private const double RightRailBreakpoint = 1280;
    private const double LeftRailBreakpoint = 1024;

    private EventHandler? _contentReadyHandler;

    public MainWindow()
    {
        InitializeComponent();
        Opened += OnOpened;

        // Restore window dimensions if saved
        try
        {
            var s = DesktopSettings.Load();
            if (s.WindowWidth >= 800 && s.WindowHeight >= 600)
            {
                Width = s.WindowWidth;
                Height = s.WindowHeight;
            }
        }
        catch { /* non-fatal */ }

        Closing += (_, _) =>
        {
            try
            {
                if (WindowState == WindowState.Normal && Bounds.Width >= 800 && Bounds.Height >= 600)
                {
                    var saved = DesktopSettings.Load();
                    saved.WindowWidth = Bounds.Width;
                    saved.WindowHeight = Bounds.Height;
                    saved.Save();
                }
            }
            catch { /* non-fatal */ }
        };

        // Flat opaque shell (beautifului): no native acrylic/mica blur.
        TransparencyLevelHint = new[] { WindowTransparencyLevel.None };
    }

    private ShellViewModel? Vm => DataContext as ShellViewModel;

    private void OnOpened(object? sender, EventArgs e)
    {
        ApplyResponsiveRailBreakpoints(Bounds.Width);
        ApplyWindowsDwmChrome();

        if (Vm is null) return;
        if (Vm.Phase == BootPhase.Failed)
        {
            // Already failed before window showed — keep overlay with error.
            return;
        }

        // Fast path: the sidecar may have become ready before the window opened,
        // in which case ContentReady already fired and the handler would never run.
        if (Vm.Workspace is not null)
        {
            ShowMain(true);
            return;
        }

        // Subscribe once and detach immediately after firing, so re-opening the
        // window (or a late retry) cannot accumulate handlers.
        _contentReadyHandler ??= (_, _) =>
        {
            if (Vm is not null && _contentReadyHandler is not null)
                Vm.ContentReady -= _contentReadyHandler;
            ShowMain(true);
        };
        Vm.ContentReady += _contentReadyHandler;
    }

    protected override void OnPropertyChanged(AvaloniaPropertyChangedEventArgs change)
    {
        base.OnPropertyChanged(change);
        if (change.Property == BoundsProperty && change.NewValue is Rect newBounds)
        {
            ApplyResponsiveRailBreakpoints(newBounds.Width);
        }
    }

    /// <summary>
    /// Derives the auto-collapse flags from the window width. Symmetric: growing
    /// the window back above a breakpoint re-expands the rail. The user's manual
    /// preference is untouched (it lives in LeftCollapsed/RightCollapsed).
    /// </summary>
    private static void ApplyResponsiveRailBreakpoints(double width)
    {
        if (width <= 0) return;
        AppServices.Ui.SetAutoRightCollapsed(width < RightRailBreakpoint);
        AppServices.Ui.SetAutoLeftCollapsed(width < LeftRailBreakpoint);
    }

    private void ShowMain(bool ready)
    {
        void Update()
        {
            var overlay = this.FindControl<Border>("BootOverlay");
            var main = this.FindControl<Control>("MainContent");
            if (overlay is not null) overlay.IsVisible = !ready;
            if (main is not null) main.IsVisible = ready;
        }

        if (Avalonia.Threading.Dispatcher.UIThread.CheckAccess()) Update();
        else Avalonia.Threading.Dispatcher.UIThread.Post(Update);
    }

    private void OnBootOverlayPointerPressed(object? sender, PointerPressedEventArgs e)
    {
        if (!e.GetCurrentPoint(this).Properties.IsLeftButtonPressed) return;
        if (e.Source is Visual visual && visual.FindAncestorOfType<Button>(includeSelf: true) is not null) return;

        if (e.ClickCount == 2)
        {
            WindowState = WindowState == WindowState.Maximized ? WindowState.Normal : WindowState.Maximized;
        }
        else
        {
            BeginMoveDrag(e);
        }
    }

    private void OnMinimizeClicked(object? sender, RoutedEventArgs e)
    {
        WindowState = WindowState.Minimized;
    }

    private void OnCloseClicked(object? sender, RoutedEventArgs e)
    {
        Close();
    }

    private delegate IntPtr WndProcDelegate(IntPtr hWnd, uint msg, IntPtr wParam, IntPtr lParam);
    private WndProcDelegate? _wndProcDelegate;
    private IntPtr _oldWndProc = IntPtr.Zero;

    [DllImport("dwmapi.dll")]
    private static extern int DwmSetWindowAttribute(IntPtr hwnd, int attr, ref int attrValue, int attrSize);

    [DllImport("user32.dll", EntryPoint = "SetWindowLongPtr", SetLastError = true)]
    private static extern IntPtr SetWindowLongPtr(IntPtr hWnd, int nIndex, IntPtr dwNewLong);

    [DllImport("user32.dll")]
    private static extern IntPtr CallWindowProc(IntPtr lpPrevWndFunc, IntPtr hWnd, uint msg, IntPtr wParam, IntPtr lParam);

    [DllImport("user32.dll")]
    private static extern bool SetWindowPos(IntPtr hWnd, IntPtr hWndInsertAfter, int X, int Y, int cx, int cy, uint uFlags);

    private void ApplyWindowsDwmChrome()
    {
        if (!OperatingSystem.IsWindows()) return;

        try
        {
            var handle = TryGetPlatformHandle()?.Handle;
            if (handle is null || handle.Value == IntPtr.Zero) return;

            var hwnd = handle.Value;

            // 1. Dark mode & DWM caption / border color matching theme (#17181A)
            int darkMode = 1;
            DwmSetWindowAttribute(hwnd, 20 /* DWMWA_USE_IMMERSIVE_DARK_MODE */, ref darkMode, sizeof(int));

            // Set DWM caption to match window background (#17181A: R=0x17, G=0x18, B=0x1A)
            int captionColor = 0x001A1817;
            DwmSetWindowAttribute(hwnd, 35 /* DWMWA_CAPTION_COLOR */, ref captionColor, sizeof(int));

            // Set DWM border to subtle hairline (#26272A)
            int borderColor = 0x002A2726;
            DwmSetWindowAttribute(hwnd, 34 /* DWMWA_BORDER_COLOR */, ref borderColor, sizeof(int));

            // Windows 11 rounded corners
            int cornerPref = 2; // DWMWCP_ROUND
            DwmSetWindowAttribute(hwnd, 33 /* DWMWA_WINDOW_CORNER_PREFERENCE */, ref cornerPref, sizeof(int));

            // 2. Subclass WndProc to eliminate the 8px non-client top frame
            _wndProcDelegate = HookWndProc;
            var newWndProc = Marshal.GetFunctionPointerForDelegate(_wndProcDelegate);
            _oldWndProc = SetWindowLongPtr(hwnd, -4 /* GWLP_WNDPROC */, newWndProc);

            // Inform DWM and Windows of frame recalculation
            SetWindowPos(hwnd, IntPtr.Zero, 0, 0, 0, 0,
                0x0001 /* SWP_NOSIZE */ | 0x0002 /* SWP_NOMOVE */ | 0x0004 /* SWP_NOZORDER */ | 0x0020 /* SWP_FRAMECHANGED */);
        }
        catch { /* non-fatal fallback */ }
    }

    private IntPtr HookWndProc(IntPtr hWnd, uint msg, IntPtr wParam, IntPtr lParam)
    {
        const uint WM_NCCALCSIZE = 0x0083;
        if (msg == WM_NCCALCSIZE && wParam != IntPtr.Zero)
        {
            // Zero out standard non-client area: client area fills the entire window rect
            return IntPtr.Zero;
        }
        return CallWindowProc(_oldWndProc, hWnd, msg, wParam, lParam);
    }
}
