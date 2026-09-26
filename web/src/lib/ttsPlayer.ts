import { synthesizeTTS } from "../app/api";
import { useTTSSettings } from "../stores/ttsSettingsStore";
import { stripAudioTags, isMiMoEngine } from "./ttsDirector";

type PlaybackListener = (currentText: string | null) => void;

export interface TTSPlayOptions {
  voice?: string;
  instruction?: string;
}

/** 合成音频缓存上限：blob URL 占着整段音频内存，不能无限增长。 */
const MAX_CACHED_AUDIO = 20;

class TTSAudioPlayer {
  private currentAudio: HTMLAudioElement | null = null;
  private currentUrl: string | null = null;
  private currentText: string | null = null;
  private audioCache = new Map<string, string>(); // key -> objectURL，按最近使用排序（LRU）
  private listeners = new Set<PlaybackListener>();
  /** 播放请求代号：stop()/新的 play() 都会让它前进，await 之后据此判断本次请求是否已作废。 */
  private playSeq = 0;

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
    this.playSeq++;
    if (this.currentAudio) {
      this.currentAudio.pause();
      this.currentAudio.currentTime = 0;
      this.currentAudio = null;
    }
    this.currentUrl = null;
    if (typeof window !== "undefined" && "speechSynthesis" in window) {
      window.speechSynthesis.cancel();
    }
    this.currentText = null;
    this.notify();
  }

  async play(text: string, voiceOrOptions?: string | TTSPlayOptions): Promise<void> {
    const trimmed = text.trim();
    if (!trimmed) return;

    // 如果正在播同一段，则停止（Toggle 行为）
    if (this.currentText === trimmed) {
      this.stop();
      return;
    }

    this.stop();
    const seq = this.playSeq;
    this.currentText = trimmed;
    this.notify();

    let voiceOverride: string | undefined;
    let instruction: string | undefined;
    if (typeof voiceOrOptions === "string") {
      voiceOverride = voiceOrOptions;
    } else if (voiceOrOptions && typeof voiceOrOptions === "object") {
      voiceOverride = voiceOrOptions.voice;
      instruction = voiceOrOptions.instruction;
    }

    const settings = useTTSSettings.getState();
    const engine = settings.engine;
    const speed = settings.speed;

    const isMiMo = isMiMoEngine(settings);

    // 若引擎不支持高级标签，或用户未启用导演模式，自动剥离括号/标签避免念出标记词
    const shouldStripTags = !isMiMo || !settings.directorMode;
    const textToSynthesize = shouldStripTags ? stripAudioTags(trimmed) : trimmed;
    const activeInstruction = isMiMo && settings.directorMode ? instruction : undefined;

    // 浏览器原生 Web Speech API
    if (engine === "web-speech") {
      this.playWebSpeech(stripAudioTags(trimmed), speed);
      return;
    }

    const voice =
      voiceOverride ||
      (engine === "openai" || engine === "mimo" ? settings.customVoice : settings.edgeVoice);
    const cacheKey = `${engine}:${voice}:${speed}:${settings.customBaseUrl}:${activeInstruction || ""}:${textToSynthesize}`;

    try {
      let objectUrl = this.audioCache.get(cacheKey);
      if (!objectUrl) {
        const needsCustomCredentials = engine === "openai" || engine === "mimo";
        const blob = await synthesizeTTS({
          engine,
          voice,
          text: textToSynthesize,
          instruction: activeInstruction,
          baseUrl: needsCustomCredentials ? settings.customBaseUrl : undefined,
          apiKey: needsCustomCredentials ? settings.customApiKey : undefined,
          model: needsCustomCredentials ? settings.customModel : undefined,
          speed,
        });
        objectUrl = URL.createObjectURL(blob);
      }
      // 合成结果照样入缓存（下次点同一段可以秒播），但请求若已被 stop / 新段落取代就不再出声
      this.rememberAudio(cacheKey, objectUrl);
      if (seq !== this.playSeq) return;

      const audio = new Audio(objectUrl);
      this.currentAudio = audio;
      this.currentUrl = objectUrl;

      audio.onended = () => {
        if (this.currentAudio === audio) {
          this.currentAudio = null;
          this.currentUrl = null;
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
      // 已作废的请求（包括 stop 打断 audio.play() 引起的拒绝）不能再降级出声
      if (seq !== this.playSeq) return;
      // 离线或后端不可用时降级为 Web Speech API
      this.fallbackSpeechSynthesis(trimmed);
    }
  }

  /** 写入 / 刷新 LRU 缓存；超限时淘汰最久未用的条目并释放其 blob URL（正在播放的除外）。 */
  private rememberAudio(key: string, url: string) {
    this.audioCache.delete(key);
    this.audioCache.set(key, url);
    for (const [k, u] of this.audioCache) {
      if (this.audioCache.size <= MAX_CACHED_AUDIO) break;
      if (u === this.currentUrl) continue;
      this.audioCache.delete(k);
      URL.revokeObjectURL(u);
    }
  }

  private playWebSpeech(text: string, speed = 1.0) {
    if (typeof window === "undefined" || !("speechSynthesis" in window)) {
      this.currentText = null;
      this.notify();
      return;
    }

    // cancel() 会异步触发旧 utterance 的 onend/onerror，只允许本次请求清理播放状态
    const seq = this.playSeq;
    const finish = () => {
      if (seq !== this.playSeq) return;
      this.currentText = null;
      this.notify();
    };
    try {
      const utter = new SpeechSynthesisUtterance(text);
      utter.lang = "zh-CN";
      utter.rate = speed;
      utter.onend = finish;
      utter.onerror = finish;
      window.speechSynthesis.speak(utter);
    } catch {
      this.currentText = null;
      this.notify();
    }
  }

  private fallbackSpeechSynthesis(text: string) {
    const speed = useTTSSettings.getState().speed;
    this.playWebSpeech(stripAudioTags(text), speed);
  }
}

export const ttsPlayer = new TTSAudioPlayer();
