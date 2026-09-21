/**
 * ttsDirector.ts
 *
 * 智能情境感知演播编译器 (Context-Aware Director Mode Compiler)
 * 为小米 MiMo 等高级自然语言受控 TTS 引擎提供电影级声音导演指令与细粒度音频标签。
 * 核心原则：完全基于当前角色卡档案、模型实时心境（liveMood）、真实场景与关系张力量表动态推导，杜绝任何静态写死。
 */

export interface DirectorContext {
  /** 角色名称 */
  characterName?: string;
  /** 角色身份/职阶（如：少年剑士、商会掌柜、隐修法师） */
  characterRole?: string;
  /** 角色性格/人设描述 */
  characterPersona?: string;
  /** 角色卡标签（用于推断性别与年龄基调） */
  characterTags?: string[];
  /** 声线原型提示（如：少年音、清脆少女、成熟女声、浑厚男声） */
  voiceArchetype?: string;
  /** 真实场景地点 (如 "落日森林边缘"、"喧闹的酒馆吧台") */
  sceneLocation?: string;
  /** 剧情/大纲阶段 (如 "初遇对峙"、"酒后倾诉") */
  sceneStage?: string;
  /** 模型实时心境文本 (如 "心怀戒备"、"羞赧不知所措"、"暴怒质问"、"释怀温和") */
  moodText?: string;
  /** 实时心境代码 (如 "alert" | "calm" | "smile" | "whisper" | "angry" | "happy" | "sad" | "fear" | "shy" 等) */
  moodCode?: string;
  /** 关系好感度与戒备度量化指标 */
  relationship?: {
    affection?: number; // -100 ~ 100
    trust?: number;     // 0 ~ 100
    alertness?: number; // 0 ~ 100
  };
  /** 本回合的旁白语境 */
  turnNarration?: string;
}

/**
 * 剥离所有导演语气标记和音频控制标签。
 * 用于 Edge-TTS, Web Speech API 等不支持高级导演语法的引擎，避免念出括号内容。
 */
export function stripAudioTags(text: string): string {
  if (!text) return "";
  return text
    // 移除圆括号语气/风格标签：(冷峻)、（冷峻深沉）
    .replace(/[（(][^）)\r\n]{1,30}[）)]/g, "")
    // 移除方括号音频标签：[停顿片刻]、[深呼吸]、[轻叹]、[轻蔑地笑]
    .replace(/[[【][^\]】\r\n]{1,30}[\]】]/g, "")
    // 清理可能产生的双空格
    .replace(/[ \t]{2,}/g, " ")
    .trim();
}

/**
 * 依据角色人设与标签，动态推断声线原型基调
 */
export function detectVoiceArchetype(ctx: {
  characterName?: string;
  characterRole?: string;
  characterPersona?: string;
  characterTags?: string[];
}): string {
  const corpus = [
    ctx.characterName ?? "",
    ctx.characterRole ?? "",
    ctx.characterPersona ?? "",
    (ctx.characterTags ?? []).join(" "),
  ].join(" ").toLowerCase();

  // 少女/幼年女性
  if (/少女|女孩|公主|小师妹|女学生|萝莉|女仆|千金|姑娘/.test(corpus)) {
    return "年轻女声，清脆灵动，充满少女感";
  }
  // 成熟女性/御姐
  if (/御姐|女王|女当家|女主人|夫人|师尊|魔女|掌门|大姐|贵妇/.test(corpus)) {
    return "成熟女声，从容优雅，声线沉着";
  }
  // 少年/青年男性
  if (/少年|男孩|王子|弟子|小哥|学弟|少侠|师弟|剑童/.test(corpus)) {
    return "清爽年轻男声，清澈明朗，富有朝气";
  }
  // 成熟/年长男性
  if (/大叔|老者|长者|师父|骑士|将军|老管家|老爷|宗师|父亲/.test(corpus)) {
    return "成熟沉稳男声，浑厚庄重，从容不迫";
  }
  // 通用性别
  if (/女|female|woman|she|her/.test(corpus)) {
    return "自然女声，咬字清晰，情感真实";
  }
  if (/男|male|man|he|him/.test(corpus)) {
    return "自然男声，沉着清晰，发声松弛";
  }

  return "发声自然清晰，角色代入感强";
}

/**
 * 根据真实角色人设、实时心境与场景动态生成专业声音导演指南 (Instruction)
 */
export function buildDirectorInstruction(ctx: DirectorContext): string {
  const parts: string[] = [];

  // 1. 角色人设与声线原型
  const charName = ctx.characterName?.trim() || "说话角色";
  const roleText = ctx.characterRole?.trim() ? `（${ctx.characterRole.trim()}）` : "";
  const archetype = ctx.voiceArchetype || detectVoiceArchetype(ctx);
  
  let personaBrief = ctx.characterPersona?.trim() || "";
  if (personaBrief.length > 50) {
    personaBrief = personaBrief.slice(0, 48) + "…";
  }
  const roleLine = personaBrief
    ? `【角色】${charName}${roleText}。${personaBrief}。声线基调：${archetype}。`
    : `【角色】${charName}${roleText}。声线基调：${archetype}。`;
  parts.push(roleLine);

  // 2. 真实场景与剧情推进阶段（不胡乱捏造）
  const loc = ctx.sceneLocation?.trim() || "剧情交谈现场";
  const stage = ctx.sceneStage?.trim();
  const sceneDesc = [loc, stage ? `阶段：${stage}` : ""].filter(Boolean).join("，");
  parts.push(`【场景】${sceneDesc}。`);

  // 3. 实时心境状态（优先使用真实大模型判定）
  if (ctx.moodText?.trim()) {
    parts.push(`【心境】${ctx.moodText.trim()}。`);
  }

  // 4. 声音演播指导（发声通道、语速顿挫、呼吸细节）
  const guidanceParts: string[] = [];
  const moodCode = (ctx.moodCode || "").toLowerCase();
  const moodText = ctx.moodText || "";
  const combinedMood = `${moodCode} ${moodText}`;

  // 情绪与呼吸节奏
  if (/angry|furious|rage|怒|火|恨/.test(combinedMood)) {
    guidanceParts.push("发声紧绷，语速偏快，气息沉重，字句间带有压迫感与质问的锐利度。");
  } else if (/fear|panic|nervous|shock|怕|慌|惊|惧/.test(combinedMood)) {
    guidanceParts.push("声线略带细微颤音与慌乱，呼吸浅急，语速稍显迟疑与无助，咬字轻短。");
  } else if (/sad|sorrow|grief|cry|悲|伤|绝望|痛/.test(combinedMood)) {
    guidanceParts.push("声线暗哑低沉，语速缓慢，带有隐忍的克制感与微弱叹息，情绪向下沉。");
  } else if (/happy|joy|laugh|smile|喜|乐|欣慰|笑/.test(combinedMood)) {
    guidanceParts.push("语调轻快飞扬，尾音微扬，带有真实的轻快笑意与放松舒畅，明亮生动。");
  } else if (/whisper|shy|soft|blush|羞|怯|轻柔|耳语/.test(combinedMood)) {
    guidanceParts.push("低语轻诉，贴近耳畔般的气声流淌，极具沉浸感与细腻情感张力，呼吸声真实可感。");
  } else if (/alert|guard|警|防|冷/.test(combinedMood)) {
    guidanceParts.push("音色沉着严谨，语调收敛警觉，字句清晰平稳，保持冷峻的审视距离。");
  } else {
    // 默认或 calm/neutral
    guidanceParts.push("发声通道松弛沉着，音调自然平缓，字句间留白恰当，节奏从容不怒自威。");
  }

  // 关系张力融入
  if (ctx.relationship) {
    if ((ctx.relationship.alertness ?? 0) >= 60) {
      guidanceParts.push("对对话者带有鲜明的戒心与阶级/立场距离感。");
    } else if ((ctx.relationship.affection ?? 0) >= 50) {
      guidanceParts.push("对对话者流露不易察觉的温情与信任，尾音微带暖意气声。");
    }
  }

  parts.push(`【指导】${guidanceParts.join(" ")}`);

  return parts.join("\n");
}

/**
 * 为纯文本台词注入与当前心境相符的电影级音频标签 (Audio Tags) 与行内风格
 */
export function enrichDialogueWithAudioTags(
  text: string,
  ctx?: DirectorContext
): string {
  const trimmed = text.trim();
  if (!trimmed) return "";

  let enriched = trimmed;

  // 1. 如果开头尚未声明情绪/风格括号，则根据实时心境动态推断前缀
  const hasStylePrefix = /^[（(][^）)\r\n]{1,15}[）)]/.test(enriched);
  if (!hasStylePrefix) {
    const moodCode = (ctx?.moodCode || "").toLowerCase();
    const moodText = ctx?.moodText || "";
    const combinedMood = `${moodCode} ${moodText}`;

    let prefix = "";
    if (/angry|furious|怒|恨/.test(combinedMood)) {
      prefix = "(愤懑质问)";
    } else if (/fear|panic|怕|慌|惊/.test(combinedMood)) {
      prefix = "(慌乱不安)";
    } else if (/sad|sorrow|悲|伤|痛/.test(combinedMood)) {
      prefix = "(哽咽低沉)";
    } else if (/happy|joy|喜|乐|欣慰/.test(combinedMood)) {
      prefix = "(欣喜轻快)";
    } else if (/shy|blush|羞|怯/.test(combinedMood)) {
      prefix = "(羞赧轻声)";
    } else if (/whisper|耳语/.test(combinedMood)) {
      prefix = "(低语轻柔)";
    } else if (/alert|guard|警|防/.test(combinedMood)) {
      prefix = "(严肃警觉)";
    } else if (moodText.length > 0 && moodText.length <= 4) {
      // 直接使用精简心境词
      prefix = `(${moodText})`;
    } else if (moodCode === "calm") {
      prefix = "(平缓沉着)";
    }

    if (prefix) {
      enriched = `${prefix}${enriched}`;
    }
  }

  // 2. 根据心境匹配笑声或叹气等动作标签
  const moodCode = (ctx?.moodCode || "").toLowerCase();
  const moodText = ctx?.moodText || "";
  const combinedMood = `${moodCode} ${moodText}`;

  if (/(?:^|[，。！？\s)）])(呵|哼)(?:[，。！？\s]|$)/.test(enriched)) {
    if (/happy|joy|smile|喜|乐/.test(combinedMood)) {
      enriched = enriched.replace(/(呵|哼)/, "[轻笑]$1");
    } else {
      enriched = enriched.replace(/(呵|哼)/, "[冷哼]$1");
    }
  } else if (/sad|sorrow|whisper|叹|疲|累/.test(combinedMood) || ctx?.turnNarration?.includes("叹")) {
    if (!enriched.includes("[轻叹]") && !enriched.includes("[深呼吸]")) {
      enriched = enriched.replace(/^([（(][^）)\r\n]+[）)])/, "$1[轻叹]");
    }
  }

  // 3. 破折号与省略号增强为具有戏剧张力的停顿（限制1处，保持自然留白）
  let ellipsisCount = 0;
  enriched = enriched.replace(/……/g, () => {
    ellipsisCount++;
    return ellipsisCount === 1 ? "……[停顿片刻]" : "……";
  });

  return enriched;
}

/**
 * 判断当前 TTS 设置是否指向支持自然语言导演控制的小米 MiMo 引擎
 */
export function isMiMoEngine(settings: {
  engine?: string;
  customModel?: string;
  customBaseUrl?: string;
}): boolean {
  if (settings.engine === "mimo") return true;
  const model = settings.customModel?.toLowerCase() || "";
  const url = settings.customBaseUrl?.toLowerCase() || "";
  return model.includes("mimo") || url.includes("xiaomimimo");
}
