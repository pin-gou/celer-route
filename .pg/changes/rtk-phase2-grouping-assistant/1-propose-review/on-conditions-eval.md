# on_conditions 评估记录

> 本文件由 `pg-gen-tasks-skeleton.py` 自动生成。
> LLM 在 review 阶段对每条规则的「机械评估」进行复核，给出最终决策 + 依据。
> 复核完成后，把「最终决策」列同步到 review-notes.md 的「on_conditions 评估记录」段。

**机械评估列说明**：
- `path`：基于 affected_paths 的 glob 匹配（来自 proposal.md 提取）
- `semantic`：基于 proposal.md 全文的关键词匹配
- `建议`：path 或 semantic 任一命中 → 命中

## scenario_tracks_decision (v3.6)

**SSOT**：`pg-gen-manifest.py` 和 `pg-gen-scenario.py` 都读此段决定是否生成对应产物。
修改本段会立即让三个产物（tasks.md / execution-manifest.yaml / scenario-<track>.yaml）不一致。
如需变更，**重跑** `pg-gen-tasks-skeleton.py --scenario-decisions ...` + `pg-gen-manifest.py` + `pg-gen-scenario.py`，禁止手工编辑。

| track_id | enabled | mode | reason |
|---|---|---|---|
| scr | **true** | explicit | 跨 role 协作验证? 是——V-plugins-5 需经 pg-gateway-api 注入请求并经 logs-db 观察压缩效果；新 API 端点? 否；跨模块联调? 是——配置解析→PreLLMHook→压缩→日志落库横跨 plugins 与 transports 模块 |

## stage 级

## track 级

---

**LLM review 操作指引**：

1. 对每行「最终决策」勾选 `[x]`（同意机械评估）或 `[~]` + 写「依据」（覆盖机械评估）
2. 复核完成后，把本文件表格内容**合并到** `.pg/changes/<change>/1-propose-review/review-notes.md` 的「on_conditions 评估记录」段
3. scenario_tracks_decision 段是三个生成产物（tasks.md / execution-manifest.yaml / scenario-<track>.yaml）的 SSOT，禁止手工修改
4. 合并后本文件可保留作为审计副本
