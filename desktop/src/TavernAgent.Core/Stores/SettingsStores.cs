using System.Text.Json;
using System.Text.Json.Serialization;

namespace TavernAgent.Core.Stores;

public enum TTSEngine { Edge, Openai, Mimo, WebSpeech }

public sealed class TTSSettings
{
    public TTSEngine Engine { get; set; } = TTSEngine.Edge;
    public string EdgeVoice { get; set; } = "zh-CN-XiaoxiaoNeural";
    public string CustomBaseUrl { get; set; } = "http://127.0.0.1:9880/v1";
    public string CustomApiKey { get; set; } = "";
    public string CustomModel { get; set; } = "tts-1";
    public string CustomVoice { get; set; } = "alloy";
    public double Speed { get; set; } = 1.0;
    public bool DialogueOnly { get; set; } = true;
    public bool DirectorMode { get; set; } = true;

    public static TTSSettings Defaults { get; } = new();
}

public enum ImageEngine { ComfyUI, OpenAI, SdWebUI }

public sealed class ImageSettings
{
    public ImageEngine Engine { get; set; } = ImageEngine.ComfyUI;
    public string CustomBaseUrl { get; set; } = "http://127.0.0.1:8188";
    public string CustomApiKey { get; set; } = "";
    public string CustomModel { get; set; } = "image_qwen_image_2_1_t2i";
    public string Size { get; set; } = "1024x576";
    public string StylePreset { get; set; } = "anime";

    public static ImageSettings Defaults { get; } = new();
}

public enum EmbeddingProviderType { Ollama, SiliconFlow, OpenAI, Custom }

public sealed class EmbeddingSettings
{
    public bool Enabled { get; set; } = true;
    public EmbeddingProviderType Provider { get; set; } = EmbeddingProviderType.Ollama;
    public string BaseUrl { get; set; } = "http://127.0.0.1:11434/v1";
    public string ApiKey { get; set; } = "";
    public string Model { get; set; } = "bge-small-zh-v1.5";
    public int Dimension { get; set; } = 512;
    public double HybridRRFWeight { get; set; } = 0.5;
    public int TopK { get; set; } = 5;

    public static EmbeddingSettings Defaults { get; } = new();
}

public enum OptionMode { Direct, FillEdit }
public enum OptionsFrequency { Auto, Always, Never }

public sealed class InteractionSettings
{
    public OptionMode OptionMode { get; set; } = OptionMode.Direct;
    public OptionsFrequency OptionsFrequency { get; set; } = OptionsFrequency.Auto;
    public bool AutoContinue { get; set; }

    public static InteractionSettings Defaults { get; } = new();
}

/// <summary>JSON-file backed settings persisted under the app data dir (parity with zustand persist stores).</summary>
public static class SettingsPersistence
{
    private static readonly JsonSerializerOptions Options = new(JsonSerializerDefaults.Web)
    {
        WriteIndented = true,
        Converters = { new JsonStringEnumConverter() },
    };

    public static string DataDir
    {
        get
        {
            var root = Environment.GetFolderPath(Environment.SpecialFolder.ApplicationData);
            if (string.IsNullOrEmpty(root)) root = Path.GetTempPath();
            return Path.Combine(root, "TavernAgent");
        }
    }

    public static T Load<T>(string fileName, T defaults) where T : class
    {
        try
        {
            var path = Path.Combine(DataDir, fileName);
            if (File.Exists(path))
            {
                var loaded = JsonSerializer.Deserialize<T>(File.ReadAllText(path), Options);
                if (loaded is not null) return loaded;
            }
        }
        catch { /* ignore corrupt/partial files */ }
        return defaults;
    }

    public static void Save<T>(string fileName, T value) where T : class
    {
        try
        {
            Directory.CreateDirectory(DataDir);
            File.WriteAllText(Path.Combine(DataDir, fileName), JsonSerializer.Serialize(value, Options));
        }
        catch { /* ignore */ }
    }

    public static TTSSettings LoadTts() => Load("tts_settings.json", TTSSettings.Defaults);
    public static void SaveTts(TTSSettings s) => Save("tts_settings.json", s);

    public static ImageSettings LoadImage() => Load("image_settings.json", ImageSettings.Defaults);
    public static void SaveImage(ImageSettings s) => Save("image_settings.json", s);

    public static EmbeddingSettings LoadEmbedding() => Load("embedding_settings.json", EmbeddingSettings.Defaults);
    public static void SaveEmbedding(EmbeddingSettings s) => Save("embedding_settings.json", s);

    public static InteractionSettings LoadInteraction() => Load("interaction_settings.json", InteractionSettings.Defaults);
    public static void SaveInteraction(InteractionSettings s) => Save("interaction_settings.json", s);
}
