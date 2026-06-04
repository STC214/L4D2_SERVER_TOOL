# L4D2 Server Filter Toolset

这是一个围绕《Left 4 Dead 2》服务器列表过滤、RPG/伪装服务器识别和屏蔽方案研究整理出来的工具集合。

当前主线目标是：让指定关键字、已知 IP、学习到的地址和自动派生地址对应的服务器，尽量从游戏内组服务器列表和服务器浏览器中消失或无法进入。

## 安全警告

本项目包含多类高风险本机分析/过滤工具，包括：

- 管理员权限运行；
- WinDivert 驱动加载；
- Windows 防火墙规则管理；
- DLL 注入到本机 32 位 L4D2 进程；
- 对 L4D2/Steamworks/Source UI 相关路径的实验性 hook。

请只在你自己的电脑上使用，只运行你信任的构建。遇到游戏崩溃、VAC/反作弊提示、系统安全软件警告或网络异常时，应立即停止使用。不要把这些工具用于他人设备、他人网络或非授权环境。

## 目录说明

```text
L4D2_Server_Filter_Tool/
  早期外部过滤工具。主要使用 WinDivert、A2S 查询和 Windows 防火墙规则来观察、导出和屏蔽服务器 IP。

l4d2_server_filter_mod_research/
  当前研究和落地主线。包含 hook 分析、匹配路径研究、row filter DLL、Go + Win32 管理器、配置默认值和各类辅助工具。

portable_releases/
  便携版发布目录。每个子目录是一套独立工具，入口 exe 放在对应子目录最外层。
```

## 推荐使用入口

日常使用优先打开：

```text
portable_releases\L4D2_Row_Filter_Manager\L4D2RowFilterManager.exe
```

这个工具用于编辑默认关键字、永久 IP、学习地址和派生地址，并启动游戏或注入已运行的游戏进程。

便携版总说明见：

```text
portable_releases\README_先读我.md
```

## 当前主力方案

当前效果最好的是外部管理器加注入式 row filter：

1. `L4D2_Row_Filter_Manager` 管理规则和启动流程。
2. `matchmaking_row_filter.dll` 注入 32 位 L4D2 进程。
3. 过滤 Steam/Source 服务器列表回调中命中的条目。
4. 自动记录、学习和派生可疑地址，用于下次更早屏蔽。

其中 LAN 局域网服务器列表不再 hook，避免不同 Steamworks 接口签名导致崩溃。

## 构建与测试

Go 模块较多，建议在根目录下逐个模块测试：

```powershell
$mods = Get-ChildItem -Recurse -Filter go.mod | Where-Object {
  $_.FullName -notmatch '\\archive\\' -and $_.FullName -notmatch '\\_windivert_extract\\'
} | ForEach-Object { $_.DirectoryName }

foreach ($m in $mods) {
  Push-Location $m
  go test ./...
  Pop-Location
}
```

Row filter DLL 需要用 MSVC 32 位工具链构建：

```powershell
cd l4d2_server_filter_mod_research\hook_analysis\tools\matchmaking_row_filter_dll
cmd /c '"C:\Program Files (x86)\Microsoft Visual Studio\2022\BuildTools\VC\Auxiliary\Build\vcvars32.bat" >nul && build_msvc_x86.bat'
```

L4D2 是 32 位进程，因此注入器和 DLL 必须保持 x86。GUI 管理器可以是 x64。

## 仓库卫生

以下内容默认不进入仓库：

- 便携版发布产物；
- exe/dll/sys/syso 等构建产物；
- WinDivert 下载包和解压目录；
- 本地日志、报告、缓存、运行配置；
- 本地图片/图标素材；
- 旧 archive 快照。

如果需要更新默认规则，请优先修改：

```text
l4d2_server_filter_mod_research\hook_analysis\tools\matchmaking_row_filter_dll\config_defaults\
```

不要直接提交运行时生成的 `blocked_keywords.txt`、`learned_connectstrings.txt`、`auto_derived_connectstrings.txt` 等本地状态文件。

## 项目状态

当前项目不是 Steam 创意工坊 VPK mod，而是以外部工具和实验性注入过滤为主。VPK mod 方向仍保留在研究文档中，但纯 VPK 很难直接拦截动态服务器列表数据。

