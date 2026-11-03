# 测试

在仓库根目录执行 `./scripts/test.ps1`。脚本会运行 Go 测试、`go vet`、前端测试和发布说明提取测试；任一失败都会停止。

也可以分别运行：

```powershell
go test ./...
go vet ./...
node --test (Get-ChildItem tests/frontend,tests/scripts -Filter '*.test.mjs').FullName
```

Go 测试覆盖映射保存、字段保留、冲突与保护检查、备份失败、读回异常、识别结果、布局迁移、摇杆校验、端口选择与重复启动、国际化加载和退出时的请求完成。前端测试覆盖按钮名称对应、布局导入、语言匹配、嵌套错误翻译和切换语言时的数据保留。

## Windows 桌面测试

托盘测试需要交互式 Windows 桌面，默认不在无桌面的 CI 中运行。会短暂显示测试图标和菜单，完成后自动清理：

```powershell
$env:HITBOX_TRAY_SMOKE = '1'
try { go test ./internal/app -run TestWindowsTrayLifecycle -count=1 }
finally { Remove-Item Env:HITBOX_TRAY_SMOKE }
```

编译 EXE 后，可以验证端口占用和实例复用。端口 29301–29303 需处于空闲状态；测试使用 `.qa/` 下的独立数据，并只退出自己创建的进程：

```powershell
./scripts/build.ps1
./tests/integration/ports.ps1
```

## 浏览器验证

模拟设备只监听 `127.0.0.1:8099`，不连接实体设备：

```powershell
node tests/helpers/mock-device.mjs
```

另开一个终端启动测试程序：

```powershell
./dist/HitboxMap.exe -port 8098 -data-dir .qa/browser-data -no-browser
```

打开 `http://127.0.0.1:8098/`，连接 `127.0.0.1:8099`。模拟设备的结果写入 `.qa/go-ui-results.json`，识别队列可通过 `POST http://127.0.0.1:8099/test/captures` 设置，例如 `[[4], [5]]`。测试结束后退出测试托盘，并用 Ctrl+C 停止模拟设备。

浏览器人工检查包括拖放、多选移动与缩放、批量删除、鼠标选择后连续按方向键、识别及覆盖确认、组合输出、配置档复制、保存预览、重载持久化、三种语言与自定义语言、窄屏布局。模拟验证不能代替实体设备测试。
