# mcl 能力补齐与参考对齐 Plan

日期：2026-11-08
状态：逐项实施中。C1–C3、C4a/C4b/C4c1已实现/构建交付；新增回归及真实JVM/处理器/游戏验证待授权。C4整体未完成，下一项C4c2为其余明确旧Forge代际，再继续剩余工具及继承基底完整身份。每项先更新本Plan，再单独commit。
主要参考：SJMCL，基线 d5b109e0abb42460a8fd665c58ca4b0cc9982182。

## 1. 目标与现状

目标不是页面数量或同名按钮一致，而是达到相应的实际功能深度、版本/平台覆盖和失败处理能力，同时保留 mcl 原生、紧凑、少打扰的体验。

当前已经具备现代版本安装/修复、Java 管理和自动选择、微软/离线账户、四种加载器的现代路径、双源市场、三类整合包导入、多安装位置、动态自动内存、日志及实例全量删除等实际链路。不要把这些已有能力重新当作空白模块建设。

用户已对近期导入/删除体验包反馈“我已验证”。这属于用户体验验证记录；没有提供详细版本、平台和异常场景清单，不扩展为所有 Minecraft/整合包、全平台或所有恢复路径均已通过。

最大的不足是：

1. 能识别整合包格式，但其中依赖 Java 8、旧 Forge 或特殊运行组件的包仍不一定能安装/运行。
2. 可以选择多个 mcl 安装位置，但不能直接扫描并接入其他启动器已经安装的游戏。
3. 下载有并发、限速、取消、缓存和校验，但缺完整持久任务、暂停续传及重启恢复。
4. 实例从创建/导入到分享/迁移/更换运行环境的生命周期尚不完整。
5. 存档、服务器、截图、投影及批量资源维护能力不够全面。
6. 诊断、更新、配置迁移和平台发行能力还没有形成完整产品闭环。

本计划以当前 SJMCL 专项为比较对象，不替代 docs/parity.md 中早期 b-launcher 总体目标。参考源码只读；不得复制 GPL 源码、品牌、凭据、私人服务或构建依赖。

## 2. 缺点、影响与必须覆盖的面

| 编号 | 主要缺点 | 对玩家的影响 | 需要覆盖的面 | 优先级 |
| --- | --- | --- | --- | --- |
| C | 历史版本/运行环境兼容窄 | 常见旧整合包不能完整使用 | Java 8、旧启动参数、历史资产布局、版本继承、旧 Forge、Java/OS/CPU 匹配 | P0 |
| D | 已有目录互通不足 | 必须重新创建或重新导入，不能利用现有安装 | 官方目录、第三方 versions、身份识别、只读发现、受管导入、跨盘迁移、冲突与占用 | P0 |
| T | 下载任务恢复能力弱 | 大包中断、暂停或重开后体验差 | 统一任务组、持久状态、部分文件、Range、暂停/继续/重试、崩溃恢复 | P0 |
| I | 实例生命周期不完整 | 分享、迁移、改加载器和分别配置不方便 | 包导出、资源跨实例复制/移动、loader 变更、全局继承/实例覆盖、标签/收藏/快捷方式 | P1 |
| R | 实例资料管理不足 | 玩家的游戏文件仍主要依靠文件管理器维护 | 存档、服务器列表、截图、投影、批量本地导入及非覆盖发布 | P1 |
| A | 资源维护未形成批量闭环 | 模组多时操作繁琐，回退和历史管理不足 | 批量更新、依赖诊断、候选选择、缺失补齐、更新历史/回退/清理、普通安装恢复 | P1 |
| L | 启动诊断和控制深度不足 | 出错后不易定位，也不便提交有效反馈 | 启动器可见性、异常分类、崩溃报告、诊断导出、日志保留、受控高级选项 | P1/P2 |
| P | Java来源及产品化不完整 | 特定环境不可获得、升级和配置迁移不顺畅 | Mojang runtime、多来源、受管 Java 回收、关于/许可、更新、本地化、配置导入导出、三平台发行 | P2 |

功能覆盖率和测试覆盖率是不同指标。本计划不凭代码行数、测试文件数量或按钮数量估算百分比。

## 3. 总体推进顺序与依赖

按以下顺序逐个交付，不一次性铺开所有模块：

C 兼容性底座 → D 既有目录接入 → T 持久下载与恢复 → I 实例生命周期 → R 游戏资料管理 → A 批量资源维护 → L 诊断/控制 → P 产品化。

- C1–C3、C4a/C4b/C4c1已完成各自实现/构建闭环，精确范围与未验证项见第18–23节；继续C4c2，不跳过其余历史格式和资源差距。
- D 可先只发现现代受支持版本，不为等待全部历史兼容而阻塞只读扫描。
- T 应复用现有下载控制/缓存/摘要保护，不能另建一套不共享预算的下载器。
- I 的资源迁移、loader 变更和 A 的批量更新依赖现有实例锁及恢复机制；新增事务不能绕过删除/更新 pending 标记。
- P 中关于/许可等小项可以独立完成，但不能用这些容易完成的页面替代 P0 底层差距。

每个子项完成后先更新状态和剩余边界，再开始下一个子项。每次交付都应能说明真实行为、影响范围以及尚未验证的内容。具体参考文件、调用链、取舍和 mcl 落点见第 17 节；不能只看本节功能名称就开始补按钮。

## 4. 第一阶段 C：版本与运行环境兼容

### C1. Java 8 启动传输

状态：已研究、已设计、已实现、已构建/交付；回归与真实 JVM/游戏验证未运行（遵循当前授权）。

原问题：自动 Java 选择拒绝 Java 8，游戏启动依赖 Java 9+ 的参数文件。现已增加独立 Java 8 传输，而不是只删除版本判断。实施记录见第18节。

实施内容：

- 明确 Java 8 与 Java 9+ 的参数传输分支；现代链路保持原行为。
- 处理 Windows 长 classpath/命令行限制，以及中文、空格、引号、反斜杠和特殊字符路径。选择具体方案前核对 JVM 规范；若采用标准 manifest classpath JAR，它只能描述现有运行库，不包含注入类、Agent 或绕过校验的补丁。
- Java 8 仍必须经过实际版本/架构/稳定性探测；自动下载按同主版本、同架构查官方包，缺少对应环境时明确失败，不能盲选高版本或抢先版。
- 凭据不写参数文件或辅助 JAR；日志/预览/异常继续脱敏。
- 保留 process.StartTracked 的受控交接、会话身份、停止与实例占用保护；临时支持文件有自己的生命周期和异常退出回收。
- 区分游戏 Java 与安装处理器 Java；不能假定某个旧游戏使用 Java 8，就让所有现代安装工具也无条件使用 Java 8。

交付闭环：能为符合当前现代元数据格式、要求 Java 8 的实例生成并执行受控启动计划；有新旧分支回归源码和构建证据。真实 Java/游戏执行须等待验证授权，未执行时明确记录。

不在 C1 内冒称完成：全部旧版 Minecraft、旧 Forge、历史资产格式及所有 ARM 旧 native。

### C2. 历史官方元数据与参数

状态：已研究、已设计、已实现、已构建/交付；回归与真实历史游戏验证未运行。继承仅限官方固定摘要链的原版；未知补丁及继承型客户端叠加 loader 尚不开放，详见第19节。

- 支持明确范围内的 minecraftArguments，并将其规范化为受控参数模型，不拼 shell。
- 对没有明确 Java 要求的历史官方版本建立可追溯兼容规则，不能仅凭任意版本名猜测。
- 支持需要覆盖的继承链：限制深度、检测循环、核验父文档身份，正确合并 main class、libraries、rules、client JAR 和参数。
- 正确区分官方历史文档、受支持 loader profile 和未知第三方运行补丁，不能简单放行所有 inheritsFrom。
- 补旧版日志配置、平台规则与启动占位符；不为了兼容偷偷下载 Agent/认证或安全补丁。

### C3. 历史资产及本地库

状态：已研究、已设计、已实现、已构建/交付；回归、历史游戏与文件系统竞争验证未运行。范围为带固定摘要索引/对象的 virtual、map_to_resources 和现有 native 格式；不扩展为所有旧版本/ARM本地库兼容，见第20节。

- 补 virtual/assets 与 map-to-resources 等历史布局，明确共享对象与实例映射的所有权。
- 覆盖旧 classifier natives、解压排除项和不同系统/架构命名；不能把 x86 的“64”当作 ARM64。
- 缺少匹配 native 时明确报告，不以“下载完成”冒充可运行。
- 保留摘要、路径/链接、展开大小和跨实例写入协调。

### C4. 旧 Forge 与 loader 兼容

状态：进行中，不标整体完成。C4a接1.12.2旧格式；C4b接独立工具Java和spec1受限旧模板；C4c1接1.7.10无处理器旧格式，均仅构建未验证。C4c2继续其余明确旧代际；剩余工具、完整基底身份与实际验收待推进，见第21–23节。

- 按安装器/配置代际列出支持范围，逐类补旧 Forge、缺失 processor/客户端后处理。
- 固定官方安装器与依赖身份；工具执行有界、可取消、使用经过检查的 Java。
- 正确处理安装器内文件、运行库合成和启动配置，不把无法解释的包当原版。
- 为导入页提供可解释的兼容结论，区分“格式已识别”“文件可导入”“环境已安装”和“游戏已运行”。

候选验收版本：先选一个 Java 8 且现代参数格式的原版（例如 1.16.5），再选一个旧参数格式的原版（例如 1.12.2），随后选择固定版本的对应 Forge 包。以上是计划样本，不是支持或通过声明。更早版本后续逐代扩展，不能用这两个样本代表全历史覆盖。

## 5. 第二阶段 D：接入已有游戏目录

### D1. 只读发现

- 显式添加官方或第三方 Minecraft 根目录，发现 versions 下的候选实例。
- 显示实际位置、版本、loader、兼容/缺失信息和来源；有界扫描、可取消、路径去重。
- 处理重复名称/ID、父子目录重叠、符号链接、外部卷离线和损坏文档。
- 发现不得自动修改、修复、删除、补文件或接管其他启动器账户/配置。

### D2. 确认后导入/受管复制

- 先定义“仅发现”“复制为 mcl 受管实例”“原地管理”三种语义；第一批优先受管复制，原地管理未实现时明确禁用，不能把它伪装成已有功能。
- 展示将复制的存档、模组、配置和资源，默认保留源目录。
- 只导入允许的游戏资料，不执行其他启动器脚本、不继承认证凭据或任意 JVM/Agent 参数。
- 保留稳定实例身份、文件校验、非覆盖发布和占用保护。

### D3. mcl 实例迁移

- 支持内置/外部位置间迁移，区分实例载荷与共享 libraries/assets/Java；移动游戏不等于移动应用数据根。
- 跨卷使用复制、核验、提交、清理的恢复流程，不用跨卷 rename 假装原子事务。
- 取消、源变化、目标冲突、空间不足或断电时可解释地恢复，不删除唯一有效副本。
- 删除行为跟随真实归属：原地发现/非受管资料不能套用“删除整个 mcl 实例目录”的逻辑。

## 6. 第三阶段 T：统一任务、暂停与续传

按 T1–T4 顺序完成：

- T1：建立可持久的任务组、文件任务、验证/发布阶段和终态原因；汇总游戏、loader、Java、市场和整合包任务，不丢失现有分阶段信息。
- T2：暂停必须等待旧执行退出并保存进度；继续/重试不得和旧 worker 重叠；取消与暂停、下载完成与发布成功必须区分。
- T3：安全恢复部分文件；校验 Range/Content-Range、ETag/Last-Modified 等可用身份信息。处理不支持 Range 的 200、206 起点错误、416、内容变化、重定向、摘要失败和损坏临时文件。
- T4：重开/崩溃恢复与任务历史；接资源文件去重和跨进程协调，保证同目标不被多个任务同时写坏。

必须继续共用现有并发、聚合限速、代理、来源与凭据规则；限速等待不能被算作网络空闲。任务持久化和显示进度不能保存令牌、CF Key 或完整认证 URL。

## 7. 第四阶段 I：实例生命周期

- I1：导出文件选择及预览，先完成 MultiMC/Prism 和 MRPack 的可回导闭环；需要覆盖的其他格式再逐项补齐。
- I2：处理远端引用、本地覆盖项、可选文件及分发许可；禁止把受限资源重新打包分发或猜下载地址。默认排除账户、认证令牌、系统路径、启动器私密配置和无关日志。
- I3：已有实例的 loader 安装/升级/移除/切换；原版和整合包固定运行环境分别制定规则，不能直接改固定包记录破坏一致性。变更先给计划，再用可恢复事务提交。
- I4：全局设置继承与实例覆盖，明确每项“继承/覆盖/恢复继承”，防止复制整份全局配置后失去继承关系。
- I5：描述编辑、标签、收藏、目录导航、游玩统计及桌面快捷方式。统计使用本应用可核验的会话，不猜其他进程时间。
- I6：跨实例资源复制/移动；检查目标兼容性、冲突和来源记录，保留依赖/更新保护，移动失败不得丢失源文件。

手动内存不随 I4 擅自恢复，继续执行用户当前的全自动内存决定。

## 8. 第五阶段 R：游戏资料管理

### 存档

- 有界解析 level.dat，显示名称、游戏版本、最后游玩等可验证信息。
- 本地 ZIP 导入、目录打开、导出和删除确认；未知/损坏字段降级而非破坏原文件。
- 游戏运行时避免危险修改，存档备份/替换操作必须能恢复；数据包与所属存档绑定。

### 服务器

- 安全读取/写入 servers.dat，支持添加/删除、去重及原子保存。
- 在线状态查询显式有界、超时/可取消，不在渲染阶段发请求；不连接任意管理协议或执行服务器命令。

### 截图和投影

- 截图缩略图/预览/打开/导出，处理大图、损坏文件和批量读取。
- 支持明确格式的 schematic/litematic 资料管理，不假装游戏已安装对应模组或一定能加载。
- UI 复用已有资源列表、图像边界与后台并发，不增加整页常驻说明或装饰卡片。

## 9. 第六阶段 A：批量资源维护

- A1：把现有多 Selections 更新计划真正接到多选 UI，先完成整组依赖/文件变化确认，不逐行盲目循环安装。
- A2：更新历史、选择回退、备份保留/清理；不能删除唯一恢复副本或仍被 pending 事务引用的文件。
- A3：补复杂版本范围、mod ID/provides、内嵌候选和跨源关系；不以文件名/mod ID 猜市场项目，无法证明的关系保持未知。
- A4：补未匹配文件关联、缺失文件/本地原包缓存丢失后的明确补齐入口，以及普通安装中断恢复。
- A5：批量启停/移除/恢复有完整依赖检查和结果报告，不能用一次成功代表整个批次成功。

这部分既有参考对齐，也有 mcl 自身未完成的维护能力；不能未经核实就声称参考拥有完整依赖求解器。

## 10. 第七阶段 L：运行控制、诊断与高级选项

- 三档启动器可见性：始终显示、启动时隐藏、运行时隐藏；失败/退出恢复，退出页面不能被误当停止游戏。
- 更清楚的准备失败、窗口前退出、游戏崩溃和正常结束分类；基于本次精确会话，不按实例名/PID 猜测。
- 诊断导出包含必要日志、元数据、版本/平台/Java信息和脱敏参数；对报告文件有边界与保留策略，不导出账户秘密或原始含令牌启动脚本。
- 在实际 JVM 支持判断后补结构化 GC/元空间等选项；其他受控参数、环境、独立游戏代理与图形/系统库能力按平台展开。
- 自定义窗口标题等必须使用平台公开能力，不能为对齐引入注入/补丁。

## 11. 第八阶段 P：Java 与产品化

- P1：Mojang 官方 runtime 目录与固定摘要安装；再评估其他发行商，明确许可、平台和主版本，不构造非官方二进制地址。
- P2：Java 批量管理、受管安装卸载、失效登记和暂存/旧代际回收；不删除用户系统 Java，不清理仍被任务或实例使用的环境。
- P3：版本/构建信息、原生关于页、源码和许可聚合，资源与发行包同步。
- P4：使用 mcl 自有发布源检查/下载/显式安装更新，验证平台载荷、摘要/签名，支持失败恢复；参考 Linux 的空安装分支不计作已覆盖能力。
- P5：去凭据的设置导入/导出、重置、备份和冲突处理；联网同步若推进需自有或可审计服务，不能复制参考私有接口。
- P6：本地化、首次语言/辅助页面偏好和日志保留；不覆盖现有游戏配置或擅自替玩家修改无障碍选择。
- P7：签名/公证、Linux 分发、干净安装/升级/卸载和文件关联/协议行为，区分二进制编译与平台可用。

## 12. 横向覆盖矩阵

每项必须记录实际覆盖，不采用“支持 Windows/所有版本”之类笼统勾选。

| 维度 | 至少要区分的情况 |
| --- | --- |
| 系统/架构 | macOS arm64/x86_64、Windows x64、Linux x64；其他架构仅在实际资源与证据具备后增加；不承诺Windows 32位 |
| Java | 8、现代常用主版本；JRE/JDK、稳定/EA、匹配/错误架构、失效显式路径、无可用官方包 |
| Minecraft | 现代release、现代snapshot、Java8现代参数版本、旧参数版本、需要继承/历史资产的版本 |
| loader | 原版、Fabric、Quilt、Forge、NeoForge；安装代际/处理器分别列出，不以loader名称覆盖全版本 |
| 整合包 | CF/MRPack/MultiMC/Prism；根/外层目录、可选/服务端文件、缺摘要、受限文件、原包丢失、未知组件 |
| 文件系统 | 中文/空格/特殊字符、同名实例、大小写/规范化冲突、链接、长路径、外部盘离线、跨卷、只读/空间不足 |
| 网络 | 超时、断网、限速、代理切换、换源、重定向、分发拒绝、部分内容变化与摘要失败 |
| 生命周期 | 取消、暂停、重试、窗口关闭、重开启动器、进程交接、旧回调、游戏仍在运行 |
| 恢复与并发 | 多任务/多启动器、更新/删除pending、写入中退出、发布前后中断、目标被外部修改 |
| 身份与隐私 | 微软/离线、刷新/失效、脱敏、凭据存储失败、诊断导出不泄露秘密 |

## 13. 每个里程碑的完成标准

逐项记录以下状态，不把它们混成一个“完成”：

1. 已研究：有参考入口、范围和失败边界。
2. 已设计：数据归属、兼容条件、并发/恢复及迁移方案明确。
3. 已实现：模型/服务/存储/平台/UI实际贯通，没有只有界面的空开关。
4. 已构建：格式整理、编译/打包证据及具体平台。
5. 已验证：记录运行过的用例、版本、平台、结果和限制；未运行就写未运行。
6. 已交付：体验包/源码版本、用户反馈与剩余项可追溯。

当前用户偏好：不主动运行测试、vet/race、UI自动化、公网/游戏/原生验收；可以按授权阶段进行必要格式整理和构建。回归源码可以随实现补齐，但未经允许不执行，不能当作已通过。

用户一句“已验证”可以作为该批体验确认；没有步骤时不伪造更细的测试报告。恢复系统性验证需后续授权，但不能因此把缺失证据写成已具备。

## 14. 工程与变更纪律

- 保留 UI → app → core 分层和入口显式注入，不复制参考的全局服务模式，不依赖 .repo 构建。
- 复用现有下载预算、凭据存储、SessionRegistry、稳定实例ID和非覆盖发布机制。
- 配置升级保留原始备份；损坏/未来schema不覆盖。跨进程普通设置写入仍需补冲突控制，不把进程内串行当成跨进程事务。
- 普通操作不能绕过资产更新或实例删除标记；新增迁移/恢复用窄权限入口。
- 用户已明确授权将此前未提交增量提交/推送，并要求逐项更新本 Plan 后单独 commit。基线为 a643553（128 个文件），已推送 origin/feature/sjmcl-settings；不改主分支、Tag 或已发布 Release。后续提交仍需清楚界定源码与构建范围，不把本机构建冒充远端 CI。
- 不声称更快/更省内存；性能结论须来自后续获准的实际测量。

## 15. 明确不自动扩大的范围

以下不计入当前必须“追平”的缺陷，不得凭参考有此功能就恢复：

- 智能、扩展插件、文档帮助、外观：此前明确排除的专项范围。
- 手动内存输入：遵循用户最新的全自动策略。
- Agent、第三方认证/皮肤注入、任意启动/包装/退出脚本、绕过Java或文件校验。
- 借用参考Client ID/CF Key、猜受限下载地址、复用私人代理/同步服务。
- OptiFine、Cleanroom等额外组件仅列候选覆盖，不在未核对许可和兼容边界前默认纳入执行。
- 服务端管理/下载、新闻与社区等非核心延伸不抢占本轮P0。

## 16. 当前执行状态与下一项

C1 实现/构建小闭环（验证状态单独保留）：

- [x] 列出 Java8/Java9+ 启动传输方案与安全边界（第18节、决策0036）。
- [x] 接通 Java8 同主版本选择、参数生成、受控创建及支持文件退出/重开回收。
- [x] 保持现代 Java 分支，补中文/空格/长 classpath、凭据隔离与交接/异常退出回归源码。
- [x] 生产及 Windows/Linux amd64 主程序编译，macOS arm64 临时签名体验包；未运行测试或游戏。
- [x] 更新本 Plan 并单独提交 C1，再开始 C2；不将本项标为历史整合包全兼容。
- [ ] 获得验证授权后执行离线回归和显式 Java8 协议/真实游戏验收。此项待验证，不伪造通过记录。

C2 实现/构建闭环：

- [x] 旧参数有界词法解析，先分词再替换；转换为现有受控参数模型，不拼 shell。
- [x] Mojang 固定摘要/ID/包路径核验后规范化；明确缺 Java 要求的历史 release 兼容表，不猜快照或任意第三方版本名。
- [x] 固定父清单快照、最多8层/循环检测、逐文档摘要与身份、主类/条件库/参数/客户端JAR合并；拒绝未知 patches/任意 jar 引用。
- [x] 补 auth_session、旧 Log4j 文件参数、平台条件默认 JVM 参数及历史资产缺口的明确错误。
- [x] 补离线回归源码并完成生产/Windows和Linux amd64编译、macOS arm64体验包，更新 Plan 后单独提交。
- [ ] 授权后执行新增离线回归及真实1.12.2/1.16.5与继承样本验收；不把合成文档当官方历史版本已验证。

C3 实现/构建闭环：

- [x] 支持散列对象与具名映射计划分离，索引/路径/别名/文件目录冲突及资源规模边界。
- [x] 实例独占 virtual 资源代际、所有权标记、私有复制/非覆盖发布/收据/启动 game_assets 路由。
- [x] map_to_resources 使用入口注入的真实游戏目录，缺项非覆盖发布、相同复用、不同冲突保留；取消/重开可重查已发布子集。
- [x] 旧 native 操作系统/位数/ARM 明确匹配、排除项校验、同FD解压前后摘要与CRC/展开边界、所有权回收。
- [x] 补回归源码，生产/Windows和Linux amd64编译及macOS arm64体验包，更新Plan后单独提交。
- [ ] 授权后执行离线回归、真实历史版本/native与外部目录/取消/竞争/崩溃验收；本轮未执行。

C4分批状态：

- [x] C4a：识别1.12.2旧install/versionInfo无processors格式，内嵌Universal及依赖固定身份、LaunchWrapper完整模板替换、独立运行库发布/缓存校验/安装收据/启动计划贯通。已构建，未运行回归或真实游戏。
- [ ] C4b：游戏Java与安装处理器Java独立解析；现代spec1配合旧minecraftArguments/LaunchWrapper，受支持处理器及客户端后处理。
- [ ] C4c：更早1.7.10等安装器代际、其余处理器与明确的继承base缓存身份，不放行未知补丁。
- [ ] 授权后执行各代际固定官方fixture、依赖缺失/摘要错误/取消恢复、实际Java/游戏/平台验收；无验证证据不勾通过。

下一项C4b，不跳到D；D–P仍未实施。

## 17. 具体如何参考我们的参考仓库

### 17.1 参考对象与使用方式

- 本专项以 .repo/SJMCL 为主参考，固定提交 d5b109e0abb42460a8fd665c58ca4b0cc9982182。下面的参考路径均相对于该目录。
- .repo/b-launcher 继续作为早期总体能力参考，需要时定向阅读，并注明来源；不能把两个仓库的同名功能、兼容范围或验证证据混在一起。
- 参考仓库只读：不修改、不在其内开发、不依赖其构建、不导入为嵌套仓库/submodule。只使用本地源码静态阅读，不为“对照”自行启动应用、下载资源或操作账户。
- 研究行为、格式、边界和数据流后独立设计 Go 实现，不复制 Rust/TypeScript 源码、逐行翻译、移植其全局状态或搬运图片/品牌。SJMCL 为 GPLv3，不能把“可以本地阅读”当作“可以直接复制进当前工程”。确需源码复用时另行审查许可证与项目分发影响并保留声明，本计划不默认采用。
- 阅读前核对参考提交与对应文件是否仍存在。升级参考基线必须明确记录，不让计划静默跟随参考 HEAD 变化。

### 17.2 每项功能固定沿这条链阅读

1. **用户入口**：阅读页面/组件及事件处理，确认用户真正能做什么、哪些选项只是显示，是否有取消/重试/确认。
2. **命令边界**：追踪页面调用的 Tauri command 与输入输出模型，确认参数、默认值、错误和服务端实际限制。
3. **核心实现**：沿 commands → helpers/下载执行器 → 文件/进程/持久化，核实功能是否真实落盘、启动或恢复，而不是止于函数名。
4. **终态反馈**：反向追踪成功/部分成功/失败/取消如何回到 UI，是否保留可继续任务、旧文件和实例记录。
5. **独立规格**：用自己的描述列出输入、输出、状态转换、文件归属、支持范围和异常分支；同时标注“采用”“改写”“不采用”“参考本身未实现/未验证”。
6. **mcl 落地**：先对照已有模块，补 core 普通值/窄接口、app 编排、storage/platform 适配，最后接 MyGo UI；入口注入，不增加全局可变容器/通用事件总线。
7. **对照完成**：检查正常路径和失败/恢复路径都贯通，再更新兼容与交付记录。没有授权的验证保留待办，不能因看过参考源码就写“已通过”。

每个小闭环只需在现有进度记录附简短对应项：参考提交与文件/符号 → 观察到的行为 → 采用/改写/不采用理由 → mcl 对应模块 → 覆盖与未覆盖场景 → 构建/验证证据。不要求为普通函数另建文档。

### 17.3 C：Java、历史版本和 loader

**先看哪里：**

- 启动入口：src-tauri/src/launch/commands.rs，重点 select_suitable_jre、validate_game_files、launch_game。
- Java 选择：src-tauri/src/launch/helpers/jre_selector.rs，重点 select_java_runtime、get_minimum_java_version_by_game；配合 src-tauri/src/launcher_config/helpers/java.rs。
- 参数与规则：src-tauri/src/launch/helpers/command_generator.rs 的 generate_launch_command，以及 src-tauri/src/instance/helpers/client_json.rs 的新旧参数、rules、libraries 和 patches 处理。
- 资源与 native：src-tauri/src/instance/helpers/asset_index.rs、client_jar.rs，以及 src-tauri/src/launch/helpers/file_validator.rs。
- loader：src-tauri/src/instance/helpers/loader/forge.rs、neoforge.rs、fabric.rs、quilt.rs、common.rs、postprocess.rs；从 src-tauri/src/instance/commands.rs 的 create_instance 追进去，不只阅读目录列表。

**提炼什么：** Java 要求来源与优先级、新旧参数规范化、classpath 与 natives 顺序、历史资源映射、loader 安装阶段及客户端后处理。区分游戏所需 Java 与执行安装器/processor 的 Java。

**落到哪里：** internal/core/meta、internal/core/launch、internal/core/install、internal/core/java、internal/core/loader；internal/app/java_selection.go、launch.go、installations.go；internal/platform/process 和相应 storage 记录。不要在 UI 增加一个“Java8兼容”开关掩盖管线缺失。

**不能照搬：** 参考的“没有同主版本就找更高 Java”不能替代 mcl 的兼容决策；脚本/wrapper、Agent/第三方认证、自定义 native 补丁和包含凭据的完整命令导出不默认采用。参考支持某个 loader 名称不等于其所有历史版本或 processor 都受支持。

### 17.4 D：目录发现、导入与迁移

**先看哪里：**

- src/pages/instances/list.tsx、src/pages/settings/general.tsx 的目录选择/注册行为。
- src-tauri/src/launcher_config/commands.rs 的 check_game_directory，及 helpers/misc.rs 的默认目录发现。
- src-tauri/src/instance/helpers/misc.rs 的 refresh_instances、refresh_instance、get_instance_subdir_paths；同时读 src-tauri/src/instance/models/misc.rs 的实例身份和路径模型。

**提炼什么：** 如何识别 versions 子目录、定位 JSON/JAR、区分游戏根/版本目录、处理同名和隔离目录。扫描发现与正式管理是两种能力，不能混为一谈。

**落到哪里：** 复用 internal/app/game_directories.go、library.go，internal/storage/game_directories.go、assets.go 和现有实例文档。新增受控发现/复制步骤，不把参考的原始路径直接写入 mcl 的受管所有权记录。

**需要自行补设计：** mcl 已有 owner 标记、外部 payload、全量删除语义和真实会话锁。迁移必须与这些机制一致；参考的目录扫描实现不能直接证明跨卷恢复或原地管理已安全，不能扫描完就把用户原目录登记为可整目录删除。

### 17.5 T：任务与下载恢复

**先看哪里：**

- 业务接入：src-tauri/src/download.rs、src/components/download-indicator.tsx，以及包导入/运行环境安装提交任务的位置。
- 模型和命令：src-tauri/crates/sjmcl-downloader/src/model.rs、command.rs、event.rs。
- 状态机：同目录 actor.rs，重点 submit_group、pause/resume/cancel/retry 与内部 cmd_pause、complete_pause、try_resume_group。
- 文件传输与保存：download.rs、executor.rs、persistence.rs、storage.rs、rate.rs；tauri.rs 仅用于理解宿主事件接入。

**提炼什么：** 任务组与单文件的关系；暂停等待旧 worker 退出、保存偏移后才允许继续的顺序；部分内容身份校验；完成前校验/发布；持久化失败如何反馈及重开如何恢复。

**落到哪里：** 扩展 internal/core/download 的既有 client/control/rate/cache，internal/app/downloads.go 与各安装服务；持久记录放 storage，UI 仅订阅有界快照并处理明确操作。是否需要数据库由 mcl 的实际状态规模决定，不因参考用 downloads.db 就照搬数据库或 Rust actor 框架。

**不能照搬：** 参考内置 CF Key/请求装饰器、私有下载入口、全局调度容器和不符合 mcl 文件所有权的清理规则。任务恢复与资产更新 journal 分层协作，不能互相冒充。

### 17.6 I：导出、loader 变更与实例设置

**先看哪里：**

- 导出交互：src/components/modals/export-modpack-modal.tsx；后端 src-tauri/src/instance/commands.rs 的 retrieve_exportable_file_list、export_modpack。
- 导出执行：src-tauri/src/instance/helpers/modpack/export.rs 的 validate_export_options、list_files、build_export_bundle、create_modpack_zip；配合 multimc.rs、modrinth.rs、curseforge.rs 核对真正构造的格式，不能只看枚举名称。
- loader 变更：commands.rs 的 check_change_mod_loader_availablity、change_mod_loader、remove_mod_loader，并追到 loader helpers。
- 实例继承：src/pages/instances/details/[id]/settings/index.tsx、advanced.tsx；src/components/game-settings-groups.tsx、game-advanced-settings-group.tsx；后端 retrieve_instance_game_config、reset_instance_game_config、get_instance_game_config 与配置模型。
- 资源迁移/快捷方式：commands.rs 的 copy_resources_to_instances、move_resource_to_instance、create_launch_desktop_shortcut。

**提炼什么：** 文件选择默认值/排除项、清单与覆盖文件生成、可分发远端引用、继承与覆盖优先级、变更前可用性检查和最终写回。

**落到哪里：** 复用 internal/core/modpack、core/loader、core/launch，app/archive_import.go、pack_import.go、instance_settings.go、runtime_settings.go 及现有事务/实例保存入口；导出作为实际 ZIP 写入任务，不能把导出按钮等同于实现。

**需要改写：** 采用 mcl 的非覆盖发布、pending/回滚和凭据隔离，不复制参考的文件替换策略、普通配置中可能存在的本机路径或自定义脚本。全局/实例设置按类型化字段建模，不照搬字符串路径反射更新。

### 17.7 R：存档、服务器、截图和投影

**先看哪里：**

- src/pages/instances/details/[id]/worlds.tsx、screenshots.tsx、schematics.tsx、overview.tsx。
- src/components/modals/world-level-data-modal.tsx。
- src-tauri/src/instance/commands.rs 的 retrieve_world_list、retrieve_world_details、retrieve_game_server_list、add_game_server、delete_game_server、retrieve_schematic_list、retrieve_screenshot_list。
- src-tauri/src/instance/helpers/world.rs、server.rs、options_txt.rs 和 models/world.rs。

**提炼什么：** 目录/文件定位、NBT 字段与容错、服务器列表保存格式、截图排序/预览和资源操作结果。先区分“列出文件”“读取详情”“修改成功”各层，不把参考存在页面当作完整编辑能力。

**落到哪里：** 通过 app/assets.go 和 storage/assets.go 的所有权校验入口访问游戏文件；新增 NBT 等解析放 core、文件保存放 storage，UI 复用现有列表与后台图像读取。需要新依赖时先说明格式需求、维护和平台影响。

**需要补强：** 扫描/解析/图片有界，后台可取消，运行中危险写入拒绝或明确协调；损坏文件、未知版本和停用/重命名路径不能被默默覆盖。

### 17.8 A：模组与资源批量维护

**先看哪里：**

- src/pages/instances/details/[id]/mods.tsx、resourcepacks.tsx、shaderpacks.tsx 及实际更新/多选事件。
- src/components/modals/alert-resource-dependency-modal.tsx、download-resource-modal.tsx、download-specific-resource-modal.tsx 和 src/components/resource-downloader.tsx。
- src-tauri/src/instance/commands.rs 的 retrieve_local_mod_list、toggle_mod_by_extension、copy_resources_to_instances；src-tauri/src/instance/helpers/mods/ 各格式解析、resourcepack.rs；沿前端真实调用继续追踪 src-tauri/src/resource 的身份/版本查询实现。

**提炼什么：** 本地身份与项目匹配、版本选择/更新提示、批量确认与逐项结果、依赖提示和启停操作的实际范围。

**落到哪里：** 继续使用 internal/core/mods、core/market、app/asset_diagnostics.go、asset_updates.go、asset_identity.go、asset_association.go，storage/asset_relationships.go、asset_update_transaction.go 和现有 UI 资源页。

**保留差异：** mcl 已有按摘要的关系保护和更新恢复，不因参考用扩展名切换就削弱保护。复杂候选求解、跨源依赖和历史回退是需要另行验证的缺口，不能看到参考“依赖提示”就宣称参考已完整求解。

### 17.9 L：日志、退出与诊断

**先看哪里：** src-tauri/src/launch/commands.rs 的 retrieve_game_log、retrieve_game_launching_state、export_game_crash_info，以及 helpers/process_monitor.rs、log_parser.rs、graphics_handler.rs；可见性和高级选项配合 launcher_config/models.rs 与游戏设置组件阅读。

**提炼什么：** 进程退出、窗口出现、隐藏/恢复、日志读取、错误分类和诊断包内容分别由谁负责，如何准确关联同一次启动。

**落到哪里：** internal/core/launch 的 output/record/session，app/game_sessions.go、launch.go，platform/process 及 UI game_sessions/launch_view。诊断导出应使用脱敏快照而不是直接保存 os/exec 参数；输出缓冲/文件保留必须有界。

**不照搬：** 任意预启动/包装/退出命令、带秘密的 full_command、按 PID 猜会话、忽略优先级失败或不受控的进程创建。

### 17.10 P：Java、配置和发布更新

**先看哪里：**

- Java：src/pages/settings/java.tsx、src/components/modals/download-java-modal.tsx；launcher_config/commands.rs 的 retrieve_java_list、validate_java、download_mojang_java；helpers/java.rs 的 build_mojang_java_download_params。
- 配置：src/pages/settings/general.tsx，launcher_config/models.rs、migrations.rs、commands.rs 的配置导入/导出/重置。
- 更新/关于：src/pages/settings/about.tsx，commands.rs 的 check_launcher_update、download_launcher_update、install_launcher_update，以及 helpers/updater.rs。

**提炼什么：** 官方 runtime 清单/文件身份、登记与发现区别、配置迁移与重置范围、更新版本判断、各平台实际安装能力及失败反馈。

**落到哪里：** core/java、app/java.go、storage/java.go；app/settings.go、storage/settings.go 与系统适配；更新使用 mcl 自有 Release/发布工作流，不使用参考品牌、应用标识、服务或二进制。

**特别核实：** 参考 install_launcher_update 的 Linux 分支没有安装实现，不能列为“Linux更新已完整支持”。参考联网配置同步及其服务器地址不采用；mcl 配置导出不能包含账户令牌、CF Key 或未批准的设备路径。

### 17.11 第一个小闭环 C1 的参考演示

实际执行时应按以下顺序，而不是先修改 major < 9 判断：

1. 阅读 select_suitable_jre → select_java_runtime，写清参考怎样合并客户端/loader 的要求及挑选环境。mcl 保留兼容显式偏好、同主版本/同架构和稳定版本检查，不照搬其更高主版本回退。
2. 阅读 generate_launch_command → launch_game，确认 class_paths 与 args 分离、最后由受控进程环境的 CLASSPATH 传入；这只是参考采用的传输方式，不是默认要求 mcl 复制。
3. 对照 mcl 的 Plan.ProcessArguments、JVMArgumentFile、process.start、GateMain 和临时文件回收，列出 Java8 不支持参数文件所影响的每一个节点。
4. 比较直接参数、受控 CLASSPATH 环境和标准 manifest classpath JAR 等候选方案，核对 Windows 命令行/环境长度、Unicode、路径编码、类加载与清理边界后再选方案。保持 Java9+ 分支，不允许从父环境继承任意 CLASSPATH/Agent。
5. 用自建的小型格式/参数案例编写回归源码，覆盖现代Java不退化、Java8传输、长路径、凭据不落盘、交接失败和残留文件。普通离线用例不得默认要求系统存在 Java；真实 JVM 协议验证另做显式集成项，当前不运行。
6. 在进度里记录参考文件/符号、选定方案、mcl 修改范围与精确边界。C1完成也只能表示这一传输闭环，旧参数/旧Forge/历史资源继续留在C2–C4。

以上路径已在当前参考基线做静态存在性与关键符号核对；没有运行两边应用或把源码阅读当成行为验收。之后发现参考实际行为与计划假设不符，应修正计划和差距，不为“追平”强行照搬。

### 关联文档

- docs/sjmcl-gap-audit.md：本轮源码差距审计与证据入口。
- docs/sjmcl-alignment.md：设置专项矩阵。
- docs/import-alignment.md：导入能力矩阵及兼容差异。
- docs/compatibility.md：确切版本与平台证据，需随实际进展补充。
- docs/verification.md：构建、验证与用户反馈记录。
- PLAN.md：当前状态和下一项任务入口。

## 18. C1 实施记录（2026-11-08）

### 研究与选择

- 参考基线已实际核对为 d5b109e0abb42460a8fd665c58ca4b0cc9982182。静态阅读 launch/commands.rs 的 select_suitable_jre/launch_game、helpers/jre_selector.rs 的 select_java_runtime/get_minimum_java_version_by_game，以及 command_generator.rs 的 generate_launch_command。
- 参考将 class_paths 与 args 分离，移除 -cp 后以进程 CLASSPATH 环境变量传递；自动选择同主版本，否则允许更高版本，并合并客户端/loader/客户端 JSON 的最低要求。mcl 不采用更高主版本回退，不采用 wrapper/脚本/Agent、完整含凭据命令导出或静默优先级失败。
- 比较三方案：直接完整 -cp 会遇到 Windows 32767 UTF-16 命令长度；CLASSPATH 环境有单值长度/继承注入边界；标准 manifest-only JAR 可把有序本地类路径放入百分号编码 file URL，不把令牌放磁盘。本轮选后者，仅 Java8 使用；Java9+ 保留原 JVM argfile。
- JAR 仅有 META-INF/MANIFEST.MF，无 Main-Class、类文件、Agent 或补丁。路径按 URL 编码，CRLF 与续行折叠不超过70字节；Class-Path 值限1MiB。Windows 上的 Java8 仅接本地盘绝对路径，UNC/设备路径明确拒绝；剩余真实参数按 os/exec 转义后的 UTF-16 长度检查，超限明确失败，不改 shell、不把游戏令牌改写到磁盘。

### 实现落点与精确范围

- app/java_selection.go：允许 Java8 进入既有真实探测/同主版本同架构稳定版选择及 Temurin 固定摘要安装链路。显式偏好只有兼容才优先；缺少对应官方包明确失败，不改默认路径。
- core/launch/plan.go、transport.go：记录实际 Java major；Java8 classpath 必须等于安装计划且唯一，支持文件不含会话 token，主类/游戏参数直接交 exec；Java9+ 的 JVMArgumentFile/ProcessArguments 分支保持原格式。Java8 游戏不接受更高 major 冒充兼容。
- platform/process/session.go、command_*、support_file.go：同一 StartTracked/register/gate 或 CREATE_SUSPENDED 交接。支持文件私有、非覆盖创建，游戏退出/创建或登记失败回收；按文件身份保留外部替换，不误删未创建的碰撞路径。文件随游戏存活保留，不能启动后立即删除尚可能被延迟读取的 classpath JAR。
- storage/sessions.go：持实例 OS 锁、核验旧会话已死后，再回收精确 jvm-<26位RFC4648 nonce>.args/.jar 普通文件；存活/身份未知/更新或删除pending不绕过，未知名称、目录与链接保留。
- 现有 ForgeInstaller 对处理器 Java 的 max(9, gameMajor) 和实际 Probe 校验保留，不将游戏 Java8 无条件交给现代参数文件处理器。Java8 Forge 处理器独立选型与旧格式仍是 C4，C1 不承诺其安装成功。Fabric/Quilt 仍依赖实际 profile/Java 要求及 native 资源，不按 loader 名称扩大兼容范围。
- 当前只覆盖现代 arguments、明确 Java8 要求、既有安装/native 规则均接受的元数据。minecraftArguments、缺 Java 要求、继承、历史资源、旧 Forge 与缺 ARM native 继续留在 C2–C4。1.16.5 是后续真实候选，不登记为已运行/验收。

### 构建与未验证

- 已执行 gofmt、CGO_ENABLED=0 ./scripts/go.sh build ./...；Windows amd64 与 Linux amd64 主程序交叉编译返回0；Go1.27.3、MyGo0.7.4，未升级依赖。
- CGO_ENABLED=0 ./scripts/go.sh tool mygo build -platform darwin/arm64 -skip-notarize 已完成编译/临时签名/DMG，交付 build/darwin-arm64/mcl-0.0.2-java8-preview-arm64.dmg（约8.5MB），未公证、未发布Release。
- 回归源码包含 Java8/17/21 传输与隐私、1000项Unicode/空格/百分号/特殊字符长类路径、边界/歧义拒绝、同主版本选择、Windows UTF-16/引号长度、受控交接失败、正常退出/创建失败清理、启动器交接前后退出、旧会话/文件替换保护。
- java8_integration_test.go 为显式 integration 源码，必须指定 MCL_INTEGRATION_JAVA8 的本机稳定同架构 Java8 JDK，才会编译自建探针，覆盖真实 JVM manifest 读取、Unicode/长类路径、参数脱敏与登记身份/重开保护。未执行此源码，未发现/下载/执行任何 Java 或游戏。
- 所有新增/修改回归均未运行，生产 build 也不代表测试源码已编译。Windows Java8 的系统编码、真实类加载行为、各平台退出/崩溃/竞争清理仍待授权验收；交叉编译不等于实机兼容。
- 用户指定的本 Plan 单独纳入版本控制以随逐项 commit 保存进度；其余原有私有文档与 .repo/.tools/build/凭据忽略规则保持，不批量公开。

## 19. C2 实施记录（2026-11-08）

### 参考与独立设计

- 参考提交仍为 d5b109e0abb42460a8fd665c58ca4b0cc9982182，实际静态阅读 launch/helpers/command_generator.rs 的 generate_launch_command：旧模板按空格拆分，补 JVM/native/品牌参数后替换；instance/helpers/client_json.rs 的 McClientInfo/reset_fields_from_patches：补丁库收集、主类/现代参数合并与旧参数替换；jre_selector.rs 的历史 Java 下限用于对照，不移植其任意更高 major 回退。
- mcl 不照搬字符串简单切空格、第三方 patches/OptiFine/脚本/注入。独立有界词法支持引号分组与有限反斜杠，先分词再替换玩家名、目录和令牌，不对替换值二次分词，不做 shell 展开。16KiB/256项；合并后最多4096库、2048个参数值/256KiB。无新依赖、无UI功能扩张。
- 官方规范化必须先核验 HTTPS Mojang piston-meta/launchermeta 精确主机、v1/packages/<SHA1>/<ID>.json 路径、内容SHA1、文档ID和类型。一般/原有缓存不能仅凭同名/同摘要晋升为官方来源；官方原文独立存 versions/mojang-v1，保留旧缓存，不把规范化后的JSON写回原文摘要键。允许下载适配注入但实际仍须匹配固定摘要。
- 缺失 javaVersion 时，仅官方已核验release且位于显式1.6.1–1.16.5已列出的正式版表内使用稳定Java8兼容策略；表不按前缀/字符串大小判断，不覆盖旧alpha/beta、快照、未知/第三方名称。显式无效/null要求不回退。这是mcl兼容决策，不声称每个旧游戏官方原始最低要求都是8，也不是实测兼容表。

### 实现闭环与限制

- core/meta/client.go/resolve.go/legacy.go：原文读取与内存规范化分离，所有调用现有VersionCatalog.Version的创建/导入/安装/准备链共用结果。原始 ParseVersion 允许有合法父ID的部分文档被解码，但 ValidateVanilla 不让未解析继承、旧参数或未知补丁直接进入安装。
- 父文档只从单次固定官方清单快照中选择，最多8个文档，检测循环；每层核验摘要/ID/来源。保留Sources内存证据，Java取父子更高要求，子主类/资产/日志显式覆盖，参数父→子追加，旧游戏模板替换。库按Maven模块组覆盖，同时保留组内全部条件变体和规则顺序；深拷贝避免父状态别名。显式null/空模板、未核验jar、未知patches、超限和缺失父文档失败。
- core/install/plan.go：ClientVersion跟随实际继承的客户端文件来源，不能把父JAR重命名为子版本冒充身份；子声明完整客户端则用子ID。显示/游戏版本仍是子ID。
- core/launch/plan.go：补auth_session（在线token:<token>:<uuid>、离线0），仍只交游戏参数/exec、沿用脱敏与C1支持文件隔离。旧日志允许-Dlog4j.configuration=，新旧日志均必须精确指向经过下载计划核验的本地文件，不能把远端地址或额外尾缀带进日志加载参数。旧默认JVM含macOS首线程规则和受控classpath/native目录。
- game_assets明确指出C3历史具名布局缺口，不错误映射为散列对象目录；virtual/map-to-resources仍未开放。既有native按架构/摘要保护不弱化，因此旧资源或ARM缺库的版本仍可能拒绝安装/启动。
- 继承型官方客户端目前只接原版；app/loader.go明确拒绝多文档Sources进入只固定单一base摘要的loader缓存，避免跨父文档变化误复用处理器代际。现有单文档官方base + 已支持Fabric/Quilt/现代Forge路径保持；更广第三方继承和loader代际放C4，不能把任意inheritsFrom当已兼容。
- 当前未登记任何新的真实MC/Java可运行组合。回归中的1.12.2/child等文档为从现有现代fixture构造的合成格式案例，不是下载的历史官方fixture，也不证明公共官方清单实际存在这些继承样本。

### 构建与验证状态

- gofmt、git diff --check；CGO_ENABLED=0 ./scripts/go.sh build ./...；Windows/Linux amd64主程序交叉编译均返回0。Go1.27.3/MyGo0.7.4，未变go.mod/go.sum。
- CGO_ENABLED=0 ./scripts/go.sh tool mygo build -platform darwin/arm64 -skip-notarize成功编译/临时签名/DMG；build/darwin-arm64/mcl-0.0.2-legacy-metadata-preview-arm64.dmg约8.5MB，未公证、未发布Release。
- 新增meta/legacy_test.go、resolve_test.go与launch/legacy_test.go，覆盖词法/Unicode/引用边界、官方来源/摘要/独立原文缓存、父链循环/深度/取消、null/未知补丁/客户端引用拒绝、条件变体/深拷贝、组合规模、旧在线/离线会话/令牌隐私、继承客户端安装和启动计划、旧日志。全部仅回归源码，未执行、未编译测试目标。
- 没有运行go test/vet/race、UI自动化、网络验收、Java或Minecraft；不将生产/交叉编译称为历史版本实机通过。下一项为C3。

## 20. C3 实施记录（2026-11-08）

### 参考与选择

- 固定参考 d5b109e0abb42460a8fd665c58ca4b0cc9982182，静态读取 launch/helpers/file_validator.rs 的 prepare_legacy_assets、get_native_library_artifacts/get_native_library_paths/extract_native_libraries，instance/helpers/asset_index.rs、command_generator.rs 的 game_assets/assets_root。参考按 legacy/pre-1.6 索引名复制到共享virtual/legacy及游戏resources，已存在则跳过；mcl不照搬按名称猜布局、无界复制或存在即成功。
- 采用实际索引的virtual/map_to_resources标记及游戏模板game_assets需求，仍只下载官方散列对象，不构造历史私有源、替换ARM库或下载安全/认证补丁。具名副本按实例隔离，不写共享virtual目录、无硬链接，规避不同实例/旧游戏修改互相影响。当前顺序复制（单项并发），不新增下载预算或后台动画。

### 完整链路与归属

- core/install/asset_plan.go：有界索引解析为共享Files和排序Mappings；摘要去重、同hash大小一致，拒绝穿越/绝对路径/Windows保留名/大小写和NFC别名（含父目录）/文件目录冲突。索引16MiB、100000项，具名对象单项512MiB、按具名路径累计16GiB；不放开原下载/解压边界。
- execute.go/asset_files.go：从同一FD重新验证索引的固定大小/摘要再解析，复制时校验实际流的SHA1、大小、FD与路径身份和修改时间。逐级打开真实目录，拒绝根内外链接重定向；复制不链接共享对象。另一个实例修复替换共享对象时，本次旧FD或完整校验副本可保留；身份变化则失败，不发布未校验内容。
- legacy_assets.go：virtual写中央 instances/<id>/legacy-assets/.prepare-<nonce>，内容位于files，成功非覆盖发布ready代际。Kind/实例/nonce/索引摘要owner标记有界读取；收据新增可选legacyAssetsDirectory，旧收据仍可读。每次准备新代际，保存成功才回收原收据代际；保存失败/取消回收本次代际，异常退出在下次持实例OS锁、确认无游戏后清理已标记遗留。无标记/未知标记/链接目录保留，不猜其可删。
- map_to_resources写真实游戏resources，内外位置均由入口注入AssetStore.OpenGame，不把外部卷离线变成内置fallback。私有同卷stage核验后逐文件MoveNoReplace；存在且大小/摘要相同才复用，内容不同或链接报错保留。不会整体清空resources，也不删除用户新增、已改或不在本索引的文件。
- 取消/普通失败/进程退出可留下已发布且校验成功的resources子集与空父目录，下次从固定索引重查/补齐，不声称整批原子事务；不删除已发布文件回滚。私有标记stage可在下次恢复清理，标记写成之前的异常遗留保留。资源冲突须由用户另行处理，不能自动覆盖“修复”。
- app.InstallationService沿SessionRegistry保留OS锁/存活/更新或删除pending保护，显式新构造器注入游戏根；虚拟代际经安装结果→收据→LaunchService选项→同实例目录校验→game_assets。assets_root仍共享散列索引/对象根，不用它冒充具名目录。任务增加真实“准备历史资源”阶段，未改布局。

### native边界

- 旧classifier按windows/linux/osx/macos与32/64/x86/x64/amd64/x86_64/arm64/aarch64判断。无CPU后缀的旧包只作为x86双位数包，不能推断ARM；缺库/不匹配明确失败。已有现代classifier架构别名也明确处理，不猜未知平台。
- 排除项最多128、验证安全相对前缀；保留META-INF排除。已验证相同归档/相同排除项去重，不同排除项冲突报错。解压同一FD前后验证归档摘要/大小/身份，ZIP CRC、链接/穿越/Unicode文件别名、10000项/128MiB归档/64MiB单项/256MiB展开边界保留。暂存目录句柄锚定，发布MoveNoReplace，现代native仍走原classPath。
- 新native代际也写kind/parent/nonce/归档计划摘要标记；自动回收只处理匹配标记目录。旧无标记代际不批量扫删；只有原收据明确引用的旧代际才在成功替换后走已有精确删除。不会凭“ready-”外形删除未知用户目录。

### 构建与待验收

- 受影响Go文件gofmt、git diff --check；CGO_ENABLED=0 ./scripts/go.sh build ./...；Windows/Linux amd64主程序交叉编译返回0。Go1.27.3/MyGo0.7.4，无依赖升级。
- CGO_ENABLED=0 ./scripts/go.sh tool mygo build -platform darwin/arm64 -skip-notarize成功编译/临时签名/DMG；交付build/darwin-arm64/mcl-0.0.2-legacy-assets-preview-arm64.dmg约8.5MB，app约18.8MB，未公证、未发布Release。
- 新增asset_plan_test.go、legacy_assets_test.go、legacy_execute_test.go并补natives/plan/launch回归，覆盖旗标/去重/路径别名、实际副本/共享隔离/冲突保留、取消重试/遗留标记、链接拒绝、摘要或路径替换、离线缓存执行链、game_assets路由和native架构/排除项。全部未执行、未编译测试目标；格式整理和生产build不等于回归通过。
- 未运行测试/vet/race、Java/游戏、公网/原生/文件系统自动化或用户目录操作。未增加任何真实历史MC/Java/OS组合“已验收”证据。pre-1.6缺摘要/旧元数据及缺ARM native等边界仍拒绝；不宣称所有历史整合包可运行。下一项C4。

## 21. C4a 首批旧 Forge 格式（2026-11-08）

### 代际矩阵（实现不等于验收）

| 格式/运行条件 | 本轮状态 | 明确边界 |
| --- | --- | --- |
| Forge 1.12.2 install/versionInfo、内嵌Universal、无processors | C4a已实现/构建，未验证 | 仅两个明确版本ID形式、匹配父ID/坐标、LaunchWrapper 1.12/FMLTweaker、固定依赖身份；缺摘要失败 |
| 同为1.12.2但新版spec1/version.json且minecraftArguments/LaunchWrapper | C4b已实现/构建，未验证 | 仅现有允许的处理器/任务、唯一匹配核心和LaunchWrapper1.12；不凭游戏版本猜安装器格式 |
| 现代spec1 + arguments + 已有受支持processor | C4b接独立工具Java | 工具按固定JAR头部下限选择同架构稳定JVM，不改变游戏major；未知/预览/超范围工具拒绝 |
| 1.7.10 install/versionInfo、内嵌Universal、无processors | C4c1已实现/构建，未验证 | 固定cpw FMLTweaker、LaunchWrapper1.11/1.12唯一入口；不能混成现代旧模板 |
| 1.8.9等中间代际及更早旧安装器/未知工具 | 待C4c2及后续 | 不从版本范围推断Tweaker/入口，不能泛化为全部旧Forge |
| NeoForge、其他loader或未知Tweaker伪装旧格式 | 拒绝 | 不借旧标记绕过现代校验 |
| 多文档官方继承base叠加loader | 仍待后续身份设计 | C2的单base摘要缓存守卫保留 |

### 参考、取舍和链路

- 静态阅读固定参考d5b109e0的instance/helpers/loader/forge.rs：现代version.json与旧install/versionInfo分支、install.path/filePath、旧模板替换；postprocess.rs/common.rs的准备/处理阶段。参考旧格式可先按name/url下载而缺固定大小/SHA；mcl不采用未固定下载、不复制参考Maven替换/覆盖策略或执行任意旧工具。
- core/loader/forge_install.go先固定官方安装包SHA1/大小并复制私有stage，再识别格式；无processors旧分支不要求Java9+、不Probe/执行Java。现代分支仍保留真实Probe、主版本最低要求和ProcessorRunner，不简单删除Java校验。
- forge_legacy.go只接受Forge1.12.2、单文档Java8官方base、匹配版本ID/父ID/install坐标、无新旧混合字段、无覆盖客户端/资源/patches。内嵌Universal从固定安装包安全提取，核对FMLTweaker类条目；LaunchWrapper必须为1.12且包含Launch入口。条目存在只是格式检查，不把它当字节码运行验收。
- 库最多128项，只接JAR、不替换native。内嵌文件摘要继承安装包信任并核算实际大小/SHA；额外artifact声明必须一致，旧checksums列表最多16条且实际摘要必须匹配其中一条，不能用后来读取的Maven身份静默绕过原清单约束。其他库优先使用完整声明或同坐标官方base的固定身份，否则仅从明确官方/公共Maven根读取.sha1和HEAD大小再下载。只有Forge自有旧files.minecraftforge.net/maven根可规范化为https://maven.minecraftforge.net；无摘要/大小、未知仓库、坐标/路径冲突即失败，禁止猜CDN或无校验兜底。
- 独立运行库代际仍私有stage、逐文件校验和非覆盖发布，原游戏目录/来源记录不改。Profile新增可选Legacy标记（false省略，原现代缓存字段指纹不变），严格校验旧代际/main/Java8/Universal/LaunchWrapper与完整游戏模板。旧模板替换原版game参数而不是重复追加，保留原版JVM/native、客户端/assets与C1私有传输。只允许已知身份/路径占位符和唯一固定FMLTweaker，不接Agent、自定义Tweaker或脚本。
- LoaderStore沿原有指纹/VerifyRuntime校验实际代际，缺失/损坏重建，不以缓存标记冒充完整。安装收据原本只接受Fabric/Quilt导致Forge/NeoForge成功产物也无法保存，本轮修正为四类已支持loader身份，仍要求固定版本/64字符十六进制指纹和实例/平台匹配。
- 旧Java8 Forge整合包确认沿既有Warnings显示“安装器将在导入后核验；文件可导入不代表游戏已可运行”。Imported/RuntimeReady与真实游戏会话继续分开，失败保留导入实例可续装，实际启动仍走Prepared.Start/StartTracked，不放低级exec捷径。

### 构建与未验证

- gofmt、git diff --check；CGO_ENABLED=0 ./scripts/go.sh build ./...及Windows/Linux amd64主程序交叉编译返回0。Go1.27.3/MyGo0.7.4，未升级依赖。
- CGO_ENABLED=0 ./scripts/go.sh tool mygo build -platform darwin/arm64 -skip-notarize成功编译/临时签名/DMG；build/darwin-arm64/mcl-0.0.2-legacy-forge-preview-arm64.dmg约8.6MB，app约18.8MB，未公证、未发布Release。
- forge_legacy_test.go为合成JSON/JAR/内存HTTP适配源码：安装器完整无Java分支、固定文件身份/代际、缓存序列化/损坏、旧模板到私有启动计划、未知格式/父ID/坐标/Tweaker/URL拒绝、现代格式隔离和取消。receipt_test.go补四类loader收据匹配。全部未执行、未编译测试目标。
- 用例中的14.23.5.2860字符串和1.12.2仅标识合成结构；安装包字节/摘要/类内容都是合成，不是该官方构建的真实样本。没有下载或运行真实Forge，不能宣称该构建实际使用旧格式、所有1.12.2已支持或已进入游戏。
- 未运行test/vet/race、Java/处理器/游戏、公网/原生自动化/实机验收。C4整体仍未完成，下一项C4b继续处理器Java分离和现代安装器的旧参数；不跳到D。


## 22. C4b 独立工具 Java 与现代安装器旧游戏模板（2026-11-08）

### 参考与分层

- 固定参考仍为d5b109e0abb42460a8fd665c58ca4b0cc9982182，静态复核instance/helpers/loader/common.rs的execute_processors：参考复用游戏配置/客户端java_version选择执行环境。本轮不照搬此耦合、GPL代码、任意仓库/脚本或隐式依赖；继续独立实现Go，未升级依赖。
- core/loader注入ProcessorJavaResolver；app.JavaService.EnsureProcessor与游戏Ensure分开，main注入同一JavaService的库存、串行受管安装器、网络/并发/缓存预算。工具环境只在实际Java处理器执行前按需准备；C4a无处理器分支和纯受控映射复制不探测/安装工具Java。
- 游戏仍按原官方/合并配置的精确主版本选择，不因工具需要17/21/25而改变Java8游戏、实例Java偏好或全局默认。工具可使用兼容显式提示，否则选择已装稳定同架构环境中最低适用major/最新补丁；缺少时固定Temurin稳定LTS17/21/25中满足要求的最小major，不以工具策略替换游戏策略。安装失败/取消保持错误，不伪装回退。

### 固定工具与模板边界

- 每个处理器及显式classpath先核验固定JAR大小/SHA1，同一FD扫描前后再验证完整摘要、路径/FD身份、修改时间；复用经过路径/链接/别名/体积校验的ZIP索引，不重复构造完整ZIP目录。archive.ReadPrefix只读8字节class头，不把未完整读取条目的CRC当作校验依据；完整JAR独立摘要才是边界。
- 全次准备最多50万条class头；保留512MiB/JAR、5万项/ZIP与4GiB展开约束。分析magic/major/minor，工具使用9–25且稳定/同架构（9下限因既有工具参数文件），拒绝预览与超出25的根class。multi-release版本9–25的class不得高于目录版本，未来版本目录不抬高根级下限且不会被当前工具JVM选中。类头下限只是保守选择依据，不是字节码/API/反射或实际运行兼容性证明。
- 主类必须与固定JAR清单一致且有根级class；工具及classpath依赖的非空Manifest Class-Path直接拒绝，不能在已固定classpath外隐式加载本地或远端代码。保留处理器/任务允许列表、参数/路径和输出身份、受控私有work目录与平台子进程取消/单项期限；未增加Agent、脚本或自定义工具入口。
- 安装处理器前重新校验所有classpath，避免自动Java准备期间的文件变化被忽略。取消/工具失败不发布运行库代际；已完成的受管Java可复用，不删除共享环境或旧代际。删除整段25分钟墙钟限制，下载继续使用主动网络预算，限速等待/自动Java安装不被累计误杀；真实子进程仍由生产Runner单项限时。
- spec1/version.json的旧模板只接Forge1.12.2、匹配ID/父ID、Java8、LaunchWrapper1.12及唯一匹配Forge核心（含受控client生成分类）、固定FMLTweaker与完整受限minecraftArguments。不接自定义主类/Tweaker、客户端/资源/patches覆盖或新旧模板混合。未知processor sides不能静默过滤为成功，server用途仍按原规则跳过。
- Profile新增omitempty的LegacyGame，区分现代处理器+旧游戏模板与C4a无处理器Legacy，不能互相冒充；默认false不改变既有现代缓存指纹。生成入口JAR必须包含FMLTweaker/Launch条目，完整游戏模板替换原版game参数，保留原版JVM/client/assets/native及C1 Java8传输，缓存/收据/真实启动继续沿原锁与StartTracked。
- 仅沿已有任务阶段显示查找/获取/下载/检查工具Java，不新增说明面板。没有扩展未知处理器任务、1.7.10、继承型base叠加loader或代际回收；C4整体未完成，下一项C4c。

### 构建与未验证

- 受影响Go文件gofmt、git diff --check；CGO_ENABLED=0 ./scripts/go.sh build ./...、Windows/Linux amd64主程序交叉编译均返回0。Go1.27.3/MyGo0.7.4，无依赖更新。
- CGO_ENABLED=0 ./scripts/go.sh tool mygo build -platform darwin/arm64 -skip-notarize完成编译/临时签名/DMG，build/darwin-arm64/mcl-0.0.2-processor-java-preview-arm64.dmg约8.6MB、app约18.8MB，未公证、未发Release。
- 新增app/processor_java_test.go、loader/processor_java_test.go、forge_legacy_game_test.go源码，涵盖游戏/工具major隔离、兼容偏好/补丁/EA/架构、缺少时的官方LTS策略、库存失败/取消、固定JAR摘要/头部/扫描预算、multi-release/清单隐式依赖拒绝、现代旧模板生成/缓存/合并与工具失败不发布。合成class仅8字节头且不可执行，模拟Runner只写合成ZIP；14.23.5.2860不是实际官方格式/运行证据。
- 所有新回归未执行、未编译为测试目标；生产build不等于测试源码通过。未运行test/vet/race、任何Java/处理器/游戏、公网/原生自动化或用户目录操作；没有增加真实可运行组合或实际下载证据。


## 23. C4c1：1.7.10 无处理器旧 Forge（2026-11-08）

### 本批行为与参考

- 继续静态阅读固定d5b109e0参考forge.rs旧LegacyInstallProfile分支的install.path/file_path、旧游戏模板与库合并；参考泛化入口/缺少依赖摘要不照搬。独立Go按明确游戏代际选择策略，不开放任意旧主类或推断整个版本范围。
- 新增1.7.10 install/versionInfo无processors格式：单文档官方Java8基底、固定官方安装器SHA1/大小、匹配带完整分支的Forge核心坐标与两个明确ID形式、内嵌Universal。Tweaker只许cpw.mods.fml.common.launcher.FMLTweaker，核对cpw/...class条目；唯一LaunchWrapper只接1.11或1.12，核对Launch条目，不混用1.12.2的net.minecraftforge命名空间。
- 仅此旧代际允许省略inheritsFrom：必须原字段确实不存在，并且install.minecraft与versionInfo.jar同时精确指向当前base。显式null/空/其他父ID不回退；不凭实例名、版本前缀、文件名或ID猜身份。其余客户端/资产/patches/Java/模板限制沿C4a，未知代际明确失败。
- Profile.Legacy复用原有校验/指纹/独立代际，游戏Java8完整模板替换原版game参数，保留官方JVM/client/assets/native与C1私有无凭据支持JAR；不Probe、选择、安装或运行处理器Java。导入/创建/修复/启动沿原LoaderService、安装收据、SessionRegistry和StartTracked，不新增旁路或配置schema。
- 旧核心和Wrapper不能条件省略、标服务端专用或夹带另一个版本/分类抢占classpath；旧缓存及C4b现代旧模板也施加相应冲突校验。现代LegacyGame仍只接1.12.2，不因共享策略扩展而误放开1.7.10 spec1或未知处理器。
- 依赖仍先固定摘要与大小；单项512MiB、累计4GiB，checksums约束不弱化。仅精确历史公共仓库根允许规范化HTTPS（Mojang、Forge、Maven Central/maven2），不改写任意artifact URL，不接受私有主机/凭据/查询/额外路径、不绕分发限制；原Maven Central根遗漏maven2路径已修正。
- Forge目录保留已由官方列出的完整-game分支，数字build的该后缀不再误标预发布；推荐信息可装饰同build/同game的已列出分支，不凭promotion构造下载坐标。未知qualifier仍非稳定，版本身份不删分支，创建前/安装器内继续复核。

### 构建与未验收

- 受影响Go文件gofmt、git diff --check；CGO_ENABLED=0 ./scripts/go.sh build ./...及Windows/Linux amd64主程序交叉编译返回0。Go1.27.3/MyGo0.7.4，无依赖变更。
- CGO_ENABLED=0 ./scripts/go.sh tool mygo build -platform darwin/arm64 -skip-notarize完成编译/临时签名/DMG；build/darwin-arm64/mcl-0.0.2-forge-1.7.10-preview-arm64.dmg约8.6MB、app约18.8MB，未公证、未发布Release。
- 新增forge_legacy_17_test.go/forge_releases_test.go，调整原有单个参数验证调用；源码覆盖两个Wrapper、显式/缺省父绑定、无工具Java完整准备/缓存/合并/私有启动计划、跨代Tweaker/入口/分支和条件冲突、null/缺失归属/现代语义拒绝、公共根精确升级及目录分支/推荐保留。所有回归未执行、未编译测试目标。
- 10.13.4.1614-1.7.10仅合成标签，安装包/JAR/类内容均非该官方构建字节，不证明其实际格式、完整安装或可运行。未运行任何Java/处理器/游戏、测试/vet/race、公网/原生自动化或用户目录操作。旧ARM native缺失仍报错，不借x86/更高Java冒充兼容。
- C4整体未完成。下一项C4c2按明确格式继续1.8.9等中间旧代际，再补剩余processor/完整继承基底身份；目前保留多文档base叠加loader守卫与未知工具错误，不跳D。
