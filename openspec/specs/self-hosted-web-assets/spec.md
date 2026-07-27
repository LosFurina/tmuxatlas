# Self-Hosted Web Assets Specification

## Purpose

TBD - created by archiving change improve-state-web-and-operations.

## Requirements

### Requirement: No runtime third-party font dependency
TmuxAtlas Web UI SHALL render its supported bundled fonts and symbols without requesting Google Fonts、Google static font hosts or any other third-party font origin at runtime.

#### Scenario: Browser loads every application view
- **WHEN** browser opens Login、Overview、Settings 和 Terminal
- **THEN** 所有 font responses 来自当前 Hub origin，且没有第三方字体或 font stylesheet request

### Requirement: Bundled font inventory and licensing
实际随 TmuxAtlas 发布的 UI/Terminal 字体 SHALL 使用 WOFF2 或同等 web format，并包含可审计的来源、版本和许可证记录。专有或平台字体 SHALL 明确标为 system font，MUST NOT 作为 bundled font 宣传。

#### Scenario: Release artifacts are built
- **WHEN** frontend assets 被嵌入 release binary
- **THEN** 每个 bundled font 都有对应 license/source metadata，且构建不下载未固定版本的运行时字体

#### Scenario: User selects a system font
- **WHEN** Settings 选择 Menlo、Monaco、Consolas 或其他 system font
- **THEN** UI 明确其依赖本机可用性，并提供确定的 bundled fallback

### Requirement: Self-hosted Nerd symbol fallback
Web UI SHALL 提供本地 Nerd Fonts Symbols Only 或有记录、可复现的最小 glyph subset，作为 UI/Terminal font fallback，而不要求用户安装 patched Nerd Font。

#### Scenario: Terminal output contains a supported Nerd glyph
- **WHEN** 当前 primary terminal font 不包含该 glyph
- **THEN** browser 从同源 Nerd symbol fallback 渲染该 glyph

#### Scenario: Symbol subset is regenerated
- **WHEN** 构建或维护流程重新生成最小 subset
- **THEN** glyph inventory、来源和许可证保持可复现且由自动化测试验证

### Requirement: Font settings reflect actual availability
Settings SHALL 区分 bundled 与 system font，并且每个 bundled option SHALL 对应构建产物中的可加载字体。选择 generic `monospace` SHALL 使用 generic family 语义，而不是把它当作不存在的 quoted family name。

#### Scenario: User selects a bundled font
- **WHEN** 用户选择 Space Mono、JetBrains Mono、Fira Code 或其他标为 bundled 的 option
- **THEN** 对应本地 font 成功加载并应用到 Terminal

#### Scenario: Bundled font asset is missing
- **WHEN** 配置声明 bundled font 但构建产物不含资源
- **THEN** build/test 失败，而不是在生产环境静默回退

### Requirement: Bounded and progressive asset loading
仅首屏必需字体 SHALL 被 preload；非首屏 Terminal 字体、Nerd symbols 和 xterm code SHALL 按实际使用加载。项目 SHALL 维护 version-controlled compressed bundle budgets，超限时 CI 必须失败或由显式 budget change 说明接受。

#### Scenario: User remains on Login
- **WHEN** 未认证用户只加载 Login
- **THEN** browser 不预加载完整 xterm chunk 和全部可选 terminal fonts

#### Scenario: Build exceeds a configured budget
- **WHEN** entry JS/CSS、lazy xterm chunk 或关键字体的压缩大小超过版本控制预算
- **THEN** CI 失败并报告超限 asset

### Requirement: Self-hosted asset verification
项目 SHALL 自动验证第三方字体请求为零、bundled font 可达、Nerd glyph inventory、license inventory、font option mapping 和 bundle budgets。

#### Scenario: Web asset test suite runs
- **WHEN** CI 构建并访问 frontend
- **THEN** suite 证明所有声明的 bundled assets 可由 Hub 同源提供，且未发生未批准的外部字体请求
