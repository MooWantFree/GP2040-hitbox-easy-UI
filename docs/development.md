# 开发

以 `manager` 文件夹作为仓库根目录。上传后，GitHub 首页应能直接看到 `go.mod`、`README.md` 和 `.github/workflows/windows.yml`；旁边的 GP2040-CE 固件仓库不需要一起上传。

```text
.github/workflows/      GitHub Actions
cmd/hitbox-map/          程序入口、Windows 图标资源
internal/app/            Go 服务、设备接口、托盘及 Go 单元测试
web/                     HTML、CSS、JavaScript 和嵌入入口
locales/                 三种语言 JSON 和嵌入入口
tests/
  frontend/              前端模型与国际化测试
  scripts/               发布说明提取测试
  integration/           Windows 端口与多实例测试
  helpers/               模拟设备和托盘测试辅助代码
scripts/                 测试、构建与发布说明脚本
tools/icon/              图标生成工具
assets/                  图标源文件
docs/                    开发与发布说明
dist/                    编译产物，忽略提交
data/                    本地布局与备份，忽略提交
.qa/                     本地验证记录和工具缓存，忽略提交
CHANGELOG.md             发布说明
```

Go 单元测试需要访问同包的内部实现，因此按 Go 的约定与源码同放在 `internal/app/`，文件以 `_test.go` 结尾。它们不会进入发布的 EXE。前端测试和独立集成测试集中在 `tests/`。

## 本地构建

使用 PowerShell 7。Go 版本记录在 `.go-version`，Node.js 22 或更新版本用于测试；最终用户运行 EXE 不需要这些工具。

```powershell
./scripts/test.ps1
./scripts/build.ps1 -Package
```

产物为 `dist/HitboxMap.exe`、`dist/HitboxMap-windows-amd64.zip` 和 `dist/SHA256SUMS.txt`。ZIP 包含程序、说明和三份语言 JSON，不包含 `data/`、测试记录或自定义翻译。

构建参数：

```powershell
# 指定 Go 编译器
./scripts/build.ps1 -GoBinary 'C:\Go\bin\go.exe' -Package

# 更新原来放在项目根目录的 EXE，继续使用旁边已有的 data/
./scripts/build.ps1 -OutputDirectory .
```

程序始终读取 EXE 旁的 `data/` 和 `locales/`。更换程序所在文件夹时，请同时迁移这两个文件夹；仅替换同一路径下的 EXE 不需要迁移数据。已运行的旧版本需从托盘退出后再启动新版。

## 资源与图标

Go 标准库服务通过 `go:embed` 嵌入 `web/` 和 `locales/`。前端为原生网页，无运行时外部依赖；Windows 托盘调用系统接口。

图标源文件为 `assets/HitboxMap.ico`，包含 16、24、32、48、64、128、256 像素尺寸。资源文件 `cmd/hitbox-map/icon_windows_amd64.syso` 应提交到仓库，正常构建无需下载图标工具。

修改 `tools/icon/main.go` 后执行：

```powershell
./scripts/build.ps1 -RegenerateIcon -Package
```

重新生成资源会使用固定版本的 [rsrc v0.10.2](https://github.com/akavel/rsrc/tree/v0.10.2)。测试入口和模拟设备见[测试说明](testing.md)，GitHub 构建与版本发布见[发布说明](releasing.md)。
