# GitHub 构建与发布

## 推送后自动发布

将代码提交并推送到 `master`：

```powershell
git push origin master
```

GitHub Actions 会运行测试、编译 Windows x64 程序，然后发布一个 `build-运行编号` 的 Release，例如 `build-2`。发布附件只有 `HitboxMap-windows-amd64.zip`，其中包含 EXE、使用说明和语言文件。下载后先解压，再运行其中的 `HitboxMap.exe`。

GitHub 还会自动显示 Source code 下载链接，那些是仓库源码，不是可运行的程序包。

所有附件上传成功后才会公开发布，并标记为 Latest。测试或编译失败时不会发布。每次推送保留独立的 Release；重跑同一次工作流会继续未完成的草稿，已经发布的文件保持不变。

自动标签指向触发该次构建的提交。工作流使用 [GitHub CLI 的 `--target` 参数](https://cli.github.com/manual/gh_release_create) 固定提交，运行编号采用 [`GITHUB_RUN_NUMBER`](https://docs.github.com/en/actions/reference/workflows-and-actions/variables)。

## Changelog 写在哪里

写在仓库根目录的 `CHANGELOG.md`，平时将变更记在 `## [Unreleased]` 下。推送 `master` 时，这部分会作为 Release 说明，并附上提交和构建链接。

保留 `Unreleased` 标题即可；内容为空时使用简短的默认说明。工作流不会自动清空或改写 Changelog。

## 使用版本号发布

标签在项目目录的终端中用 `git tag` 指定。需要 `v0.1.0` 这样的版本号时，先在 `CHANGELOG.md` 中把本次条目移到对应版本的标题下；标题不带 `v`：

```markdown
## [Unreleased]

## [0.1.0] - 2026-11-04

### Added

- 支持英语、中文和日语。

### Changed

- 语言选择移到顶部栏。
```

提交 Changelog 与代码后，在仓库目录执行以下命令，将 `v0.1.0` 换成本次版本号：

```powershell
git tag v0.1.0
git push origin v0.1.0
```

`git tag` 将标签指向当前提交，推送该标签后才会触发版本发布。下次发布使用新标签，例如 `v0.1.1`。普通 `master` 推送仍会另外生成 `build-运行编号` 版本。

工作流会测试、编译，提取 `## [0.1.0]` 下的内容并自动发布 Release，附件与普通推送相同。缺少对应标题、标题重复或内容为空会让工作流失败。`v0.1.0-rc.1` 这类标签会发布为 Prerelease，不标记为 Latest。

## 只编译 EXE

打开 **Actions → Windows build → Run workflow**。运行完成后，在该次运行的 **Artifacts** 下载 `HitboxMap-windows-amd64`，解压后可找到程序 ZIP。手动运行和 Pull Request 只测试、编译并上传产物，不创建 Release。

发布使用 GitHub 自动提供的 `GITHUB_TOKEN`，只在 Release 任务授予 `contents: write`，无需配置个人令牌。本地 `data/`、`.qa/` 和 EXE 已加入 `.gitignore`，无需提交。

## 本地检查

```powershell
./scripts/test.ps1
./scripts/build.ps1 -Package
node scripts/release-notes.mjs --unreleased
```

检查指定版本的说明时，改用 `node scripts/release-notes.mjs --tag v0.1.0`。
