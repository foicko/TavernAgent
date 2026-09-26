# TavernAgent 角色卡适配测试报告（RisuAI Realm 实测）

- 日期：2026-09-26
- 测试范围：**15 张角色卡** = 10 张自 <https://realm.risuai.net/> 新下载 + 5 张既有 `charx/` 目录内样例
- 测试方式：直接调用工程内**真实解析器**（`application.ImportCardPNG / ImportCardJSON / ImportCardCharx`）+ 与前端 `renderCardMarkers` / `resolveCardAsset` 完全一致的图片标记解析规则
- 结论摘要：**导入 15/15 全部成功**；图片标记语法 **100% 识别**；图片能否真正显示，取决于「卡内资源是否被打包进文件」以及「解析器是否把资源提取出来」。

---

## 一、下载方式

Realm 下载 API（见 <https://realm.risuai.net/help/api>）：

```
GET https://realm.risuai.net/api/v1/download/{format}/{id}
     format ∈ { png-v2, json-v2, png-v3, json-v3, charx-v3, dynamic }
```

实测要点：

- `dynamic/{id}` 是最可靠的入口：资产卡返回 `application/charx`，纯文本卡返回 PNG。
- `png-v2` / `png-v3` / `json-*` 对多数卡返回**同一份 PNG**；V2/V3 由 PNG 内嵌的 `chara` / `ccv3` 载荷决定。因此本报告对每张 PNG 额外单独导入了它的 **V2 载荷**。
- `charx-v3` 对部分卡返回 403，改用 `dynamic` 即可。
- 首次下载中 `sayaka` 因中断被截断（无 IEND / 无元数据），重新下载后正常——**说明 403/截断多为网络或限速问题，而非格式不兼容**。

### 下载清单（10 张）

| 卡 | Realm ID | 请求格式 | 实际得到 |
|---|---|---|---|
| Ayame | `54792b53-1971-45e0-8ff4-f97c2c73089c` | png-v3 | PNG（V3+内嵌 V2） |
| Ralsei | `3d9f19a6-7fc3-4ea9-9648-7fbc8a6a5452` | png-v3 | PNG（V3+内嵌 V2） |
| Jiyan | `3811201e-e832-4e5c-a84d-4126cf8971a5` | png-v3 | PNG（V3+内嵌 V2） |
| 천마 키우기 | `317c6c43-4add-4223-a317-6b9c9d9d6d31` | dynamic | PNG（V3+内嵌 V2） |
| Sayaka Hisamitsu | `eb18c272-cb74-49bf-91c6-623be7cff93f` | png-v3 | PNG（V3，资源在扩展块内） |
| 유언 | `c4eacd6a-0868-4bc2-81fa-314706dbec01` | dynamic | charx（ZIP） |
| 아르젠타 폰 크림슨 | `949f8dea-aea8-40f9-9ed2-1647ebd12084` | dynamic | charx（ZIP，355 资源） |
| DEEMO · The Tree of Memory | `1b7e1545-1e9a-4edd-978d-47bfd712c694` | dynamic | charx（ZIP，202 资源，141 MB） |
| EVE (Wall-E, humanization) | `74523659-0156-4e05-88d1-0cc1af350004` | dynamic | charx（**JPEG+ZIP polyglot**） |
| Manche Mühen | `e9c8e8a0-f6b2-4e43-856d-fb312f50aef4` | dynamic | charx（**JPEG+ZIP polyglot**，96 资源） |

原始文件存放于 `charx/realm-test/`（git 忽略）。

---

## 二、总体结果

| 指标 | 数值 |
|---|---|
| 测试卡片 | 15 |
| 导入成功 | **15 / 15** |
| charx 格式 | 10（含 2 张 JPEG+ZIP polyglot） |
| PNG（chara_card_v2/v3） | 5 |
| 卡内资源合计 | 2 844 |
| 图片标记出现次数 | 376 |
| 图片键成功解析 | 169（45%） |
| 标记写法分布 | `eq` 234 · `src` 38 · `bg` 36 · `cmd` 36 · `obj` 32 |

> 「解析率 45%」偏低的主因不是语法，而是**资源是否真的随卡打包**：Fera 的 180 处立绘和 Sayaka 的 11 处立绘都指向卡内不存在/未提取的图片（详见第四节 P0/P2）。

---

## 三、逐卡结果

| 卡片 | 导入格式 | spec | 可用资源 | 图片标记 | 已解析 | 未解析 | 标记写法 |
|---|---|---|---:|---:|---:|---:|---|
| Fera Academy.charx | charx | 3.0 | 74/74 | 252 | 72 | 180 | bg=36, eq=184, obj=32 |
| The Magical Revolution….charx | charx | 3.0 | 719/719 | 24 | 18 | 6 | cmd=24 |
| 디지땅.charx | charx | 3.0 | 752/752 | 0 | 0 | 0 | – |
| 소라카 (Soraka) v1.0.1.charx | charx | 3.0 | 126/126 | 44 | 40 | 4 | eq=44 |
| 하게해주는 3의 여동생.charx | charx | 3.0 | 361/361 | 25 | 21 | 4 | src=25 |
| argenta_dynamic.charx | charx | 3.0 | 355/355 | 12 | 10 | 2 | cmd=12 |
| ayame_png-v3.png | chara_card_v3 | 3.0 | 3/3 | 0 | 0 | 0 | – |
| cheonma_dynamic.charx | chara_card_v3 | 3.0 | 21/21 | 2 | 2 | 0 | src=2 |
| deemo_dynamic.charx | charx | 3.0 | 202/202 | 0 | 0 | 0 | – |
| eve_dynamic.charx | charx | 3.0 | 1/1 | 0 | 0 | 0 | – |
| jiyan_png-v3.png | chara_card_v3 | 3.0 | 2/2 | 0 | 0 | 0 | – |
| manche_dynamic.charx | charx | 3.0 | 96/96 | 0 | 0 | 0 | – |
| ralsei_png-v3.png | chara_card_v3 | 3.0 | 1/1 | 0 | 0 | 0 | – |
| sayaka_dynamic.charx | chara_card_v3 | 3.0 | 103/103 | 11 | **0** | 11 | src=11 |
| yueon_dynamic.charx | charx | 3.0 | 28/28 | 6 | 6 | 0 | eq=6 |

### V2 载荷单独导入（PNG 卡）

| 卡片 | 格式 | spec | 资源 | 说明 |
|---|---|---|---|---|
| ayame / jiyan / ralsei [V2 payload] | chara_card_v2 | 2.0 | 0 | V2 规范无 `assets` 字段 |
| cheonma [V2 payload] | chara_card_v2 | 2.0 | 0 | 2 处 `src` 标记因无资源而无法解析 |

---

## 四、图片标记语法支持矩阵

| 写法 | 状态 | 实测来源 |
|---|---|---|
| `<img cmd="key">` | ✅ 识别 / 解析 | Magical、argenta |
| `<img="key">`（旧式） | ✅ 识别 / 解析 | Soraka、Fera、유언 |
| `<img_obj="key">` | ✅ 识别 / 解析 | Fera（robe/hunter/elgail…） |
| `<img src="key" …>`（带附加属性） | ✅ 识别 / 解析 | 하게해주는、cheonma |
| `<imgBG="key">` | ✅ 识别（按内联图渲染） | Fera（背景图，⚠ 语义见 P4） |
| `[IMG:key]` | ✅ 识别 | 语法支持，本批未出现 |

> 上一轮已修复「只认 `<img cmd>`」的问题：正则与资源解析统一扩展到上表全部写法，并新增**忽略大小写**与**文件扩展名**回退。本轮实测确认有效（Soraka、하게해주는、유언、argenta 均命中）。

---

## 五、发现的问题（按优先级）

### P0 —— PNG 内嵌扩展资源未提取（影响面最大） ✅ 已修复（见第八节）

- **现象**：`sayaka` 有 103 个资源声明，但 11 处 `<img src>` 全部无法解析。
- **根因**：这类 RisuAI PNG 卡把每张图以 base64 存进 PNG 的 `tEXt` 扩展块，键名形如 `chara-ext-asset_:0`、`chara-ext-asset_:1` …；V3 的 `assets[]` 用 `uri="__asset:N"` 引用它们。
  实测块 0 的原始值解码后是 WebP（`RIFF….WEBP`）。
- **代码现状**：`internal/application/pngcard.go` 的 `readPNGMeta` 只读取 `chara` / `ccv3` / Creator / Notes / Comment，**完全忽略 `chara-ext-asset_:*`**；`mapV3Assets` 也把 `__asset:N` 当作普通 URI 原样保留。结果：资源字节从未落盘，前端拿到的是无效 URL `__asset:N`。
- **影响**：**所有采用 RisuAI PNG 外置资源写法的卡（典型的 100+ 立绘资产卡）图片都无法显示**。这是本轮最大的兼容缺口。

### P1 —— 资源名解析缺「前缀 + 数字 + 扩展名」组合 ✅ 已修复（见第八节）

- **现象**：`sayaka` 标记写 `dressed_happy` / `dressed_neutral`，资源名却是 `dressed_happy1.png` / `dressed_neutral1.png`。
- **现有回退**：精确、忽略大小写、`name`+纯数字后缀、`name`+文件扩展名；但 **`name`+数字+扩展名** 这一组合未覆盖（`1.png` 既不全是数字、也不以 `.` 直接接前缀后）。
- **影响**：即使 P0 修复，这类命名仍需 P1 才能命中。

### P2 —— charx 立绘未随卡打包（数据缺失，非代码问题）

- **现象**：`Fera Academy` 有 180 处（38 个不同键，如 `Alice.bridling`、`Anna.flustered`）无法解析。
- **根因**：这些立绘**根本不在归档里**——`data.assets` 只有 icon/BG/物件；`module.risum` 经检查不含任何图片签名。属卡作者未随包分发资源（realm 卡片页另附 Proton 资源模块）。
- **结论**：无法通过解析修复，需要用户另行安装资源模块。

### P3 —— V2-only 导入天然无 `assets`

- V2 规范没有 `assets` 字段，因此仅含 V2 载荷的 PNG 卡无法携带卡内图片（`cheonma` 的 V2 载荷实测 2/2 未解析）。属规范限制，行为正确（回退为纯文本），但应在 UI 上提示。

### P4 —— `<imgBG>` 语义

- 目前 `<imgBG="key">` 被当作普通内联图插入正文；RisuAI 原义是设置聊天/场景背景。功能可用但视觉语义不符，建议后续单列「背景通道」。

### P5 —— 模板占位符（预期行为，无需修复）

- `Soraka_<Emotion Command>`、`argenta_[Keyword]`、`[dressed or nude]_[state]` 等出现在提示词说明里，未命中资源时**原样保留为文本**，符合「宁可显示原文也不静默吞掉」的设计。

---

## 六、修复建议

1. **P0（优先）**：在 `readPNGMeta` 中同时提取 `chara-ext-asset_:N` 扩展块（base64→图片字节），建立 `__asset:N → 字节` 映射；导入时把它们登记为待落地资源（复用 `CharxAsset`/`persistCharxAssets` 的落盘通道），并把 `assets[].uri=__asset:N` 重写为可服务的 `Ref`。前端 `cardAssetsBody` 逻辑无需改动。
2. **P1**：解析规则改为「先去掉资源名末尾扩展名，再做 精确 / 忽略大小写 / 数字后缀 匹配」，可一并覆盖 `name1.png`、`name.png`、`name.avif` 等全部变体。
3. **P3/P4（可选）**：导入报告中对 V2-only 卡显式提示「无内嵌图片」；为 `<imgBG>` 增加背景专用渲染通道。
4. **P2**：无需改动；建议在导入报告里对「标记引用了不存在的资源键」给出汇总提示（例如 Fera 提示「38 个图片键在卡内无对应资源，可能缺少资源模块」）。

---

## 七、修复记录（本次）

针对 P0、P1 已落地修复并通过全量测试（Go `go test ./...` + `go vet ./...`；前端 437 单测 + typecheck + lint 全部通过）。

### P0：提取 PNG 内嵌扩展资源

- `internal/application/pngcard.go`
  - PNG 文本块过滤放行 `chara-ext-asset_:N` 前缀（`readTextChunk`）。
  - 新增 `collectPNGExtAssets`：把 `assets[].uri = "__asset:N"` 与 `chara-ext-asset_:N` 块对齐，base64 解码后登记为 `EmbeddedAssets` 并回填 `assets[].ref`；**找不到数据块的声明清空 URI**，让前端退回纯文本而不是渲染断裂图片。
  - 新增魔数嗅探 `detectImageExt`、`__asset:N` 解析 `pngAssetIndex`，并有单项/总量上限（沿用 `maxMetaChunk` 与新增 `maxEmbeddedTotalBytes = 64MB`）。
  - `ImportReport` 新增 `EmbeddedAssets []EmbeddedAsset`（内存字节，`json:"-"`）。
- `internal/application/cardlibrary.go`：新增 `persistEmbeddedAssets`，在 `SaveReport` 中把内嵌字节写入卡资源存储（复用现有 `CardAssetStore.SaveAsset`），落盘失败只告警。
- 前端无需改动：`cardAssetsBody` 见到 `ref` 就生成 `/api/v1/cards/{id}/assets/{ref}`，`sniffMime` 会按内容返回真实 MIME。

### P1：资源名解析统一为「先剥扩展名再匹配」

- `web/src/lib/risuImages.tsx`：`resolveCardAsset` 改为「去掉已知图片扩展名（png/jpg/webp/avif/gif/svg…）后再做 精确 / 忽略大小写 / 数字后缀 匹配」，一次覆盖 `main ↔ main.png`、`robe.default ↔ robe.default.avif`、`dressed_happy ↔ dressed_happy1.png`；只剥离**已知图片扩展名**，避免把 `nami.flustered` 这类名字误切。

### 验证

- 新增单测：`internal/application/pngcard_extassets_test.go`（提取、Ref 回填、缺失清空 URI、落盘 + MIME）、前端 `risuImages.test.tsx`（数字+扩展名、双向扩展名、不误剥非图片后缀）。
- 端到端：用真实二进制导入 `sayaka`（PNG），卡片 API 返回 103 个资源且带合法 URL；请求 `dressed_happy1.png` 实际返回 `image/webp`（内容嗅探）。修复前该卡 11/11 图片标记无法解析，现已可显示。

> 说明：第三节“逐卡结果”与第二节统计为**修复前**的实测快照；P0/P1 修复后，`sayaka` 的 11 处标记与 `cheonma` V2 载荷之外的部分可解析。

---

## 八、复现与附件

- 原始卡片：`charx/realm-test/`（10 张）
- 既有样例：`charx/*.charx`（5 张）
- 机器可读结果：`cardcheck.json`（realm）、`cardcheck_bundled.json`（既有）
- 测试工具：临时 Go 程序 `cmd/cardcheck`（调用真实 `internal/application` 解析器；已从仓库移除，方法见本报告第一节）
