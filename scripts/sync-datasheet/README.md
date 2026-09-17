# sync-datasheet — 定价/模型参数数据表维护

celer-route 的定价与模型参数数据表（datasheet）来自上游，分四个副本存放，本脚本负责让它们保持一致：

```
framework/modelcatalog/datasheet/fallback/datasheet.json         # 二进制内嵌兜底（定价）
framework/modelcatalog/datasheet/fallback/model-parameters.json.gz # 二进制内嵌兜底（模型参数，gzip）
website/static/datasheet/datasheet.json                          # GitHub Pages 发布（网关实际拉取）
website/static/datasheet/model-parameters.json                   # GitHub Pages 发布
```

## 数据来源（优先级从高到低）

1. **现有内置数据**（`framework/.../fallback/`）——celer-route 手工维护的条目（opencode-zen、wafer、runware 等）以及历次同步结果。**冲突时永远以现有数据为准**，上游只补空缺、不覆盖。
2. **LiteLLM** `model_prices_and_context_window.json` —— 开源、持续更新，覆盖绝大多数 provider。按 `scripts/sync-datasheet/sync-datasheet.py` 里的 `PROVIDER_MAP` 把 LiteLLM provider 名映射到 celer-route 的规范 provider 名后合入。
3. **`--custom` 手工条目** —— 用于 LiteLLM 完全没有的 celer-route 独家 provider，优先级最高。仓库已带 `custom.json`：
   - `zhipu` 21 条（价格取自 <https://docs.z.ai/guides/overview/pricing>）
   - `siliconflow` 11 条（价格取自 <https://siliconflow.com/pricing>，官方定价页仅公布这些特色模型，全量目录需 API key 枚举后再扩充）
   - `baidu` 6 条（`qianfan.cloud.baidu.com/price`，¥ 按 **7.0 汇率**换算 USD，与 minimax 既有条目口径一致）
   - `stepfun` 4 条（`platform.stepfun.com/docs/zh/guides/pricing/details.md`）
   - `minimax_cn` 3 条（`platform.minimaxi.com/docs/guides/pricing-paygo.md`）
   - `yi`、`coze` 暂缺：01.AI 已转向企业服务不再公开按量 API 定价；Coze 文档全客户端渲染且无公开按 token 价格表，两者需按实例用定价覆盖。
   - **汇率约定**：人民币计价源统一按 ¥→$ 7.0 换算（与现有 `minimax` 条目隐含汇率一致）；`source` 字段记录原始页，供复核。

## 用法

```bash
# 联网拉取 LiteLLM 主文件，补齐缺失 provider（默认目标见 DEFAULT_TARGETS）
make sync-datasheet

# 离线：使用本地 LiteLLM 快照（例如之前下载的 model_prices_and_context_window.json）
make sync-datasheet LITELLM_LOCAL=/path/to/model_prices.json

# 只补指定 provider / 预览不落盘
python3 scripts/sync-datasheet/sync-datasheet.py --only-providers azure_ai,nvidia
python3 scripts/sync-datasheet/sync-datasheet.py --dry-run

# 携带手工条目
python3 scripts/sync-datasheet/sync-datasheet.py --litellm-local /tmp/p.json --custom custom.json
```

跑完后建议验证（Go 侧能正确解析产物）：

```bash
cd framework && go test ./modelcatalog/datasheet/... -count=1
```

## 需要手工条目（无上游来源）的 provider

`PROVIDER_MAP` 中映射为空的 provider（当前：`alibaba_tokenplan`、`opencode`、`runware`、`wafer`）需要手工编写条目，格式与现有 `fallback/datasheet.json` 一致，并通过 `--custom` 合入：

```json
{
  "runware/foo-model": {
    "input_cost_per_token": 0.000001,
    "output_cost_per_token": 0.000002,
    "provider": "runware",
    "base_model": "foo-model",
    "mode": "chat",
    "source": "https://runware.ai/pricing"
  }
}
```

## 注意事项

- **provider 命名**：条目里的 `provider` 字段必须是 celer-route 规范 provider 名（`core/schemas/bifrost.go` 的 `ModelProvider`），因为加载时 `normalizeProvider` 会再归一化一次。已知坑：`bedrock_mantle` 会被 `normalizeProvider` 按子串折叠到 `bedrock`（与既有行为一致）。
- **幂等**：脚本只做增量合入，重复运行无变化。
- **gzip 确定性**：`model-parameters.json.gz` 用 `mtime=0` 压缩，多次生成字节一致。
- 网关侧的 24h 同步间隔是「拉取频率」，不是本数据的发布频率；数据更新靠重新跑本脚本并提交。
