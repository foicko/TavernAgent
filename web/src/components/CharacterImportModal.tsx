// 导入与开局使用同一份预览，保留玩家身份和所选开场。
import React, { useEffect, useRef, useState } from "react";
import { useUi } from "../stores/uiStore";
import { CHARACTER_PRESETS, SELECTABLE_PRESET_KEYS, getPresetCardJson } from "../lib/characterPresets";
import { saveImportedCard } from "../lib/characterCardStore";
import type { CardPreview } from "../app/types";
import { useStory } from "../stores/storyStore";
import { api } from "../app/api";
import { DEFAULT_PLAYER_NAME } from "../lib/characterMacros";
import { describeCapabilities } from "../lib/cardCapabilities";
import { Avatar } from "../ui/Avatar";
import { Banner } from "../ui/Banner";
import { Button } from "../ui/Button";
import { Field, Select, TextArea, TextInput } from "../ui/Field";
import { Icon } from "../ui/Icon";
import { Modal } from "../ui/Modal";
import { Tag } from "../ui/Pill";
import "./CharacterImportModal.css";

export const CharacterImportModal: React.FC = () => {
  const { charImportOpen, setCharImportOpen, pendingCardId, notifyQuiet } = useUi();
  const createSession = useStory((s) => s.createSession);

  const [parsing, setParsing] = useState(false);
  const [creating, setCreating] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const parseSequence = useRef(0);
  const [importedPreview, setImportedPreview] = useState<CardPreview | null>(null);
  const [playerName, setPlayerName] = useState(DEFAULT_PLAYER_NAME);
  const [playerRole, setPlayerRole] = useState("");
  const [openingId, setOpeningId] = useState("");
  const [customOpening, setCustomOpening] = useState("");

  useEffect(() => {
    if (!charImportOpen) {
      parseSequence.current++;
      setImportedPreview(null);
      setParsing(false);
      setCreating(false);
      setError(null);
      setCustomOpening("");
    }
  }, [charImportOpen]);

  // 从卡库直接开新局：卡已在服务端，只需拉回预览，不必让用户重新上传文件。
  useEffect(() => {
    if (!charImportOpen || !pendingCardId) return;
    const sequence = ++parseSequence.current;
    setParsing(true);
    setError(null);
    setImportedPreview(null);
    api
      .getCard(pendingCardId)
      .then((preview) => {
        if (sequence !== parseSequence.current) return;
        setImportedPreview(preview);
        setOpeningId(preview.openings?.[0]?.variantId ?? "");
        setCustomOpening("");
      })
      .catch((err: unknown) => {
        if (sequence !== parseSequence.current) return;
        const msg = err instanceof Error ? err.message : String(err);
        setError("读取角色卡失败：" + msg);
      })
      .finally(() => {
        if (sequence === parseSequence.current) setParsing(false);
      });
  }, [charImportOpen, pendingCardId]);

  const handlePresetSelect = (key: string) => {
    if (creating) return;
    const preset = CHARACTER_PRESETS[key];
    parseSequence.current++;
    setParsing(false);
    setError(null);
    setImportedPreview({ name: preset.name, avatar: preset.avatar, format: "内置角色", characterJson: getPresetCardJson(key),
      supported: [], ignored: [], warnings: [], openings: [{ variantId: `open_${preset.key}`, title: "开场序幕", text: preset.prologue }] });
    setOpeningId(`open_${preset.key}`);
  };

  const close = () => { if (!creating) setCharImportOpen(false); };

  const handleFile = async (file: File) => {
    if (creating) return;
    const sequence = ++parseSequence.current;
    setParsing(true);
    setError(null);
    setImportedPreview(null);
    try {
      // 角色的唯一权威解析在服务端：前端只负责上传与呈现错误。
      const preview = await api.importCardFile(file);
      if (sequence !== parseSequence.current) return;
      setImportedPreview(preview);
      setOpeningId(preview.openings[0]?.variantId ?? "");
      setCustomOpening("");
      notifyQuiet(`已成功解析角色卡：${preview.name} (${preview.format || "标准卡"})`);
    } catch (err: unknown) {
      if (sequence !== parseSequence.current) return;
      const msg = err instanceof Error ? err.message : String(err);
      // 只留 Banner（就地、可读），不再叠加一条重复的 toast。
      setError("卡片解析失败：" + msg);
    } finally {
      if (sequence === parseSequence.current) setParsing(false);
    }
  };

  const handleDrop = (e: React.DragEvent) => {
    e.preventDefault();
    const file = e.dataTransfer.files[0];
    if (file) void handleFile(file);
  };

  const handleConfirmImport = async () => {
    if (!importedPreview || creating || parsing) return;
    if (!playerName.trim() || (importedPreview.openings.length === 0 && !customOpening.trim())) {
      setError("请填写玩家姓名和开场内容。");
      return;
    }
    const sequence = parseSequence.current;
    setCreating(true);
    setError(null);
    try {
      notifyQuiet(`正在为 ${importedPreview.name} 创建全新跑团冒险...`);
      // 本地角色库只影响刷新后的立绘展示，写入失败不阻断建会话，但要如实告知。
      let storageWarning = "";
      try {
        const saved = saveImportedCard({
          cardId: importedPreview.cardId,
          name: importedPreview.name,
          shortName: importedPreview.shortName,
          role: importedPreview.role,
          avatar: importedPreview.avatar,
          format: importedPreview.format,
          tags: importedPreview.tags,
          creator: importedPreview.creator,
          characterVersion: importedPreview.characterVersion,
          description: importedPreview.description,
          personality: importedPreview.personality,
          scenario: importedPreview.scenario,
          firstMes: importedPreview.firstMes,
          mesExample: importedPreview.mesExample,
          systemPrompt: importedPreview.systemPrompt,
          creatorNotes: importedPreview.creatorNotes,
          characterJson: importedPreview.characterJson,
        });
        if (!saved.persisted) {
          storageWarning = "本地角色库空间不足，这张卡未能写入；刷新页面后可能需要重新导入。";
        } else if (saved.evicted > 0) {
          storageWarning = `本地角色库空间不足，已清理 ${saved.evicted} 张最早的卡片记录。`;
        }
      } catch {
        // 本地存储异常不阻断创建会话
      }

      await createSession({
        title: `${importedPreview.name} 的冒险`,
        // 卡已在服务端卡库时只传 cardId：请求体不再携带兆级 characterJson。
        cardId: importedPreview.cardId,
        characterJson: importedPreview.cardId ? undefined : importedPreview.characterJson,
        playerName: playerName.trim(),
        playerRole: playerRole.trim() || undefined,
        openingVariantId: importedPreview.openings.length > 0 ? openingId : undefined,
        openingText: importedPreview.openings.length === 0 ? customOpening.trim() : undefined,
      });
      if (sequence !== parseSequence.current) return;
      setCharImportOpen(false);
      if (window.innerWidth < 1024) useUi.getState().closeAllDrawers();
      notifyQuiet(storageWarning || "角色已成功导入并开启新篇章！");
    } catch (error) {
      if (sequence !== parseSequence.current) return;
      const message = "导入失败：" + (error instanceof Error ? error.message : String(error));
      setError(message);
      notifyQuiet(message);
    } finally {
      if (sequence === parseSequence.current) setCreating(false);
    }
  };

  return (
    <Modal
      open={charImportOpen}
      onClose={close}
      label="选择角色，开始新故事"
      id="char-import-modal"
      dialogClassName="char-import-window"
      closeLabel="关闭角色导入"
      // creating 期间拦截关闭入口：按钮可见但禁用，Esc 与遮罩点击同样无效。
      dismissible={!creating}
      closeDisabled={creating}
      title={<span id="char-import-title" className="char-import-title">选择角色，开始新故事</span>}
    >
      {/* 拖拽上传区 */}
      <div
        className="import-dropzone"
        role="button"
        tabIndex={creating ? -1 : 0}
        aria-label="选择角色卡文件"
        onKeyDown={event => { if (!creating && (event.key === "Enter" || event.key === " ")) { event.preventDefault(); document.getElementById("char-card-file-input")?.click(); } }}
        onDragOver={(e) => e.preventDefault()}
        onDrop={handleDrop}
        onClick={() => { if (!creating) document.getElementById("char-card-file-input")?.click(); }}
      >
        <input
          id="char-card-file-input"
          className="char-card-file-input"
          type="file"
          accept=".png,.json,.charx"
          disabled={creating}
          onChange={(e) => {
            const f = e.target.files?.[0];
            e.target.value = "";
            if (f) void handleFile(f);
          }}
        />
        <div className="import-dropzone__icon"><Icon name="download" size={22} /></div>
        <div className="import-dropzone__title">
          {parsing ? "正在读取角色设定与开场…" : "点击或拖拽 PNG / JSON / CHARX 角色卡至此"}
        </div>
        <div className="import-dropzone__hint">
          支持原生角色卡、Tavern V2 / V3 与 charx 归档（含内嵌立绘）；导入后可选择身份与开场。
        </div>
      </div>

      {/* 解析成功预览 */}
      {error && <Banner tone="error">{error}</Banner>}
      {importedPreview && (
        <div className="char-import-preview">
          <div className="char-import-preview__row">
            <Avatar src={importedPreview.avatar} name={importedPreview.name} size={64} shape="rounded" className="char-import-preview__avatar" />
            <div className="char-import-preview__main">
              <div className="char-import-preview__title-row">
                <span className="char-import-preview__name">解析就绪：{importedPreview.name}</span>
                {importedPreview.format && (
                  <Tag tone="info">{importedPreview.format}</Tag>
                )}
                {importedPreview.creator && (
                  <span className="char-import-preview__creator">by {importedPreview.creator}</span>
                )}
              </div>
              {importedPreview.tags && importedPreview.tags.length > 0 && (
                <div className="char-import-preview__tags">
                  {importedPreview.tags.slice(0, 5).map((t, idx) => (
                    <Tag key={idx}>#{t}</Tag>
                  ))}
                </div>
              )}
              <div className="char-import-preview__blurb">
                {(importedPreview.openings.find(opening => opening.variantId === openingId)?.text ?? customOpening).slice(0, 240)}
              </div>
            </div>
          </div>
          <div className="char-import-preview__fields">
            <Field label="玩家姓名">
              <TextInput value={playerName} disabled={creating} maxLength={80} onChange={event => setPlayerName(event.target.value)} />
            </Field>
            <Field label="身份或背景（可选）">
              <TextInput value={playerRole} disabled={creating} maxLength={400} placeholder="例如：来自南方的游历者" onChange={event => setPlayerRole(event.target.value)} />
            </Field>
          </div>
          {importedPreview.openings.length > 0 ? (
            <Field label="选择开场" className="char-import-preview__opening">
              <Select value={openingId} disabled={creating} onChange={event => setOpeningId(event.target.value)}>
                {importedPreview.openings.map((opening, index) => <option key={opening.variantId || index} value={opening.variantId}>{opening.title || `开场 ${index + 1}`}</option>)}
              </Select>
            </Field>
          ) : (
            <Field label="这张卡没有开场，请填写故事的第一幕" className="char-import-preview__opening">
              <TextArea rows={4} value={customOpening} disabled={creating} onChange={event => setCustomOpening(event.target.value)} />
            </Field>
          )}
          {describeCapabilities(importedPreview.capabilities) && (
            <Banner tone="info">{describeCapabilities(importedPreview.capabilities)}</Banner>
          )}
          {importedPreview.warnings && importedPreview.warnings.length > 0 && (
            <Banner tone="warn">兼容提示：{importedPreview.warnings.join("；")}</Banner>
          )}
          <div className="char-import-preview__actions">
            <Button
              variant="primary"
              loading={creating}
              disabled={creating || parsing || !playerName.trim() || (importedPreview.openings.length === 0 && !customOpening.trim())}
              onClick={handleConfirmImport}
            >
              {creating ? "创建中…" : "开启冒险"}
            </Button>
          </div>
        </div>
      )}

      {/* 预设卡片一键体验矩阵（仅在存在内置预设时展示） */}
      {SELECTABLE_PRESET_KEYS.length > 0 && (
        <div>
          <div className="char-preset-picker__title"><Icon name="sparkles" size={13} /> 选择内置角色</div>
          <div className="char-preset-picker__grid">
            {SELECTABLE_PRESET_KEYS.map((key) => {
              const c = CHARACTER_PRESETS[key];
              if (!c) return null;
              return (
                <button
                  key={c.key}
                  className="char-preset-preview-card"
                  disabled={creating}
                  type="button"
                  onClick={() => handlePresetSelect(c.key)}
                >
                  <img src={c.avatar} alt={c.name} className="char-preset-preview-card__avatar" />
                  <div className="char-preset-preview-card__meta">
                    <div className="char-preset-preview-card__name">{c.name}</div>
                    <div className="char-preset-preview-card__role">{c.role}</div>
                  </div>
                </button>
              );
            })}
          </div>
        </div>
      )}
    </Modal>
  );
};
