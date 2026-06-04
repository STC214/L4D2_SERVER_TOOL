# L4D2 Server Filter Mod Research

本目录用于承接下一阶段需求：研究是否能把“按服务器名关键字过滤 + 同步屏蔽 IP”的能力，从当前外部工具推进到 Left 4 Dead 2 的 VPK Mod 形态。

本地游戏目录：

```text
E:\SteamLibrary\steamapps\common\Left 4 Dead 2
```

## 目标

1. 分析游戏内“组服务器列表 / 服务器刷新区域 / 内置服务器浏览器”的数据来源和 UI 扩展边界。
2. 判断 VPK Mod 能否在不注入、不改二进制的前提下，对服务器列表做关键字过滤。
3. 如果纯 VPK 无法完成动态过滤，则设计一个可审核的降级方案：
   - VPK 内置配置文件保存过滤关键字。
   - 独立 Go + Win32 配置器修改 Mod 配置文件。
   - 外部工具继续负责 IP 观察、导出、系统防火墙屏蔽。
4. 保持最终 Mod 可以走 Steam 创意工坊审核边界：不包含 Hook DLL、注入器、驱动、内存修改器、反 VAC 或规避逻辑。

## 目录说明

```text
README.md                         总览和当前结论
scope_and_risk.md                 边界、VAC/创意工坊风险、可接受实现范围
feasibility_matrix.md             三个方案的可行性对比
vpk_mod_design.md                 VPK Mod 结构、配置文件和 UI 设想
go_win32_configurator_design.md   Go + Win32 配置器设计，重点避免 UI 卡死
hook_research_plan.md             只用于离线分析的 Hook/逆向观察清单
mod_skeleton/                     未来 VPK 包的草案结构
```

## 当前判断

最可能落地的路线是“VPK + 外部配置器 + 现有外部屏蔽工具”组合：

1. VPK 负责提供游戏内可见的资源、说明、默认配置和可能的 UI 替换。
2. Go + Win32 配置器负责编辑关键字配置、生成/刷新 VPK 或 loose addon 文件。
3. 当前工具继续负责从网络观察/A2S 中获得服务器 IP，并用 Windows 防火墙阻断随机匹配进入命中服务器。

纯 VPK 直接拦截并删除服务器列表项的可行性偏低。L4D2 的服务器列表和 matchmaking 逻辑大概率在客户端二进制与 Steamworks/Source UI 代码内完成，普通 addon 通常只能覆盖资源、脚本、材质、声音、部分 UI `.res` 文件，不能执行任意客户端逻辑。

Hook 研究可以帮助确认真实调用链和数据结构，但它不能进入最终可审核 Mod 包。

