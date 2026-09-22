using System.Text.Json;

namespace TavernAgent.Core.Api;

public sealed class ApiError : Exception
{
    public int Status { get; }
    public string Body { get; }

    public ApiError(int status, string body)
        : base(ExtractMessage(body, status))
    {
        Status = status;
        Body = body;
    }

    private static string ExtractMessage(string body, int status)
    {
        if (string.IsNullOrWhiteSpace(body)) return $"HTTP {status}";
        try
        {
            using var doc = JsonDocument.Parse(body);
            if (doc.RootElement.ValueKind == JsonValueKind.Object &&
                doc.RootElement.TryGetProperty("message", out var msg) &&
                msg.ValueKind == JsonValueKind.String)
            {
                return msg.GetString() ?? body;
            }
        }
        catch (JsonException)
        {
            // Plain-text errors retain their original message.
        }
        return body;
    }
}
