这张图讲的核心方法是：**利用 Google Trends（谷歌趋势）从一个种子词出发，通过「相关查询 → 发现新词 → 扩大时间范围 → 再次查询」不断挖掘有增长趋势的关键词。**

它不是简单地找搜索量最大的词，而是找：

1. 哪些词正在变热？
2. 哪些新需求正在出现？
3. 哪些关键词背后存在尚未被满足的用户需求？
4. 哪些趋势值得做成产品、内容或者 AI 应用？

我把它拆成一个可以直接执行的工作流。

## **一、先理解整个找词逻辑**

**种子词（Seed Keyword）**

例如：AI Agent

**Google Trends → Related Queries**

发现相关搜索词、上升词

**筛选值得关注的新词**

增长速度、持续时间、用户需求

**将新词作为下一轮种子词**

继续扩展相关查询

**循环迭代**

这个过程本质上是一个关键词图谱的扩展过程。

例如：

- AI Agent
  - AI coding agent
  - Browser agent
  - Computer use agent
  - Personal AI assistant
- Browser agent
  - Browser automation
  - Web agent
  - AI web research
- AI coding agent
  - Coding agent benchmark
  - Claude Code
  - OpenHands

每发现一个有价值的新词，就可以继续向下挖掘。

## **二、实际操作：一步一步怎么找**

### **第 1 步：打开 Google Trends**

进入：

[Google Trends – Explore](https://trends.google.com/trends/explore?utm_source=chatgpt.com)

先设置三个条件：

| 参数   | 建议设置                  |
| ---- | --------------------- |
| 地区   | Worldwide（全球）或目标市场    |
| 时间   | Past 7 days / 30 days |
| 搜索类型 | Web Search            |
| 搜索对象 | 一个种子词                 |

例如输入：

**Seed Keyword**

示例

# **AI Agent**

搜索入口：Google Trends Explore

注意：如果 Google 下拉框同时提供 Topic 和 Search term，优先考虑 Topic。Topic 能覆盖同一概念的不同语言和表达方式；Search term 更适合研究具体词组。

[image](https://www.google.com/s2/favicons?domain=https://newsinitiative.withgoogle.com\&sz=32)

Google News Initiative+1



### **第 2 步：找到 Related Queries**

在搜索结果页面向下滚动，找到：

[How to Compare Terms on Google Trends: Comprehensive Guide](https://images.openai.com/static-rsc-4/M_GHf4Lbp9kF_UD5kZSfiXykHghCnUCRR--auOorV0-A_481AXwMyz9jJVptoBmAJznyEsb_0I8-Jp_K7HuztXscmJlhA2IS19t6xkJdjIn3lEMP6i74dTA-159jqyIa3Qo_ENoROj4rGoA0HEPm-5ePjRK24wioz7V7_NBHzqc?purpose=inline)

[sem1.spyessentials.ai](https://sem1.spyessentials.ai/blog/google-trends-compare/?utm_source=chatgpt.com)

这里最重要的是两个选项：

| 选项     | 含义             | 用途    |
| ------ | -------------- | ----- |
| Top    | 当前搜索量相对较高的相关查询 | 找成熟需求 |
| Rising | 最近增长最快的相关查询    | 找新兴机会 |

**找新词，重点看 Rising。**

例如，假设你输入 AI Agent，看到：

| 相关词（假设示例）             | 发现价值      |
| --------------------- | --------- |
| AI coding agent       | AI 编程需求   |
| Browser agent         | 浏览器自动化需求  |
| Computer use          | 计算机操作能力需求 |
| Personal AI assistant | 个人助理需求    |

这些是说明方法的示例，不是本次实时查询结果。

如果看到 `Breakout`，代表该词的搜索兴趣相较之前增长超过 5000%，但不代表它一定有很大的绝对搜索量。

[image](https://www.google.com/s2/favicons?domain=https://support.google.com\&sz=32)

Trends Help+1



### **第 3 步：把新词作为下一轮种子词**

这是整个方法最重要的地方。

例如第一轮发现 Browser Agent：

再次在 Google Trends 输入 Browser Agent。

然后继续看它的 Related Queries → Rising。

这样可能发现更具体的需求：

- Browser automation
- AI web research
- Web browsing agent
- Browser use API
- Website interaction automation

然后再对这些词分别做第二轮查询。

这个过程可以理解为：

**关键词扩展树**

AI Agent

Browser Agent

Browser Automation

Web Research Agent

Browser Use API

Coding Agent

AI Code Review

Coding Agent Benchmark

Autonomous Coding

示意图：每个节点都可以成为下一轮查询的种子词。

### **第 4 步：验证这个词是不是真的在增长**

截图中特别强调了一个容易忽略的问题：

**不要把一个老词的短期波动误认为新趋势。**

对于发现的每一个词，建议依次切换：

1. Past 30 days：近期是否增长？
2. Past 90 days：是否持续增长？
3. Past 12 months：是不是季节性波动？
4. Past 5 years：是不是长期趋势？

例如：

- 30 天上涨，90 天也上涨：值得关注。
- 30 天上涨，12 个月没有明显变化：可能只是短期事件。
- 30 天上涨，但过去 5 年一直在增长：可能是长期赛道，而不是刚出现的新机会。

这里要注意，Google Trends 的数据是归一化指数，不是绝对搜索次数。不同时间窗口的指数也不能直接当作搜索量比较。

[image](https://www.google.com/s2/favicons?domain=https://newsinitiative.withgoogle.com\&sz=32)

Google News Initiative+1



## **三、怎么判断一个词值得做？**

我建议不要只看增长率，而是建立一个筛选机制。

| 维度     | 判断问题              | 目的     |
| ------ | ----------------- | ------ |
| 增长性    | 近期是否快速增长？         | 找机会    |
| 持续性    | 90 天、12 个月是否仍有趋势？ | 排除噪声   |
| 需求具体性  | 是否表达了明确的问题？       | 找真实需求  |
| 产品化可能性 | 是否能做成软件、工具或服务？    | 判断商业价值 |
| 竞争情况   | 是否已经有大量成熟产品？      | 判断进入难度 |

举个例子：

| 关键词             | 产品化方向        |
| --------------- | ------------ |
| AI Agent        | 太宽泛，需要继续拆分   |
| Browser Agent   | 浏览器自动化工具     |
| AI Web Research | AI 研究工作台     |
| Coding Agent    | AI 编程工具      |
| Agent Memory    | Agent 长期记忆服务 |

这就从单纯找词，变成了寻找产品机会。

## **四、截图里还有两个很重要的技巧**

### **1. 不要只看 Related Queries，也要看 Related Topics**

Google Trends 下面还有一个区域：

- Related Queries：用户具体搜索了什么词。
- Related Topics：用户关注的是什么主题。

两者的价值不同：

举个例子：

假设你研究 AI Agent。

Related Queries 可能发现：

- AI Agent framework
- AI Agent tutorial
- AI Agent builder

Related Topics 可能发现：

- Artificial intelligence
- Software development
- Automation

前者帮助你发现用户的具体搜索表达，后者帮助你理解需求所属的主题。

### **2. 不要只停留在一个关键词上**

假设你发现：

`AI Agent Memory`

不要马上认为这是一个产品机会。

继续研究：

- AI Agent Memory
- Agent Long Term Memory
- Agent Persistent Memory
- LLM Memory
- AI Memory Management

你要搞清楚用户到底在寻找什么，以及这些词是否指向同一个需求。

## **五、把整个过程做成一张表**

建议用 Excel 或 Google Sheets 记录每一轮的结果。

| 字段                  | 记录内容           |
| ------------------- | -------------- |
| Seed Keyword        | 原始种子词          |
| Discovered Keyword  | 新发现的词          |
| Country             | 搜索地区           |
| Discovery Date      | 发现日期           |
| 7D Trend            | 7 天趋势          |
| 30D Trend           | 30 天趋势         |
| 90D Trend           | 90 天趋势         |
| 12M Trend           | 年度趋势           |
| Search Intent       | 用户搜索意图         |
| Product Opportunity | 潜在产品机会         |
| Status              | 待验证 / 已验证 / 放弃 |

你可以按照这个流程持续积累关键词，而不是每次重新开始。

## **六、能不能自动化？**

可以。截图最后提到的就是这个方向。

Google 在 2025 年推出了 Google Trends API Alpha，提供程序化访问趋势数据的能力，包括时间序列、地区数据和多关键词比较。不过目前 API 仍是有限范围的 Alpha 测试，并非所有人都能直接使用。

[image](https://www.google.com/s2/favicons?domain=https://developers.google.com\&sz=32)

Google for Developers+1



可以设计一个自动化系统：

**Seed Keywords**

人工输入初始关键词

**Google Trends Collector**

获取 Top / Rising / 时间序列 / 地区数据

**Keyword Expansion**

去重、分类、发现新关键词、扩展下一轮种子

**Trend Analysis + AI**

判断增长、持续性、需求和产品化机会

**Keyword Database**

持续积累关键词图谱，定期重新监控

## **七、我建议你真正这样开始**

不要一开始就开发自动化系统。

先手动跑一轮，理解数据的价值。

如果目标是发现 AI 领域的一人产品机会，我建议从这五个种子词开始：

1. AI Agent
2. AI Coding
3. AI Workspace
4. AI Automation
5. AI Personal Assistant

每个种子词向下扩展两轮，每轮记录 Top 和 Rising。

**最终目标不是收集 1000 个关键词，而是从 1000 个关键词中找到 5～10 个值得进一步验证的真实需求。**

有一个关键认知：Google Trends 告诉你的是用户搜索兴趣的变化，不直接告诉你用户愿不愿意付钱。还需要结合 Reddit 讨论、竞品、产品评论和真实用户需求验证。

你可以从这里开始：

[打开 Google Trends 开始找词](https://trends.google.com/trends/explore?utm_source=chatgpt.com)。
