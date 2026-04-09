---
title: "选择一个目标"
description: "实用的公共指南，用于选择与您想要的插件交付方式相匹配的目标。"
canonicalId: "page:guide:choose-a-target"
section: "guide"
locale: "zh"
generated: false
translationRequired: true
---
# 选择一个目标

当您已经知道需要 `plugin-kit-ai` 时，请使用此页面，但您仍然需要将存储库与您想要发送插件的方式相匹配。

选择目标意味着选择产品今天需要的主要路径，而不是永远锁定回购。

<MermaidDiagram
  :chart='`
flowchart TD
  Need[What does the product need right now] --> Exec{可执行行为}
  需要 --> 工件{包或扩展}
  需要 --> 配置{Repo 托管集成}
  执行 --> Codex[codex-runtime]
  执行 --> Claude[克劳德]
  工件 --> CodexPackage[codex-package]
  神器 --> Gemini[双子座]
  配置 --> OpenCode[开放代码]
  配置 --> Cursor[光标]
`'
/>

## 短规则

- 当您想要最强的默认运行时路径时，选择 `codex-runtime`
- 当 Claude 挂钩是实际产品要求时，选择 `claude`
- 当产品是官方 Codex 包时，选择 `codex-package`
- 当产品是 Gemini 扩展包时，选择 `gemini`
- 当存储库应拥有集成/配置设置时，选择 `opencode` 或 `cursor`

## 目标目录

|目标|当 | 时选择它车道 |
| --- | --- | --- |
| `codex-runtime` |您想要默认的可执行插件路径 |推荐的运行时路径 |
| `claude` |您特别需要 Claude 钩子 |推荐的 Claude 路径 |
| `codex-package` |您需要 Codex 打包输出 |推荐包路径|
| `gemini` |您正在运送 Gemini 扩展包 |推荐扩展路径 |
| `opencode` |您想要回购拥有的 OpenCode 集成设置 |回购拥有的集成设置 |
| `cursor` |您想要回购拥有的 Cursor 集成设置 |回购拥有的集成设置 |

## 安全默认值

如果您不确定，请从 `codex-runtime` 和默认 Go 路径开始。

在您选择更窄或更专业的路径之前，这为您提供了最干净的生产起点。

当您稍后移动到 `codex-package` 时，官方包通道将遵循官方 `.codex-plugin/plugin.json` 包布局。

如果您有意开始使用受支持的 Node/TypeScript 或 Python，则会改变语言选择，而不需要在第一天就决定每个打包或集成细节。

## 当您需要多个目标时该怎么办

- 选择定义当今产品的主要路径
- 保持仓库统一
- 仅当出现实际交付或集成需求时才添加更多目标

当您想要更广泛的多目标心智模型时，请阅读[一个项目，多个目标](/zh/guide/one-project-multiple-targets)。
