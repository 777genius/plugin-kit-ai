---
title: "如何发布插件"
description: "将 plugin-kit-ai 项目发布到 Codex、Claude 和 Gemini 的实用指南，避免将本地应用与发布计划混淆。"
canonicalId: "page:guide:how-to-publish-plugins"
section: "guide"
locale: "zh"
generated: false
translationRequired: true
---
# 如何发布插件

当您的存储库已在 `plugin-kit-ai` 中编写，并且您希望为 Codex、Claude 或 Gemini 发布提供最清晰的下一步时，请使用本指南。

## 本指南涵盖的内容

- 哪些平台支持真正的本地应用今天
- 哪个平台使用计划和准备状态
- 首先运行哪个命令
- 命令完成后预期的结果

## 快速比较

|平台|出版模型 |真正适用于`plugin-kit-ai` |主命令 |你得到什么 |
|---|---|---:|---|---|
| Codex |本地市场根 |是的 | `publish --channel codex-marketplace` | `.agents/plugins/marketplace.json` 加 `plugins/<name>/...` |
| Claude |本地市场根 |是的 | `publish --channel claude-marketplace` | `.claude-plugin/marketplace.json` 加 `plugins/<name>/...` |
| Gemini |存储库/发布准备|没有| `publish --channel gemini-gallery --dry-run` |有界出版计划和准备诊断|

## 简短规则

- 当您需要发布工作流程时，请使用 `publish`
- 当您想要先进行检查或医生查看时，请使用 `publication`
- Codex 和 Claude 支持立即本地申请
- Gemini 在 v1 中使用计划和准备情况发布，而不是本地应用

回购协议的形状保持不变：

- `plugin.yaml` 是核心插件清单
- `targets/...` 保存特定于目标的创作输入
- `publish/...` 持有发布意图
- `publication` 是检查和刮刀表面
- `publish` 是发布工作流程界面

## 发布到 Codex

对于 Codex 来说，发布意味着实现本地市场根基。

首先运行这个：

```bash
plugin-kit-ai publish ./my-plugin --channel codex-marketplace --dest ./local-codex-marketplace --dry-run
```

当计划看起来正确时应用它：

```bash
plugin-kit-ai publish ./my-plugin --channel codex-marketplace --dest ./local-codex-marketplace
```

预期结果：

- `.agents/plugins/marketplace.json`
- `plugins/<name>/...`

这样的本地根目录已经可以充当 Codex 插件源。

## 发布到 Claude

对于 Claude 来说，发布还意味着实现本地市场根基。

首先运行这个：

```bash
plugin-kit-ai publish ./my-plugin --channel claude-marketplace --dest ./local-claude-marketplace --dry-run
```

当计划看起来正确时应用它：

```bash
plugin-kit-ai publish ./my-plugin --channel claude-marketplace --dest ./local-claude-marketplace
```

预期结果：

- `.claude-plugin/marketplace.json`
- `plugins/<name>/...`

## 发布到 Gemini

对于 Gemini，发布并不意味着建立本地市场根基。

在 v1 中，`plugin-kit-ai` 做了三件有界的事情：

- 验证发布意图
- 检查存储库准备情况
- 制定出版计划

从准备开始：

```bash
plugin-kit-ai publication doctor ./my-plugin --target gemini
```

然后检查发布计划：

```bash
plugin-kit-ai publish ./my-plugin --channel gemini-gallery --dry-run
```

预期先决条件：

- 公共 GitHub 存储库
- 有效的 `origin` 远程指向 GitHub
- GitHub 主题 `gemini-cli-extension`
- `gemini-extension.json` 在正确的根目录中

Gemini 在 v1 中使用计划和准备发布，而不是本地应用。

## 跨所有创作渠道进行规划

当一个存储库作者有多个发布渠道时使用此选项：

```bash
plugin-kit-ai publish ./my-plugin --all --dry-run --dest ./local-marketplaces --format json
```

重要规则：- 它仅使用创作的 `publish/...` 通道
- 它不会从 `targets` 推断通道
- 仅在 v1 中进行规划
- 仅当创作渠道包含 Codex 或 Claude 本地市场流量时才需要 `--dest`
- 仅 Gemini 编排不需要 `--dest`

如果存储库作者只有 `gemini-gallery`，这也有效：

```bash
plugin-kit-ai publish ./my-plugin --all --dry-run --format json
```

## 我应该运行哪个命令？

- 我想要本地 Codex 市场根： `plugin-kit-ai publish --channel codex-marketplace --dest <marketplace-root>`
- 我想要本地 Claude 市场根： `plugin-kit-ai publish --channel claude-marketplace --dest <marketplace-root>`
- 我想要 Gemini 出版准备：`plugin-kit-ai publication doctor --target gemini`
- 我想要 Gemini 出版计划：`plugin-kit-ai publish --channel gemini-gallery --dry-run`
- 我想要一个组合发布计划：`plugin-kit-ai publish --all --dry-run`，并在包含 Codex 或 Claude 创作频道时添加 `--dest <marketplace-root>`

## 进一步阅读

- [CLI 自述文件发布部分](https://github.com/777genius/plugin-kit-ai/tree/main/cli/plugin-kit-ai)
- [`plugin-kit-ai publish`](/zh/api/cli/plugin-kit-ai-publish)
- [`plugin-kit-ai publication`](/zh/api/cli/plugin-kit-ai-publication)
- [`plugin-kit-ai publication doctor`](/zh/api/cli/plugin-kit-ai-publication-doctor)