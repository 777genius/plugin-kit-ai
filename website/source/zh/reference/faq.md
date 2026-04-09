---
title: "FAQ"
description: "对团队在启动和扩展 plugin-kit-ai 存储库时最常提出的问题的简短回答。"
canonicalId: "page:reference:faq"
section: "reference"
locale: "zh"
generated: false
translationRequired: true
---
# FAQ

## 我应该从 Go、Python 还是 Node 开始？

从 Go 开始，除非您有真正的理由不这样做。

选择 Node/TypeScript 作为主要支持的非 Go 路径。当插件位于存储库本地并且您的团队已经是 Python 时，请选择 Python。

## 最简单的 Python 设置是什么？

首先使用默认的 Python 脚手架：

```bash
plugin-kit-ai init my-plugin --platform codex-runtime --runtime python
plugin-kit-ai doctor ./my-plugin
plugin-kit-ai bootstrap ./my-plugin
plugin-kit-ai generate ./my-plugin
plugin-kit-ai validate ./my-plugin --platform codex-runtime --strict
```

然后编辑插件，重新生成并再次验证。

请参阅[构建 Python 运行时插件](/zh/guide/python-runtime)。

## 我什么时候应该使用`--runtime-package`？

仅当您有意希望跨多个存储库共享一个辅助依赖项时，才使用 `--runtime-package` 。

大多数团队应该首先从默认的本地助手开始。

## npm 和 PyPI `plugin-kit-ai` 包运行时 APIs 吗？

不。他们安装了 CLI。它们不是运行时 APIs，也不是 SDKs。

## 我什么时候应该使用捆绑命令？

当另一台机器需要可移植的 Python 或 Node 工件来获取或安装时，请使用捆绑命令。

不要将捆绑包交付与主 CLI 安装路径混淆。

## 我可以保留本机目标文件作为我的事实来源吗？

不会。预期的长期模型是将真实来源保留在包标准布局中，并将目标文件视为生成的输出。

## `generate` 是可选的吗？

不，如果您想要托管项目流程，则不需要。 `generate` 是工作流程的一部分。

## `validate --strict` 是可选的吗？

将其视为主要的准备情况检查，尤其是对于本地 Python 和 Node 运行时存储库。

## 一个回购协议可以拥有多个目标吗？

是的。

实际规则是：

- 将创作状态保存在一个托管仓库中
- 从您今天需要的主要目标开始
- 仅在出现实际产品、交付或集成需求时添加更多目标

请参阅[一个项目，多个目标](/zh/guide/one-project-multiple-targets) 和 [目标模型](/zh/concepts/target-model)。

## 所有目标都同样稳定吗？

不。

不同的路径承载着不同的支持承诺。使用 [Support Boundary](/zh/reference/support-boundary) 作为简短答案，使用 [Target Support](/zh/reference/target-support) 作为精确矩阵。