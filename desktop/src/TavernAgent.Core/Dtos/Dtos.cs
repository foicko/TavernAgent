using System.Text.Json;
using System.Text.Json.Serialization;

namespace TavernAgent.Core.Dtos;

public static class JsonDefaults
{
    public static readonly JsonSerializerOptions Options = new(JsonSerializerDefaults.Web)
    {
        DefaultIgnoreCondition = JsonIgnoreCondition.WhenWritingNull,
        NumberHandling = JsonNumberHandling.AllowReadingFromString,
    };

    public static readonly JsonSerializerOptions Indented = new(Options) { WriteIndented = true };
}

// ---- Narrative / turn content (web/src/app/types.ts) ----

public sealed class TextBlock
{
    public string Kind { get; set; } = "narration";
    public string? SpeakerId { get; set; }
    public string Text { get; set; } = "";
}

public sealed class Option
{
    public string OptionId { get; set; } = "";
    public string Intent { get; set; } = "chaotic";
    public string Text { get; set; } = "";
    public string? ActionRef { get; set; }
}

public sealed class ActionView
{
    public string ActionId { get; set; } = "";
    public string Label { get; set; } = "";
    public string Attribute { get; set; } = "";
    public int Dc { get; set; }
    public List<string>? ItemIds { get; set; }
}

public sealed class CheckResult
{
    public string RollId { get; set; } = "";
    public string ActionId { get; set; } = "";
    public string Attribute { get; set; } = "";
    public int AttributeModifier { get; set; }
    public int Natural { get; set; }
    public int Total { get; set; }
    public int Dc { get; set; }
    public string Outcome { get; set; } = "failure";
    public string? PermanentEffect { get; set; }
}

public sealed class TurnContent
{
    public string InputKind { get; set; } = "";
    public string InputText { get; set; } = "";
    public string? InputNote { get; set; }
    public string? OptionsMode { get; set; }
    public int? SuppressedOptions { get; set; }
    public List<StateChange>? Changes { get; set; }
    public List<MemoryRef>? InjectedMemories { get; set; }
    public Provenance? Provenance { get; set; }
    public List<CheckResult>? Checks { get; set; }
    public List<TextBlock> Blocks { get; set; } = new();
    public List<Option> Options { get; set; } = new();
    public MoodRef? Mood { get; set; }
}

public sealed class Provenance
{
    public string? TurnId { get; set; }
}

public sealed class MoodRef
{
    public string CharacterId { get; set; } = "";
    public string MoodCode { get; set; } = "";
    public string Text { get; set; } = "";
}

public sealed class StateChange
{
    public string Kind { get; set; } = "";
    public string Label { get; set; } = "";
    public string Text { get; set; } = "";
    public double? Delta { get; set; }
}

public sealed class MemoryRef
{
    public string MemoryId { get; set; } = "";
    public string Text { get; set; } = "";
    public string? Kind { get; set; }
}

public sealed class PlotNode
{
    public string NodeId { get; set; } = "";
    public string SessionId { get; set; } = "";
    public string ParentId { get; set; } = "";
    public string Kind { get; set; } = "";
    public int Depth { get; set; }
    public int TurnNumber { get; set; }
    public string ContentJson { get; set; } = "";
    public string CreatedAt { get; set; } = "";
}

public sealed class BranchView
{
    public string BranchId { get; set; } = "";
    public string Name { get; set; } = "";
    public string HeadNodeId { get; set; } = "";
    public int Version { get; set; }
    public string? ActiveTurnId { get; set; }
}

public sealed class RelationValue
{
    public double Affection { get; set; }
    public double Trust { get; set; }
    public double Alertness { get; set; }
}

public sealed class ItemInstance
{
    public string InstanceId { get; set; } = "";
    public string? TemplateId { get; set; }
    public string Name { get; set; } = "";
    public string? OwnerId { get; set; }
    public int Quantity { get; set; }
    public int? Protection { get; set; }
    public bool? Keepsake { get; set; }
    public bool? Unique { get; set; }
}

public sealed class CharacterInfo
{
    public string CharacterId { get; set; } = "";
    public string Name { get; set; } = "";
    public string? Description { get; set; }
    public bool? Participant { get; set; }
    public string? Avatar { get; set; }
    public string? Personality { get; set; }
    public string? Scenario { get; set; }
    [JsonPropertyName("mes_example")] public string? MesExample { get; set; }
    [JsonPropertyName("system_prompt")] public string? SystemPrompt { get; set; }
    [JsonPropertyName("post_history_instructions")] public string? PostHistoryInstructions { get; set; }
    [JsonPropertyName("creator_notes")] public string? CreatorNotes { get; set; }
    public List<string>? Tags { get; set; }
    public string? Creator { get; set; }
    [JsonPropertyName("character_version")] public string? CharacterVersion { get; set; }
    public string? Nickname { get; set; }
}

public sealed class PromiseItem
{
    public string PromiseId { get; set; } = "";
    public List<string>? ParticipantIds { get; set; }
    public string Content { get; set; } = "";
    public string SourceNodeId { get; set; } = "";
    public string State { get; set; } = "proposed";
    public string? SettledByReceiptId { get; set; }
}

public sealed class MoodEntry
{
    public string MoodCode { get; set; } = "";
    public string Text { get; set; } = "";
}

public sealed class SceneRef
{
    public string SceneId { get; set; } = "";
    public string? Title { get; set; }
}

public sealed class WorldState
{
    public Dictionary<string, RelationValue> Relationships { get; set; } = new();
    public Dictionary<string, ItemInstance> Items { get; set; } = new();
    public Dictionary<string, PromiseItem> Promises { get; set; } = new();
    public Dictionary<string, MoodEntry> Moods { get; set; } = new();
    public Dictionary<string, CharacterInfo> Characters { get; set; } = new();
    public SceneRef? Scene { get; set; }
}

public sealed class SessionView
{
    public DirectorSummary? Director { get; set; }
    public string SessionId { get; set; } = "";
    public string CharacterId { get; set; } = "";
    public List<ActionView>? Actions { get; set; }
    public string Title { get; set; } = "";
    public string RootNodeId { get; set; } = "";
    public BranchView Branch { get; set; } = new();
    public List<BranchView>? Branches { get; set; }
    public string? ViewNodeId { get; set; }
    public PlotNode? ViewNode { get; set; }
    public PlotNode HeadNode { get; set; } = new();
    public List<PlotNode>? Nodes { get; set; }
    public Dictionary<string, List<PlotNode>>? CandidateGroups { get; set; }
    public WorldState? State { get; set; }
    public List<SecretView>? Secrets { get; set; }
    public OutlineView? Outline { get; set; }
    public SummaryArtifact? ActiveSummary { get; set; }
    public bool? HasMore { get; set; }
    public string? OldestTurnId { get; set; }
}

public sealed class SummaryArtifact
{
    public string SummaryId { get; set; } = "";
    public string FromNodeId { get; set; } = "";
    public string ToNodeId { get; set; } = "";
    public string SourceHash { get; set; } = "";
    public string? VisibilityScope { get; set; }
    public string Text { get; set; } = "";
    public string? ModelConfigVersion { get; set; }
    public int? SummaryVersion { get; set; }
}

public sealed class SecretView
{
    public string SecretId { get; set; } = "";
    public string? Title { get; set; }
    public int? Order { get; set; }
    public string? Content { get; set; }
    public bool Revealed { get; set; }
}

// ---- Director ----

public sealed class DirectorBeat
{
    public string BeatId { get; set; } = "";
    public string Title { get; set; } = "";
    public string Instruction { get; set; } = "";
    public string CompletionCriteria { get; set; } = "";
}

public sealed class DirectorPlan
{
    public string? PlanId { get; set; }
    public string? RevisionId { get; set; }
    public string Title { get; set; } = "";
    public string Guidance { get; set; } = "";
    public List<DirectorBeat> Beats { get; set; } = new();
}

public sealed class DirectorEvidence
{
    public int BlockSeq { get; set; }
    public string Quote { get; set; } = "";
}

public sealed class DirectorProgressEntry
{
    public string Status { get; set; } = "completed";
    public string SourceNodeId { get; set; } = "";
    public List<DirectorEvidence>? Evidence { get; set; }
}

public sealed class DirectorState
{
    public DirectorPlan Plan { get; set; } = new();
    public string Status { get; set; } = "active";
    public string CurrentBeatId { get; set; } = "";
    public Dictionary<string, DirectorProgressEntry> Progress { get; set; } = new();
    public string? Warning { get; set; }
}

public sealed class DirectorSummary
{
    public string Title { get; set; } = "";
    public string RevisionId { get; set; } = "";
    public string Status { get; set; } = "active";
    public string CurrentBeatId { get; set; } = "";
    public string CurrentBeatTitle { get; set; } = "";
    public int Completed { get; set; }
    public int Total { get; set; }
    public string? Warning { get; set; }
}

public sealed class DirectorDraft
{
    public string SessionId { get; set; } = "";
    public string BranchId { get; set; } = "";
    public int Version { get; set; }
    public string BaseRevisionId { get; set; } = "";
    public DirectorPlan Plan { get; set; } = new();
    public string UpdatedAt { get; set; } = "";
}

public sealed class DirectorRequest
{
    public string RequestId { get; set; } = "";
    public string SessionId { get; set; } = "";
    public string BranchId { get; set; } = "";
    public string BaseNodeId { get; set; } = "";
    public int DraftVersion { get; set; }
    public string Text { get; set; } = "";
    public string Status { get; set; } = "generating";
    public string? Reply { get; set; }
    public DirectorPlan? Candidate { get; set; }
    public bool DraftApplied { get; set; }
    public string? Error { get; set; }
    public string CreatedAt { get; set; } = "";
}

public sealed class DirectorView
{
    public DirectorState? State { get; set; }
    public DirectorDraft? Draft { get; set; }
    public List<DirectorRequest> Requests { get; set; } = new();
    public bool ReadOnly { get; set; }
    public string ViewNodeId { get; set; } = "";
}

// ---- Lorebook / graph / outline ----

public sealed class LorebookEntry
{
    public string? EntryId { get; set; }
    public string? Title { get; set; }
    public List<string> Keys { get; set; } = new();
    public List<string>? SecondaryKeys { get; set; }
    public string Content { get; set; } = "";
    public bool Enabled { get; set; }
}

public sealed class LorebookMatch
{
    public int BookIndex { get; set; }
    public int EntryIndex { get; set; }
    public string BookName { get; set; } = "";
    public LorebookEntry Entry { get; set; } = new();
}

public sealed class LorebookPage
{
    public List<LorebookMatch> Entries { get; set; } = new();
    public bool HasMore { get; set; }
    public int NextOffset { get; set; }
}

public sealed class GraphNode
{
    public string NodeId { get; set; } = "";
    public string? ParentId { get; set; }
    public string Kind { get; set; } = "";
    public int? TurnNumber { get; set; }
    public int Depth { get; set; }
    public int? ChildCount { get; set; }
    public bool? IsCandidate { get; set; }
}

public sealed class GraphView
{
    public string SessionId { get; set; } = "";
    public string TargetNodeId { get; set; } = "";
    public List<GraphNode> Nodes { get; set; } = new();
    public bool? Truncated { get; set; }
}

public sealed class OutlineEntry
{
    public string MilestoneId { get; set; } = "";
    public string Description { get; set; } = "";
}

public sealed class GoalEntry
{
    public string GoalId { get; set; } = "";
    public string CharacterId { get; set; } = "";
    public string Text { get; set; } = "";
}

public sealed class OutlineView
{
    public List<OutlineEntry> Milestones { get; set; } = new();
    public List<GoalEntry> Goals { get; set; } = new();
    public string? Scene { get; set; }
}

// ---- Results ----

public sealed class DeriveResult
{
    public BranchView Branch { get; set; } = new();
    public string BranchId { get; set; } = "";
    public string TurnId { get; set; } = "";
    public string Status { get; set; } = "";
    public string? StatusUrl { get; set; }
    public string? EventsUrl { get; set; }
}

public sealed class ImportScope
{
    public bool Full { get; set; }
    public string? BranchId { get; set; }
    public string? BranchName { get; set; }
}

public sealed class ImportResult
{
    public string SessionId { get; set; } = "";
    public string Title { get; set; } = "";
    public string RootNodeId { get; set; } = "";
    public ImportScope? Scope { get; set; }
    public Dictionary<string, int>? Counts { get; set; }
    public int? Assets { get; set; }
}

public sealed class AssistantEditResult
{
    public BranchView Branch { get; set; } = new();
    public string BranchId { get; set; } = "";
    public string NodeId { get; set; } = "";
    public string Mode { get; set; } = "";
}

public sealed class SessionSummary
{
    public string SessionId { get; set; } = "";
    public string CharacterId { get; set; } = "";
    public string? UpdatedAt { get; set; }
    public string Title { get; set; } = "";
    public string CreatedAt { get; set; } = "";
    public string? RootNodeId { get; set; }
}

public sealed class AcceptResult
{
    public string TurnId { get; set; } = "";
    public string Status { get; set; } = "";
    public string StatusUrl { get; set; } = "";
    public string EventsUrl { get; set; } = "";
}

public sealed class TurnInputRef
{
    public string Kind { get; set; } = "";
    public string Text { get; set; } = "";
    public string? ActionRef { get; set; }
    public OptionRef? OptionRef { get; set; }
}

public sealed class OptionRef
{
    public string NodeId { get; set; } = "";
    public string OptionId { get; set; } = "";
}

public sealed class TurnView
{
    public string TurnId { get; set; } = "";
    public string? SessionId { get; set; }
    public TurnInputRef? Input { get; set; }
    public string BranchId { get; set; } = "";
    public string Status { get; set; } = "";
    public string Mode { get; set; } = "";
    public string? FailureCode { get; set; }
    public string? FailureMessage { get; set; }
    public PlotNode? ResultNode { get; set; }
    public TurnDraftSnapshot? Draft { get; set; }
}

public sealed class TurnDraftSnapshot
{
    public string AttemptId { get; set; } = "";
    public InFlightBlock? InFlight { get; set; }
}

public sealed class InFlightBlock
{
    public int Seq { get; set; }
    public string Kind { get; set; } = "";
    public string? SpeakerId { get; set; }
    public string Text { get; set; } = "";
}

public sealed class SSEEvent
{
    public string Id { get; set; } = "";
    public string Event { get; set; } = "message";
    public JsonElement Data { get; set; }
}

// ---- Model config ----

public sealed class ProviderConfig
{
    public string Slot { get; set; } = "";
    public bool Enabled { get; set; }
    public string? ModelId { get; set; }
    public string? ResolvedModelId { get; set; }
    public string? Kind { get; set; }
    public string? BaseUrl { get; set; }
    public string? Model { get; set; }
    public string? ApiKey { get; set; }
    public bool? HasApiKey { get; set; }
    public double? Temperature { get; set; }
    public int? MaxTokens { get; set; }
    public int? ContextWindow { get; set; }
    public string? ReasoningEffort { get; set; }
}

public sealed class ModelInstance
{
    public string Id { get; set; } = "";
    public string Name { get; set; } = "";
    public string Kind { get; set; } = "";
    public string? BaseUrl { get; set; }
    public string? Model { get; set; }
    public string? ApiKey { get; set; }
    public bool? HasApiKey { get; set; }
    public double? Temperature { get; set; }
    public int? MaxTokens { get; set; }
    public int? ContextWindow { get; set; }
    public string? ReasoningEffort { get; set; }
}

public sealed class ModelCatalog
{
    public List<ModelInstance> Models { get; set; } = new();
    public List<ProviderConfig> Slots { get; set; } = new();
}

public sealed class ProbeResult
{
    public string Slot { get; set; } = "";
    public bool Ok { get; set; }
    public string? Kind { get; set; }
    public string? Model { get; set; }
    public string? ConnectMsg { get; set; }
    public bool? FormatTested { get; set; }
    public bool? FormatOk { get; set; }
    public string? FormatMsg { get; set; }
    public int? LatencyMs { get; set; }
}

// ---- Memory ----

public sealed class MemoryEvidence
{
    public string? SourceNodeId { get; set; }
    public string? SourceQuote { get; set; }
    public string? Reasoning { get; set; }
    public string Confidence { get; set; } = "medium";
    public bool? AutoDowngraded { get; set; }
}

public sealed class MemoryView
{
    public string MemoryId { get; set; } = "";
    public string SourceNodeId { get; set; } = "";
    public string Kind { get; set; } = "observed";
    public List<string>? OwnerIds { get; set; }
    public string Content { get; set; } = "";
    public List<string>? EntityIds { get; set; }
    public double? Confidence { get; set; }
    public bool Pinned { get; set; }
    public string? Supersedes { get; set; }
    public bool Hidden { get; set; }
    public bool Effective { get; set; }
    public bool? Protected { get; set; }
    public double? Importance { get; set; }
    public List<string>? MergedFrom { get; set; }
    public MemoryEvidence? Evidence { get; set; }
    public string? SubjectKey { get; set; }
    public int? CreatedTurn { get; set; }
    public int? ValidFromTurn { get; set; }
    public int? ValidUntilTurn { get; set; }
}

public sealed class MemoryUsage
{
    public int Used { get; set; }
    public int Protected { get; set; }
    public int Limit { get; set; }
    public int Reclaimable { get; set; }
    public int Headroom { get; set; }
    public string Tier { get; set; } = "Normal";
}

public sealed class MemoryPage
{
    public List<MemoryView> Memories { get; set; } = new();
    public MemoryUsage? Usage { get; set; }
    public string? NodeId { get; set; }
    public string? NextCursor { get; set; }
    public int? Total { get; set; }
    public Dictionary<string, int>? Counts { get; set; }
}

// ---- Cards / import ----

public sealed class CardOpening
{
    public string VariantId { get; set; } = "";
    public string Title { get; set; } = "";
    public string Text { get; set; } = "";
}

public sealed class CardPreview
{
    public string? CardId { get; set; }
    public string? ShortName { get; set; }
    public string? Role { get; set; }
    public string Format { get; set; } = "";
    public string? SpecVersion { get; set; }
    public string? Source { get; set; }
    public string Name { get; set; } = "";
    public string? Avatar { get; set; }
    public string? Description { get; set; }
    public string? Personality { get; set; }
    public string? Scenario { get; set; }
    public string? FirstMes { get; set; }
    public string? MesExample { get; set; }
    public string? SystemPrompt { get; set; }
    public string? PostHistoryInstructions { get; set; }
    public string? CreatorNotes { get; set; }
    public List<string>? Tags { get; set; }
    public string? Creator { get; set; }
    public string? CharacterVersion { get; set; }
    public string? Nickname { get; set; }
    public List<string> Supported { get; set; } = new();
    public List<string> Ignored { get; set; } = new();
    public List<string> Warnings { get; set; } = new();
    public string CharacterJson { get; set; } = "";
    public List<CardOpening> Openings { get; set; } = new();
}

public sealed class CardLibrarySummary
{
    public string CardId { get; set; } = "";
    public string Name { get; set; } = "";
    public string? ShortName { get; set; }
    public string? Avatar { get; set; }
    public string? Format { get; set; }
    public string? Role { get; set; }
    public string? CreatedAt { get; set; }
    public string? UpdatedAt { get; set; }
    public string? LastUsedAt { get; set; }
}

// ---- Auth / desktop / TTS / images / embeddings ----

public sealed class AuthStatus
{
    public bool Authenticated { get; set; }
    public bool Required { get; set; }
}

public sealed class PairResult
{
    public bool Ok { get; set; }
    public string Token { get; set; } = "";
}

public sealed class DesktopInfo
{
    public bool Desktop { get; set; }
}

public sealed class TTSVoice
{
    public string Id { get; set; } = "";
    public string Name { get; set; } = "";
    public string Gender { get; set; } = "";
    public string Locale { get; set; } = "";
    public string Description { get; set; } = "";
}

public sealed class TTSVoicesResult
{
    public List<TTSVoice> Voices { get; set; } = new();
}

public sealed class TTSSynthesizeParams
{
    public string Text { get; set; } = "";
    public string? Instruction { get; set; }
    public string? Voice { get; set; }
    public string? Engine { get; set; }
    public string? BaseUrl { get; set; }
    public string? ApiKey { get; set; }
    public string? Model { get; set; }
    public double? Speed { get; set; }
}

public sealed class GenerateImageParams
{
    public string Prompt { get; set; } = "";
    public string? NegativePrompt { get; set; }
    public string? Engine { get; set; }
    public string? BaseUrl { get; set; }
    public string? ApiKey { get; set; }
    public string? Model { get; set; }
    public string? Size { get; set; }
    public string? Quality { get; set; }
    public string? Style { get; set; }
}

public sealed class GeneratedImageResult
{
    public string Id { get; set; } = "";
    public string Url { get; set; } = "";
    public string Prompt { get; set; } = "";
    public string MimeType { get; set; } = "";
    public string CreatedAt { get; set; } = "";
}

public sealed class EmbeddingProbeRequest
{
    public string BaseUrl { get; set; } = "";
    public string? ApiKey { get; set; }
    public string Model { get; set; } = "";
    public string? SampleText { get; set; }
}

public sealed class EmbeddingProbeResponse
{
    public bool Ok { get; set; }
    public string Model { get; set; } = "";
    public int Dimension { get; set; }
    public int LatencyMs { get; set; }
    public List<double>? Preview { get; set; }
    public string? Error { get; set; }
}

public sealed class EmbeddingComputeRequest
{
    public List<string> Texts { get; set; } = new();
    public string? BaseUrl { get; set; }
    public string? ApiKey { get; set; }
    public string? Model { get; set; }
    public int? Dimension { get; set; }
}

public sealed class EmbeddingComputeResponse
{
    public string Model { get; set; } = "";
    public int Dimension { get; set; }
    public List<List<double>> Embeddings { get; set; } = new();
}

// ---- Request bodies ----

public sealed class CreateSessionRequest
{
    public string? IdempotencyKey { get; set; }
    public string? Title { get; set; }
    public string? CardId { get; set; }
    public string? CharacterJson { get; set; }
    public string PlayerName { get; set; } = "";
    public string? PlayerRole { get; set; }
    public List<object>? PlayerBackpack { get; set; }
    public string? OpeningVariantId { get; set; }
    public string? OpeningText { get; set; }
}

public sealed class CreateSessionResult
{
    public string SessionId { get; set; } = "";
    public string BranchId { get; set; } = "";
    public string RootNodeId { get; set; } = "";
    public int BranchVersion { get; set; }
    public string OpeningText { get; set; } = "";
    public WorldStateDto? State { get; set; }
}

public sealed class WorldStateDto
{
    public Dictionary<string, RelationValue>? Relationships { get; set; }
    public Dictionary<string, ItemInstance>? Items { get; set; }
    public Dictionary<string, CharacterInfo>? Characters { get; set; }
}

public sealed class AcceptTurnRequest
{
    public string IdempotencyKey { get; set; } = "";
    public string ExpectedHeadId { get; set; } = "";
    public int ExpectedVersion { get; set; }
    public string ExpectedCharacterId { get; set; } = "";
    public TurnSubmitInput Input { get; set; } = new();
}

public sealed class TurnSubmitInput
{
    public string Kind { get; set; } = "text";
    public string Text { get; set; } = "";
    public string? Note { get; set; }
    public string? Options { get; set; }
    public OptionRef? OptionRef { get; set; }
    public string? ActionRef { get; set; }
}

public sealed class ForkBranchRequest
{
    public string FromNodeId { get; set; } = "";
    public string? Name { get; set; }
    public string ExpectedCharacterId { get; set; } = "";
}

public sealed class ForkBranchResult
{
    public BranchView Branch { get; set; } = new();
}

public sealed class RegenerateRequest
{
    public string NodeId { get; set; } = "";
    public string? IdempotencyKey { get; set; }
    public string? Name { get; set; }
    public string ExpectedCharacterId { get; set; } = "";
    public bool? Recheck { get; set; }
    public string? Note { get; set; }
    public string? Options { get; set; }
}

public sealed class EditTurnRequest
{
    public string NodeId { get; set; } = "";
    public string ExpectedCharacterId { get; set; } = "";
    public TurnSubmitInput? Input { get; set; }
    public List<TextBlock>? Blocks { get; set; }
    public string? IdempotencyKey { get; set; }
}

public sealed class AssignSlotRequest
{
    public string Slot { get; set; } = "";
    public bool Enabled { get; set; }
    public string ModelId { get; set; } = "";
}

public sealed class OkProvidersResult
{
    public bool Ok { get; set; }
    public List<ProviderConfig> Providers { get; set; } = new();
}

public sealed class OkResult
{
    public bool Ok { get; set; }
}

public sealed class TurnStatusResult
{
    public string TurnId { get; set; } = "";
    public string Status { get; set; } = "";
    public string? EventsUrl { get; set; }
}

public sealed class NodeResult
{
    public PlotNode Node { get; set; } = new();
    public WorldState? State { get; set; }
    public List<PlotNode> Candidates { get; set; } = new();
}

public sealed class SaveDraftRequest
{
    public string ExpectedCharacterId { get; set; } = "";
    public int ExpectedDraftVersion { get; set; }
    public string BaseRevisionId { get; set; } = "";
    public DirectorPlan Plan { get; set; } = new();
    public string? ViewNodeId { get; set; }
}

public sealed class DirectorCommandRequest
{
    public string ExpectedCharacterId { get; set; } = "";
    public string ExpectedHeadId { get; set; } = "";
    public int ExpectedVersion { get; set; }
    public string IdempotencyKey { get; set; } = "";
    public string Action { get; set; } = "";
    public string? BeatId { get; set; }
    public int? DraftVersion { get; set; }
    public bool? Replace { get; set; }
}

public sealed class DirectorCommandResult
{
    public string NodeId { get; set; } = "";
    public int Version { get; set; }
    public DirectorState State { get; set; } = new();
}

public sealed class DirectorMessageRequest
{
    public string ExpectedCharacterId { get; set; } = "";
    public string ExpectedHeadId { get; set; } = "";
    public int ExpectedDraftVersion { get; set; }
    public string IdempotencyKey { get; set; } = "";
    public string Text { get; set; } = "";
}

public sealed class DirectorMessageResult
{
    public string RequestId { get; set; } = "";
    public string Status { get; set; } = "";
    public DirectorRequest Request { get; set; } = new();
    public string StatusUrl { get; set; } = "";
    public string EventsUrl { get; set; } = "";
}

public sealed class CancelDirectorRequest
{
    public string ExpectedCharacterId { get; set; } = "";
}

public sealed class PatchMemoryRequest
{
    public string? Content { get; set; }
    public bool? Pinned { get; set; }
    public bool? Hidden { get; set; }
    public string ExpectedCharacterId { get; set; } = "";
    public string ExpectedHeadId { get; set; } = "";
    public int ExpectedVersion { get; set; }
    public string? IdempotencyKey { get; set; }
}

public sealed class OrganizeMemoriesRequest
{
    public string ExpectedCharacterId { get; set; } = "";
}

public sealed class OrganizeMemoriesResult
{
    public List<object>? Merged { get; set; }
}

public sealed class ThemeRequest
{
    public string Mode { get; set; } = "dark";
}
