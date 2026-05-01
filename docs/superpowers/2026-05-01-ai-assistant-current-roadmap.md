# AI 助手当前路线图

**日期**: 2026-05-01
**状态**: 当前活文档；P0、P1、P2、P3 均已完成浏览器验收，下一步进入回归整理与发布前检查

## 定位

AI 页面已经不再只是“伴读”页面，而是 CiteBox 的 AI 助手与研究调度中心。它的核心职责是把用户的自然语言请求路由到应用内部已有能力，包括本地文献库全文搜索、外部学术检索、文献阅读/对比和图文检索。

旧的 `docs/superpowers/specs/*` 和 `docs/superpowers/plans/*` 保留为归档设计与实施记录。后续 AI 助手方向的优先级以本文档为准。

## 已完成基础

- `/ai` 已经是服务端持久化会话，不再是浏览器内存里的单篇伴读。
- 已有会话列表、重命名、删除、搜索、多文献 pin、`@` 文献选择、自动标题、导出和上下文预算。
- 已有 AI orchestrator/tool 架构，覆盖全库搜索、外部搜索、文献阅读/对比和图文检索。
- 已有 process strip、result cards、citation 恢复和 run/tool/card 持久化。
- 内部文献搜索已经支持全文扫描，并可走轻量 Master/Sub-Agent 风格的 planner/classifier。
- 外部搜索复用 Semantic Scholar 能力，并已接入 Master 多查询规划器和 Sub-Agent 证据判定，把候选文本中能对应用户原句的证据句标注回结果卡；规划失败时回退到本地启发式查询。
- 图文检索支持直接搜图，也支持通过全文候选文献 fallback。
- 当前路线明确禁止 embedding 和 vector database。

## 当前优先级

### P0: 网页真实可用性验收

目标是确认 AI 助手四类核心请求在浏览器中真实可用，而不是只在后端测试里成立。

- [x] 内部全文搜索：例如“查找包括 ChIP-seq 数据的文章”，应显示规划、全文扫描、Sub-Agent 判定、命中文献卡片和证据片段。
  - 2026-05-01 已在浏览器验证 ChIP-seq / ATAC 类请求；结果展示 `Master规划`、`全文扫描`、`Sub-Agent判定`、命中文献卡片和本地全文证据片段。
  - 已额外用非法文硬编码的跨语言手工提问验证内部检索不会只依赖中文关键词。
- [x] 外部搜索：例如“查一下外部有没有 single-cell ATAC 综述”，应独立触发外部检索，失败时给出可解释状态。
  - 2026-05-01 已验证外部搜索可独立触发 Semantic Scholar。
  - 已补 Master 外部查询规划器；法文手工问题“给遗传发现速度变慢找出处”可命中 Cell 2025 文献，并引用摘要证据。
  - 针对“正向遗传筛选饱和导致基因发现减少”类出处查找，已升级为 Master 多查询并行召回、Sub-Agent 候选判定和结果卡证据标注，降低单一长查询排序偏移导致错失正确出处的风险。
- [x] 文献阅读/对比：选择 1-2 篇文献后提问，应读取选中文献全文并返回对比结果。
  - 2026-05-01 已在浏览器创建双文献 pin 场景，触发 `读文献`，验证 `全文扫描 2篇`、`命中 6段`、`paper_compare` 结果卡和证据片段可渲染。
  - 同次验收发现当前 OpenAI Responses 兼容接口可能不返回 delta；已补 `response.output_text.done` / `response.completed` 解析、空流错误兜底，以及工具结果 fallback，确保模型最终回答失败时仍返回已完成的文献阅读/对比证据。
- [x] 图文检索：例如“在文献库中找一张 ChIP-seq 相关的图”，应返回图卡、图注和证据来源；直接搜不到时使用全文候选 fallback。
  - 2026-05-01 已修复自然语言图文请求秒回“证据不足”的问题；直接图检索无结果时会先用全文候选文献 fallback，再返回图卡和全文证据来源。
- [x] 重新打开历史会话后，process strip、result cards、citation 和折叠状态应能合理恢复。
  - 2026-05-01 已通过历史会话回放检查 process strip、结果卡片、citation 和默认折叠卡片恢复。

验收方式:

- 使用已经运行的 `make dev` 页面做手工验证。
- 必要时配合 Playwright 截图检查页面空白态、思考态、卡片折叠、证据高亮和长对话滚动条。
- 每次发现问题，优先补最小回归测试或前端语法检查，再修代码。

### P1: 调度过程可观察

目标是让用户能理解“AI 正在查什么、为什么命中、在哪里失败”，但不把页面变成复杂任务中心。

- [x] process strip 继续保持紧凑，但阶段名称需要稳定，例如 `Master规划`、`全文扫描`、`Sub-Agent判定`、`命中`、`生成回答`。
  - 2026-05-01 已统一工具阶段名，并在回答生成前后增加 `生成回答 running/completed` 阶段；历史会话持久化完成态。
- [x] 内部搜索需要显示候选数量、判定数量、命中数量。
  - 内部搜索现在保留 `Master规划`、`全文扫描`、`Sub-Agent判定`、`命中`；阶段 detail 带检索词，Sub-Agent 失败会显示失败篇数并保留全文命中候选。
- [x] 外部搜索需要显示搜索源、返回数量和失败原因。
  - 外部搜索现在显示 `Semantic Scholar` 来源、实际 search query、返回数量；失败时 process note 与 AnswerContext 都写明失败原因。
- [x] 图文检索需要显示是直接命中图，还是从全文候选文献 fallback。
  - 图文检索现在在 process note/detail 中区分直接图片库检索和全文候选文献 fallback。
- [x] 失败时不要只返回“证据不足”，而要说明是没有命中、检索失败、模型判定失败，还是上下文不足。
  - 内部搜索、外部搜索、图文检索和文献阅读的空结果/失败/跳过路径已补明确 AnswerContext，避免最终回答只能泛化成“证据不足”。

### P2: 模型场景绑定

目标是让强模型做规划与综合，便宜模型做高并发判定，从配置上明确 Master/Sub-Agent 的职责。

- [x] settings 中明确 Master 模型和 Sub-Agent 模型的绑定关系。
  - 2026-05-01 已在场景绑定区增加 AI 助手绑定摘要，直接显示 Master/Sub-Agent 当前模型、职责和是否拆分。
- [x] Master 默认适合使用强推理模型，例如 GPT-5.5 thinking/xhigh 或同等级模型。
  - 通过“套用 AI 助手推荐绑定”根据已配置模型选择强推理模型作为 Master；不会在没有 API Key 的情况下自动创建不可用模型。
- [x] Sub-Agent 默认适合使用便宜并发模型，例如 DeepSeek Flash。
  - 推荐绑定会优先选择 DeepSeek/Flash/mini/nano/lite/fast/cheap 类模型作为 Sub-Agent，并尽量与 Master 拆分。
- [x] 支持 provider-specific 参数，例如 DeepSeek `thinking`、`reasoning_effort`，以及 OpenAI Responses 不支持 `temperature` 的模型差异。
  - OpenAI 兼容模型支持 Responses / Chat Completions 切换；Chat Completions 可发送 `thinking` 和 `reasoning_effort`，Responses 使用 `reasoning.effort`；GPT-5/o 系列自动省略 `temperature`，也可手动勾选不发送。
- [x] 模型检查应能提示具体 unsupported parameter，而不是让用户猜。
  - 2026-05-01 已给模型检查错误追加参数级处理建议，例如 `temperature` 不支持时提示勾选“不发送 temperature 参数”。
  - 已用 settings 页面做浏览器验收：绑定摘要可渲染，`thinking` 和 `Reasoning Effort` 控件在 OpenAI 兼容模型下可启用，推荐算法会把 GPT-5.5 thinking/xhigh 选为 Master、DeepSeek Flash 选为 Sub-Agent。

### P3: AI 页面体验收口

目标是让 AI 页面继续像传统对话页面，而不是工具堆叠页。

- [x] 左侧会话区、主对话区和结果卡片的视觉层级保持一致。
  - 2026-05-01 已把 AI 页面收束为居中的应用工作区，侧栏、主对话区、composer 和结果卡片使用一致的面板层级。
  - 2026-05-01 已将侧栏历史会话限制为按更新时间倒序的最近 10 条；更多历史对话进入会话管理弹窗继续查看、搜索、重命名和删除。
- [x] 删除确认继续使用应用内原生 modal，不使用浏览器 prompt/confirm。
  - AI 会话列表和顶部删除都走 `Utils.confirm`；已补标准 dialog 属性、Escape 关闭和按钮类型。
- [x] `@` 文件选择框保持向上弹出，适配底部输入框。
  - 已在浏览器验证 `@` 选择框位于输入框上方，适配底部 composer。
- [x] 输入框自动增高，不允许手动拖拽破坏布局。
  - 输入框继续由 mirror 同步自动增高，并强制 `resize: none`；内容过长时内部滚动。
- [x] 成功生成后可编辑最后一问并重新发送。
  - 2026-05-01 已在最后一条用户消息上提供“编辑重发”；重新发送会先截断服务端最后一轮及其流程/结果卡片，再用修改后的问题重新生成。
- [x] 黑色、黄色和蓝色主题下长对话滚动条都应协调。
  - 已为 AI 主滚动区、侧栏、`@` 选择框、证据 blockquote 和输入框滚动条统一主题变量；暗色主题使用中性滚动条，避免过亮色块。
- [x] 结果卡片默认折叠，证据片段支持关键词高亮。
  - 已在浏览器验证 `paper_hit` 结果卡默认折叠，证据片段会按 `highlight_terms` 高亮。

## 暂不做

- 不做 embedding。
- 不做 vector database。
- 不做长时间后台任务中心。
- 不做复杂多 Agent 面板。
- 不做移动端重设计，除非当前桌面布局已经稳定。

## 下一步执行建议

1. 基于 P0-P3 验收结果，整理 release QA checklist。
2. 做一次完整 AI 页面回归：新建会话、历史会话、更多会话、编辑重发、四类任务快捷入口。
