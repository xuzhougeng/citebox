# Magpie 模型接入

通过独立的 [CiteBox Agent 插件](https://github.com/xuzhougeng/magpie-agents)，可以由 Magpie 管理 CiteBox 的文本模型。需要包含 `internal/magpie` 读取逻辑的新构建；旧版主程序不会自动读取配置。

## 使用

先在 Magpie 配置模型供应商，再从插件仓库安装：

```sh
magpie plugin add /absolute/path/to/magpie-agents/packages/agent-citebox
magpie citebox magpie/provider/model
```

将 `provider/model` 换成 Magpie 中可用的模型，并保持网关运行。npm 包发布后，也可以用 `magpie plugin add @xuzhougeng/magpie-agent-citebox@alpha` 安装。

接入和切换在下一次 AI 操作生效，不需要重启 CiteBox。正在运行的操作保留已有配置快照。断开时执行一次 `magpie citebox default`，后续操作恢复之前的配置。

## 配置与个人设置

配置固定为 `~/.config/citebox/magpie.json`，Windows 使用 `%USERPROFILE%\.config\citebox\magpie.json`。服务端部署时，这是运行 CiteBox 的系统账户的主目录。`XDG_CONFIG_HOME`、`STORAGE_DIR` 和数据库路径都不会改变它。

Magpie 写入 `baseURL`、`apiKey` 和 `model` 三个字符串，包含模型信息的插件版本另写可选的 `modelInfo`（见下文）；CiteBox 只读该文件。文件缺失、不完整、格式错误、超过 64 KiB，或内容为 `{}` 时，使用个人模型配置。

接入后，文本模型池在运行时使用同一个 Magpie 模型，保留模型 ID、场景绑定和提示词。翻译、解读、助手以及模型辅助检索都会读取运行时配置。调用使用 OpenAI Chat Completions；图片输入仍要求所选模型支持相应能力。独立图像生成、外部提取服务和 TTS 供应商配置保留各自的设置。

设置页仍展示、保存和验证个人模型。网关密钥不会写入 CiteBox 数据库，个人设置也不会覆盖 `magpie.json`。断开后，Magpie 恢复外部文件之前的字段；如果之前没有合法文件，则恢复 CiteBox 个人模型。之前已有合法外部文件时，则继续使用恢复后的外部配置。

## 模型参数与思考强度

`modelInfo: { id, context, output, efforts }` 描述 Magpie 所选模型，只包含 Magpie 确知的值。CiteBox 只采用 `id` 与 `model` 一致的快照；`id` 不一致、字段类型错误、容量不是正整数或档位名不合法时整体忽略，不影响三项连接字段。

| 值 | 来源 |
| --- | --- |
| 最大输出 Token | `modelInfo.output`；未知时 8192 |
| 思考强度档位 | `modelInfo.efforts`；未知时不发送 `reasoning_effort`，使用供应商默认 |
| 默认思考强度 | 依次取 medium、high、首个档位 |

接入后，设置页“模型配置”中、个人模型列表上方显示 Magpie 面板：模型、上下文容量、最大输出，以及只列出 `modelInfo.efforts` 的思考强度选择。选择单独保存在数据库的 `magpie_settings` 中，不写入个人模型配置，也不含网关地址或密钥，下一次 AI 操作生效。切换到不支持该档位的模型时使用默认档位，切回支持的模型后恢复之前的选择。CiteBox 不再强制关闭思考；请求只发送 `reasoning_effort`，不发送部分兼容网关使用的 `thinking` 开关。

AI 助手的检索规划和证据判定是回答前的辅助调用，返回简短 JSON 并有硬超时。Magpie 接入时它们改用 low 档位（其次 minimal；用户已选更低档位时沿用），输出上限在原值上增加 4096 Token 供推理使用，超时从 20 秒放宽到 45 秒。实测经 Magpie 调用 `codex/gpt-6.1-sol` 时，检索规划在 low 约 13 秒、medium 约 16 秒、high 约 21 秒、xhigh 约 32 秒，12 条证据的判定在 low 约 22 秒。超时或失败时仍跳过该步骤照常回答。个人模型配置的这两项调用保持原有档位、输出上限和 20 秒超时。

Magpie v0.1.1986 有一个已验证的上游问题：连续执行第二次 `default` 可能删除刚恢复的 `model` 字段。每次连接只断开一次，详情见插件仓库验证报告。

## 验证

```sh
go test ./internal/magpie ./internal/service/... ./internal/handler -run Magpie -count=1
make ci
```

`make ci` 使用仓库 CI 对应的 Node 22、Go 和 PowerShell。测试通过临时数据库、临时配置和本地假服务验证，不需要真实模型密钥。`TestMagpieExternalGateway` 仅在显式设置 `MAGPIE_TEST_CONFIG` 时运行，供插件仓库的官方网关联调使用；`MAGPIE_TEST_REASONING_EFFORT` 模拟设置页保存的思考强度。
