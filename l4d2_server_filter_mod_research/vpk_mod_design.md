# VPK Mod Design

## 设计原则

1. VPK 只承担 addon 能合法承担的内容：资源、配置、说明、可覆盖 UI。
2. 过滤关键字配置使用简单文本格式，方便手动编辑和 Go 工具编辑。
3. IP 屏蔽不放进 VPK，由外部工具应用到 Windows 防火墙。
4. VPK 中的任何 UI 都不能暗示它会注入或修改游戏进程。

## 候选文件结构

```text
left4dead2/
  addons/
    l4d2_server_filter/
      addoninfo.txt
      cfg/
        l4d2_server_filter/
          keywords.txt
          keywords.example.txt
          README.txt
      resource/
        ui/
          server_filter_notes.res
```

实际打包为 VPK 时，根目录应从 addon 内容根开始：

```text
addoninfo.txt
cfg/l4d2_server_filter/keywords.txt
cfg/l4d2_server_filter/keywords.example.txt
cfg/l4d2_server_filter/README.txt
resource/ui/server_filter_notes.res
```

## 关键字格式

`keywords.txt`：

```text
# one keyword per line
# empty lines and lines starting with # are ignored
rpg
vip
shop
```

解析规则建议与当前工具保持一致：

- trim 空白。
- 忽略空行。
- 忽略 `#` 开头的注释。
- 大小写不敏感。
- 对服务器名做简单标准化后匹配。

## IP 屏蔽策略

VPK 不直接存 IP 黑名单，因为服务器 IP 会变化，且 VPK 不能可靠阻止随机匹配。建议：

1. 关键字由配置器写入 `keywords.txt`。
2. 外部工具读取同一份关键字。
3. 外部工具从 Steam master/A2S/游戏网络观察中解析服务器名和 IP。
4. 命中关键字后，把当前有效 IPv4 写入屏蔽记录。
5. 由 Windows 防火墙阻断入站和出站 UDP。

## 大网段误伤防护

导出或应用 IP 屏蔽时必须保留现有安全规则：

- 不把 `*.0.0`、`*.0.0.0`、`0.0.0.0`、广播地址、私有异常地址当作普通服务器 IP 批量屏蔽。
- 端口不是 5 位数时，不作为有效命中服务器处理。
- IP 命中必须尽量关联到服务器名或 A2S 信息；未知候选服务器可以标记，但应用规则前需要走专门的安全校验。

## VPK 构建

可使用游戏自带 `vpk.exe`：

```text
E:\SteamLibrary\steamapps\common\Left 4 Dead 2\bin\vpk.exe
```

构建输入应是 addon 内容目录。后续配置器可以提供“生成 VPK”按钮，也可以默认使用 loose addon 目录，方便调试和编辑。

