# 功能设计文档：导出命名自定义 & 主机项目属性

> 版本: v1.0
> 日期: 2026-07-16

---

## 1. 导出文件命名自定义

### 1.1 现状

| 导出功能 | 当前文件名 | 代码位置 |
|---------|-----------|---------|
| 架构图 PDF | `architecture-{timestamp}.pdf` | [Diagram.vue L656](file:///Users/tangran/server_controller_manager/watchtower-frontend/src/views/Diagram.vue#L656) |
| 主机 Excel | `hosts_export.xlsx` | [Hosts.vue L950](file:///Users/tangran/server_controller_manager/watchtower-frontend/src/views/Hosts.vue#L950) |

两个导出都是**固定文件名**，用户无法自定义。

### 1.2 方案

#### 架构图 PDF：导出前弹出命名弹窗

- 点击"PDF"按钮后，弹出一个小弹窗，包含：
  - 文件名输入框（默认值 `architecture-{timestamp}`）
  - "确定"和"取消"按钮
- 用户输入自定义名称后，点击"确定"才执行导出
- 导出的文件名为 `{用户输入的名称}.pdf`

#### 主机 Excel：同机弹出命名弹窗

- 点击"导出 Excel"后，弹出相同样式的命名弹窗
- 默认值 `hosts_export-{timestamp}`
- 确认后导出 `{用户输入的名称}.xlsx`

#### 弹窗 UI 设计

```
┌─────────────────────┐
│  导出文件命名          │
│                      │
│  文件名:              │
│  ┌─────────────────┐  │
│  │ architecture-xxx│  │
│  └─────────────────┘  │
│                      │
│     [取消]   [确定]    │
└─────────────────────┘
```

- 校验：名称不能为空，去掉前后空格
- 已自动添加对应扩展名（`.pdf` / `.xlsx`），用户无需输入扩展名

### 1.3 涉及改动

| 文件 | 改动内容 |
|------|---------|
| `watchtower-frontend/src/views/Diagram.vue` | PDF 按钮点击改为先弹窗，再导出 |
| `watchtower-frontend/src/views/Hosts.vue` | Excel 按钮点击改为先弹窗，再导出 |
| （共享组件）`watchtower-frontend/src/components/ExportDialog.vue` | **新建**：导出命名弹窗通用组件 |

---

## 2. 主机项目属性

### 2.1 现状

`Host` 模型字段：

```
id, ip, hostname, cpu, memory, disk, status, maintenance, last_check_time
```

缺少项目/分组字段，无法将主机归类到同一项目。

### 2.2 方案

#### 数据库层

在 `Host` 表中新增 `project` 字段：

```
`project` TEXT DEFAULT ''  — 所属项目/分组名称
```

- 空字符串表示"未分配"
- 不做外键约束，直接存项目名称字符串
- 迁移：ALTER TABLE 添加列（与现有自动迁移风格一致）

#### API 层

| API | 改动 |
|-----|------|
| `POST /api/hosts` | 请求体新增 `project` 字段（可选） |
| `POST /api/hosts/update` | 支持更新 `project` 字段 |
| `GET /api/hosts/export` | Excel 导出增"项目"列 |
| `GET /api/dashboard` | 返回数据中携带 `project` 字段 |

不需要新增 API，基于现有接口扩展即可。

#### 前端层

**添加主机/编辑主机表单**：

```
┌──────────────────────────┐
│  添加主机                  │
│                          │
│  主机名: [___________]    │
│  IP:    [___________]    │
│  项目:  [___________] ─┐  │
│  CPU:   [___________]  │  │
│  ...                     │
└──────────────────────────┘
```

- **项目字段**：输入框（支持搜索建议）+ 下拉选择
- **搜索建议逻辑**：
  - 输入时，实时匹配已有项目名（从已存在的主机中提取不重复的 project 值）
  - 可选从下拉列表中已有项目，或输入新项目名
  - 类似于"标签输入"体验

**主机列表/卡片展示**：

- 主机卡片上展示项目标签（如果有项目）
- 项目标签可点击筛选：点击后只显示该项目的所有主机
- 项目筛选器（Dropdown）：左侧增加"全部项目"下拉筛选项

**大盘页面**：

- 大盘统计卡片可选按项目维度查看（后续迭代，v1 不做）
- 主机卡片同样展示项目标签

#### 数据库模型改动

```go
type Host struct {
    ID            string    `json:"id"`
    IP            string    `json:"ip"`
    Hostname      string    `json:"hostname"`
    Project       string    `json:"project"`       // ← 新增
    CPU           string    `json:"cpu"`
    Memory        string    `json:"memory"`
    Disk          string    `json:"disk"`
    Status        int       `json:"status"`
    Maintenance   bool      `json:"maintenance"`
    LastCheckTime time.Time `json:"last_check_time"`
}
```

#### 接口示例

```json
// POST /api/hosts 新增字段
{
    "hostname": "web-01",
    "ip": "10.0.0.1",
    "project": "线上商城",          // 新增，可选
    "cpu": "8",
    "memory": "32",
    "disk": "/:100,/data:200"
}

// GET /api/hosts 返回新增字段
{
    "success": true,
    "data": [
        {
            "id": "xxx",
            "hostname": "web-01",
            "ip": "10.0.0.1",
            "project": "线上商城",    // 新增
            "cpu": "8",
            "memory": "32",
            ...
        }
    ]
}

// GET /api/hosts?project=线上商城  （可选，按项目筛选）
```

### 2.3 项目字段的搜索建议实现

前端维护一个**项目名列表**，来源：
1. 页面加载时，从已返回的主机列表中提取不重复的 `project` 值
2. 用户在输入时，列表实时过滤匹配项
3. 输入空值或无匹配项时，输入的内容作为新项目名

### 2.4 涉及改动

| 层级 | 文件 | 改动内容 |
|------|------|---------|
| **数据库** | `internal/store/models.go` | `Host` 结构体新增 `Project string` 字段 |
| **数据库** | `internal/store/sqlite.go` | `migrate()` 中 ALTER TABLE 新增 `project` 列；`InsertHost`、`UpdateHost` 方法适配；`ListHosts` 返回 project 字段 |
| **后端接口** | `internal/handler/dashboard.go` | `handleAddHost`、`handleUpdateHost` 解析 `project` 字段；`handleExport` Excel 新增"项目"列 |
| **前端** | `watchtower-frontend/src/views/Hosts.vue` | 添加/编辑表单新增项目输入框（含搜索建议）；主机卡片展示项目标签；项目筛选器 |
| **前端** | `watchtower-frontend/src/views/Dashboard.vue` | 主机卡片展示项目标签（只读） |

### 2.5 不涉及的改动

- 不新增独立 API（全部复用现有 CRUD 接口）
- 不做独立的"项目管理"页面
- 不做按项目维度的统计卡片（第一期不包含）
- 架构图的节点不自动关联项目（手动拖拽编排，不自动化）

---

## 3. 实施优先级

| 优先级 | 功能 | 预估工作量 |
|--------|------|-----------|
| P0 | 导出命名弹窗（架构图 PDF + 主机 Excel） | 小（约 0.5 天） |
| P1 | 主机项目属性 — 后端（数据库 + API） | 中（约 1 天） |
| P2 | 主机项目属性 — 前端（表单 + 卡片 + 筛选） | 中（约 1 天） |
