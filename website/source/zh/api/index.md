---
title: "API"
description: "为 plugin-kit-ai 生成了 API 参考。"
canonicalId: "page:api:index"
section: "api"
locale: "zh"
generated: false
translationRequired: true
aside: false
outline: false
---
<div class="docs-hero docs-hero--compact">
  <p class="docs-kicker">生成的参考</p>
  <h1>API 表面</h1>
  <p class="docs-lead">
    本部分收集公共 plugin-kit-ai APIs：CLI、Go SDK、运行时帮助程序、平台事件和功能。
  </p>
</div>

<div class="docs-grid">
  <a class="docs-card" href="./cli/">
    <h2>CLI</h2>
    <p>从实时 Cobra 树导出的命令.</p>
  </a>
  <a class="docs-card" href="./go-sdk/">
    <h2>Go SDK</h2>
    <p>公共 Go 用于生产就绪运行时插件的包.</p>
  </a>
  <a class="docs-card" href="./runtime-node/">
    <h2>Node 运行时</h2>
    <p>JS 和 TS 使用者的类型化运行时助手。</p>
  </a>
  <a class="docs-card" href="./runtime-python/">
    <h2>Python 运行时</h2>
    <p>公共 Python 仅运行时帮助程序，不安装包装器.</p>
  </a>
  <a class="docs-card" href="./platform-events/">
    <h2>平台活动</h2>
    <p>按目标平台分组的事件表面。</p>
  </a>
  <a class="docs-card" href="./capabilities/">
    <h2>功能</h2>
    <p>跨平台和事件分组的功能。</p>
  </a>
</div>

## 打开右侧表面

- 当您需要命令、标志或创作工作流程时，打开 `CLI`。
- 当您在 Go 中构建生产就绪的运行时插件时，打开 `Go SDK`。
- 当您需要共享助手 API 作为存储库本地运行时时，打开 `Node Runtime` 或 `Python Runtime` 。
- 当您选择特定目标事件时，打开 `Platform Events`。
- 当您想查看跨平台存在哪些操作和扩展点时，请打开 `Capabilities`。

## API 部分涵盖的内容

- 实时 Cobra 命令树
- 公共 Go 包
- Node 和 Python 的共享运行时助手
- 特定于平台的事件
- 能力级跨平台元数据