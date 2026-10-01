# 跨夜班次本地考勤核算工具

纯 Go 标准库实现的命令行考勤核算工具。逐行读取 JSON 请求、逐行返回 JSON
结果，数据保存在本地目录的单个 JSON 文件中（写入时临时文件 + 原子替换）。

## 构建与运行

```bash
go build ./cmd/attendance
./attendance -dir ./data
```

- `-dir`：本地数据目录，默认 `./data`，首次运行自动创建。
- 标准输入每行一个 JSON 请求，标准输出每行一个 JSON 响应。
- 单行出错返回 `{"ok":false,"error":"..."}`，不影响后续行。
- 每次成功写操作立即落盘，重开程序后结果一致。

## 时间与边界约定

- 所有时间为北京时间，格式 `YYYY-MM-DD HH:mm`；日期参数格式 `YYYY-MM-DD`。
- 所有时间区间**左闭右开** `[start, end)`，因此首尾相接的两个区间不重叠。
- 班次可跨午夜，但起止不得相同、止不得早于起。
- 报告按**班次开始时刻的北京日期**归属；跨夜班只产生一条出勤记录。
- 日期筛选含起止两端（`start_date <= 班次开始日期 <= end_date`）。

## 请求格式

每行形如：`{"op":"<操作>","payload":<参数>}`

### 1. 登记员工 `register_employee`

```json
{"op":"register_employee","payload":{"id":"e1","name":"张三"}}
```

### 2. 批量导入排班 `import_shifts`

```json
{"op":"import_shifts","payload":[
  {"id":"s1","employee_id":"e1",
   "start":"2026-10-01 22:00","end":"2026-10-02 06:00",
   "breaks":[{"start":"2026-10-02 02:00","end":"2026-10-02 02:30"}]}
]}
```

- `breaks` 可省略；必须完整落在班次区间内，重叠休息导入时自动取并集。
- 同一员工的班次区间不得重叠（含同批内检查）。

### 3. 批量导入请假 `import_leaves`

```json
{"op":"import_leaves","payload":[
  {"id":"l1","employee_id":"e1","start":"2026-10-02 03:00","end":"2026-10-02 04:00"}
]}
```

### 4. 批量导入打卡 `import_punches`

```json
{"op":"import_punches","payload":[
  {"id":"p1","employee_id":"e1","shift_id":"s1","kind":"in","at":"2026-10-01 22:00"},
  {"id":"p2","employee_id":"e1","shift_id":"s1","kind":"out","at":"2026-10-02 06:00"}
]}
```

- `kind` 只能是 `in`（进入）或 `out`（离开）。
- 打卡归属的员工必须与班次所属员工一致。
- 打卡时刻不要求落在班次内（班外打卡允许，用于暴露异常）。

### 5. 报告 / 明细查询 `report`、`detail`

```json
{"op":"report","payload":{"employee_id":"e1","start_date":"2026-10-01","end_date":"2026-10-31"}}
```

- 三个字段均可省略：省略员工表示全员，省略日期表示不限。
- `detail` 与 `report` 返回相同的逐班明细；字段即“汇总 + 明细”。

成功响应：

```json
{"ok":true,
 "shifts":[ { ...逐班明细...} ],
 "employees":[ { ...员工汇总...} ]}
```

员工汇总字段：`shift_count`（班次数）、`required_minutes`、`work_minutes`、
`late_minutes`、`early_minutes`、`absent_minutes`（各类分钟合计）、
`missing_card_shifts`（缺卡班次数）、`anomaly_records`（异常记录条数）。

## 导入与冲突规则

- 拒绝：未知员工、未知班次、非法时间字符串、止早于起、起止相同。
- 拒绝：重复 ID 但内容不同（`already exists with different content`）。
- 批量导入是原子的：任一条失败，整批不落任何记录。
- 相同记录（所有业务字段一致）重复导入不增加记录，返回 `added` 为实际新增数。

## 核算规则

1. 班次区间减去休息并集，再减去与工作区间重叠的请假并集，得到
   `required_intervals`（应出勤区间）与 `required_minutes`。
2. 多张请假重叠只抵扣一次；整班请假时应出勤为 0，该班仍出现在报告中。
3. 打卡按时刻、再按 ID 排序后逐一配对成在场区间；同一分钟进出计 0 分钟。
4. 出现以下任一情况标记 `missing_card` 并在 `anomalies` 中列出具体记录：
   开头就是离开（`out_without_in`）、连续进入（`consecutive_in`）、
   连续离开（`out_without_in`）、最后缺少离开（`in_without_out`）。
5. 缺卡班次不猜测补卡时间、不参与分钟汇总：工作/迟到/早退均为 0，
   应出勤全部计入缺勤；异常计入 `missing_card`。
6. 完全没有打卡**不算**缺卡，按无在场时间处理（全部缺勤）。
7. 完整配对时，在场区间与应出勤区间求交得到有效出勤（`work_minutes`），
   不计算加班。
8. 应出勤区间中：早于首次有效出勤的部分为迟到（`late_minutes`），
   晚于最后有效出勤的部分为早退（`early_minutes`），
   中间未覆盖部分为缺勤（`absent_minutes`）。
9. 恒有 `work_minutes + late_minutes + early_minutes + absent_minutes
   = required_minutes`。

## 报告字段

- `shift_id` / `employee_id` / `date`：班次标识、员工、归属日期。
- `shift_start` / `shift_end`：原始班次时刻。
- `breaks` / `leave_deductions` / `required_intervals`：休息、实际请假抵扣、
  应出勤区间（均为合并后的规范区间）。
- `required_minutes`：应出勤分钟。
- `punches`：该班次原始打卡（按时刻、ID 排序）。
- `presence_intervals`：配对出的在场区间。
- `anomalies`：缺卡异常，含原因与对应打卡 ID、类型、时刻。
- `missing_card`：是否缺卡。
- `work_minutes` / `late_minutes` / `early_minutes` / `absent_minutes`：
  四类分钟汇总。

## 完整调用示例

```bash
go run ./cmd/attendance -dir ./data <<'EOF'
{"op":"register_employee","payload":{"id":"e1","name":"张三"}}
{"op":"import_shifts","payload":[{"id":"s1","employee_id":"e1","start":"2026-10-01 22:00","end":"2026-10-02 06:00","breaks":[{"start":"2026-10-02 02:00","end":"2026-10-02 02:30"}]}]}
{"op":"import_punches","payload":[{"id":"p1","employee_id":"e1","shift_id":"s1","kind":"in","at":"2026-10-01 22:05"},{"id":"p2","employee_id":"e1","shift_id":"s1","kind":"out","at":"2026-10-02 06:00"}]}
{"op":"report","payload":{"employee_id":"e1","start_date":"2026-10-01","end_date":"2026-10-01"}}
EOF
```

跨夜班按 2026-10-01 归属，应出勤 450 分钟，迟到 5 分钟，有效出勤 445 分钟。

## 测试

```bash
go test ./...
```

覆盖：跨夜班、休息/请假重叠只抵扣一次、乱序打卡、各类缺卡、
整班请假、班外打卡、完全无打卡、批量原子性、重复导入幂等、
重开后结果一致与多员工多班汇总。
