# Changelog

## 1.2.0 - 2026-04-17

**Breaking:** 移除实例 `keywords` 字段

- **数据模型：** `browser_profiles` 表通过 SQLite migration v8 删除 `keywords` 列；Go `Profile` / `ProfileInput` 结构体同步移除 `Keywords` 字段；前端 TypeScript 类型同步清理。
- **前端 UI：** 实例编辑页、实例列表页、`KeywordsModal` 弹窗、"搜索关键字值"筛选器、以及实例表格中的关键字列均已移除。
- **后端方法：** `BrowserProfileSetKeywords` Wails 绑定方法已删除。
- **`/api/launch` 接口：** 不再识别 `key` / `keyword` / `keywords` 选择器参数，传入时会被静默忽略（不返回 400）。使用 `tag` / `tags` / `groupId` 替代。
- **迁移建议：** 如果你依赖 `keywords` 字段的值，请在升级前先导出备份。升级后请改用 `tags` 和 `groupId` 来对实例分类和选择。

## 1.1.0 - 2026-03-19

- 完善 Linux 支持：补齐 Linux 环境下的开发、打包、安装、启动与运行链路，并持续修复安装版启动与退出稳定性问题。
- 新增 SOCKS 代理测试支持：SOCKS 代理能力已进入测试阶段，后续会继续验证稳定性与兼容性。
- 实验性支持接口触发浏览器：支持通过接口启动浏览器实例，便于后续接入自动化流程。
