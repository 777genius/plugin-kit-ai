---
title: "词汇表"
description: "plugin-kit-ai 文档中使用的公共术语的简短定义。"
canonicalId: "page:reference:glossary"
section: "reference"
locale: "zh"
generated: false
translationRequired: true
---
# 词汇表

当文档术语拖慢您的速度时，请使用此页面。目标不是完美的理论。目标是快速共享意义。

## 授权状态

您的团队直接拥有的存储库部分。 `generate` 将此源转换为特定于目标的输出。

## 生成的目标文件

生成后针对特定目标生成的文件。它们是真实的交付输出，但并不是长期的事实来源。

## 路径

构建和发布插件的实用方法。示例包括默认的 Go 运行时路径、本地 Node/TypeScript 路径以及存储库拥有的集成设置。

## 目标

您的目标输出，例如 `codex-runtime`、`claude`、`codex-package`、`gemini`、`opencode` 或 `cursor`。

## 运行时路径

存储库直接拥有可执行插件行为的路径。

## 包或扩展路径

专注于生成正确的包或扩展工件而不是主要可执行运行时形状的路径。

## Repo 拥有的集成设置

存储库主要为另一个工具或工作区传送签入配置的路径。

## 安装频道

安装 CLI 的方法，例如 Homebrew、npm、PyPI 或经过验证的脚本。它不是公共运行时 API。

## 共享运行时包

已批准的 Python 和 Node 使用的 `plugin-kit-ai-runtime` 依赖项会流动，而不是将帮助程序文件复制到每个存储库中。

## 支持边界

项目默认推荐的内容、更谨慎支持的内容以及保持实验性的内容之间的公共界限。

## 准备门

您应该将支票视为回购协议足够健康、可以移交的信号。对于大多数存储库来说，这是 `validate --strict`。

## 切换

另一个队友、另一台机器或另一个用户可以在没有隐藏设置知识的情况下使用存储库。

相关页面：[目标模型](/zh/concepts/target-model)、[支持边界](/zh/reference/support-boundary) 和[生产准备情况](/zh/guide/production-readiness)。
