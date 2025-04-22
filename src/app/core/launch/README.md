# Minecraft 启动器核心模块文档

本文档为开发者提供 Minecraft 启动器核心模块的概述，以帮助快速理解各文件的功能和它们之间的依赖关系。

## 文件结构与功能

### config.ts

配置管理模块，负责处理启动器的所有配置数据。

- 提供 `getConfig` 和 `setConfig` 函数用于读取和保存配置
- 存储关键的全局状态变量，如当前 Minecraft 版本、Java 路径等
- 导出基础路径变量和辅助功能函数

**依赖**: 独立模块，不依赖其他模块

### types.ts

类型定义模块，包含启动器所需的各种 TypeScript 接口和类定义。

- `McLaunchOptions`: 启动选项类
- `McLibToken`: 库文件令牌接口
- `McLoginType`: 登录类型枚举
- 各种登录数据类型定义

**依赖**: 独立模块，不依赖其他模块

### utils.ts

工具函数集合，提供各种通用功能。

- 文件路径处理（`shortenPath`, `fileExists`）
- 安全过滤器（`secretFilter`）
- 异步辅助函数（`sleep`, `runInUi`, `runInUiWait`）
- 哈希计算与字符串处理

**依赖**: 独立模块，仅依赖 Node.js 内置模块

### arguments.ts

Minecraft 启动参数生成器，负责构建游戏启动命令行。

- `getMcLaunchArguments`: 主要参数生成函数
- 分别处理旧版和新版 Minecraft 的JVM和游戏参数
- 处理参数替换和自定义参数
- 生成启动脚本

**依赖**: `config.ts`, `utils.ts`, `types.ts`, `launch-controller.ts`, `java.ts`, `natives.ts`

### launch-controller.ts

启动控制器，协调整个启动过程。

- `mcLaunchStart`: 启动主函数，触发启动流程
- `mcLaunchLog`: 启动日志记录
- 处理启动流程中的加载器和状态管理
- 错误处理和用户反馈

**依赖**: `arguments.ts`, `utils.ts`, `config.ts`, `types.ts`

### pre-launch.ts

启动前准备工作处理器。

- `mcLaunchPrerun`: 主预启动函数
- 配置 GPU 偏好设置
- 更新 launcher_profiles.json
- 设置游戏选项和资源包

**依赖**: `launch-controller.ts`, `utils.ts`, `config.ts`

### post-launch.ts

游戏启动后处理器。

- `mcLaunchEnd`: 启动完成后处理
- `handleGameExit`: 游戏退出处理
- 控制启动器行为（隐藏/最小化/关闭）
- 音乐播放控制

**依赖**: `config.ts`, `launch-controller.ts`, `utils.ts`

### natives.ts

原生库文件管理器。

- `mcLaunchNatives`: 解压并管理原生库文件
- `getNativesFolder`: 获取原生库文件夹路径
- 处理ZIP文件解压和文件管理

**依赖**: `types.ts`, `launch-controller.ts`, `utils.ts`

### java.ts

Java 运行时管理器。

- `mcLaunchJava`: Java选择和准备
- `extractJavaWrapper`: Java包装器提取
- 版本兼容性检测
- Java下载和安装

**依赖**: `launch-controller.ts`, `config.ts`, `utils.ts`

### login/login-controller.ts

用户登录控制器。

- 微软账户、离线模式、第三方验证服务器登录
- 处理登录状态和刷新令牌
- 皮肤和用户资料管理

**依赖**: `config.ts`, `utils.ts`, `types.ts`, `launch-controller.ts`

### process.ts

进程管理模块。

- 游戏进程启动和监控
- 进程输出捕获
- 进程终止处理

**依赖**: `config.ts`, `utils.ts`, `launch-controller.ts`

## 依赖关系图

```
                    +-------------+
                    |   config.ts |
                    +-------------+
                           ^
                           |
         +----------------+|+---------------+
         |                 |                |
+----------------+  +-------------+  +--------------+
|    utils.ts    |  |   types.ts   |  |  process.ts  |
+----------------+  +-------------+  +--------------+
         ^                ^                  ^
         |                |                  |
+----------------+  +-------------+  +--------------+
|  arguments.ts  |->| launch-     |<-|  java.ts     |
+----------------+  | controller  |  +--------------+
         ^          +-------------+          ^
         |                ^                  |
         |                |                  |
+----------------+  +-------------+  +--------------+
|  natives.ts   |->|  pre/post-  |->| login/login- |
+----------------+  |  launch.ts  |  | controller  |
                    +-------------+  +--------------+
```

## 启动流程

1. `mcLaunchStart` 在 `launch-controller.ts` 中被调用启动整个流程
2. 执行预检查和版本选择
3. 创建加载器队列（Java选择、登录、文件补全、参数生成、解压原生文件等）
4. 生成启动参数
5. 执行预启动处理（`pre-launch.ts`）
6. 执行自定义命令
7. 启动游戏进程
8. 等待游戏窗口出现
9. 执行启动后处理（`post-launch.ts`）

## 启动器配置项

主要配置项通过 `config.ts` 中的 `getConfig` 和 `setConfig` 函数管理:

- `LaunchArgumentWindowType`: 窗口启动模式（全屏、窗口等）
- `LaunchArgumentInfo`: 启动信息
- `LaunchAdvanceGame/Jvm`: 高级启动参数
- `LoginType`: 登录类型
- `UiMusicStop/Start`: 启动时音乐控制
- `LaunchArgumentVisible`: 启动后启动器可见性控制

开发新功能时，请确保遵循模块的责任边界，并维护现有的依赖结构，以确保代码的可维护性。
