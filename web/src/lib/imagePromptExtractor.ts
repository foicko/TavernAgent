import type { TextBlock } from "../app/types";
import type { ImageStylePreset } from "../stores/imageSettingsStore";

export interface ExtractedImagePrompt {
  prompt: string;
  negativePrompt: string;
  excerpt: string;
}

const STYLE_PROMPTS: Record<ImageStylePreset, { positive: string; negative: string }> = {
  anime: {
    positive: "masterpiece, best quality, anime visual novel CG, 16:9 wallpaper, Makoto Shinkai aesthetic, atmospheric lighting, detailed background",
    negative: "low quality, worst quality, deformed hands, extra limbs, bad anatomy, blurry, text, watermark, signature",
  },
  fantasy: {
    positive: "masterpiece, dark high fantasy illustration, intricate details, dramatic rim lighting, epic scene, painterly style",
    negative: "worst quality, low quality, modern elements, deformed, blurry, bad hands, artifacts, watermark",
  },
  cinematic: {
    positive: "masterpiece, 35mm film still, cinematic lighting, depth of field, atmospheric, volumetric light, 8k wallpaper",
    negative: "anime, cartoon, sketch, worst quality, low quality, deformed, blurry, watermark",
  },
  realistic: {
    positive: "masterpiece, ultra realistic photography, natural soft lighting, sharp focus, 8k uhd, highly detailed texture",
    negative: "drawing, painting, illustration, cartoon, anime, worst quality, low quality, deformed, blurry",
  },
};

/**
 * 从故事回合的文本块或纯文本中提取画面核心描述，并结合艺术风格组装为提示词。
 */
export function extractImagePrompt(
  source: {
    blocks?: TextBlock[];
    text?: string;
    characterName?: string;
  },
  stylePreset: ImageStylePreset = "anime"
): ExtractedImagePrompt {
  let content = "";

  if (source.blocks && source.blocks.length > 0) {
    // 优先提取场景描写 (narration) 与 角色言行
    content = source.blocks
      .map((b) => b.text)
      .join(" ")
      .trim();
  } else {
    content = (source.text ?? "").trim();
  }

  // 清洗括号注释、心理描写与 Markdown 符号
  content = content
    .replace(/（[^）]*）/g, "")
    .replace(/\([^)]*\)/g, "")
    .replace(/[*#_`]/g, " ")
    .replace(/\s+/g, " ")
    .trim();

  // 截取前 200 个字符作为精简场景概括
  const runes = Array.from(content);
  const excerpt = runes.slice(0, 180).join("");

  const styleConfig = STYLE_PROMPTS[stylePreset] || STYLE_PROMPTS.anime;

  // 组装最终生图 Prompt
  let prompt = excerpt ? `${excerpt}, ${styleConfig.positive}` : styleConfig.positive;
  if (source.characterName) {
    prompt = `Character: ${source.characterName}, ${prompt}`;
  }

  return {
    prompt,
    negativePrompt: styleConfig.negative,
    excerpt,
  };
}
