# L4D2 组服务器过滤器

这是一个深色主题的 Go + Win32 外部管理器。过滤核心仍由 `matchmaking_row_filter.dll` 提供，GUI 负责配置、启动、注入和查看日志。

## 安全警告

本工具会以管理员权限运行，并会把本项目内的 DLL 注入到本机 L4D2 进程中；部分配套分析工具还会加载 WinDivert 驱动来观察或过滤本机网络流量。请只在你自己的电脑上使用，只使用你信任的发行包，不要在 VAC/反作弊敏感环境中误用，也不要把它用于拦截、干扰或分析他人的设备与网络。

如果只是想正常游戏，请关闭本工具并恢复默认网络状态。遇到游戏崩溃、网络异常、反作弊提示或系统安全软件警告时，应立即停止使用并移除注入/驱动相关组件。

## 功能

- 编辑屏蔽关键字。
- 编辑永久屏蔽 IP / IP:端口。
- 将手动输入的 IP / IP:端口置顶加入永久屏蔽列表。
- 将手动输入的关键字置顶加入关键字列表。
- 编辑学习地址。
- 编辑自动派生地址。
- 恢复默认种子规则。
- 启动 L4D2 并尽早注入过滤核心。
- 对已经运行的游戏执行注入。
- 只读取日志尾部，避免 UI 被大日志拖死。

## Go + Win32 防卡死检查

- 窗口过程不执行 PowerShell、不等待进程、不扫描大文件、不 sleep。
- 耗时任务在 goroutine 中执行，并带超时。
- 工作线程不直接调用 `SetWindowText`、`EnableWindow` 等 UI API。
- 工作线程只通过 `PostMessage` 和小状态标记回到 UI 线程。
- 日志框只读取 `matchmaking_row_filter.log` 的尾部。
- 启动、注入、恢复按钮在任务运行时会禁用。
- 子 PowerShell 进程隐藏窗口并设置超时。
- 配置保存使用临时文件和原子替换。
- DLL 的自动派生配置写入不发生在 Steam 回调路径上。
- 深色主题区分背景、面板、输入框和文本颜色，避免元素糊在一起。

## 使用

双击：

```text
L4D2RowFilterManager.exe
```

程序启动时会申请管理员权限。

推荐流程：

1. 点击 `恢复默认`。
2. 检查或编辑规则。
3. 点击 `保存配置`。
4. 点击 `启动并注入`。
5. 等待游戏主界面完成组服务器后台刷新。
6. 重新进入组服务器列表检查效果。
7. 点击 `刷新日志` 查看 `auto-derived`、`disguised_default_large_pool`、`ServerResponded dropped` 等关键行。

## 构建

```powershell
rsrc.exe -manifest .\app.manifest -ico .\assets\app_icon.ico -o .\rsrc.syso
go build -ldflags="-H windowsgui" -o L4D2RowFilterManager.exe .
```
