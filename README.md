# pveRH-mod-server

植物大战僵尸融合版 Mod 服务器，提供 mod 文件浏览、下载和文件夹格式化功能。

## 快速启动

```bash
# 编译
go build -o pveRH-mod-server .

# 启动服务器（默认 :8443）
pveRH-mod-server -dir D:\Downloads\PVERH-MOD -listen :8443

# 指定外网 URL（用于生成下载链接）
pveRH-mod-server -dir D:\Downloads\PVERH-MOD -url https://pve.example.com
```

## CLI 格式化

从夸克网盘下载的 mod 文件夹结构不统一，使用 `-format` 命令标准化：

```bash
# 预览（不实际修改）
pveRH-mod-server -dir D:\Downloads\PVERH-MOD -format=高数羽衫 -dry-run

# 执行格式化
pveRH-mod-server -dir D:\Downloads\PVERH-MOD -format=高数羽衫
```

支持 5 位作者：`高数羽衫` `林秋鲑鱼` `慕容孤晴` `梧萱梦汐` `恒暝`

## API

| 端点 | 说明 |
|---|---|
| `GET /api/authors` | 作者列表 |
| `GET /api/versions` | 版本列表 |
| `GET /api/mods` | mod 列表，支持 `?ver=&author=&type=` 过滤 |
| `GET /api/path?path=&name=` | 下载 mod 文件 |
| `GET /api/formatpath?author=` | 格式化指定作者的 mod 文件夹 |

### 示例

```bash
# 获取所有作者
curl http://localhost:8443/api/authors

# 获取所有版本
curl http://localhost:8443/api/versions

# 获取高数羽衫 3.7 版本的植物MOD
curl "http://localhost:8443/api/mods?author=高数羽衫&ver=3.7&type=植物MOD"

# 下载文件
curl -O "http://localhost:8443/api/path?path=慕容孤晴/慕容孤晴-3.7/植物MOD/(金银钻)杨桃&name=MoneyStar.dll"
```

### 响应格式

`/api/mods` 返回：

```json
[
  {
    "game_ver": "3.7",
    "author": "慕容孤晴",
    "mod_type": "植物MOD",
    "name_cn": "(金银钻)杨桃",
    "file_name": "MoneyStar.dll",
    "url": "http://localhost:8443/api/path?path=慕容孤晴/慕容孤晴-3.7/植物MOD/(金银钻)杨桃&name=MoneyStar.dll"
  }
]
```

## 格式化后的目录结构

```
{作者}/
  └── {作者}-{版本}/
        ├── 植物MOD/{MOD名称}/{MOD文件}
        ├── 僵尸MOD/{MOD名称}/{MOD文件}
        ├── 皮肤MOD/{MOD名称}/{MOD文件}
        ├── 关卡/{关卡文件}
        ├── 修改器/{修改器文件}
        └── 其他/{其他文件}
```

## 项目结构

```
pveRH-mod-server/
├── main.go           # 服务器入口 + API
├── format.go         # 格式化引擎（4 个作者）
├── doc/
│   ├── 项目介绍.md
│   ├── 高数羽衫.md
│   ├── 林秋鲑鱼.md
│   ├── 慕容孤晴.md
│   └── 梧萱梦汐.md
├── go.mod
└── go.sum
```

## 构建

```bash
# Go 1.25+
go build -o pveRH-mod-server .
```
