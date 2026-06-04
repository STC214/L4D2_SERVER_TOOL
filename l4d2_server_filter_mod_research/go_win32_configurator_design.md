# Go Win32 Configurator Design

## 目标

写一个轻量配置器，用于编辑 Mod 的关键字配置，并和当前外部工具共享过滤规则。

## 核心功能

1. 自动定位 L4D2 目录，默认使用：

```text
E:\SteamLibrary\steamapps\common\Left 4 Dead 2
```

2. 支持手动选择游戏目录。
3. 编辑 `cfg/l4d2_server_filter/keywords.txt`。
4. 保存前自动备份。
5. 校验关键字格式。
6. 可选：生成 loose addon 目录。
7. 可选：调用 `vpk.exe` 打包 VPK。
8. 可选：同步写入当前工具的 `config.json`。

## 避免 Win32 UI 卡死的规则

这些规则应当作为实现硬约束：

- UI 线程只处理窗口消息、控件绘制和轻量状态更新。
- 文件扫描、VPK 打包、A2S 查询、防火墙操作全部放到后台 goroutine。
- 后台 goroutine 不直接操作 Win32 控件；通过 channel 或 `PostMessage` 把结果投递回 UI 线程。
- 长任务必须有取消标志或 context。
- 所有外部命令调用必须设置超时。
- `vpk.exe` 打包期间禁用重复点击按钮，并显示进度/状态。
- 保存配置使用临时文件 + rename，避免中途崩溃写坏配置。
- 写入前检查目标路径是否仍在 addon 工作目录内，避免路径拼接错误覆盖其他文件。

## UI 草案

主窗口：

- 游戏目录输入框 + 浏览按钮。
- 当前 addon 配置路径只读文本。
- 多行关键字编辑框。
- 保存按钮。
- 生成 loose addon 按钮。
- 打包 VPK 按钮。
- 打开 addon 目录按钮。
- 状态栏。

## 配置文件

配置器自身配置：

```json
{
  "game_dir": "E:\\SteamLibrary\\steamapps\\common\\Left 4 Dead 2",
  "addon_dir": "E:\\SteamLibrary\\steamapps\\common\\Left 4 Dead 2\\left4dead2\\addons\\l4d2_server_filter",
  "sync_external_tool_config": true
}
```

## 保存流程

1. UI 线程读取编辑框文本。
2. 解析并规范化关键字。
3. 后台 goroutine 写入临时文件。
4. 校验临时文件可读。
5. 备份旧文件。
6. 原子替换。
7. `PostMessage` 通知 UI 线程刷新状态。

