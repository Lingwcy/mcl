---
title: Windows 兼容性说明
description: mcl 在 Windows 11 (AMD64) 上对各 Minecraft 版本、模组加载器、自动 Java 运行时与网络镜像回退的实测支持范围与已知边界。
section: 参考与开发
sectionOrder: 3
order: 4
---

> 本页面当前**仅记录 Windows 平台（Windows 11 AMD64）**的实机兼容情况。macOS 与 Linux 等其他桌面平台尚未开始实机兼容测试，待后续测试完成后补充。

## Loader × Minecraft 版本支持矩阵（Windows）

下表汇总 Windows 11 桌面会话下各版本节点与五大运行环境的实际支持情况：

<div class="compat-legend" aria-label="图例说明">
  <span class="compat-legend-item"><span class="compat-mark compat-ok" aria-hidden="true">✓</span> 支持（已通过或同一标准链路可用）</span>
  <span class="compat-legend-item"><span class="compat-mark compat-no" aria-hidden="true">✕</span> 不支持 / 无此版本</span>
  <span class="compat-legend-item"><span class="compat-mark compat-warn" aria-hidden="true">!</span> 未知 / 受限于网络未完成实机验证</span>
</div>

<div tabindex="0" role="region" aria-label="Loader × Minecraft 版本支持矩阵（Windows）" class="compat-table-wrap">
  <table class="compat-table">
    <thead>
      <tr>
        <th scope="col">MC 版本</th>
        <th scope="col"><span class="compat-th"><img src="/loaders/vanilla.svg" width="22" height="22" alt="" class="pixel-img" aria-hidden="true" /><span>Vanilla</span></span></th>
        <th scope="col"><span class="compat-th"><img src="/loaders/fabric.svg" width="22" height="22" alt="" class="pixel-img" aria-hidden="true" /><span>Fabric</span></span></th>
        <th scope="col"><span class="compat-th"><img src="/loaders/quilt.svg" width="22" height="22" alt="" class="pixel-img" aria-hidden="true" /><span>Quilt</span></span></th>
        <th scope="col"><span class="compat-th"><img src="/loaders/forge.svg" width="22" height="22" alt="" class="pixel-img" aria-hidden="true" /><span>Forge</span></span></th>
        <th scope="col"><span class="compat-th"><img src="/loaders/neoforge.svg" width="22" height="22" alt="" class="pixel-img" aria-hidden="true" /><span>NeoForge</span></span></th>
      </tr>
    </thead>
    <tbody>
      <tr>
        <th scope="row">26.3 / 26.4-snap</th>
        <td><span class="compat-mark compat-ok" title="支持"><span aria-hidden="true">✓</span><span class="sr-only">支持</span></span></td>
        <td><span class="compat-mark compat-ok" title="支持（26.3、26.4-snapshot-8 实测通过）"><span aria-hidden="true">✓</span><span class="sr-only">支持</span></span></td>
        <td><span class="compat-mark compat-ok" title="支持（26.3 / 0.30.2 实测通过）"><span aria-hidden="true">✓</span><span class="sr-only">支持</span></span></td>
        <td><span class="compat-mark compat-ok" title="支持（26.3 / 66.1.8 实测通过）"><span aria-hidden="true">✓</span><span class="sr-only">支持</span></span></td>
        <td><span class="compat-mark compat-warn" title="未知（目录已通，无代理无法读取官方 .sha1）"><span aria-hidden="true">!</span><span class="sr-only">未知</span></span></td>
      </tr>
      <tr>
        <th scope="row">26.1</th>
        <td><span class="compat-mark compat-ok" title="支持"><span aria-hidden="true">✓</span><span class="sr-only">支持</span></span></td>
        <td><span class="compat-mark compat-ok" title="支持"><span aria-hidden="true">✓</span><span class="sr-only">支持</span></span></td>
        <td><span class="compat-mark compat-ok" title="支持（0.30.0-beta.7 实测通过）"><span aria-hidden="true">✓</span><span class="sr-only">支持</span></span></td>
        <td><span class="compat-mark compat-ok" title="支持（62.0.9 实测通过）"><span aria-hidden="true">✓</span><span class="sr-only">支持</span></span></td>
        <td><span class="compat-mark compat-warn" title="未知（目录与整合包文件已通，安装器官方 .sha1 受网限）"><span aria-hidden="true">!</span><span class="sr-only">未知</span></span></td>
      </tr>
      <tr>
        <th scope="row">1.21.5 / 1.21.11</th>
        <td><span class="compat-mark compat-ok" title="支持（1.21.11 实测通过）"><span aria-hidden="true">✓</span><span class="sr-only">支持</span></span></td>
        <td><span class="compat-mark compat-ok" title="支持"><span aria-hidden="true">✓</span><span class="sr-only">支持</span></span></td>
        <td><span class="compat-mark compat-ok" title="支持"><span aria-hidden="true">✓</span><span class="sr-only">支持</span></span></td>
        <td><span class="compat-mark compat-ok" title="支持（1.21.5 / 55.1.0 实测通过）"><span aria-hidden="true">✓</span><span class="sr-only">支持</span></span></td>
        <td><span class="compat-mark compat-warn" title="未知（未完成实机验证）"><span aria-hidden="true">!</span><span class="sr-only">未知</span></span></td>
      </tr>
      <tr>
        <th scope="row">1.21.1</th>
        <td><span class="compat-mark compat-ok" title="支持"><span aria-hidden="true">✓</span><span class="sr-only">支持</span></span></td>
        <td><span class="compat-mark compat-ok" title="支持（0.16.7 实测通过）"><span aria-hidden="true">✓</span><span class="sr-only">支持</span></span></td>
        <td><span class="compat-mark compat-ok" title="支持（0.29.2 实测通过）"><span aria-hidden="true">✓</span><span class="sr-only">支持</span></span></td>
        <td><span class="compat-mark compat-ok" title="支持（52.1.10 实测通过）"><span aria-hidden="true">✓</span><span class="sr-only">支持</span></span></td>
        <td><span class="compat-mark compat-warn" title="未知（未完成实机验证）"><span aria-hidden="true">!</span><span class="sr-only">未知</span></span></td>
      </tr>
      <tr>
        <th scope="row">1.20.1 / 1.20.4</th>
        <td><span class="compat-mark compat-ok" title="支持"><span aria-hidden="true">✓</span><span class="sr-only">支持</span></span></td>
        <td><span class="compat-mark compat-ok" title="支持（1.20.1 / 0.15.10 实测通过）"><span aria-hidden="true">✓</span><span class="sr-only">支持</span></span></td>
        <td><span class="compat-mark compat-ok" title="支持（1.20.1 / 0.26.0 实测通过）"><span aria-hidden="true">✓</span><span class="sr-only">支持</span></span></td>
        <td><span class="compat-mark compat-ok" title="支持（1.20.1 / 47.4.29 实测通过）"><span aria-hidden="true">✓</span><span class="sr-only">支持</span></span></td>
        <td><span class="compat-mark compat-warn" title="未知（目录已通，安装未完成实机验证）"><span aria-hidden="true">!</span><span class="sr-only">未知</span></span></td>
      </tr>
      <tr>
        <th scope="row">1.17.1 – 1.19.4</th>
        <td><span class="compat-mark compat-ok" title="支持"><span aria-hidden="true">✓</span><span class="sr-only">支持</span></span></td>
        <td><span class="compat-mark compat-ok" title="支持"><span aria-hidden="true">✓</span><span class="sr-only">支持</span></span></td>
        <td><span class="compat-mark compat-ok" title="支持"><span aria-hidden="true">✓</span><span class="sr-only">支持</span></span></td>
        <td><span class="compat-mark compat-ok" title="支持（1.18.2 / 40.2.21 实测通过）"><span aria-hidden="true">✓</span><span class="sr-only">支持</span></span></td>
        <td><span class="compat-mark compat-no" title="不支持 / 无此版本"><span aria-hidden="true">✕</span><span class="sr-only">不支持</span></span></td>
      </tr>
      <tr>
        <th scope="row">1.14 – 1.16.5</th>
        <td><span class="compat-mark compat-ok" title="支持"><span aria-hidden="true">✓</span><span class="sr-only">支持</span></span></td>
        <td><span class="compat-mark compat-ok" title="支持（1.16.5 / 0.18.5 实测通过）"><span aria-hidden="true">✓</span><span class="sr-only">支持</span></span></td>
        <td><span class="compat-mark compat-ok" title="支持（共享 Java 8 -cp 修复）"><span aria-hidden="true">✓</span><span class="sr-only">支持</span></span></td>
        <td><span class="compat-mark compat-ok" title="支持（1.16.5 / 36.2.42 实测通过）"><span aria-hidden="true">✓</span><span class="sr-only">支持</span></span></td>
        <td><span class="compat-mark compat-no" title="不支持 / 无此版本"><span aria-hidden="true">✕</span><span class="sr-only">不支持</span></span></td>
      </tr>
      <tr>
        <th scope="row">1.12.2</th>
        <td><span class="compat-mark compat-ok" title="支持"><span aria-hidden="true">✓</span><span class="sr-only">支持</span></span></td>
        <td><span class="compat-mark compat-no" title="不支持"><span aria-hidden="true">✕</span><span class="sr-only">不支持</span></span></td>
        <td><span class="compat-mark compat-no" title="不支持"><span aria-hidden="true">✕</span><span class="sr-only">不支持</span></span></td>
        <td><span class="compat-mark compat-ok" title="支持（14.23.5.2859 实测通过）"><span aria-hidden="true">✓</span><span class="sr-only">支持</span></span></td>
        <td><span class="compat-mark compat-no" title="不支持 / 无此版本"><span aria-hidden="true">✕</span><span class="sr-only">不支持</span></span></td>
      </tr>
      <tr>
        <th scope="row">1.7.10 / 1.8.9</th>
        <td><span class="compat-mark compat-ok" title="支持"><span aria-hidden="true">✓</span><span class="sr-only">支持</span></span></td>
        <td><span class="compat-mark compat-no" title="不支持"><span aria-hidden="true">✕</span><span class="sr-only">不支持</span></span></td>
        <td><span class="compat-mark compat-no" title="不支持"><span aria-hidden="true">✕</span><span class="sr-only">不支持</span></span></td>
        <td><span class="compat-mark compat-ok" title="支持（1.7.10、1.8.9 实测通过）"><span aria-hidden="true">✓</span><span class="sr-only">支持</span></span></td>
        <td><span class="compat-mark compat-no" title="不支持 / 无此版本"><span aria-hidden="true">✕</span><span class="sr-only">不支持</span></span></td>
      </tr>
      <tr>
        <th scope="row">1.6.4</th>
        <td><span class="compat-mark compat-ok" title="支持"><span aria-hidden="true">✓</span><span class="sr-only">支持</span></span></td>
        <td><span class="compat-mark compat-no" title="不支持 / 无此版本"><span aria-hidden="true">✕</span><span class="sr-only">不支持</span></span></td>
        <td><span class="compat-mark compat-no" title="不支持 / 无此版本"><span aria-hidden="true">✕</span><span class="sr-only">不支持</span></span></td>
        <td><span class="compat-mark compat-ok" title="支持（纯净 9.11.1.1345 实测通过）"><span aria-hidden="true">✓</span><span class="sr-only">支持</span></span></td>
        <td><span class="compat-mark compat-no" title="不支持 / 无此版本"><span aria-hidden="true">✕</span><span class="sr-only">不支持</span></span></td>
      </tr>
    </tbody>
  </table>
</div>

## 关键实测版本明细

- **Vanilla（原版）**：已实测 `1.21.11` 端到端安装与启动，历史版本（如 `1.6.4`、`1.7.10`、`1.8.9`、`1.12.2`）历史资源目录与原版链路均就绪。
- **Fabric**：已实测 `1.16.5`（`0.18.5`，Java 8 直传 `-cp`）、`1.20.1`（`0.15.10`）、`1.21.1`（`0.16.7`）、`26.3`（`0.19.5`）及 `26.4-snapshot-8`（`0.20.1`）。
- **Quilt**：已实测 `1.20.1`（`0.26.0`）、`1.21.1`（`0.29.2`，兼容内建 `mixinextras`）、`26.1`（`0.30.0-beta.7`）与 `26.3`（`0.30.2`）；`1.14 – 1.16.5` 共享 Java 8 `-cp` 修复。
- **Forge**：已实测 `1.6.4`（纯净 `9.11.1.1345`）、`1.7.10`、`1.8.9`（`11.15.1.1902`）、`1.12.2`（`14.23.5.2859`）、`1.16.5`（`36.2.42`）、`1.18.2`（`40.2.21`）、`1.20.1`（`47.4.29`）、`1.21.1`（`52.1.10`）、`1.21.5`（`55.1.0`）、`26.1`（`62.0.9`）与 `26.3`（`66.1.8`）。

## Java 自动安装与网络镜像行为

在 **设置 → 下载** 中，默认下载模式已设为 **自动（`SourceAuto`）**，会在严格保留完整摘要与文件大小校验的前提下优先保障可用性与回退速度：

- **Java 运行时自动安装（Windows）**：已在 Windows 11 上完整验证 **Eclipse Temurin `8` / `17` / `21` / `25`**（支持清华大学 TUNA 镜像回退与 `SHA-256` 校验）以及 **Azul Zulu `8` / `17` / `21`** 自动安装；当 Temurin 通道不可达时会自动回退到 Azul Zulu 并容忍其目录接口的 100 字节取整误差。
- **固定制品与目录回退**：支持 BMCLAPI（`Forge` / `NeoForge` / `Fabric` / `Minecraft` 目录与 `/maven` 制品）、Fabric-meta 镜像及阿里云 Maven Central 的固定版本二进制回退，并在无用户限速且存在备用源时自动触发快速故障转移。
- **校验安全原则**：`mcl` **绝不从非官方镜像读取 `.sha1` 等校验旁路文件，也不对未核验字节自算哈希**。对于旧版 Forge（如 `1.8.9`），若官方 `.sha1` 请求因网络超时失败，仅当官方安装器本身已携带经过完整 `SHA-1` 校验的 `checksums`（或属非 Forge 的标准 Maven Central 构件可改用 `repo.maven.apache.org/maven2/` 官方 `.sha1`）时，才会继续完成校验与下载。

## 已知边界与注意事项

### NeoForge 在无代理大陆网络下的安装限制

- 当前通过镜像回退已可正常获取 **NeoForge**（`1.20.1`、`1.20.4`、`26.1`、`26.3`）版本列表并完成整合包本体模组下载。
- 不过，NeoForge 安装器及部分无固定哈希的 Maven 依赖仍需从官方仓库读取 `.sha1` 旁路文件以核验身份；当本地无系统代理且直连 `maven.neoforged.net` 不可达时，由于公共镜像站缺少可用官方 `.sha1`，`mcl` 会拒绝跳过校验并明确提示无法连接加载器服务。如需完整安装与启动 NeoForge，请在直连畅通或配置可用代理后进行。

### 个别历史版本与模组行为说明

- **`Forge 1.6.4` 遗留整合包自崩**：`mcl` 本身已完成 `Forge 1.6.4`（`9.11.1.1345`）纯净实例安装、历史资源发布与窗口拉起验证；但部分 2013 年古老整合包中的模组（例如旧版 `CodeChickenCorePlugin`）会在游戏启动时自行联网拉取早已下线的远端库而崩溃，属于模组自身历史问题而非启动器兼容故障。
- **`Forge 1.12.2` 默认全屏首轮退出**：部分内置 `fullscreen:true` 的 `1.12.2` 整合包在桌面无焦点/首次初始化场景下可能于菜单阶段直接退出，再次启动即可正常进入游戏窗口。

## 明确不支持的范围

- **其他旧版 / 社区加载器**：当前仅支持 Vanilla、Fabric、Quilt、Forge 与 NeoForge；不支持 Legacy-Fabric、Babric、Ornithe、LiteLoader 等未内置加载器。
- **非白名单外链资产**：拒绝加载或下载指向任意第三方非可信域名的整合包依赖外链。
- **注入与第三方认证**：按项目架构边界拒绝 `-javaagent:`、皮肤/披风补丁注入、`authlib-injector` 及第三方认证扩展。
