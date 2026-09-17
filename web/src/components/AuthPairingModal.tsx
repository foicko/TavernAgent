// AuthPairingModal: 局域网设备首次访问安全配对弹窗 (SEC-01)
import React, { useState, useEffect } from "react";
import { api, setStoredAuthToken } from "../app/api";
import { clearSessionCache } from "../stores/storyStore";
import { Banner } from "../ui/Banner";
import { Button } from "../ui/Button";
import { Modal } from "../ui/Modal";
import "./AuthPairingModal.css";

interface Props {
  open: boolean;
  onSuccess: () => void;
  onClose?: () => void;
}

export const AuthPairingModal: React.FC<Props> = ({ open, onSuccess, onClose }) => {
  const [pin, setPin] = useState("");
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    if (open) {
      setPin("");
      setError(null);
    }
  }, [open]);

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!pin.trim()) {
      setError("请输入 6 位数字配对码");
      return;
    }
    setLoading(true);
    setError(null);
    try {
      const res = await api.pair(pin.trim());
      if (res.ok && res.token) {
        setStoredAuthToken(res.token);
        clearSessionCache(); // 身份变了，旧身份下缓存的会话视图不能再用
        onSuccess();
      } else {
        setError("配对失败，请检查配对码是否正确");
      }
    } catch (err: unknown) {
      let msg = "配对验证失败";
      const failure = err as { body?: string; message?: string };
      try {
        const parsed = JSON.parse(failure.body || failure.message || "{}");
        if (parsed.code === "INVALID_PIN") {
          msg = "配对码错误，请核对控制台输出的 6 位 PIN 码";
        } else if (parsed.code === "TOO_MANY_ATTEMPTS") {
          msg = "尝试次数过多，已被暂时限流，请稍后再试";
        } else if (parsed.message) {
          msg = parsed.message;
        }
      } catch {
        msg = failure.message || "连接服务器失败";
      }
      setError(msg);
    } finally {
      setLoading(false);
    }
  };

  return (
    <Modal
      open={open}
      label="局域网访问安全配对"
      title="局域网访问安全配对"
      size="sm"
      // 没有 onClose 时是阻塞式门禁：不给关闭按钮，也不能用 Esc / 点遮罩绕过。
      dismissible={Boolean(onClose)}
      onClose={() => onClose?.()}
      dialogClassName="pairing-dialog"
    >
      <div className="pairing-lead">
        <span className="pairing-icon" aria-hidden="true">
          🔐
        </span>
        <p>
          检测到您正通过局域网远程连接本服务。为保护您的故事记录与模型 API Key 额度，请输入服务端启动控制台打印的{" "}
          <b>6 位数字配对码</b>。
        </p>
      </div>

      <form onSubmit={handleSubmit} className="pairing-form">
        <input
          type="text"
          autoFocus
          maxLength={12}
          className="ui-input pairing-pin"
          aria-label="6 位配对码"
          placeholder="请输入 6 位配对码 (如 849201)"
          value={pin}
          onChange={(e) => setPin(e.target.value)}
        />

        {error ? <Banner tone="error">{error}</Banner> : null}

        <div className="pairing-actions">
          {onClose ? <Button onClick={onClose}>取消</Button> : null}
          <Button type="submit" variant="primary" loading={loading}>
            {loading ? "正在配对…" : "配对并授权连接"}
          </Button>
        </div>
      </form>
    </Modal>
  );
};
