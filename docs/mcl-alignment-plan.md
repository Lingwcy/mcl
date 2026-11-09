# mcl 能力补齐与参考对齐 Plan

日期：2026-11-09
状态：C1–C4、D1–D3已按明确范围交付，Swift反馈修正与v0.0.4发布见第28节/本地维护记录。T1–T4实现与构建统一收尾见第29节；新增回归、原生交互、断网/崩溃及真实游戏验收均待用户统一执行。I尚未开始。
主要参考：SJMCL，基线 d5b109e0abb42460a8fd665c58ca4b0cc9982182。

## 1. 目标与现状

目标不是页面数量或同名按钮一致，而是达到相应的实际功能深度、版本/平台覆盖和失败处理能力，同时保留 mcl 原生、紧凑、少打扰的体验。

当前已经具备现代版本安装/修复、Java 管理和自动选择、微软/离线账户、四种加载器的现代路径、双源市场、三类整合包导入、多安装位置、动态自动内存、日志及实例全量删除等实际链路。不要把这些已有能力重新当作空白模块建设。

用户已对近期导入/删除体验包反馈“我已验证”。这属于用户体验验证记录；没有提供详细版本、平台和异常场景清单，不扩展为所有 Minecraft/整合包、全平台或所有恢复路径均已通过。

最大的不足是：

1. Java8、历史资源、旧Forge与完整继承身份已接实现；仍缺真实版本/异常场景验收。范围外旧式客户端改写、未知运行组件和缺失native不能宣称可运行。
2. 已有目录现可只读发现并确认复制为受管实例，可恢复迁移已有 mcl 载荷；不原地接管他人目录、不承诺直接复用外部运行库或所有来源可运行。D行为尚待验收。
3. 下载现接统一持久任务、暂停排空、范围续传与重开恢复；行为尚未验收，未知凭据URL和已改变的目标仍需重新确认。
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

- C1–C4的实现/构建闭环统一完成，当前范围/安全差异/待验收见第24节；第18–23节保留各历史时点。用户要求C一次交付，本次不再拆C4c2等批次，不自动开始D。
- D 可先只发现现代受支持版本，不为等待全部历史兼容而阻塞只读扫描。
- T 应复用现有下载控制/缓存/摘要保护，不能另建一套不共享预算的下载器。
- I 的资源迁移、loader 变更和 A 的批量更新依赖现有实例锁及恢复机制；新增事务不能绕过删除/更新 pending 标记。
- P 中关于/许可等小项可以独立完成，但不能用这些容易完成的页面替代 P0 底层差距。

实施记录持续更新，交付说明真实行为、影响范围和未验证项。用户2026-11-08最新要求C整体连续完成后统一汇报，覆盖此前C分批汇报安排；后续阶段仍按用户决定推进。具体参考文件、调用链、取舍和 mcl 落点见第 17 节；不能只看本节功能名称就开始补按钮。

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

状态：已研究、已设计、已实现、已构建/交付；回归与真实历史游戏验证未运行。固定官方继承链已能叠加四类已支持loader，缓存绑定完整来源和处理器平台；未知第三方补丁仍拒绝，见第19/24节。

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

状态：第24节范围内实现/构建统一收尾；旧Forge明确代际、spec1处理器、独立工具Java、完整继承身份均贯通。运行验收未执行，不能称所有历史Forge/所有平台已兼容。第21–23节是早期批次记录，不作为当前剩余开发项。

- 按安装器/配置代际列出支持范围，逐类补旧 Forge、缺失 processor/客户端后处理。
- 固定官方安装器与依赖身份；工具执行有界、可取消、使用经过检查的 Java。
- 正确处理安装器内文件、运行库合成和启动配置，不把无法解释的包当原版。
- 为导入页提供可解释的兼容结论，区分“格式已识别”“文件可导入”“环境已安装”和“游戏已运行”。

候选验收版本：先选一个 Java 8 且现代参数格式的原版（例如 1.16.5），再选一个旧参数格式的原版（例如 1.12.2），随后选择固定版本的对应 Forge 包。以上是计划样本，不是支持或通过声明。更早版本后续逐代扩展，不能用这两个样本代表全历史覆盖。

## 5. 第二阶段 D：接入已有游戏目录

### D1. 只读发现

状态：已研究参考并接入有界只读发现、独立来源登记与原生工作区；仅构建，未运行扫描/测试或实机验收，范围见第26节。

- 显式添加官方或第三方 Minecraft 根目录，发现 versions 下的候选实例。
- 显示实际位置、版本、loader、兼容/缺失信息和来源；有界扫描、可取消、路径去重。
- 处理重复名称/ID、父子目录重叠、符号链接、外部卷离线和损坏文档。
- 发现不得自动修改、修复、删除、补文件或接管其他启动器账户/配置。

### D2. 确认后导入/受管复制

状态：已实现固定来源/文件预览、明确资料位置与分类、官方环境复核、非覆盖受管复制及恢复/撤销。源目录保留，运行环境另由原安装/启动链准备；仅构建，见第27节。原地管理未实现且不提供入口。

- 先定义“仅发现”“复制为 mcl 受管实例”“原地管理”三种语义；第一批优先受管复制，原地管理未实现时明确禁用，不能把它伪装成已有功能。
- 展示将复制的存档、模组、配置和资源，默认保留源目录。
- 只导入允许的游戏资料，不执行其他启动器脚本、不继承认证凭据或任意 JVM/Agent 参数。
- 保留稳定实例身份、文件校验、非覆盖发布和占用保护。

### D3. mcl 实例迁移

状态：已实现内置↔外部及外部↔外部的复制/核验/提交/保留旧副本、持久恢复及明确清理，真实会话/OS锁和删除/资产pending保护保持；仅构建，见第27节。

- 支持内置/外部位置间迁移，区分实例载荷与共享 libraries/assets/Java；移动游戏不等于移动应用数据根。
- 跨卷使用复制、核验、提交、清理的恢复流程，不用跨卷 rename 假装原子事务。
- 取消、源变化、目标冲突、空间不足或断电时可解释地恢复，不删除唯一有效副本。
- 删除行为跟随真实归属：原地发现/非受管资料不能套用“删除整个 mcl 实例目录”的逻辑。

## 6. 第三阶段 T：统一任务、暂停与续传

状态：T1–T4已按第29节范围实现并构建，等待用户统一验收；不把构建或回归源码当作已验证。

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

C4当前状态（统一交付；历史分批记录见第21–23节）：

- [x] C4a：识别1.12.2旧install/versionInfo无processors格式，内嵌Universal及依赖固定身份、LaunchWrapper完整模板替换、独立运行库发布/缓存校验/安装收据/启动计划贯通。已构建，未运行回归或真实游戏。
- [x] 游戏Java与工具Java独立解析；工具8使用私有清单classpath，9+参数文件，缺失复用官方稳定安装；不改变游戏major。
- [x] 旧Forge明确1.6/1.7/1.8–1.12代际、匹配FML/Wrapper/完整分支，spec0和spec1旧模板受限规范化，不放行未知入口。
- [x] 客户端工具任务：MCP/提取/目录/Bundler主JAR原生处理、官方映射固定下载及离线规范化、其他已允许的Java转换/修补工具，顺序输入/输出和取消发布保护。
- [x] 完整官方继承链摘要纳入loader请求/缓存；实际父客户端路径保留；处理器代际按OS/CPU隔离、条件库组保留、平台资源前置检查。
- [x] 补回归源码、四目标主程序编译及统一macOS arm64体验包，更新Plan后统一提交；未执行测试目标。
- [ ] 授权后执行各代际固定官方fixture、依赖缺失/摘要错误/取消恢复、实际Java/游戏/平台验收；无验证证据不勾通过。

C阶段实现矩阵见第24节，用户1.6.1反馈修复见第25节；D1已开始并接只读发现，见第26节。下一项D2受管复制，不能把D1候选当已导入或取代C的待授权验收。

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

### 历史代际矩阵（当时状态；当前以第24节为准，实现不等于验收）

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


## 24. C 阶段统一收尾（2026-11-09，最新用户决定）

用户明确要求“把C一次完成，不要分批汇报，不会就参考仓库”，随后再次“继续”。本次连续补齐C范围内剩余代码后统一交付，不再把1.8.9、工具任务和继承身份拆成下一次C批次。**实现/构建收尾与验收分开**：遵守不运行测试、Java、安装器、游戏或真实目录操作的偏好，验收复选框仍不勾选。

### 参考和独立实现证据

- 主参考SJMCL仍固定d5b109e0abb42460a8fd665c58ca4b0cc9982182：loader/forge.rs、neoforge.rs、common.rs、postprocess.rs，launch/helpers/jre_selector.rs、command_generator.rs；读取安装格式、内嵌Maven/补丁、变量、处理器顺序和Java选择。辅助参考b-launcher固定948c5e0eca5551167960a0acce7cf97308c45a0f的core/installer/loader_forge.cpp：旧/新格式、临时资源、输出与映射处理。两仓库只读，不作为构建依赖。
- 为核对参考未细分的任务语义，另只读官方源码：MinecraftForge/InstallerTools固定4928f1f18b934be9ca32aa8ae78d868bbfad4033，Tasks、McpData、ExtractFiles、CreateDirectory/Parents、BundlerExtract、DownloadMojmaps、Chain/MergeMappings；SrgUtils固定3c679e5388d35debfd56a735e1bc23f89b99d449，InternalUtils的ProGuard类型/描述符和MappingFile的TSRG格式/排序。公开源码读取不等于公网资源安装验收；GitHub API曾403，直接读取公开raw源码，不用凭据或私人代理。
- 研究格式与行为后以Go独立实现，没有复制GPL/LGPL源码、品牌、Key、私有服务或生成Java辅助类。无新依赖、无Go/MyGo升级、无参考仓库修改。

### 当前实现覆盖矩阵

| 面 | 当前实现 | 拒绝/差异边界 |
| --- | --- | --- |
| 游戏Java | 明确官方要求、稳定同major同架构；Java8独立传输，现代保留参数文件 | 不用高major/EA/错误CPU冒充游戏兼容；官方无对应包报错 |
| 历史官方元数据 | 固定包SHA1/ID/主机后规范化；明确release表补Java8，表扩至列明1.0–1.16.5正式版；继承8层有界 | 不按前缀猜快照/alpha/beta；缺固定下载/索引、未知patch/applet改写或第三方组件不冒充支持 |
| 历史资源/native | 既有virtual/map_to_resources/game_assets、真实受管游戏目录、条件native/CPU/排除项/摘要/ZIP边界 | 没有ARM库明确失败；不下载自定义补丁、不把x86“64”当ARM64；resources冲突保留 |
| Forge旧格式 | install/versionInfo（无spec或明确spec0），1.6.1/1.6.2/1.6.4/1.7.2/1.7.10使用cpw FMLTweaker；列明1.8–1.12.2正式版使用net.minecraftforge FMLTweaker | 唯一固定核心/Wrapper和类条目；1.6接明确1.3/1.8/1.12 Wrapper，1.7.2接1.8/1.9/1.11/1.12，1.7.10接1.11/1.12，后续接1.12；不推断任意版本 |
| 旧模板身份 | 1.6使用auth_session，后续使用uuid/accessToken；完整模板替换；支持固定userProperties和具名assets占位符 | 不允许两套身份混合、额外Tweaker/Agent/脚本。缺父字段可由固定install.minecraft+检查ID/核心绑定；null/空/其他父ID仍失败 |
| spec1 Forge/NeoForge | 现代参数，或符合对应明确代际的旧LaunchWrapper模板；固定安装包/依赖、内嵌Maven/资源、client.lzma默认BINPATCH | 服务端步骤过滤但未知sides拒绝；未知processor坐标/任务/输出拒绝；不能把失败结果当原版或RuntimeReady |
| 工具Java | 每个实际Java步骤按固定classpath的class头保守下限独立选择；旧class8固定Java8；9–25选择稳定适用环境，缺少用官方LTS17/21/25 | 游戏设置不变；Java8工具用标准manifest-only classpath，不执行辅助类；class头不是API/实际运行证明；预览/未来class失败 |
| 客户端文件任务 | 原生MCP_DATA单文件/有界目录、EXTRACT_FILES/optional、CREATE_DIR/PARENTS、Bundler1.0唯一主JAR；每文件固定来源/散列记录 | 只写本次私有stage；链接/别名/穿越拒绝；不执行EXTRACT_FILES的execs或服务端全量目录行为；不按存在即成功 |
| 映射/转换/修补 | DOWNLOAD_MOJMAPS经共享下载校验官方固定映射，本地ProGuard→TSRG/TSRG2规范化并核对安装器输出摘要；受控Java处理MCP映射链/继承/重命名/修补/拆分/CSV | 不让下载工具自行请求可变清单/绕下载策略；格式/摘要不符明确失败；其他插件/脚本仍拒绝 |
| 完整基底与平台 | Sources逐项ID/来源/顺序摘要→BaseChainSHA256→缓存key/指纹；Forge代际另绑OS/CPU；保留实际ClientVersion文件路径 | 父元数据变更、来源替换或跨平台不能误复用旧代际；单文档可选新字段省略不破坏原JSON读取 |
| 组合/生命周期 | 条件库同文件各rules保留，不混不同版本；加载前平台资源预检；已有实例/OS锁、pending、收据及StartTracked | 不绕会话锁、不改共享输入、不覆盖已发布代际；取消失败只撤销私有stage，旧代际和共享Java保留 |

### 关键实现与安全边界

- meta/source_identity.go只消费内部resolver的Sources证据；校验最多8项、官方包路径/摘要、末端与实例叶摘要和ClientVersion归属。loader.Request新增可选完整链/TargetOS/TargetArch，app.LoaderService不再硬拒多文档官方基底，Merge/Forge准备再次核验。客户端工具输入沿install.Build一致的真实父JAR路径，不把父文件重命名伪装子版本。
- forge_legacy_policy/version.go按显式代际处理FML/Wrapper和历史ID形式；省略父字段由固定安装器绑定，显式无效字段不回退。旧/新标记仍分开，固定内嵌入口只检查结构与摘要，不将class条目当运行验证。
- forgeData支持有界16层变量依赖、循环拒绝、引号标量、固定Maven坐标和内嵌资源；文件变量保持类型，只能从受管路径起点展开。未知标量/路径/URL/递归插值失败，跨步骤输出大小写/Unicode别名提前拒绝。
- 原生文件任务在同FD上校验输入归档前后固定摘要/身份和mtime，复用既有安全ZIP解压；MCP目录最多4096文件、每项摘要后进入known，后续目录输入遍历最多5万项且拒绝未知文件/链接。Bundler额外校验versions.list唯一性和主JAR SHA256。输出不离开libraries/私有work，目录任务无文件产物也不伪造进度。
- 顺序执行固定输入再处理输出：只有完整已知来源且声明摘要一致的输出才可复用；重写/原地转换仅限本次私有stage，既有未知文件保留并失败。已发布代际及共享缓存不作为可覆盖目标。累积4GiB、单项512MiB、50万class和原ZIP边界继续约束；变更不会放宽游戏包解压上限。
- ProGuard转换有界64MiB/100万行/75万成员/5万类，处理数组/构造方法/行号、JavaUTF16排序与已知库代际字段排序，输出最多128MiB，只有固定输出SHA1匹配才落盘；不是为任意映射格式猜测输出。工具清单Class-Path仅允许已经显式固定的相对依赖；外部URL/未知文件/跨根引用拒绝。
- 工具Runner重新Probe真实环境后选Java8/现代传输，私有支持文件、命令长度检查、有限环境、4分钟实际子进程期限和进程树取消保持。游戏仍经Prepared.Start/StartTracked登记后放行，安装工具不注册为游戏、不改游戏major或默认偏好。
- 没有增加UI占位页或兼容开关，任务沿已有阶段/取消/错误Toast；导入仍区分文件已导入、RuntimeReady和实际游戏会话，不因解包成功声称可运行。

### 构建证据、未验证与交付

- gofmt（仅变更Go文件）、git diff --check；CGO_ENABLED=0 ./scripts/go.sh build ./...返回0。Windows amd64、Linux amd64、macOS amd64主程序交叉编译均成功；不代表对应平台可运行。
- CGO_ENABLED=0 ./scripts/go.sh tool mygo build -platform darwin/arm64 -skip-notarize返回0，临时签名app约18.9MB、统一体验包build/darwin-arm64/mcl-0.0.2-c-compatibility-preview-arm64.dmg约8.5MB；未公证、未新建标签或Release。
- 新增base_identity_test.go、compatibility_matrix_test.go、conditional_libraries_test.go、forge_complete_chain_test.go、forge_native_tasks_test.go、forge_directory_tasks_test.go、processor_compatibility_test.go等合成回归源码并调整原有边界；覆盖完整父链/跨平台缓存、早期旧模板矩阵、条件库、原生数据/路径/未知输入、规范化摘要和工具Java8/固定清单。全部未运行、未编译测试目标；生产build不等于测试通过。
- 未执行test/vet/race、Java探测、任何处理器/安装器/游戏、真实目录变更、UI自动化或资源公网安装验收。只有只读参考/官方源码访问；不编造真实Forge版本、游戏主菜单或异常恢复通过记录。
- C的上述实现范围统一收尾，验证债务完整保留。范围外老式客户端改写/缺摘要资源、未知Tweaker/插件/脚本、缺CPU native、未来格式与服务端安装语义继续明确不支持，不将这些包装成“所有历史版本已兼容”。D–P不在本次执行。


## 25. 用户1.6.1截图：OS version缺失修复（2026-11-09）

- 用户实际体验1.6.1时，安装任务报“客户端平台兼容性: library org.lwjgl.lwjgl:lwjgl:2.9.0: OS version is required to evaluate this rule”。这是用户提供的失败证据，不是成功运行验收；截图未确认OS/CPU/Java，不填造具体兼容组合。
- 根因是入口将meta.CurrentPlatform()直接交给LibraryService与InstallationService。它只含编译OS/CPU，Version始终为空；旧LWJGL的os.version正则在原版前置检查遇到空值后按设计拒绝。规则检查本身不能跳过，不能写死10.0、改成“缺省放行”或把这次错误解释成用户Java/下载损坏。
- 新增platform.HostPlatform与host_darwin/windows/linux适配，应用依赖组装时读取一次并注入同一快照：macOS用kern.osproductversion（产品版本，不是Darwin内核），Windows用RtlGetNtVersionNumbers的major.minor（不受GetVersionEx清单兼容值影响、不使用营销版本名/构建号），Linux用uname release（不是发行版VERSION_ID）。沿安装/修复/启动准备/参数选择的现有平台值传递，不在UI渲染或核心规则解析时探测，不执行Java/shell。
- 系统版本读取失败不伪造；保留OS/CPU供不依赖版本的现代功能使用，日志保留真实探测错误。版本敏感规则仍明确失败，错误改为中文“无法判断系统版本规则：未读取到当前操作系统版本”。无新依赖、无配置/实例/缓存格式变更，不要求删除实例或重下载完整游戏。
- 新增platform/host_test.go（注入探测函数的纯值测试）、meta/os_version_test.go和install/os_version_test.go（旧LWJGL规则结构与计划传递）。覆盖三平台值、旧macOS匹配/非匹配、不同OS短路、读取失败/缺版本仍拒绝；均仅合成回归源码，未执行或编译测试目标。
- gofmt与git diff --check；生产build、Windows/Linux/macOS amd64主程序交叉编译、MyGo darwin/arm64编译/临时签名/DMG均成功。体验包build/darwin-arm64/mcl-0.0.2-os-version-fix-arm64.dmg约8.5MB；Windows单文件主程序build/mcl-os-version-fix-windows-amd64.exe约27MB，不是NSIS安装包。
- 没有运行测试/vet/race、原生系统版本探测、Java/处理器/游戏或用户真实目录操作，不把编译当作1.6.1已启动成功。用户替换修正版后可在原实例“再次校验/重试”；后续实际缺CPU native等错误仍需按真实结果处理。


## 26. D1：已有游戏目录只读发现（2026-11-09）

### 先阅读参考，再界定安全差异

- 固定只读SJMCL d5b109e0abb42460a8fd665c58ca4b0cc9982182，读取launcher_config/commands.rs的check_game_directory：选择根/直接.minecraft或minecraft子目录的检测、重复位置检查；instance/helpers/misc.rs的refresh_instances/refresh_instance/get_instance_subdir_paths：versions遍历、JSON/JAR、库/patch线索、隔离游戏数据位置，以及模型/实例页入口。没有运行参考程序、复制GPL源码/品牌/凭据或改参考工作区。
- 参考发现会调用unify_instance_name重命名原版本目录、保存启动器配置；并以JSON/JAR均存在过滤候选。mcl不采用这些写行为和缺项即消失：D1不改源目录，缺JSON/JAR/父文档、损坏或链接入口仍报告候选/原因，不把“扫描”当“接管”。
- 来源登记独立于现有GameDirectoryStore：existing-sources.json是应用数据目录里的schema1/最多32项/256KiB登记，修改持独立应用目录系统锁并原子保存。不在来源创建owner、锁、实例记录或.mcl目录；也不将来源加入默认游戏位置、删除服务或启动链。

### 已接实际闭环

本节保留D1交付时点，D2/D3后续状态以第27节为准。

- 新建→导入工作台提供“发现已有游戏（只读）”，进入现有三栏右侧工作区；可以显式添加来源、选择来源、重新扫描、取消、搜索、打开候选详情、仅移除来源登记。没有假导入/原地启动/迁移按钮，不改创建表单的三模式、480DIP/无外层滚动约束。
- 选择真实Minecraft根目录；父目录只有一个直接.minecraft/minecraft且有普通versions时可规范化到该真实子目录，两者都存在则拒绝猜测。选择链接会固定其规范目标路径；保存后不跟随新链接。拒绝重复实际目录、大小写/NFC路径别名与父子重叠、应用数据或其父子目录；不可用磁盘不会创建fallback。
- core/discovery有界扫描versions：最多2048个目录项、单份4MiB/累计64MiB元数据、2048次文档读取与8层父链，检测循环/不安全ID/损坏JSON/ID不符。累计预算包括读取后发生竞争或校验失败的字节；截断明确显示不是完整清单。单次顺序读取，无无限递归、网络或JAR执行。
- 读取逐组件Lstat/OpenRoot/身份检查，普通文档同FD前后核对路径/FD/大小/mtime；版本/文件/内部目录链接均不跟随，扫描结束复查来源根身份。原目录名与文档ID不同时只报告，不重命名或写回。
- 展示来源完整路径、版本目录、文档ID、本地父文档推导的基底提示、已知库/patch的Fabric/Quilt/Forge/NeoForge线索、未知组件、客户端JAR路径存在情况、共享assets/libraries目录缺失和隔离数据候选。原版主类仅作为本地声明展示，库线索不升级为官方身份或兼容结论。
- shared根/versions/<id>/及其.minecraft/minecraft子目录可能同时存在；全部作为数据候选，不猜另一个启动器是否启用了隔离。D2须明确选择且重读核验，不能继承账户、JVM参数、脚本、私有配置或认证资料。本轮不读取launcher_profiles.json、账户或启动器私有配置，也不打开/执行客户端JAR。
- 报告不做文件哈希/完整依赖/assets/native核验或联网补文件；“格式已识别”不是已安装/可启动。详情直接说明仅本地声明、JAR仅检查存在，D1不会进入InstanceStore/SessionRegistry/TrashItem。MetaJSONSHA256仅是本次内容指纹，不是官方来源证据，不能直接用于C的受信任加载器请求。
- native UI沿原圆角/紧凑列表、列表内滚动、语义色/固定图标/Tooltip规则和通用Toast。显式任务后台执行，通过Window.Update交付；来源/候选稳定ID，取消、换源、离页或关窗会提升generation并拒绝旧列表/旧扫描结果。页面重建不发起扫描、不做文件读取。
- 来源登记先成功保存后才开始扫描；取消/关闭扫描不会撤销用户已保存的只读来源，重开可重新扫描。只读失败保留原目录及已有登记，不显示假成功或把离线当空目录。

### 构建与未验收

- 新增core/discovery扫描/解析/安全读取，app.ExistingGamesService窄接口，storage.ExistingGamesStore，入口异步适配和UI控制器/页面；复用现有Go/x/text/MyGo，无新依赖/版本升级/配置迁移。
- 变更Go文件gofmt、git diff --check；CGO_ENABLED=0 ./scripts/go.sh build ./...及Windows/Linux/macOS amd64主程序交叉编译成功；MyGo darwin/arm64临时签名体验包build/darwin-arm64/mcl-0.0.3-existing-games-preview-arm64.dmg约8.5MB，app约19.1MB，未公证、未发布Release。
- 新增discovery/scan_test.go、storage/existing_games_test.go、ui/existing_games_test.go源码，覆盖缺项/损坏/父链/链接/ID错配/取消、来源重复/重叠/离线/损坏登记保留、无源目录写入与移除保留、过期回调和MyGo额外帧/详情/尺寸切换。全部未执行、未编译测试目标；生产build不证明测试通过。
- 未跑test/vet/race、UI自动化、原生文件选择或任何用户真实目录扫描/变更，未运行Java/游戏/公网验收。D1仅实现/构建，D2复制预览/固定文件计划/非覆盖导入及D3跨盘可恢复迁移仍明确未完成，D整体不能勾完成。


## 27. D 阶段统一收尾：确认复制与可恢复迁移（仅构建）

### 用户安排、参考与语义

- 用户要求连续完成整个D后统一交付。本节与第26节共同覆盖D1–D3实现；不启动T，不自动合并main、打新标签或发布Release。
- 固定只读SJMCL d5b109e0：继续读取instance/commands.rs的copy_resources_to_instances、move_resource_to_instance及helpers/misc.rs的get_instance_subdir_paths。参考按version_isolation选共享根/版本目录，部分路径辅助函数会创建目录，资源移动用fs::rename；这些不是跨卷/崩溃恢复设计，不照搬其写源目录或跨卷rename行为。独立Go实现，无参考代码/品牌/凭据/许可资源复制，无新依赖。
- 三种语义明确区分：发现只读；受管复制生成新稳定实例ID、保持原资料；原地管理未实现、不提供原地启动/全量删除入口。D3只移动已经属于mcl的载荷，不接管D1登记的外部来源。

### D2：明确来源、固定清单、确认复制

- 入口：新建→导入→发现已有游戏→候选详情→“复制为受管实例…”。右侧紧凑工作台编辑名称、明确选择共享根/隔离资料目录、资料分类和目标位置；只有单一数据候选时自动填入，多个候选不猜隔离设置。目录选择固定稳定ID、不改变全局默认。
- 允许分类为saves、mods、config、resourcepacks、shaderpacks、screenshots、texturepacks、options.txt、optionsof.txt、servers.dat。config默认不选；其他已知游戏资料默认选，文件清单与排除项可预览，确认前可取消整个分类。配置内可能含私人信息，不宣称通用内容检测器能识别所有秘密。
- 不读取或继承其他启动器账户、launcher_profiles、JVM/Agent参数、入口脚本、外部版本JSON/JAR/libraries/assets/natives或私有启动器配置。显式排除账户/凭据/令牌名称、.env、session.lock、证书/私钥和常见脚本/本地可执行文件；JAR仅允许在mods分类。复制模组是用户明确选择的游戏资料，不代表运行其处理器或现在执行代码。
- 预览/运行均重读登记来源和版本继承链，各文档摘要参与固定指纹；D1提示仍不充当官方认证。游戏基底必须在本项目官方目录中，重新读取其受信任元数据；已知单loader按该游戏官方版本目录核对。未知组件/未确认包装器/循环或多loader不猜成原版。缺失外部JAR/库允许由mcl重建，但不能宣称已可运行。
- SHA-256固定选中文件，复核源资料，私有暂存内复制/同步/核验；只以MoveNoReplace发布。新实例中央元数据与pending一同发布，目标可为内置或已登记外部位置。复制前后重读源文件清单，竞争、源变化、离线、目标冲突、写满或取消不修改/删除原来源。普通失败保留可继续的任务，不生成安装完成收据。
- 最终directory-import.json只记录注册/候选身份、官方环境和相对文件证据，不把外部本机路径作为持久启动引用；待恢复日志暂存固定源路径以便重开复核，成功后移除。成功文案明确“游戏资料已复制，运行环境仍需安装/修复”，后续沿现有官方安装、自动Java/加载器和StartTracked链，复制不等于运行环境就绪。

### D3：固定位置提交、恢复、旧副本及全量删除

- 入口：实例设置→游戏目录行“迁移…”。支持内置↔外部、外部↔外部；同位置拒绝，预览只读，不改实例名称/版本/loader/Java/图标/身份。仅迁移game（含存档/日志/配置等所有普通文件）、removed-assets和asset-updates；中央账户/来源/收据/会话/runtime/legacy-assets/natives及公共客户端/libraries/assets/Java保持原位。
- 目录事务独立系统锁位于应用数据directory-locks，真实会话租约与原OS实例锁继续生效。directory-transfer.pending.json保护普通Reserve、资产恢复ReserveForRecovery及删除ReserveForDeletion；只有专门ReserveForDirectoryTransfer可继续本事务，仍不能绕过游戏存活、删除或资产更新pending。AssetStore在pending时拒绝旧路由，不创建隐藏内置fallback。
- 复制目标使用所在卷私有stage。日志阶段为copying→publishing→committed→backing-up→complete；文件全部核验并非覆盖发布后，原子提交唯一的GameDirectoryID，保留文档原版本的更高值。中央配置可能已提交而日志尚未更新时，按完整前后快照识别，不用实例ID/PID猜状态。共享资源/启动交接接口未绕过。
- 跨卷永远复制+核验，不跨卷rename。日志、所属标记及文件先flush；Unix增加目录同步屏障，目录内发布/同卷旧副本归档用非覆盖rename。Windows无普通用户POSIX目录fsync等价物，依赖文件flush和文件系统元数据日志；不承诺多文件硬件级断电原子性。原副本不永久删除，持久日志与旧副本用于恢复。
- 发布未完成可重建丢失的暂存；提交后缺少目标文件时，只从固定原载荷/保留旧副本显式非覆盖补齐，不覆盖变动或无所属证据的目标。外部卷离线/owner或规范路径改变时等待原卷恢复，不伪造空目录成功。目标内容不符保留冲突和日志，用户可核对后重试。
- 提交前可确认撤销：仅本事务所属的新副本/暂存移入系统废纸篓，来源不动；未标记的冲突数据保留。提交后不可按旧预览强行回滚，需完成恢复，再另起回迁。停止等待worker终态，不提前释放互斥；导航可以离开正在运行的任务，重开“下载与任务→目录迁移与复制任务…”继续/撤销。关窗取消本应用任务、不操作游戏。
- 旧载荷归档为其原卷.mcl-dir-backup-<token>，不自动永久清理；记录directory-history的实际旧文件证据。如果提交后的旧源又出现变化，保留并显示“旧副本包含后续变化”，不冒称已合入新位置。提供“核对旧副本”打开真实所属目录，以及单独确认、重验文件清单后移入废纸篓的清理；外部位置/只读来源在相关任务或旧副本仍被引用时不能移除登记。
- 实例“删除全部数据”同步重算历史旧副本范围，将中央/现有外部载荷以及先前迁移的所属旧副本一并列入确认及指纹；分步移入废纸篓、失败保留删除pending并可重试，不留下未确认的外部旧副本、不永久RemoveAll。非受管发现来源、外部位置owner和共享数据不删除。

### 边界、UI与验证状态

- 文件树最多10万项、64层、128GiB总量/16GiB单文件，路径/元信息预算8MiB，编码清单12MiB、日志32MiB，单实例最多256条迁移历史。大小写/NFC别名、链接/特殊文件/越界路径拒绝；不提供软链接跟随或断点文件字节Range。恢复复用已完成且校验相符的整文件，半文件仅为私有分片，T的通用下载任务并未实现。
- UI复用原圆角、44DIP设置行/26–28DIP按钮、Popover/Radio/Checkbox、列表内滚动、语义色/Toast和原生确认；页面身份不含输入/进度/busy。后台工作经Window.Update，最多一份待交付进度快照，取消/替代/离页/关闭generation保护，渲染不扫描或哈希。
- 已添加core/transfer、storage/directory_transfer及ui/directory_transfer回归源码：允许/排除资料、链接/别名/冲突/取消、复制保留原始数据、双向迁移、稳定身份/共享数据不动、真实实例锁与各pending入口、旧token拒绝、丢失暂存/提交后缺文件恢复、离线卷无fallback、备份变化不清理、全量删除范围以及UI过期回调/停止等待/额外帧/尺寸切换。全部未执行、未编译测试目标；gofmt只检查语法，不证明这些回归可编译或通过。
- 按当前偏好，仅gofmt、生产build与Windows/Linux/macOS amd64主程序交叉编译、MyGo darwin/arm64临时签名DMG；不运行test/vet/race、自动化、原生选择/真实目录读写/Trash、跨卷或断电模拟、Java/处理器/游戏或公网安装验收。最终体验包及构建结果在本节末追加。
- D实现/构建范围已收尾，但原地管理、任意第三方自定义运行组件、外部运行库直接采纳、共享缓存迁盘、字节级断点与完整系统/实机验收均不在本次完成声明中。下一阶段为T，尚未开始；不得以构建成功推导所有来源/文件系统/平台可安全运行。


### 本次最终构建结果

- gofmt覆盖本次变更的生产与回归源码；git diff --check通过。
- CGO_ENABLED=0 ./scripts/go.sh build ./...成功（仅生产目标）。Windows/Linux/macOS amd64使用同一Go1.27.3主程序交叉编译均成功，产物分别为build/mcl-d-directory-windows-amd64.exe（28,504,576字节）、build/mcl-d-directory-linux-amd64（27,835,227字节）、build/mcl-d-directory-darwin-amd64（28,522,832字节）。
- CGO_ENABLED=0 ./scripts/go.sh tool mygo build -platform darwin/arm64 -skip-notarize成功；MyGo0.7.4报告app19.4MB、DMG8.6MB，临时签名且无公证。独立体验包build/darwin-arm64/mcl-0.0.3-directory-stage-preview-arm64.dmg（8,969,216字节），不替换GitHub v0.0.3 Release。
- 未运行或编译测试目标，未跑test/vet/race、原生/UI自动化、真实目录扫描/改动/Trash、跨卷/断电模拟、Java/游戏或公网安装验收。新增回归仅源码，构建不证明这些行为已验收。D1–D3实现/构建收尾，T尚未开始。


## 28. 用户体验反馈：Swift profiles布局与发现页减法

- 用户反馈指定Swift Craft Launcher的Fabulously Optimized实例没有被发现，直接添加profile目录又报“没有普通versions目录”；并明确要求移除截图中的常驻提示、空态与“（只读）”。这是D交付后的真实缺口，不以此前“D完成”推导所有第三方布局均支持。
- 已获授权对明确指出的目录做针对性只读诊断：该profile只有游戏资料，debug-profile.json为Minecraft HUD配置；Swift根下meta保存公共运行文件，data/data.db与WAL中的game_versions独立登记游戏/loader。只查看结构、实例表schema和指定实例的允许字段，未打开原库SQLite连接、写源目录或查询账户；调查时通过Python标准SQLite在内存中读取结构，新产品代码不依赖Python。
- 新增core/discovery的WAL内存快照、Swift候选转换，storage的固定布局解析与稳定FD读取；macOS platform使用现有purego绑定系统SQLite，只读deserialize私有影像，固定SQL提取ID/working_path/game_name及gameVersion/modLoader/modVersion/mainClass，不选launchCommand/JVM/Java/认证字段，不执行外部命令。Go.mod/go.sum不变，没有新增SQLite Go驱动或运行时CLI。
- 支持选择Swift根、profiles或具体profile；单个profile只查询/发现该名称，登记路径保持原选择、不提升到父目录。只允许实例位于当前根的profiles/<safe-name>，工作根必须匹配，不以DB任意路径重定向。UUID、目录别名、未知loader/入口、缺失实例目录都明确核对；不按名称/缓存版本猜身份。关联meta/data所在根也受mcl应用数据重叠保护。
- 数据库和WAL同FD前后身份/大小/mtime核对，二者查询后再次复核，检查SQLite/WAL头、salt、checksum及最后commit；原文件不checkpoint、不建SHM/锁/临时文件。单文件及内存影像32MiB、累计读取64MiB、最多2048条。原始影像可能含其他配置，因此不落盘/缓存/日志，结束后清零Go缓冲。macOS只读内存读取器不使其他平台获得伪支持，Windows/Linux遇Swift索引明确报暂不支持；标准versions仍使用原链。
- 复制来源复核与发现共用适配器，Swift game/loader声明仍须经原官方目录核对；只复制已选游戏资料，外部账户、启动参数、Java/JVM和原运行库不导入，不创建安装完成收据。Candidate新增format仅非空时序列化，旧标准来源pending指纹不因空字段变化。
- 发现入口/原生目录对话框取消“（只读）”，隐藏无来源选择器，删除“只读发现…”常驻条、选择目录说明、无结果空态、重复来源路径和成功状态标签；详情仅保留实际版本/loader/实例路径及真实问题，不堆文档ID/主类/客户端JAR说明。实际异常/截断通过Toast，底层只读和危险操作确认不变。
- 新增SQLite/WAL校验、最后提交/未提交帧隔离、原输入不变、三种Swift选择范围、D2重读、关联根保护、源变化拒绝、未知路径拒绝，以及页面无提示/空态的回归源码。全部仅格式整理，不执行或编译测试目标；不运行UI自动化/新原生读取器或导入/迁移/Trash/Java/游戏，用户复试仍待。最终构建记录见本节末，T仍未开始。


### Swift修正版最终构建记录

- gofmt与git diff --check通过；CGO_ENABLED=0 ./scripts/go.sh build ./...成功。Windows/Linux/macOS amd64主程序交叉编译成功，分别为build/mcl-swift-discovery-windows-amd64.exe（28,587,008字节）、build/mcl-swift-discovery-linux-amd64（27,924,281字节）、build/mcl-swift-discovery-darwin-amd64（28,620,288字节）。跨平台构建不表示Windows/Linux新增了Swift索引支持。
- CGO_ENABLED=0 ./scripts/go.sh tool mygo build -platform darwin/arm64 -skip-notarize成功，MyGo0.7.4报告app19.4MB/DMG8.6MB；独立体验包build/darwin-arm64/mcl-0.0.3-swift-discovery-fix-arm64.dmg（8,982,016字节），临时签名、未公证。不修改已发布Release。
- 新增回归/Tester未执行、未编译测试目标。仅做用户问题所需的原目录/登记格式只读诊断与SDK声明检查；新macOS索引读取器、mcl识别/复制流程和精简UI未运行实机验收，也未跑test/vet/race/UI自动化、真实目录导入/迁移/Trash或Java/游戏。需用户使用新体验包复试，不将诊断查询当作新代码验收通过。


## 29. T 阶段统一交付：持久任务、暂停排空、续传与重开恢复

### 授权、只读参考与取舍

- 用户要求开始T并参考.repo/sjmcl，整个阶段完成后统一验收。实际参考路径.repo/SJMCL，固定d5b109e0abb42460a8fd665c58ca4b0cc9982182、Git状态保持干净。本次在feature/persistent-download-tasks开发，不升级Go/MyGo，不新发版、不自动开始I。
- 阅读src-tauri/src/download.rs与src/components/download-indicator.tsx的服务接入，以及crates/sjmcl-downloader/src/model.rs、command.rs、actor.rs、executor.rs、download.rs、persistence.rs、storage.rs、rate.rs：任务组/文件/后处理、暂停handoff等待running归零再持久化、更新token后恢复、Range/416/200回退、最后文件大小、聚合限速、重启reconcile和历史存储。
- 只研究行为和边界，独立Go实现。没有复制GPL代码、全局actor/事件总线、SQLite/新ORM、内置CF Key、私有服务或授权绕过。继续复用sharedDownloads、TransferControl、DownloadNetwork、受管缓存、实例租约和原发布链路。没有新Go依赖、CLI、系统软件或全局环境变更。

### T1：固定根任务、文件状态与持久化

- core/tasks定义明确Intent、Group/File状态与终态，core/download提供类型化context观察接口；app.TaskService显式注入各服务，storage.DownloadTaskStore原子保存到应用数据download-tasks-v1/<id>.json。入口注入DownloadCoordination适配原生OS锁，核心不依赖UI/app/platform。
- 根任务覆盖游戏安装/修复、启动检查/启动准备、独立Java下载安装、市场普通资产、整合包/地图读取与导入、整合包缺失文件恢复、资产版本更新；加载器/Java/基础文件/assets作为当前根任务的阶段与文件接入。嵌套工作共享当前组，不重复生成互相独立的子安装记录；没有对应业务范围的原始文件Run仍可生成文件任务。
- 文件记录含固定路径/对象键、received、verified、total及pending/checking/downloading/verifying/publishing/done等阶段，组状态区分queued/running/pausing/paused/canceling/interrupted/failed/completed/canceled。下载完成不等于业务完成；安装收据、资产提交、整合包发布及Java注册成功才使根任务完成，真实失败原因归类显示，不把原始错误URL存入历史。
- 关键状态先原子写盘再交接，进度最多每秒合并一次写盘；状态保存失败取消执行，不能冒称暂停已落盘。内存摘要事件驱动，UI最多一份待交付快照；常规进度不遍历所有磁盘JSON，跨进程变化显式刷新。
- 单实例任务租约未改为任务ID猜PID；游戏/资产/删除/目录pending的原保护继续生效。任务持有自己独立系统锁，暂停后不持实例/网络租约，但保持当前任务的执行交接所有权，不能多个启动器同时继续同一组。

### T2：暂停排空、继续、取消和发布

- 暂停先进入pausing并取消本次context，停止排队/网络/处理器工作；等待原业务调用返回（其下载worker已join、实例锁/Java安装slot和FD已释放），保存分片最终大小/前缀摘要与Paused后才答复。中途保存失败保留失败状态，不把失败归为暂停。
- 继续只有在Paused确认后才能触发，进入queued并更新attempt/context；旧worker与新worker不并存。原调用者等待恢复，最终只收到一个终态；取消暂停任务同样等待取消记录落盘。组取消与失败分开，业务已提交成功的竞争返回completed，不能因晚到取消冒称回滚了已安装内容。
- 发布阶段有明确检查点，主动暂停拒绝正在发布；显式取消仍走原安全失败/回滚链。暂停不把旧context复用、不延迟成功、不在渲染里启动任务；关闭启动器保留interrupted/paused，不在重开时自动执行下载/安装或启动游戏。
- 任务重试先复核固定的实例ID、位置、游戏SHA1、pack/loader身份；名称等无关展示修改不使资源计划失效，目录/版本/loader变化必须重新确认。锁内再次复核，不能只用暂停前的UI快照。

### T3：受管分片、Range、摘要和资源预算

- 共享下载器新增SHA-256对象支持，Temurin生产下载注入同一个Client，沿用官方GitHub重定向限制、并发、总限速、代理代际与独立缓存。限速等待仍在网络idle预算之外；没有新建不受控制的下载器或放开HTTP来源规则。
- 分片位于应用数据download-parts-v1，键固定目标路径、大小、SHA-1/SHA-256/附加SHA-512；marker只保存对象、偏移、前缀SHA-256、散列后的来源、强ETag/有效Last-Modified与时间，不保存URL/查询或请求头。整次传输持目标OS锁及维护共享锁，目录和普通文件身份检查、不跟随链接。
- 重开以实际part大小为准，核对已保存前缀、文件类型与固定对象；来源变化/分片损坏只重建受管分片，不覆盖用户实例内容。偏移>0发送Range与可用If-Range；206严格核对单位/start/end/total、Content-Length、identity编码及已知validator，禁止拼接错位或变内容。
- 对忽略Range的200使用完整响应并从零截断；400/405/501不支持范围、416偏移不符、错位206或validator变化只作有限完整GET回退。完整分片先离线核对全摘要再发布，不凭416/大小冒充成功；短读保留真实分片，超长和摘要错误不发布并在有限重试内处理，重试耗尽可显式继续。
- 最终发布前/后核对整个对象摘要及FD身份/mtime，只有校验通过才替换受管下载/cache目标。新增fileio.ReplaceManagedFile用已打开父目录句柄执行同卷替换，实例资产/世界发布依旧MoveNoReplace/原保护，不把共享缓存替换推广为用户资产覆盖。
- 目标互斥在所有现有缓存核验、缓存复制、下载及发布期间持有，按NFC/大小写键协调跨实例/跨进程同目标。原ObjectCache仍负责独立已验证副本，不把所有cache目录当可删除下载垃圾。

### T4：恢复配方、重入核验和维护

- 启动加载只列出任务，非终态显示可继续，不自动恢复。继续/重试持原任务ID系统锁，复核原意图并经注入的业务服务重建临时URL/阶段，复用已验证缓存和分片。当前进程暂停恢复保留回调；退出后不保存账户或launch回调，原启动任务只继续资源准备，用户需重新明确点击启动。
- 游戏与loader按原官方元数据重新规划，首次解析的完整Mojang继承链摘要落盘，重开/继续后拒绝来源链变化；相同目标路径出现不同文件对象键也拒绝静默换内容。Java固定官方包身份；市场按固定项目/版本/文件摘要与大小重新读取官方目录及CF分发设置；依赖版本不会改成“最新”。CF Key只从当前会话取，缺少授权明确失败，保留任务供配置后继续。
- 整合包重读固定原始archive及已选optional清单，比较游戏/loader/每个所选文件身份，不重复创建已经发布的实例。通过中央modpack.json复核已发布来源；外部payload已发布但中央登记中断时，只复用本事务所属、与重新暂存内容完全一致的载荷，未知/修改内容保留，不猜原目录成功。
- 地图已发布而任务记录未完成时，以固定archive/存档名/实例/owner及全文件对比核验重入，绝不接管同名旧世界。包修复重入必须核对已出现文件的真实摘要，不能把“存在/被保留”当已修复。
- 资产更新恢复只允许精确匹配固定计划的pending/journal，经原ReserveForRecovery和RecoverUpdate回滚/完成后再决定继续；已提交的结果须对来源记录、目标摘要和停用状态逐项核对。不同更新、目录/删除pending或真实存活进程仍拒绝，不能绕过原事务。
- 提供已结束历史清理与独立分片清理，原生确认，失败/部分完成Toast。分片清理持维护独占锁，只处理匹配marker与键的普通文件，未知/链接保留；存在活动分片写入时拒绝。取消与删除任务记录不隐式删共享分片或已安装数据，清理分片不删完成资源/实例/Java/原独立缓存。

### 界面、数据界限和待验收项

- “下载与任务”主区为紧凑行列表：暂停、继续/重试、取消、文件详情、错误类别及查看实例；右上角刷新/维护菜单，不放空态长文或额外解释Label。会话、日志、目录迁移和本地扫描等非下载任务留在“会话与其他任务”，旧功能入口仍可返回统一任务页。
- 文件详情为显式读取快照并可刷新；全局组进度事件驱动。页面身份只含所选组，不含进度或busy；过期详情/离页/关闭拒绝旧回调。暂停时启动页、Java/市场/包修复等可见旧页面停止循环加载，不把暂停误显示为工作进行。
- 同时最多32个运行/暂停根任务，历史最多256项；单组最多110000个文件（覆盖原assets上限100000+库/Java等），Intent 4MiB/记录48MiB、part marker 8KiB、单文件仍2GiB/Java目录原1GiB。历史与分片不是无限存储，不自动清掉用户未处理的失败/暂停任务，满额时显式提示维护。
- 本次不保存访问/刷新令牌、CF Key、代理凭据、账户ID或完整认证URL；只保留已知公共无凭据/无查询URL。签名/未知私有URL从任务JSON移除，市场/pack等从原目录重新授权重建；没有可重建身份的原始文件任务须回原功能重新确认，不能编造CDN或借别的Key。子对象marker只存来源散列，不把URL误当可信文件身份。
- JSON历史是有界原子文件格式，不使用参考的SQLite/ORM；损坏/未知版本/链接记录保留报错，不空覆盖。Unix同步目录、Windows使用已同步文件及系统元数据保证；不宣称多文件硬件断电原子性。恢复仍复查摘要、实例占用及已有业务发布证据。
- 新增离线回归源码覆盖正确Range/强validator、错误offset/total/ETag/416/不支持Range/忽略Range、短读新Client续传、前缀/最终摘要损坏、链接/非法协议、暂停排空/旧worker互斥、重开不自动执行、检查点失败、秘密剥离与大整数、发布阶段拒绝暂停、任务损坏保留/锁/历史、更新与所选optional身份、包载荷冲突、UI过期回调/额外帧/缩放/导航。全部未执行且未编译测试目标。
- 遵循用户统一验收安排：仅gofmt、git diff --check、生产build与主程序交叉编译和macOS体验包；不运行test/vet/race、httptest、Tester、UI自动化、真实网络下载/Java/加载器处理器/游戏、用户实例修改、真实断网/崩溃/断电模拟或分片清理。构建产物与结果在本节末追加；T实现范围收尾不等于功能验收完成，等待用户体验反馈，I尚未开始。


### T最终构建与交付记录

- 变更生产及回归Go源码已gofmt，git diff --check通过。CGO_ENABLED=0 ./scripts/go.sh build ./...退出0；没有运行或编译测试目标。
- Windows/Linux/macOS amd64主程序交叉编译全部退出0：build/mcl-tasks-preview-windows-amd64.exe（33,935,872字节）、build/mcl-tasks-preview-linux-amd64（33,228,495字节）、build/mcl-tasks-preview-darwin-amd64（34,108,160字节）。这些是编译证据，不是对应平台行为验收。
- CGO_ENABLED=0 ./scripts/go.sh tool mygo build -platform darwin/arm64 -sign - -skip-notarize退出0，MyGo0.7.4报告app22.8MB/DMG9.6MB。独立体验包build/darwin-arm64/mcl-0.0.4-tasks-preview-arm64.dmg（10,027,520字节），临时签名、未公证；因未设置更新私钥，未生成签名自动更新，这与仅交付本地体验包的范围一致，不写入/索取私钥。
- 未跑test/vet/race/httptest/Tester、UI自动化、公网资源下载、Java/处理器/游戏、真实断网/进程崩溃/断电模拟、用户实例导入/更新或分片清理。回归只有源码，格式/生产构建不证明这些回归可编译或通过。
- 体验入口：“下载与任务”默认统一列表，可暂停/继续或重试/取消、查看文件快照、右上角刷新与维护；会话/日志/目录等从“会话与其他任务”进入。重开后明确点击继续；原启动任务冷恢复只准备资源，不自动运行游戏。
- T1–T4按本节范围实现/构建交付，等用户统一验收；I尚未开始。代码留feature/persistent-download-tasks，未合并main、更新版本号、打标签或发布Release。Go/MyGo/依赖及只读SJMCL工作区未改。


## 30. T体验反馈：Forge整合包被Fabric声明误拦截

用户截图：Zombie Invade 100 Days导入已下载130/130文件，发布前报“没有匹配当前加载器的声明；fabric / forge”。静态定位guardStagedPackMods → mods.GuardTransition → check：只读取到其他loader的描述符时，直接产生结构性Error，导致合法辅助元数据/兼容层或当前读取器不认识的入口被当作已证实不兼容。未读取用户真实JAR，不能据此证明该包必定兼容或确定具体文件。

- 未匹配当前loader的描述符统一为待确认诊断，保留原路径/声明loader/实例loader；既不当作当前已加载模块或provider，也不作为阻止整包发布的确定错误。相关完整性降为未知，不能由缺少已识别provider反推依赖必定缺失。
- 仍优先匹配当前loader，Quilt的Fabric后备不变；已识别重复ID、确定的冲突，以及从Yes变No/Unknown的已有必需关系仍拦截。没有自动换loader/停用/删除JAR或增加兼容模组，没有跳过下载摘要或实例发布保护。
- 本地声明阻塞增加稳定ErrUnsafeChange类别，持久任务显示安全固定原因，不把原始元数据/URL写入历史；即时错误包含对应文件路径，便于定位，不再只有“fabric/forge”。
- 新增回归源码覆盖外部描述符不阻止发布/不冒充provider、缺失判断保持未知、已满足依赖失效仍拦截、重复与冲突仍拦截、双描述符按当前loader选择和Quilt后备。遵循既有偏好，全部未运行且未编译测试目标；只格式、生产build及体验包，不接触用户真实实例/公网/游戏。
- 新包体验时在原任务点击“重试”，继续固定计划并复用通过摘要验证的已有缓存；不要求删除原任务/已下载资源或重建实例。重试后仍可能出现其他确切依赖/运行环境错误，不能把本次规则修正称作该包运行验收。


本修正版构建：gofmt、git diff --check、CGO_ENABLED=0 ./scripts/go.sh build ./...通过；Windows amd64主程序交叉编译成功（build/mcl-pack-declarations-fix-windows-amd64.exe，33,935,360字节）；MyGo0.7.4 darwin/arm64 -sign - -skip-notarize打包成功（build/darwin-arm64/mcl-0.0.4-pack-declarations-fix-arm64.dmg，10,022,912字节，约9.6MiB，临时签名、未公证）。回归源码已格式整理，未执行/未编译测试目标；未运行公网、用户实例修改、Java/处理器/游戏或原生交互。仍在feature/persistent-download-tasks，未合并main/发版。


## 31. T体验：Forge 47工具被误固定为Java8

用户后续截图确认包已导入，但配置Forge47.4.10时提示“准备安装工具Java（至少8）：官方目录暂无osx/arm64的Java8”。此前Fabric/Forge声明拦截已经越过；当前失败发生在后续运行环境安装，不能删实例或重新导入掩盖。

静态定位runProcessors把所有工具初始下限设成8，仅按class版本提高；processorRuntime/EnsureProcessor又为历史兼容把minimum8固定Java8。现代工具的Java8字节码并不等于现代游戏安装必须使用Java8，导致现代Forge错误走历史策略并查询缺失的ARM64 Java8目录。

本次把处理器选择下限设为max(8, 官方游戏Java要求, 全部已固定工具classpath的class最低要求)。现代游戏17/21下的旧字节码工具复用已有兼容稳定游戏Java，或沿原服务安装稳定工具Java；如果工具要求更高，仍提升独立工具下限，不改变游戏major或用户偏好。历史游戏8/工具8仍固定Java8，历史游戏8/工具17仍使用独立工具17；未改游戏exact-major、架构/EA校验、处理器顺序、私有stage、输出摘要和process安全适配。

新增回归源码通过synthetic class header + 假runner串联runProcessors/选择器，覆盖现代17+旧class8（osx/arm64）、现代21+旧class8、工具21+游戏17、历史8+工具8、历史8+工具17，并断言不更改game major、不执行Java。遵循用户现行偏好，源码未执行/未编译测试目标，不做真实网络/安装器/游戏/原生验收。

体验修正版后，在现有页面点击“继续安装运行环境”或原任务重试；使用已导入的同一个实例，不重复创建、不要求手动装Java8。此前成功导入也不等于本次运行环境/真实游戏已经验收。


本修正版构建记录：gofmt、git diff --check、CGO_ENABLED=0 ./scripts/go.sh build ./...通过。Windows amd64主程序交叉编译成功（build/mcl-forge-tool-java-fix-windows-amd64.exe，33,935,360字节）；MyGo0.7.4 darwin/arm64 -sign - -skip-notarize打包成功（build/darwin-arm64/mcl-0.0.4-forge-tool-java-fix-arm64.dmg，10,022,912字节，约9.6MiB，临时签名、未公证）。回归仅格式整理，未执行/未编译测试目标；未运行Java/真实处理器、用户实例续装、公网下载或游戏。feature/persistent-download-tasks独立修复，不合并main/发版。


## 32. T体验：导入成功直达实例

用户要求删除截图中的“整合包已导入/运行环境已就绪/查看实例”成功结果页。finishPackImport在确认Imported和RuntimeReady成功、收据与实例摘要更新后，清除当前导入编辑器并通过selectInstance进入对应实例；地图成功也返回目标实例。导航只在用户仍看本次市场导入页或本地导入工作台时执行，离开去其他页面后不抢导航。原任务指针不符的晚到回调仍拒绝；文件导入但环境失败/取消保留继续安装入口，不冒称整体成功。所有成功导航发生在原完成回调，不在每帧渲染里操作。

用户另询问删除实例后同包几乎秒完成，本次只静态核对并解释，不改缓存/删除策略：cache/market/<SHA1>保留原压缩包与包内下载文件；共享客户端/libraries/assets、loader代际及受管Java独立于实例；共享副本缓存也能按摘要恢复。实例删除仍回收专属载荷/中央记录/专属备份至系统废纸篓，不删其他实例共用资源。重新导入生成新实例、从已验证资源重建，不从缓存恢复旧存档。任务字节进度包含校验复用的对象体积，不能视作实际网络传输字节；本轮未读取用户磁盘或网络日志，不声称识别了该次每个命中的具体文件。

遵循现行偏好，仅格式、生产构建及macOS体验包，不跑测试/自动化/原生导入验收。没有新配置/API/缓存行为变更，没有访问或清理用户真实数据。


本次构建记录：gofmt、git diff --check与CGO_ENABLED=0 ./scripts/go.sh build ./...成功；Windows amd64主程序交叉编译成功（build/mcl-import-navigation-windows-amd64.exe，33,929,216字节）。MyGo0.7.4 darwin/arm64 -sign - -skip-notarize成功，独立体验包build/darwin-arm64/mcl-0.0.4-import-navigation-arm64.dmg（10,024,448字节，约9.6MiB，临时签名、未公证）。未运行test/vet/Tester/原生自动化或真实导入/删除/缓存清理，本次不新增测试；功能交互待用户体验。


## 33. T体验：下载缓存常显占用和真实路径

用户要求下载缓存占用自动显示，移除“默认位置”占位Label并直接显示实际路径。下载设置入口点击后显式发起后台Inspect，成功保存偏好、清理（含取消/部分失败后）和任务完成时在当前页刷新，不从渲染里读磁盘或启动扫描；没有新增轮询/计时器。统计中显示“正在统计…”，完成后始终显示字节格式（空缓存0B），未获取显示—，移除菜单“查看占用”。

路径由入口注入DownloadCache.DirectoryFor纯函数解析，空配置显示真实默认受管子目录，自定义显示选定父目录下的真实受管子目录；已成功统计时用Info.Directory。没有在UI猜缓存根/复制存储常量或把父目录标签当真实缓存路径，长路径沿原单行截断样式，保留可访问文本和打开目录按钮。

统计复用原有取消/generation保护，离页拒绝晚到结果，关闭窗口主动取消并屏蔽回调；设置写入成功后才更新提交快照并重查，新草稿不冒用旧位置占用。清理确认/范围/系统锁未变，失败或部分清理仍Toast，不把未知统计显示成0。不修改schema、缓存范围、用户实例或其他下载行为；只格式构建，无测试/自动化/实机扫描验收。


本批构建：gofmt、git diff --check及CGO_ENABLED=0 ./scripts/go.sh build ./...通过，Windows amd64主程序编译成功（build/mcl-cache-display-windows-amd64.exe，33,928,704字节），MyGo0.7.4 darwin/arm64 -sign - -skip-notarize打包成功（build/darwin-arm64/mcl-0.0.4-cache-display-arm64.dmg，10,023,936字节，临时签名、未公证）。未跑测试/vet/Tester/自动化/原生/用户真实缓存扫描或清理，未新增回归，不把构建称为交互验收。留在feature/persistent-download-tasks，不合并main/发版。


## 34. T体验：精简整合包确认流程与项目图标

用户连续反馈：移除常驻“从本地补齐”、优化确认页和描述输入、使用对应市场整合包icon；同时明确删除只有名称/游戏目录/继续的中间页。

- 在线选择整合包版本后直接发起原后台Inspect并固定当前游戏目录，进入读取状态后到最终确认；不会自动发布或跳过可选文件/固定清单确认。成功后仍直达实例。读取失败提供重新读取，返回直接离开当前导入，不恢复多余中间页。地图仍需真实目标实例/存档选择。
- 确认页48DIP图标、可编辑名称与单行来源/版本/Minecraft/loader元信息集中在顶部；删除重复整合包名称/大输入框和独立元信息块。描述改MyGo原生TextAreaBase，64DIP可换行编辑，使用共同内容组的内边距和细边框，不嵌单行灰输入框；目录行仍紧凑。清单圆角细边框和自身滚动，底部右对齐30DIP安装按钮，无外层表单Scroll。
- 移除无条件“从本地补齐”和空工具栏。真实受限/缺失文件仍明确阻止安装、保留原项目页，并只在对应缺失文件行提供选择文件图标；不跳摘要、不绕分发，不删除原手动补齐后端或限制说明。原始包受限时仍须选择对应压缩包。
- 新增显式InstanceIcon异步动作，与原市场图片共用四项并发和平台图片适配：仅公共Modrinth/CF图像CDN、2MiB/2048x2048、80x80归一化、后台PNG编码、Window.Update交付；UI渲染不联网/读盘/编码。市场已有bitmap可先显示，完成后把PNG副本放入导入Request和Instance，下载/导入进度以及创建后实例均可使用，不依赖市场列表bitmap寿命或只存URL。
- 详情中补全IconURL后按同来源项目ID带入；元信息回调不能重置先到的项目图标；手动预设/本地图片覆盖会取消网络任务并递增generation，迟到图标不能覆盖。切换导入、返回、关窗主动取消，旧任务指针/代际均拒绝。用户选择自动时可重新加载项目图标；失败Toast并明确回到默认图标，不能仅显示缓存缩略图却静默保存空图标。本地包没有远端市场身份时继续原图标规则。
- 新增回归源码覆盖版本选择立即读取/固定目录、图标先到不被元信息重置、PNG进入固定导入快照与独立字节所有权、取消/手动预设/替换任务拒绝迟到回调；全部未运行、未编译测试目标。仅格式/生产构建/体验包，不访问公网或运行原生交互、真实包/Java/游戏验收；没有新增依赖或配置schema。


本批构建记录：gofmt、git diff --check与CGO_ENABLED=0 ./scripts/go.sh build ./...通过；Windows amd64主程序交叉编译成功（build/mcl-pack-editor-windows-amd64.exe，33,943,040字节）。MyGo0.7.4 darwin/arm64 -sign - -skip-notarize打包成功（build/darwin-arm64/mcl-0.0.4-pack-editor-arm64.dmg，10,026,496字节，临时签名、未公证）。新增回归只源码、未执行/未编译测试目标，未跑vet/Tester/原生交互、公共图标请求/真实整合包导入或Java/游戏验收。无新依赖/配置schema，参考目录未动；留feature/persistent-download-tasks，不合并main/发版。


## 35. T体验：整合包读取失败留下空白重读页

用户FunDinoModpack（市场版本1.7.10）截图只剩标题/重新读取。已确认的界面回归：inspectMarketArchive把错误写入i.err，但archiveImportView不展示且Toast桥接未接该字段，因此失败原因完全不可见。仅凭截图无法确认实际错误属于版本接口/网络/清单/官方游戏元数据，不能直接声称1.7.10不支持或已修复该包兼容性；本轮没有读取用户缓存或公网重现。

首次在线读取失败现在发出带具体原因的错误Toast，取消该次图标任务并退出空确认状态，恢复原项目版本列表；用户已离页时不修改当前page。主动取消静默，已替换任务的迟到回调忽略；已有确认清单的重新读取失败保留名称/图标/目录/可选项，显示Toast。异常未初始化状态的恢复动作缩为普通工具行，不再用占满页尾的大主按钮伪装正常步骤。

应用层为原错误补读取版本/文件信息、获取压缩包、解析清单、官方游戏列表/指定版本及核对阶段上下文，仍使用%w保留取消/错误分类，不保存原始凭据或URL。没有放开ZIP边界、摘要、加载器、Java或平台限制，也不把市场显示版本当运行验收。新增回归源码覆盖首次失败退回+Toast、取消静默/离页不导航、旧回调拒绝和重读保留草稿，未执行/未编译测试目标。只格式构建体验包，不跑公网/原生/Java/游戏验收。


本批构建记录：gofmt、git diff --check与CGO_ENABLED=0 ./scripts/go.sh build ./...通过；Windows amd64交叉编译成功（build/mcl-pack-read-feedback-windows-amd64.exe，33,946,624字节），MyGo0.7.4 darwin/arm64 -sign - -skip-notarize打包成功（build/darwin-arm64/mcl-0.0.4-pack-read-feedback-arm64.dmg，10,025,984字节，临时签名、未公证）。回归源码未运行/未编译测试目标，未做公网/原生或该用户整合包实机验收，不把错误反馈修复称为已解决具体包底层失败。留feature/persistent-download-tasks，不合并main/发版。


## 36. 用户再次明确：移除Forge导入常驻说明

删除“Forge安装器将在导入后核验；文件可导入不代表游戏已可运行”的生成与确认页Warnings展示分支，旧任务中保留的该字段也不会重现提示。不改为Toast、Tooltip或替代说明，不留下布局空白。真实工作错误仍沿原Toast/任务诊断反馈。用户反复要求不要在UI加入这类提示，后续不得再以兼容边界/实现说明为由恢复。仅格式、生产构建及体验包，未运行测试/原生验收。

构建记录：gofmt、git diff --check、CGO_ENABLED=0生产build及MyGo darwin/arm64临时签名打包成功；build/darwin-arm64/mcl-0.0.4-no-import-notice-arm64.dmg，10,024,960字节，未公证。未测试/原生验收。
