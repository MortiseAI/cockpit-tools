# Cockpit 共享数据目录

桌面端和核心库默认都使用 `~/.antigravity_cockpit`。开发版与正式版的账号、加密密钥、配置和网关额度快照共用同一份数据。`COCKPIT_TOOLS_PROFILE` 仍可控制开发窗口标识和诊断行为，但不再选择数据目录。

`COCKPIT_TOOLS_DATA_DIR` 可显式指定其他目录，适用于测试或独立安装。桌面端已有的测试目录环境变量也继续保留。切换开发版与正式版时先退出当前应用，避免两个实例同时管理同一网关。

Stem CLI 等本地集成默认读取：

```text
~/.antigravity_cockpit/codex_local_access_sidecar/quota-pool-state.json
```

## 处理已有的两份数据

如果以前使用过 `~/.antigravity_cockpit_dev`，先决定沿用哪一份完整数据。两份账号文件可能使用不同的密钥，迁移工具不会逐文件混合覆盖。

沿用当前开发版数据时，先预览：

```sh
node scripts/unify-data-dir.cjs --source=dev
```

确认来源后执行：

```sh
node scripts/unify-data-dir.cjs --source=dev --apply
```

工具保留开发版实际存储位置，将原正式版目录完整移入 `~/.cockpit-data-dir-backups/unify-*/`，然后让标准目录 `~/.antigravity_cockpit` 链接到保留的存储位置。两个路径指向同一份实际数据；已有数据库连接、密钥及配置中的绝对路径继续有效。新的程序都从标准目录访问，旧版程序通过旧路径访问的也是同一份数据。**兼容链接的目标目录仍是实际数据，不应作为废弃目录删除。**

如果选择沿用正式版数据，将参数改为 `--source=prod`。这时原开发版目录会完整备份，旧开发目录成为标准目录的兼容链接。

即将被备份替换的那份目录不能有运行中的 Cockpit 实例；工具会检查其 `server.json` 中的进程并拒绝替换。被保留的数据目录可以继续使用。重复执行已完成的迁移不会再次备份或创建另一份数据。

运行结果会打印标准路径、实际存储路径和完整备份路径，不输出账号或密钥内容。需要恢复被备份的另一份数据时，先退出 Cockpit 和网关，移除对应的兼容链接，再将备份目录移回原位置；不要删除被保留的实际存储目录。
