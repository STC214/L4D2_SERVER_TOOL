# L4D2 Server Tool

一个不修改《Left 4 Dead 2》游戏文件的外部服务器筛选工具。  
工具通过 Win32 UI 展示扫描/观察到的服务器，按关键词实时标记命中结果，并可导出 IP 或生成 Windows 防火墙/火绒 IP 黑名单规则。

## 核心功能

- 不注入游戏、不 Hook DLL、不修改 VPK/cfg/游戏目录文件。
- 支持外部扫描 Steam master/A2S 服务器信息。
- 支持观察 `left4dead2.exe` 网络流量，从真实 UDP 包中提取 L4D2 服务器 IPv4。
- 结果区每 3 秒增量更新，避免整表刷新闪动。
- 左侧关键词可实时生效，每次刷新都会重新计算命中状态。
- 左侧关键词变化会自动保存到 `config.json`，关闭程序时也会兜底保存。
- 支持“仅显示命中结果”和结果区模糊搜索。
- 自动保存当前结果到 `scan_results.json`，程序重开后自动读取。
- 再次扫描/观察时按服务器名对比 IP，若同名服务器 IP 变更，会标记为 `变更` 并显示旧 IP。
- 导出/防火墙应用前会强制刷新命中状态，IP 变更条目使用当前最新 IP。
- 支持导出 `blocked_ips.txt`。
- 支持导出火绒 IP 黑名单 JSON：`huorong_ip_blacklist.json`。
- 支持生成并应用 Windows 防火墙入站/出站 UDP 阻断规则。
- 支持移除本工具创建的防火墙规则。

## 快速使用

1. 运行 `L4D2ServerTool.exe`。
2. UAC 弹窗中允许管理员权限。
3. 左侧关键词框中每行写一个关键词，例如 `rpg`、`vip`、`shop`。
4. 点击 `观察游戏`。
5. 打开 L4D2，进入游戏内服务器列表或 Steam 组服务器列表，等待游戏自动刷新。
6. 右侧结果区会显示观察到的 IPv4 服务器。
7. 勾选 `仅显示命中结果` 可只看命中项。
8. 在顶部第二行的搜索框输入内容，可模糊过滤服务器名、地址、地图、标签、命中原因等字段。
9. 确认命中结果后，可点击：
   - `导出IP`
   - `导出火绒`
   - `应用屏蔽`

## 观察模式说明

观察模式使用 WinDivert，并采用双层监听：

1. Flow 层识别 `left4dead2.exe` 使用的本地 UDP 端口。
2. Network 层监听真实 UDP 包。
3. 根据游戏本地端口提取实际远端服务器 `IPv4:Port`。
4. 对候选服务器执行 A2S_INFO 查询，尽量获取服务器名、地图、人数、Ping 和标签。

这比单纯读取 WinDivert Flow 远端地址更可靠，因为 L4D2 刷新服务器列表时可能使用一个本地 UDP socket 与大量服务器通信，Flow 层远端地址可能出现 `0.0.255.255:*` 这类通配/非真实服务器地址。

观察模式不会自动屏蔽任何 IP。只有点击 `应用屏蔽` 或导出后手动导入火绒，才会进入真正阻断流程。

## 结果区

结果区字段：

```text
------+---------+---------+----------------------+--------------------------+--------------------------------------------+----------------------------------+
| 状态 | 延迟    | 人数    | 地图                 | 地址                     | 服务器                                     | 命中原因                         |
```

状态含义：

- `正常`：未命中当前筛选规则。
- `命中`：服务器名命中左侧关键词。
- `变更`：同名服务器与上次保存记录相比，当前 IP 已变化。

刷新行为：

- 扫描/观察过程中，每 3 秒刷新一次结果。
- 刷新时不会清空整个列表，而是增量更新变化行。
- 用户当前查看位置会尽量保持，不会每次跳回顶部。
- 左侧关键词改变后，无需重新扫描，下一次刷新会重新计算命中状态。

## 关键词与筛选

左侧文本框中每行一个服务器名关键词。命中状态只按服务器名匹配，不会因为地图、标签、Ping、人数、密码或 IP 规则被标成 `命中`。工具会对服务器名做简单标准化后匹配：

- 转小写。
- 压缩空格。
- 去掉部分装饰符号。

因此英文关键词大小写不敏感。无论关键词写成 `RPG`、`rpg`、`RpG`，都可以匹配服务器名中的 `RPG`、`rpg` 或混合大小写形式。

示例：

```text
rpg
vip
shop
无限
多特
QQ群
```

命中后，该服务器会被标记为 `命中`，并加入当前可导出/可屏蔽 IP 集合。

## 搜索框

顶部第二行的搜索框支持模糊搜索，空白时会显示灰色占位提示。搜索范围包括：

- 服务器地址
- 服务器名
- 地图
- Game/Folder
- 标签
- 命中原因
- Ping
- 人数
- 旧 IP

搜索只影响界面显示，不会改变命中规则本身。

## 自动保存与 IP 变更

工具会自动维护 `scan_results.json`：

- 程序启动时读取上次结果。
- 程序关闭时保存当前结果。
- 扫描/观察完成时保存当前结果。

如果再次扫描/观察时发现“同名服务器”的 IP 发生变化：

- 状态列显示 `变更`。
- 原因列显示 `IP变更:旧IP`。
- 导出火绒、导出 IP、应用防火墙时使用当前新 IP。
- 旧 IP 只用于提示，不会被当作当前屏蔽目标。

## 导出 IP

点击 `导出IP` 后生成：

```text
blocked_ips.txt
```

格式为每行一个当前命中 IP。

导出前工具会重新读取左侧关键词并计算命中状态，确保导出的 IP 是当前规则下的最新结果。

## 导出火绒 IP 黑名单

点击 `导出火绒` 后生成：

```text
huorong_ip_blacklist.json
```

格式参考火绒 IP 黑名单导入/导出的 JSON：

```json
{
    "ver": "6.0",
    "tag": "ipblacklist",
    "data": [
        {
            "id": 1,
            "tmp_field_sel": true,
            "raddr": "1.2.3.4",
            "memo": "服务器名 | 命中原因"
        }
    ]
}
```

`raddr` 使用当前命中的最新 IPv4。  
如果某服务器 IP 发生过变化，旧 IP 只写在界面提示里，不会导出为阻断目标。

## 应用 Windows 防火墙屏蔽

点击 `应用屏蔽` 后，工具会：

1. 重新计算当前命中状态。
2. 提取当前命中的最新 IPv4。
3. 写入 `blocked_records.json`。
4. 生成 `apply_l4d2_firewall.ps1`。
5. 使用管理员权限运行脚本。
6. 通过 `netsh advfirewall` 创建入站和出站 UDP 阻断规则。

防火墙规则名前缀：

```text
L4D2 Server Tool Block
L4D2 Server Tool Block OUT
L4D2 Server Tool Block IN
```

其中 OUT 规则阻止本机向目标服务器发送 UDP 查询，IN 规则阻止目标服务器向本机返回 UDP 响应。两者同时使用时，更有利于让游戏内服务器列表不再显示这些服务器。

如果 IP 很多，工具会自动拆成多条规则。

## 恢复屏蔽

点击 `恢复屏蔽` 后，工具会生成并运行：

```text
remove_l4d2_firewall.ps1
```

该操作只删除本工具创建的 `L4D2 Server Tool Block`、`L4D2 Server Tool Block OUT`、`L4D2 Server Tool Block IN` 规则，不会清空其他防火墙配置。

## 第三方引用

本工具的观察模式依赖 WinDivert：

- WinDivert：Windows packet capture/divert driver and user-mode library。
- 项目地址：<https://github.com/basil00/WinDivert>
- 文档地址：<https://github.com/basil00/WinDivert/wiki/WinDivert-Documentation>
- 许可证：WinDivert 官方文档说明其采用 LGPLv3 或 GPLv2 双许可证。

便携版中随附的 `WinDivert.dll` 和 `WinDivert64.sys` 来自 WinDivert 发行包，用于在管理员权限下观察 L4D2 的网络流量。本工具不修改 WinDivert 文件。

## 文件说明

```text
L4D2ServerTool.exe          主程序
WinDivert.dll               WinDivert 用户态 DLL
WinDivert64.sys             WinDivert x64 驱动
config.json                 筛选配置
scan_results.json           自动保存的扫描/观察结果
blocked_records.json        每次应用屏蔽的记录
blocked_ips.txt             导出的命中 IP 列表
huorong_ip_blacklist.json   导出的火绒 IP 黑名单
apply_l4d2_firewall.ps1     应用屏蔽时自动生成
remove_l4d2_firewall.ps1    恢复屏蔽时自动生成
startup.log                 启动和观察调试日志
```

源码文件：

```text
main.go             Win32 UI、结果刷新、保存读取、导出入口
observer.go         WinDivert 观察模式
scanner.go          Steam master/A2S 查询和筛选规则
firewall.go         防火墙脚本、IP 导出、火绒导出
block_records.go    屏蔽记录
process_windows.go  进程检测
```

## 常见问题

### 为什么需要管理员权限？

WinDivert 驱动加载和 Windows 防火墙规则修改都需要管理员权限。

### 为什么只显示 IPv4？

L4D2/Source 服务器生态绝大多数是 IPv4，Steam master/A2S 查询也主要围绕 `IPv4:Port` 工作。工具当前只把明确符合 L4D2/Source 服务器端口规则的 IPv4 结果显示到列表中。

### 游戏里能刷新，工具扫描失败怎么办？

很多加速器/代理只处理 `steam.exe` 或 `left4dead2.exe` 的流量，独立 exe 访问 Steam master 可能超时。  
这种情况下优先使用 `观察游戏`，让工具从游戏进程的真实网络包中提取服务器。

### 为什么有些服务器没有名称？

工具观察到服务器 IP 后会尝试 A2S_INFO 查询。部分服务器不响应、超时或被网络环境拦截时，可能只能显示候选地址。

### 会有 VAC 风险吗？

工具不注入游戏、不 Hook 游戏 DLL、不修改游戏文件。它只做外部网络观察、A2S 查询和系统防火墙规则管理。

## 重新构建

生成 GUI exe：

```powershell
go build -ldflags "-H=windowsgui" -o L4D2ServerTool.exe .
```

运行测试：

```powershell
go test ./...
```

如果修改图标资源：

```powershell
go run .\tools\makeico.go
windres.exe app.rc -O coff -o rsrc.syso
go build -ldflags "-H=windowsgui" -o L4D2ServerTool.exe .
```
