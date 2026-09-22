using System.Text.Json;
using TavernAgent.Core.Api;
using TavernAgent.Core.Dtos;
using Xunit;

namespace TavernAgent.Core.Tests;

public class ParseSseBlockTests
{
    [Fact]
    public void Parses_Event_Id_And_Json_Data()
    {
        var raw = "event: turn.thinking\nid: turn_1:3\ndata: {\"thinking\":\"hi\"}";
        var evt = SseClient.ParseSseBlock(raw, "");
        Assert.NotNull(evt);
        Assert.Equal("turn.thinking", evt!.Event);
        Assert.Equal("turn_1:3", evt.Id);
        Assert.True(evt.HasIdLine);
        Assert.Equal("hi", evt.Data.GetProperty("thinking").GetString());
    }

    [Fact]
    public void Inherits_PrevId_When_No_Id_Line_And_Keeps_Unnumbered_Raw_Data()
    {
        var raw = "event: block.delta\ndata: not-json";
        var evt = SseClient.ParseSseBlock(raw, "turn_1:4");
        Assert.NotNull(evt);
        Assert.Equal("turn_1:4", evt.Id);
        Assert.False(evt.HasIdLine);
        Assert.Equal(JsonValueKind.String, evt.Data.ValueKind);
        Assert.Equal("not-json", evt.Data.GetString());
    }

    [Fact]
    public void Ignores_Comment_Heartbeat_And_Joins_Multiline_Data()
    {
        var raw = ": ping\ndata: line1\ndata: line2";
        var evt = SseClient.ParseSseBlock(raw, "x");
        Assert.NotNull(evt);
        Assert.Equal("line1\nline2", evt.Data.GetString());
    }

    [Fact]
    public void Returns_Null_When_No_Data_Lines()
    {
        Assert.Null(SseClient.ParseSseBlock(": keep-alive", "x"));
    }

    [Fact]
    public void Supports_Crlf_Separators_In_Block_Lines()
    {
        var raw = "event: message\r\nid: a:1\r\ndata: {\"k\":1}";
        var evt = SseClient.ParseSseBlock(raw, "");
        Assert.NotNull(evt);
        Assert.Equal("a:1", evt.Id);
        Assert.Equal(1, evt.Data.GetProperty("k").GetInt32());
    }
}
