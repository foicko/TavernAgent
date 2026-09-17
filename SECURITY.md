# 安全问题报告

TavernAgent 面向本机和可信局域网。请勿将监听端口直接暴露到公网。局域网配对码允许访问故事和模型调用能力，应仅交给可信使用者。

请通过 GitHub 仓库的 **Security → Report a vulnerability** 私下报告漏洞。维护者应在公开发布仓库时启用 Private vulnerability reporting；启用前，请先发不包含漏洞细节的 issue 请求私密联系渠道。不要在公开 issue 中上传可利用的复现、密钥或私人故事。

报告请包含版本、操作系统、最小复现、影响范围和脱敏日志。不要附原始 `secrets.json`、API Key 或完整用户数据库。建议用原创合成角色卡复现。

当前维护线为最新发布版本。修复优先级以数据损坏、跨会话污染、未授权访问和秘密泄漏为最高。发布前扫描 Go 与前端依赖，修复后保留回归和升级说明。

浏览器只保存档案引用及非敏感偏好。模型密钥存放在后端数据目录，或通过 `TAVERNAGENT_KEY_PRIMARY`、`TAVERNAGENT_KEY_ASSIST`、`TAVERNAGENT_KEY_REFLECTION` 提供。后端文件没有声称采用操作系统密钥保险库加密；请限制数据目录的账户访问权限，并保护备份。
