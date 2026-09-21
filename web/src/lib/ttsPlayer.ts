import { synthesizeTTS } from "../app/api";
import { useTTSSettings } from "../stores/ttsSettingsStore";

type PlaybackListener = (currentText: string | null) => void;

class TTSAudioPlayer {
  private currentAudio: HTMLAudioElement | null = null;
  private currentText: string | null = null;
  private audioCache = new Map<string, string>(); // key -> objectURL
  private listeners = new Set<PlaybackListener>();

  subscribe(listener: PlaybackListener): () => void {
    this.listeners.add(listener);
    listener(this.currentText);
    return () => this.listeners.delete(listener);
  }

  private notify() {
    for (const l of this.listeners) {
      l(this.currentText);
    }
  }

  getCurrentText(): string | null {
    return this.currentText;
  }

  stop() {
    if (this.currentAudio) {
      this.currentAudio.pause();
      this.currentAudio.currentTime = 0;
      this.currentAudio = null;
    }
    if (typeof window !== "undefined" && "speechSynthesis" in window) {
      window.speechSynthesis.cancel();
    }
    this.currentText = null;
    this.notify();
  }

  async play(text: string, voiceOverride?: string): Promise<void> {
    const trimmed = text.trim();
    if (!trimmed) return;

    // 如果正在播同一段，则停止（Toggle 行为）
    if (this.currentText === trimmed) {
      this.stop();
      return;
    }

    this.stop();
    this.currentText = trimmed;
    this.notify();

    const settings = useTTSSettings.getState();
    const engine = settings.engine;
    const speed = settings.speed;

    // 浏览器原生 Web Speech API
    if (engine === "web-speech") {
      this.playWebSpeech(trimmed, speed);
      return;
    }

    const voice =
      voiceOverride ||
      (engine === "openai" ? settings.customVoice : settings.edgeVoice);
    const cacheKey = `${engine}:${voice}:${speed}:${settings.customBaseUrl}:${trimmed}`;

    try {
      let objectUrl = this.audioCache.get(cacheKey);
      if (!objectUrl) {
        const blob = await synthesizeTTS({
          engine,
          voice,
          text: trimmed,
          baseUrl: engine === "openai" ? settings.customBaseUrl : undefined,
          apiKey: engine === "openai" ? settings.customApiKey : undefined,
          model: engine === "openai" ? settings.customModel : undefined,
          speed,
        });
        objectUrl = URL.createObjectURL(blob);
        this.audioCache.set(cacheKey, objectUrl);
      }

      const audio = new Audio(objectUrl);
      this.currentAudio = audio;

      audio.onended = () => {
        if (this.currentAudio === audio) {
          this.currentAudio = null;
          this.currentText = null;
          this.notify();
        }
      };

      audio.onerror = () => {
        if (this.currentAudio === audio) {
          this.fallbackSpeechSynthesis(trimmed);
        }
      };

      await audio.play();
    } catch {
      // 离线或后端不可用时降级为 Web Speech API
      this.fallbackSpeechSynthesis(trimmed);
    }
  }

  private playWebSpeech(text: string, speed = 1.0) {
    if (typeof window === "undefined" || !("speechSynthesis" in window)) {
      this.currentText = null;
      this.notify();
      return;
    }

    try {
      const utter = new SpeechSynthesisUtterance(text);
      utter.lang = "zh-CN";
      utter.rate = speed;
      utter.onend = () => {
        this.currentText = null;
        this.notify();
      };
      utter.onerror = () => {
        this.currentText = null;
        this.notify();
      };
      window.speechSynthesis.speak(utter);
    } catch {
      this.currentText = null;
      this.notify();
    }
  }

  private fallbackSpeechSynthesis(text: string) {
    const speed = useTTSSettings.getState().speed;
    this.playWebSpeech(text, speed);
  }
}

export const ttsPlayer = new TTSAudioPlayer();
