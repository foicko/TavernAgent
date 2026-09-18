"""配置辅助模块的契约：只建实例 + 只按引用指派槽位。

锁住 v2 的关键约束，避免有人把 kind/baseUrl/model 又写回槽位请求
（后端会以 422 SLOT_TAKES_REFERENCE_ONLY 拒绝，此前评测脚本就是这样失效的）。
"""
import sys
from pathlib import Path
import unittest

sys.path.insert(0, str(Path(__file__).resolve().parents[1]))
import config_v2


class RecordingClient:
    def __init__(self, responses=None):
        self.calls = []
        self.responses = list(responses or [])

    def must(self, method, path, body=None):
        self.calls.append((method, path, body))
        return self.responses.pop(0) if self.responses else {}


class SaveInstanceTests(unittest.TestCase):
    def test_posts_an_instance_not_a_slot(self):
        client = RecordingClient([{"id": "m_1"}])
        instance = config_v2.save_instance(client, name="Gemini", kind="openai-responses",
                                           base_url="http://127.0.0.1:8045/v1", model="gemini-3.8-flash",
                                           key="sk-x", temperature=0.4, max_tokens=4096, context_window=32768)
        self.assertEqual(instance, "m_1")
        method, path, body = client.calls[0]
        self.assertEqual((method, path), ("POST", "/api/v1/config/models"))
        self.assertEqual(body["kind"], "openai-responses")
        self.assertEqual(body["apiKey"], "sk-x")
        self.assertEqual(body["maxTokens"], 4096)
        self.assertNotIn("slot", body)
        self.assertNotIn("enabled", body)

    def test_omits_optional_fields_when_not_set(self):
        client = RecordingClient([{"id": "m_2"}])
        config_v2.save_instance(client, name="x", kind="openai-chat", base_url="u", model="m")
        _, _, body = client.calls[0]
        for optional in ("apiKey", "temperature", "maxTokens", "contextWindow"):
            self.assertNotIn(optional, body)


class SlotBindingTests(unittest.TestCase):
    def test_configure_slot_creates_then_binds_reference_only(self):
        client = RecordingClient([{"id": "m_3"}, {}])
        instance = config_v2.configure_slot(client, "assist", name="a", kind="anthropic-messages",
                                            base_url="u", model="m", max_tokens=1024, context_window=32768)
        self.assertEqual(instance, "m_3")
        _, path, body = client.calls[1]
        self.assertEqual(path, "/api/v1/config/provider")
        self.assertEqual(body, {"slot": "assist", "enabled": True, "modelId": "m_3"})
        # 槽位请求绝不能携带连接字段（后端会 422）。
        for forbidden in ("kind", "baseUrl", "model", "apiKey"):
            self.assertNotIn(forbidden, body)

    def test_empty_model_id_means_follow_primary(self):
        client = RecordingClient([{}])
        config_v2.assign_slot(client, "reflection", None)
        _, _, body = client.calls[0]
        self.assertEqual(body, {"slot": "reflection", "enabled": True, "modelId": ""})


if __name__ == "__main__":
    unittest.main()
