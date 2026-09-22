using System.Net;
using System.Net.Sockets;

namespace TavernAgent.Sidecar;

/// <summary>Allocates a free loopback TCP port by binding then releasing it.</summary>
public static class PortAllocator
{
    /// <summary>
    /// Binds 127.0.0.1:0 to discover a free ephemeral port, then closes the socket
    /// so the child process can bind the same port. Small TOCTOU window is accepted
    /// for desktop use (single machine, short gap between close and child listen).
    /// </summary>
    public static int GetFreeLoopbackPort()
    {
        var listener = new TcpListener(IPAddress.Loopback, 0);
        listener.Start();
        try
        {
            return ((IPEndPoint)listener.LocalEndpoint).Port;
        }
        finally
        {
            listener.Stop();
        }
    }
}
