# GitHub 构建与发布

上传 `manager` 内的文件到仓库根目录，保留隐藏目录 `.github/`。不要上传 `data/`、`.qa/` 和本地 EXE；这些路径已加入 `.gitignore`。

## 只编译 EXE

1. 将代码推送到默认分支。
2. 打开 GitHub 的 **Actions → Windows build → Run workflow**。
3. 运行完成后，在该次运行的 **Artifacts** 下载 `HitboxMap-windows-amd64`。

里面包含单独的 EXE、含语言文件的 ZIP，以及 `SHA256SUMS.txt`。普通分支推送和手动运行只上传构建产物，不创建 Release。`main` / `master` 推送和 Pull Request 也会运行测试和编译。

工作流需先存在于默认分支，GitHub 才会提供手动运行入口。[GitHub 手动运行说明](https://docs.github.com/en/actions/how-tos/manage-workflow-runs/manually-run-a-workflow)

## Changelog 写在哪里

写在仓库根目录的 `CHANGELOG.md`。平时将变更记在 `## [Unreleased]` 下。发布前，把本次条目移到对应版本的标题下，例如：

```markdown
## [Unreleased]

## [0.1.0] - 2026-11-04

### Added

- 支持英语、中文和日语。

### Changed

- 语言选择移到顶部栏。
```

版本标题用 `0.1.0`，Git 标签用 `v0.1.0`。提交 Changelog 与代码后，再创建标签：

```powershell
git tag v0.1.0
git push origin v0.1.0
```

工作流会测试、编译，提取 `## [0.1.0]` 到下一个二级标题之间的内容，写入 **Release 草稿**并附上 EXE、ZIP 和校验文件。在仓库的 Releases 中检查草稿，点击 **Publish release** 即可发布。

缺少对应标题、标题重复或内容为空会让工作流失败，避免发布时误用别的版本说明。`v0.1.0-rc.1` 这类标签会生成预发布草稿。重新运行同一个标签可更新尚未发布的草稿；已经发布的 Release 不会被覆盖，请使用新版本号。

Release 草稿中的说明也可以直接在 GitHub 编辑；为了保留记录，建议同步修改 `CHANGELOG.md`。草稿重跑时会重新读取标签提交中的 Changelog。

发布使用 GitHub 自动提供的 `GITHUB_TOKEN`，只在 Release 任务授予 `contents: write`，无需单独配置个人令牌。流程通过 [`gh release create --notes-file`](https://cli.github.com/manual/gh_release_create) 读取发布说明。

## 上传前本地检查

```powershell
./scripts/test.ps1
./scripts/build.ps1 -Package
node scripts/release-notes.mjs --tag v0.1.0
```

最后一条需要先在 `CHANGELOG.md` 中准备该版本的条目。本地校验和构建不会上传 GitHub；Actions 的实际运行结果在上传后查看。
