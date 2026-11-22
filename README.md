# seo

从 Google Trends 出发，按「种子词 → 相关查询 → 趋势验证 → 机会评估」挖掘有增长趋势的关键词。

## 能力

- 种子扩展：相关查询 Top / Rising / Breakout，自然键去重，深度 ≤ 2
- 趋势判定：7D / 30D / 90D / 12M / 5Y 交叉验证
- 信号分析：动量、加速度、尖峰比、季节强度、结构增长、意图、品牌占比、语言偏移
- 机会评估：外部证据 + 准入闸门（趋势合格 / 不过宽 / 至少一条证据）
- 两种数据源：确定性 Fake；chromedp 驱动本机 Chrome（可连接已运行实例）

## 快速开始

```bash
make            # 启动可调试 Chrome（9222）并使用真实数据
# 或
make run-fake   # 纯本地假数据，不启动浏览器
```

打开 http://127.0.0.1:8080，按 ①～⑥ 逐步操作。

## 结构

```
cmd/seotrend/                    入口与依赖组装
internal/discovery/domain/hotsearch   关键词聚合、趋势判定、信号
internal/discovery/application        用例（command / query / support）
internal/discovery/ports              外部网关接口
internal/evaluation/                  机会评估上下文
internal/infrastructure/              SQLite、内存、Trends 适配器
internal/interfaces/web/              服务端渲染控制台
docs/                                 业务规则与调研
```

## 验证

```bash
make test   # go test ./...
make build  # bin/seotrend
```
