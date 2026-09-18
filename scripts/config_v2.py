"""配置 API v2 的小工具：模型实例 + 槽位引用。

背景：配置从 v1（槽位内联 kind/baseUrl/model/密钥/窗口/温度）迁到 v2 后，
连接参数只属于「模型实例」，槽位仅保存 enabled + modelId（空 = 跟随主线）。
评测/冒烟脚本常需要按槽位或窗口差异建多个实例，这里收敛成唯一复用点，
避免每个脚本各写一遍 v1 形状（那样会被后端以 422 SLOT_TAKES_REFERENCE_ONLY 拒绝）。
"""


def save_instance(client, *, name, kind, base_url, model, key="",
                  temperature=None, max_tokens=0, context_window=0):
    """创建模型实例并返回其 ID。空 key 表示交给槽位级环境变量提供。"""
    body = {"name": name[:128], "kind": kind, "baseUrl": base_url, "model": model}
    if key:
        body["apiKey"] = key
    if temperature is not None:
        body["temperature"] = temperature
    if max_tokens:
        body["maxTokens"] = max_tokens
    if context_window:
        body["contextWindow"] = context_window
    return client.must("POST", "/api/v1/config/models", body)["id"]


def assign_slot(client, slot, model_id=None, *, enabled=True):
    """把槽位指向实例；model_id 为空表示跟随主线。"""
    return client.must("PUT", "/api/v1/config/provider",
                       {"slot": slot, "enabled": enabled, "modelId": model_id or ""})


def configure_slot(client, slot, *, name, kind, base_url, model, key="",
                   temperature=None, max_tokens=0, context_window=0, enabled=True):
    """一步完成「建实例 + 指派槽位」——v1 单次 PUT 的最接近等价物。"""
    instance = save_instance(client, name=name, kind=kind, base_url=base_url, model=model,
                             key=key, temperature=temperature,
                             max_tokens=max_tokens, context_window=context_window)
    assign_slot(client, slot, instance, enabled=enabled)
    return instance
