import { ChangeEvent, FormEvent, startTransition, useEffect, useState } from "react";

type Panel = "workspace" | "backtesting" | "evolution" | "datalab";
type BacktestSource = "champion" | "gene" | "custom";
type SpawnMode = "inherit" | "random_once" | "manual";

type Health = {
  status: string;
  app_role: string;
  time: string;
};

type User = {
  id: number;
  email: string;
  role: string;
  plan: string;
};

type Strategy = {
  ID: number;
  TemplateKey: string;
  Name: string;
  Version: string;
  Manifest?: unknown;
};

type Instance = {
  ID: number;
  Name: string;
  Symbol: string;
  Status: string;
  CapitalQuotaUSDT: number;
  MonthlyInjectUSDT: number;
  ColdSealedAssetQty: number;
  MaxDrawdownPct: number;
  Template?: {
    TemplateKey: string;
    Name: string;
    Version: string;
  };
};

type AgentStatus = {
  connected: boolean;
  version?: string;
  connected_at?: number;
  last_heartbeat_at?: number;
};

type SystemStatus = {
  app_role: string;
  engine_status: string;
  agent: AgentStatus;
  reconcile_required: boolean;
  server_time: number;
};

type DashboardSummary = {
  instance_count: number;
  running_instance_count: number;
  stopped_instance_count: number;
  error_instance_count: number;
  total_equity: number;
  total_usdt_balance: number;
  total_dead_asset_qty: number;
  total_float_asset_qty: number;
  total_cold_sealed_asset_qty: number;
};

type DashboardInstanceOverview = {
  id: number;
  template_id: string;
  template_name: string;
  name: string;
  symbol: string;
  status: string;
  capital_quota_usdt: number;
  monthly_inject_usdt: number;
  configured_cold_sealed_asset_qty: number;
  max_drawdown_pct: number;
  current_price: number;
  usdt_balance: number;
  dead_asset_qty: number;
  float_asset_qty: number;
  cold_sealed_asset_qty: number;
  total_equity: number;
  last_processed_bar_time: number;
  last_synced_at?: number;
  trade_count_total: number;
  trade_count_month: number;
  snapshot_count: number;
  decision_count: number;
  latest_snapshot_time?: number;
  first_decision_time?: number;
  created_at: number;
  updated_at: number;
};

type DashboardOverview = {
  summary: DashboardSummary;
  instances: DashboardInstanceOverview[];
};

type EquitySnapshot = {
  snapshot_time: number;
  source: string;
  usdt_balance: number;
  dead_asset_qty: number;
  float_asset_qty: number;
  cold_sealed_asset_qty: number;
  total_equity: number;
  mark_price: number;
};

type EvolutionTask = {
  ID: number;
  StrategyID: string;
  Symbol: string;
  Status: string;
  ProgressJSON?: unknown;
  ConfigJSON?: unknown;
  ResultGeneID?: number | null;
  ErrorText?: string;
  CreatedAt?: string;
};

type GeneRecord = {
  ID: number;
  StrategyID: string;
  Symbol: string;
  Role: string;
  ParamPack?: unknown;
  ScoreTotal?: number;
  MaxDrawdown?: number;
  WindowScores?: unknown;
  CreatedAt?: string;
};

type BacktestTask = {
  ID: number;
  StrategyID: string;
  Symbol: string;
  Status: string;
  RequestJSON?: unknown;
  ResultJSON?: unknown;
  ErrorText?: string;
  CreatedAt?: string;
};

type CoverageItem = {
  symbol: string;
  interval: string;
  count: number;
  first_open_time: number;
  last_open_time: number;
  last_close: number;
};

type Bar = {
  OpenTime: number;
  Open: number;
  High: number;
  Low: number;
  Close: number;
  Volume: number;
};

type BacktestResult = {
  final_equity?: number;
  total_injected?: number;
  max_drawdown?: number;
  roi?: number;
  trade_count?: number;
  nav?: number[];
};

type EvolutionProgress = {
  generation?: number;
  best_score?: number;
  mutation_prob?: number;
  mutation_scale?: number;
  current_drawdown?: number;
};

type EvolutionConfig = {
  template_id?: string;
  pop_size?: number;
  max_generations?: number;
  spawn_mode?: string;
  spawn_point?: unknown;
};

type EnvelopeChampion = {
  champion?: GeneRecord;
};

type EnvelopeTasks = {
  tasks: EvolutionTask[];
};

type EnvelopeGenomes = {
  genomes: GeneRecord[];
};

type EnvelopeStrategies = {
  strategies: Strategy[];
};

type EnvelopeInstances = {
  instances: Instance[];
};

type EnvelopeCoverage = {
  coverage: CoverageItem[];
};

type EnvelopeBars = {
  bars: Bar[];
};

type EquitySnapshotsEnvelope = {
  instance_id: number;
  range_days: number;
  snapshots: EquitySnapshot[];
};

const defaultApiBase =
  typeof window === "undefined"
    ? "http://localhost:18080"
    : window.localStorage.getItem("quantsaas.apiBase") || "http://localhost:18080";
const defaultToken =
  typeof window === "undefined" ? "" : window.localStorage.getItem("quantsaas.token") || "";

async function apiFetch<T>(
  apiBase: string,
  path: string,
  token: string,
  init?: RequestInit,
): Promise<T> {
  const isFormData = init?.body instanceof FormData;
  const response = await fetch(`${apiBase}${path}`, {
    ...init,
    headers: {
      ...(isFormData ? {} : { "Content-Type": "application/json" }),
      ...(token ? { Authorization: `Bearer ${token}` } : {}),
      ...(init?.headers || {}),
    },
  });

  if (!response.ok) {
    const payload = (await response.json().catch(() => ({}))) as { error?: string };
    throw new Error(payload.error || `Request failed with ${response.status}`);
  }

  return (await response.json()) as T;
}

export default function App() {
  const [activePanel, setActivePanel] = useState<Panel>("workspace");
  const [apiBase, setApiBase] = useState(defaultApiBase);
  const [token, setToken] = useState(defaultToken);
  const [email, setEmail] = useState("you@example.com");
  const [password, setPassword] = useState("change-me");
  const [notice, setNotice] = useState("");
  const [error, setError] = useState("");
  const [loading, setLoading] = useState(false);
  const [refreshKey, setRefreshKey] = useState(0);

  const [health, setHealth] = useState<Health | null>(null);
  const [me, setMe] = useState<User | null>(null);
  const [strategies, setStrategies] = useState<Strategy[]>([]);
  const [instances, setInstances] = useState<Instance[]>([]);
  const [agentStatus, setAgentStatus] = useState<AgentStatus | null>(null);
  const [systemStatus, setSystemStatus] = useState<SystemStatus | null>(null);
  const [dashboardData, setDashboardData] = useState<DashboardOverview | null>(null);
  const [selectedInstanceID, setSelectedInstanceID] = useState<number | null>(() => readInstanceIDFromURL());
  const [equityRangeDays, setEquityRangeDays] = useState<7 | 30 | 90>(30);
  const [equitySnapshots, setEquitySnapshots] = useState<EquitySnapshot[]>([]);

  const [workspaceTemplateID, setWorkspaceTemplateID] = useState("core-btc-v1");
  const [instanceName, setInstanceName] = useState("");
  const [capitalQuota, setCapitalQuota] = useState("10000");
  const [monthlyInject, setMonthlyInject] = useState("500");
  const [coldSealed, setColdSealed] = useState("0");
  const [maxDrawdown, setMaxDrawdown] = useState("0.35");

  const [labTemplateID, setLabTemplateID] = useState("core-btc-v1");
  const [evolutionTasks, setEvolutionTasks] = useState<EvolutionTask[]>([]);
  const [genomes, setGenomes] = useState<GeneRecord[]>([]);
  const [challengers, setChallengers] = useState<GeneRecord[]>([]);
  const [champion, setChampion] = useState<GeneRecord | null>(null);

  const [backtestSource, setBacktestSource] = useState<BacktestSource>("champion");
  const [backtestGeneID, setBacktestGeneID] = useState("");
  const [backtestCapital, setBacktestCapital] = useState("10000");
  const [backtestColdAsset, setBacktestColdAsset] = useState("0");
  const [customParamPack, setCustomParamPack] = useState(
    JSON.stringify(
      {
        chromosome: {
          signal_weight_mean_rev: 0.85,
          signal_weight_momentum: -0.25,
        },
      },
      null,
      2,
    ),
  );
  const [backtestTask, setBacktestTask] = useState<BacktestTask | null>(null);

  const [evolutionPopSize, setEvolutionPopSize] = useState("120");
  const [evolutionGenerations, setEvolutionGenerations] = useState("12");
  const [spawnMode, setSpawnMode] = useState<SpawnMode>("inherit");
  const [manualSpawnPoint, setManualSpawnPoint] = useState(
    JSON.stringify(
      {
        policy: {
          monthly_inject_usdt: 500,
          idle_deploy_deadline_days: 21,
          extra_buy_cap_pct: 0.2,
          bear_extra_buy_cap_pct: 0.1,
          max_release_pct: 0.4,
        },
        risk: {
          taker_fee_bps: 10,
          slippage_bps: 8,
          global_drawdown_guard: 0.65,
        },
      },
      null,
      2,
    ),
  );

  const [dataLabSymbol, setDataLabSymbol] = useState("BTCUSDT");
  const [syncLimit, setSyncLimit] = useState("600");
  const [coverage, setCoverage] = useState<CoverageItem[]>([]);
  const [recentBars, setRecentBars] = useState<Bar[]>([]);
  const [csvFile, setCSVFile] = useState<File | null>(null);

  const signedIn = token.trim() !== "";
  const appRole = systemStatus?.app_role || health?.app_role || "unknown";
  const labEnabled = appRole === "dev" || appRole === "lab";
  const runningEvolutionTask = evolutionTasks.find((task) => task.Status === "running") || null;
  const backtestRunning = backtestTask?.Status === "running";
  const dashboardInstances = dashboardData?.instances || [];
  const selectedDashboardInstance =
    dashboardInstances.find((item) => item.id === selectedInstanceID) || dashboardInstances[0] || null;
  const equityValues = equitySnapshots.map((item) => item.total_equity);

  useEffect(() => {
    if (typeof window !== "undefined") {
      window.localStorage.setItem("quantsaas.apiBase", apiBase);
    }
  }, [apiBase]);

  useEffect(() => {
    if (typeof window !== "undefined") {
      window.localStorage.setItem("quantsaas.token", token);
    }
  }, [token]);

  useEffect(() => {
    void loadOverview();
  }, [apiBase, token, refreshKey]);

  useEffect(() => {
    if (!signedIn || !labEnabled) {
      return;
    }
    void loadLabData(labTemplateID, dataLabSymbol);
  }, [apiBase, token, signedIn, labEnabled, labTemplateID, dataLabSymbol, refreshKey]);

  useEffect(() => {
    if (!signedIn) {
      return;
    }
    const timer = window.setInterval(() => {
      startTransition(() => setRefreshKey((value) => value + 1));
    }, 30_000);
    return () => window.clearInterval(timer);
  }, [signedIn]);

  useEffect(() => {
    if (!signedIn || !labEnabled || !runningEvolutionTask) {
      return;
    }
    const timer = window.setInterval(() => {
      void loadLabData(labTemplateID, dataLabSymbol);
    }, 5_000);
    return () => window.clearInterval(timer);
  }, [signedIn, labEnabled, runningEvolutionTask, labTemplateID, dataLabSymbol, apiBase, token]);

  useEffect(() => {
    if (!signedIn || !labEnabled || !backtestRunning || !backtestTask) {
      return;
    }
    const timer = window.setInterval(() => {
      void refreshBacktestTask(backtestTask.ID);
    }, 3_000);
    return () => window.clearInterval(timer);
  }, [signedIn, labEnabled, backtestRunning, backtestTask, apiBase, token]);

  useEffect(() => {
    if (!signedIn) {
      setEquitySnapshots([]);
      return;
    }
    if (!selectedDashboardInstance) {
      setEquitySnapshots([]);
      return;
    }
    void loadEquitySnapshots(selectedDashboardInstance.id, equityRangeDays);
  }, [signedIn, selectedDashboardInstance?.id, equityRangeDays, refreshKey, apiBase, token]);

  useEffect(() => {
    if (dashboardInstances.length === 0) {
      if (selectedInstanceID !== null) {
        setSelectedInstanceID(null);
      }
      return;
    }

    if (selectedInstanceID && dashboardInstances.some((item) => item.id === selectedInstanceID)) {
      return;
    }

    const fromURL = readInstanceIDFromURL();
    if (fromURL && dashboardInstances.some((item) => item.id === fromURL)) {
      setSelectedInstanceID(fromURL);
      return;
    }

    setSelectedInstanceID(dashboardInstances[0].id);
  }, [dashboardInstances, selectedInstanceID]);

  useEffect(() => {
    writeInstanceIDToURL(selectedInstanceID);
  }, [selectedInstanceID]);

  async function loadOverview() {
    try {
      const nextHealth = await apiFetch<Health>(apiBase, "/healthz", "");
      setHealth(nextHealth);
      setError("");
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to load health status");
      return;
    }

    if (!signedIn) {
      setMe(null);
      setStrategies([]);
      setInstances([]);
      setAgentStatus(null);
      setSystemStatus(null);
      setDashboardData(null);
      setEquitySnapshots([]);
      return;
    }

    setLoading(true);
    setError("");
    try {
      const [mePayload, strategyPayload, instancePayload, nextSystemStatus, nextDashboardData] = await Promise.all([
        apiFetch<{ user: User }>(apiBase, "/api/v1/auth/me", token),
        apiFetch<EnvelopeStrategies>(apiBase, "/api/v1/strategies", token),
        apiFetch<EnvelopeInstances>(apiBase, "/api/v1/instances", token),
        apiFetch<SystemStatus>(apiBase, "/api/v1/system/status", token),
        apiFetch<DashboardOverview>(apiBase, "/api/v1/dashboard", token),
      ]);
      setMe(mePayload.user);
      setStrategies(strategyPayload.strategies);
      setInstances(instancePayload.instances);
      setSystemStatus(nextSystemStatus);
      setAgentStatus(nextSystemStatus.agent);
      setDashboardData(nextDashboardData);
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to load workspace");
    } finally {
      setLoading(false);
    }
  }

  async function loadLabData(templateID: string, symbol: string) {
    if (!signedIn || !labEnabled) {
      return;
    }
    setError("");
    try {
      const [taskPayload, genomePayload, challengerPayload, coveragePayload, recentPayload] =
        await Promise.all([
          apiFetch<EnvelopeTasks>(
            apiBase,
            `/api/v1/evolution/tasks?template_id=${encodeURIComponent(templateID)}`,
            token,
          ),
          apiFetch<EnvelopeGenomes>(
            apiBase,
            `/api/v1/evolution/genomes?template_id=${encodeURIComponent(templateID)}`,
            token,
          ),
          apiFetch<EnvelopeGenomes>(
            apiBase,
            `/api/v1/genome/challengers?template_id=${encodeURIComponent(templateID)}`,
            token,
          ),
          apiFetch<EnvelopeCoverage>(
            apiBase,
            `/api/v1/data-lab/coverage?symbol=${encodeURIComponent(symbol)}`,
            token,
          ),
          apiFetch<EnvelopeBars>(
            apiBase,
            `/api/v1/data-lab/recent?symbol=${encodeURIComponent(symbol)}&limit=24`,
            token,
          ),
        ]);

      setEvolutionTasks(taskPayload.tasks);
      setGenomes(genomePayload.genomes);
      setChallengers(challengerPayload.genomes);
      setCoverage(coveragePayload.coverage);
      setRecentBars(recentPayload.bars);

      try {
        const championPayload = await apiFetch<EnvelopeChampion>(
          apiBase,
          `/api/v1/genome/champion?template_id=${encodeURIComponent(templateID)}`,
          token,
        );
        setChampion(championPayload.champion || null);
      } catch {
        setChampion(null);
      }
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to load lab data");
    }
  }

  async function loadEquitySnapshots(instanceID: number, rangeDays: number) {
    try {
      const payload = await apiFetch<EquitySnapshotsEnvelope>(
        apiBase,
        `/api/v1/dashboard/equity-snapshots?instance_id=${instanceID}&range_days=${rangeDays}`,
        token,
      );
      setEquitySnapshots(payload.snapshots);
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to load equity snapshots");
    }
  }

  async function refreshBacktestTask(id: number) {
    try {
      const nextTask = await apiFetch<BacktestTask>(apiBase, `/api/v1/backtests/${id}`, token);
      setBacktestTask(nextTask);
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to refresh backtest task");
    }
  }

  async function handleLogin(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setLoading(true);
    setError("");
    setNotice("");
    try {
      const payload = await apiFetch<{ token: string; user: User }>(
        apiBase,
        "/api/v1/auth/login",
        "",
        {
          method: "POST",
          body: JSON.stringify({ email, password }),
        },
      );
      setToken(payload.token);
      setMe(payload.user);
      setNotice("登录成功，正在刷新工作台。");
      startTransition(() => setRefreshKey((value) => value + 1));
    } catch (err) {
      setError(err instanceof Error ? err.message : "登录失败");
    } finally {
      setLoading(false);
    }
  }

  function handleLogout() {
    setToken("");
    setMe(null);
    setStrategies([]);
    setInstances([]);
    setAgentStatus(null);
    setSystemStatus(null);
    setDashboardData(null);
    setSelectedInstanceID(null);
    setEquitySnapshots([]);
    setEvolutionTasks([]);
    setGenomes([]);
    setChallengers([]);
    setChampion(null);
    setBacktestTask(null);
    setCoverage([]);
    setRecentBars([]);
    setNotice("本地 token 已清除。");
  }

  async function handleCreateInstance(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setLoading(true);
    setError("");
    setNotice("");
    try {
      const created = await apiFetch<Instance>(apiBase, "/api/v1/instances", token, {
        method: "POST",
        body: JSON.stringify({
          template_id: workspaceTemplateID,
          name: instanceName,
          capital_quota_usdt: Number(capitalQuota),
          monthly_inject_usdt: Number(monthlyInject),
          cold_sealed_asset_qty: Number(coldSealed),
          max_drawdown_pct: Number(maxDrawdown),
        }),
      });
      setNotice("实例已创建。");
      setInstanceName("");
      setSelectedInstanceID(created.ID);
      startTransition(() => setRefreshKey((value) => value + 1));
    } catch (err) {
      setError(err instanceof Error ? err.message : "创建实例失败");
    } finally {
      setLoading(false);
    }
  }

  async function mutateInstance(instanceID: number, action: "start" | "stop" | "delete") {
    setLoading(true);
    setError("");
    setNotice("");
    try {
      await apiFetch(apiBase, `/api/v1/instances/${instanceID}${action === "delete" ? "" : `/${action}`}`, token, {
        method: action === "delete" ? "DELETE" : "POST",
      });
      setNotice(`实例 ${action} 操作已提交。`);
      startTransition(() => setRefreshKey((value) => value + 1));
    } catch (err) {
      setError(err instanceof Error ? err.message : `实例 ${action} 失败`);
    } finally {
      setLoading(false);
    }
  }

  async function handleBacktestSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (!labEnabled) {
      setError("当前 app_role 不开放回测能力。");
      return;
    }

    const payload: Record<string, unknown> = {
      template_id: labTemplateID,
      initial_capital_usdt: Number(backtestCapital),
      initial_cold_asset: Number(backtestColdAsset),
    };

    if (backtestSource === "gene") {
      payload.gene_id = Number(backtestGeneID);
    }
    if (backtestSource === "custom") {
      try {
        payload.param_pack = JSON.parse(customParamPack);
      } catch {
        setError("自定义参数 JSON 解析失败。");
        return;
      }
    }

    setLoading(true);
    setError("");
    setNotice("");
    try {
      const task = await apiFetch<BacktestTask>(apiBase, "/api/v1/backtests", token, {
        method: "POST",
        body: JSON.stringify(payload),
      });
      setBacktestTask(task);
      setNotice("回测任务已启动。");
      setActivePanel("backtesting");
    } catch (err) {
      setError(err instanceof Error ? err.message : "启动回测失败");
    } finally {
      setLoading(false);
    }
  }

  async function handleEvolutionSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (!labEnabled) {
      setError("当前 app_role 不开放进化能力。");
      return;
    }

    const payload: Record<string, unknown> = {
      template_id: labTemplateID,
      pop_size: Number(evolutionPopSize),
      max_generations: Number(evolutionGenerations),
      spawn_mode: spawnMode,
    };
    if (spawnMode === "manual") {
      try {
        payload.spawn_point = JSON.parse(manualSpawnPoint);
      } catch {
        setError("手动 SpawnPoint JSON 解析失败。");
        return;
      }
    }

    setLoading(true);
    setError("");
    setNotice("");
    try {
      await apiFetch<EvolutionTask>(apiBase, "/api/v1/evolution/tasks", token, {
        method: "POST",
        body: JSON.stringify(payload),
      });
      setNotice("进化任务已启动。");
      await loadLabData(labTemplateID, dataLabSymbol);
      setActivePanel("evolution");
    } catch (err) {
      setError(err instanceof Error ? err.message : "启动进化失败");
    } finally {
      setLoading(false);
    }
  }

  async function handlePromote(taskID: number) {
    setLoading(true);
    setError("");
    setNotice("");
    try {
      await apiFetch(apiBase, `/api/v1/evolution/tasks/${taskID}/promote`, token, {
        method: "POST",
      });
      setNotice("challenger 已晋升为 champion。");
      await loadLabData(labTemplateID, dataLabSymbol);
    } catch (err) {
      setError(err instanceof Error ? err.message : "晋升失败");
    } finally {
      setLoading(false);
    }
  }

  async function handleSyncMarketData() {
    setLoading(true);
    setError("");
    setNotice("");
    try {
      await apiFetch(apiBase, "/api/v1/data-lab/sync", token, {
        method: "POST",
        body: JSON.stringify({
          symbol: dataLabSymbol,
          limit: Number(syncLimit),
        }),
      });
      setNotice(`${dataLabSymbol} 历史数据同步完成。`);
      await loadLabData(labTemplateID, dataLabSymbol);
    } catch (err) {
      setError(err instanceof Error ? err.message : "同步历史数据失败");
    } finally {
      setLoading(false);
    }
  }

  async function handleImportCSV(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (!csvFile) {
      setError("请先选择 CSV 文件。");
      return;
    }

    const formData = new FormData();
    formData.append("symbol", dataLabSymbol);
    formData.append("file", csvFile);

    setLoading(true);
    setError("");
    setNotice("");
    try {
      const result = await apiFetch<{ processed_rows: number }>(
        apiBase,
        "/api/v1/data-lab/import-csv",
        token,
        {
          method: "POST",
          body: formData,
        },
      );
      setNotice(`CSV 导入完成，处理了 ${result.processed_rows} 条记录。`);
      setCSVFile(null);
      await loadLabData(labTemplateID, dataLabSymbol);
    } catch (err) {
      setError(err instanceof Error ? err.message : "CSV 导入失败");
    } finally {
      setLoading(false);
    }
  }

  const backtestResult = parseBacktestResult(backtestTask?.ResultJSON);
  const runningProgress = parseObject(runningEvolutionTask?.ProgressJSON) as EvolutionProgress | null;
  const runningConfig = parseObject(runningEvolutionTask?.ConfigJSON) as EvolutionConfig | null;

  return (
    <main className="shell">
      <section className="hero">
        <div>
          <p className="eyebrow">QuantSaaS v1 Workspace</p>
          <h1>总览、曲线、回测和进化已经在同一套壳里连起来了</h1>
          <p className="lede">
            这一版已经有了真实的 dashboard 数据面。你现在可以登录 SaaS、创建实例、看引擎和
            Agent 状态、查看实例权益曲线，同时继续使用回测、GA 和 Data Lab。
          </p>
        </div>
        <div className="hero-meta">
          <label className="field compact">
            <span>API Base</span>
            <input value={apiBase} onChange={(event) => setApiBase(event.target.value)} />
          </label>
          <button className="ghost-button" onClick={() => startTransition(() => setRefreshKey((value) => value + 1))}>
            刷新
          </button>
        </div>
      </section>

      <section className="status-grid">
        <StatusCard label="引擎状态" value={systemStatus?.engine_status || health?.status || "offline"} detail={appRole} />
        <StatusCard
          label="Agent 连接"
          value={agentStatus?.connected ? "online" : "offline"}
          detail={agentStatus?.version || "waiting"}
        />
        <StatusCard
          label="总权益"
          value={formatMoney(dashboardData?.summary.total_equity)}
          detail={`${dashboardData?.summary.running_instance_count || 0} running`}
        />
        <StatusCard
          label="实例数量"
          value={String(dashboardData?.summary.instance_count ?? instances.length)}
          detail={loading ? "refreshing" : "synced"}
        />
      </section>

      {(notice || error) && (
        <section className={`banner ${error ? "banner-error" : "banner-ok"}`}>
          {error || notice}
        </section>
      )}

      <section className="panel-switcher">
        {(
          [
            ["workspace", "工作台"],
            ["backtesting", "回测"],
            ["evolution", "进化"],
            ["datalab", "Data Lab"],
          ] as Array<[Panel, string]>
        ).map(([panel, label]) => (
          <button
            key={panel}
            className={`tab-button ${activePanel === panel ? "tab-button-active" : ""}`}
            onClick={() => setActivePanel(panel)}
          >
            {label}
          </button>
        ))}
      </section>

      {activePanel === "workspace" && (
        <>
          <section className="dashboard-shell">
            <article className="card dashboard-selector">
              <div className="card-heading">
                <div>
                  <h2>Dashboard</h2>
                  <p className="muted">实例总览与选择</p>
                </div>
                <span className="muted">{dashboardInstances.length} instances</span>
              </div>

              {dashboardInstances.length === 0 ? (
                <p className="empty-state">还没有可展示的实例。先创建一个 BTC 或 ETH 核心实例。</p>
              ) : (
                <div className="dashboard-instance-list">
                  {dashboardInstances.map((item) => (
                    <article
                      className={`dashboard-instance-row ${selectedDashboardInstance?.id === item.id ? "dashboard-instance-row-active" : ""}`}
                      key={item.id}
                      onClick={() => setSelectedInstanceID(item.id)}
                    >
                      <div>
                        <p className="instance-label">{item.template_id}</p>
                        <h3>{item.name}</h3>
                        <p className="instance-meta">
                          {item.symbol} · 权益 {formatMoney(item.total_equity)}
                        </p>
                      </div>
                      <div className="instance-actions">
                        <span className={`badge badge-${item.status.toLowerCase()}`}>{item.status}</span>
                        {item.status === "RUNNING" ? (
                          <button
                            className="ghost-button"
                            onClick={(event) => {
                              event.stopPropagation();
                              void mutateInstance(item.id, "stop");
                            }}
                          >
                            停止
                          </button>
                        ) : (
                          <button
                            className="ghost-button"
                            onClick={(event) => {
                              event.stopPropagation();
                              void mutateInstance(item.id, "start");
                            }}
                          >
                            启动
                          </button>
                        )}
                        <button
                          className="danger-button"
                          onClick={(event) => {
                            event.stopPropagation();
                            void mutateInstance(item.id, "delete");
                          }}
                        >
                          删除
                        </button>
                      </div>
                    </article>
                  ))}
                </div>
              )}
            </article>

            <div className="dashboard-main">
              <article className="card">
                <div className="card-heading">
                  <div>
                    <h2>策略概况</h2>
                    <p className="muted">
                      {selectedDashboardInstance
                        ? `${selectedDashboardInstance.template_name} · ${selectedDashboardInstance.symbol}`
                        : "选择一个实例查看详情"}
                    </p>
                  </div>
                  {selectedDashboardInstance && (
                    <span className={`badge badge-${selectedDashboardInstance.status.toLowerCase()}`}>
                      {selectedDashboardInstance.status}
                    </span>
                  )}
                </div>

                {!selectedDashboardInstance ? (
                  <p className="empty-state">当前没有实例可展示。</p>
                ) : (
                  <div className="metric-grid dashboard-metrics">
                    <MetricCard
                      label="总权益"
                      value={formatMoney(selectedDashboardInstance.total_equity)}
                      detail={`价格 ${formatMoney(selectedDashboardInstance.current_price)}`}
                    />
                    <MetricCard
                      label="可用资金"
                      value={formatMoney(selectedDashboardInstance.usdt_balance)}
                      detail={`配额 ${formatMoney(selectedDashboardInstance.capital_quota_usdt)}`}
                    />
                    <MetricCard
                      label="长期底仓"
                      value={formatAsset(selectedDashboardInstance.dead_asset_qty, selectedDashboardInstance.symbol)}
                      detail={`月投 ${formatMoney(selectedDashboardInstance.monthly_inject_usdt)}`}
                    />
                    <MetricCard
                      label="浮动仓位"
                      value={formatAsset(selectedDashboardInstance.float_asset_qty, selectedDashboardInstance.symbol)}
                      detail={`冷封存 ${formatAsset(selectedDashboardInstance.cold_sealed_asset_qty, selectedDashboardInstance.symbol)}`}
                    />
                  </div>
                )}
              </article>

              <article className="card">
                <div className="card-heading">
                  <div>
                    <h2>PnL 净值曲线</h2>
                    <p className="muted">
                      {selectedDashboardInstance
                        ? `${selectedDashboardInstance.name} · ${equitySnapshots.length} points`
                        : "等待实例选择"}
                    </p>
                  </div>
                  <div className="range-switch">
                    {[7, 30, 90].map((range) => (
                      <button
                        className={`range-button ${equityRangeDays === range ? "range-button-active" : ""}`}
                        key={range}
                        onClick={() => setEquityRangeDays(range as 7 | 30 | 90)}
                      >
                        {range}D
                      </button>
                    ))}
                  </div>
                </div>

                <SimpleLineChart values={equityValues} />
                {equitySnapshots.length > 1 && (
                  <div className="chart-summary">
                    <span>起点 {formatMoney(equitySnapshots[0].total_equity)}</span>
                    <span>最新 {formatMoney(equitySnapshots[equitySnapshots.length - 1].total_equity)}</span>
                    <span>
                      区间变化{" "}
                      {formatMoney(
                        equitySnapshots[equitySnapshots.length - 1].total_equity - equitySnapshots[0].total_equity,
                      )}
                    </span>
                  </div>
                )}
              </article>

              <article className="card">
                <div className="card-heading">
                  <div>
                    <h2>策略旅程</h2>
                    <p className="muted">运行节奏、决策与成交的最小画像</p>
                  </div>
                </div>

                {!selectedDashboardInstance ? (
                  <p className="empty-state">当前没有实例可展示。</p>
                ) : (
                  <div className="journey-grid">
                    <MetricCard
                      label="首次创建"
                      value={formatTimestamp(selectedDashboardInstance.created_at)}
                      detail={`首次决策 ${formatTimestamp(selectedDashboardInstance.first_decision_time)}`}
                    />
                    <MetricCard
                      label="累计决策"
                      value={String(selectedDashboardInstance.decision_count)}
                      detail={`快照 ${selectedDashboardInstance.snapshot_count} 条`}
                    />
                    <MetricCard
                      label="本月成交"
                      value={String(selectedDashboardInstance.trade_count_month)}
                      detail={`累计 ${selectedDashboardInstance.trade_count_total} 笔`}
                    />
                    <MetricCard
                      label="最后更新"
                      value={formatTimestamp(selectedDashboardInstance.latest_snapshot_time)}
                      detail={`最近同步 ${formatTimestamp(selectedDashboardInstance.last_synced_at)}`}
                    />
                  </div>
                )}
              </article>
            </div>
          </section>

          <section className="workspace-grid">
            <article className="card">
              <div className="card-heading">
                <h2>身份与连接</h2>
                {signedIn && (
                  <button className="ghost-button" onClick={handleLogout}>
                    清除 Token
                  </button>
                )}
              </div>
              <form className="stack" onSubmit={handleLogin}>
                <label className="field">
                  <span>Email</span>
                  <input value={email} onChange={(event) => setEmail(event.target.value)} />
                </label>
                <label className="field">
                  <span>Password</span>
                  <input
                    type="password"
                    value={password}
                    onChange={(event) => setPassword(event.target.value)}
                  />
                </label>
                <button className="primary-button" type="submit" disabled={loading}>
                  {signedIn ? "重新登录" : "登录 SaaS"}
                </button>
              </form>
              <div className="account-blurb">
                <p>当前用户：{me?.email || "尚未登录"}</p>
                <p>权限：{me?.role || "visitor"}</p>
                <p>套餐：{me?.plan || "none"}</p>
              </div>
            </article>

            <article className="card">
              <div className="card-heading">
                <h2>快速建实例</h2>
                <span className="muted">Bitget 现货 / 1h bar / 纯市价</span>
              </div>
              <form className="stack" onSubmit={handleCreateInstance}>
                <label className="field">
                  <span>模板</span>
                  <select value={workspaceTemplateID} onChange={(event) => setWorkspaceTemplateID(event.target.value)}>
                    <option value="core-btc-v1">core-btc-v1</option>
                    <option value="core-eth-v1">core-eth-v1</option>
                  </select>
                </label>
                <label className="field">
                  <span>实例名</span>
                  <input
                    value={instanceName}
                    onChange={(event) => setInstanceName(event.target.value)}
                    placeholder="比如 BTC Core Sleeve"
                  />
                </label>
                <div className="two-col">
                  <label className="field">
                    <span>资金配额 USDT</span>
                    <input value={capitalQuota} onChange={(event) => setCapitalQuota(event.target.value)} />
                  </label>
                  <label className="field">
                    <span>月度 DCA USDT</span>
                    <input value={monthlyInject} onChange={(event) => setMonthlyInject(event.target.value)} />
                  </label>
                </div>
                <div className="two-col">
                  <label className="field">
                    <span>冷封存资产</span>
                    <input value={coldSealed} onChange={(event) => setColdSealed(event.target.value)} />
                  </label>
                  <label className="field">
                    <span>最大回撤警戒</span>
                    <input value={maxDrawdown} onChange={(event) => setMaxDrawdown(event.target.value)} />
                  </label>
                </div>
                <button className="primary-button" type="submit" disabled={!signedIn || loading}>
                  创建实例
                </button>
              </form>
            </article>
          </section>

          <section className="workspace-grid">
            <article className="card">
              <div className="card-heading">
                <h2>策略模板</h2>
                <span className="muted">已冻结为双模板白名单</span>
              </div>
              <div className="strategy-grid">
                {strategies.map((strategy) => (
                  <div className="strategy-pill" key={strategy.TemplateKey}>
                    <p>{strategy.TemplateKey}</p>
                    <span>
                      {strategy.Name} · {strategy.Version}
                    </span>
                  </div>
                ))}
              </div>
            </article>

            <article className="card">
              <div className="card-heading">
                <h2>LocalAgent</h2>
                <span className="muted">JWT 登录 + WS 长连接 + delta_report</span>
              </div>
              <div className="agent-panel">
                <p>{agentStatus?.connected ? "Agent 已上线，SaaS 可下发命令。" : "Agent 尚未连接。"}</p>
                <p>版本：{agentStatus?.version || "unknown"}</p>
                <p>最近心跳：{formatTimestamp(agentStatus?.last_heartbeat_at)}</p>
                <p>首次连接：{formatTimestamp(agentStatus?.connected_at)}</p>
              </div>
            </article>
          </section>

          <section className="card wide-card">
            <div className="card-heading">
              <h2>实例工作台</h2>
              <span className="muted">实例生命周期已接通</span>
            </div>

            {instances.length === 0 ? (
              <p className="empty-state">还没有实例。先登录，再创建一个 `core-btc-v1` 或 `core-eth-v1`。</p>
            ) : (
              <div className="instance-list">
                {instances.map((item) => (
                  <article className="instance-card" key={item.ID}>
                    <div>
                      <p className="instance-label">{item.Template?.TemplateKey || item.Symbol}</p>
                      <h3>{item.Name}</h3>
                      <p className="instance-meta">
                        {item.Symbol} · 配额 {formatMoney(item.CapitalQuotaUSDT)} · 月投{" "}
                        {formatMoney(item.MonthlyInjectUSDT)}
                      </p>
                    </div>
                    <div className="instance-actions">
                      <span className={`badge badge-${item.Status.toLowerCase()}`}>{item.Status}</span>
                      {item.Status === "RUNNING" ? (
                        <button className="ghost-button" onClick={() => void mutateInstance(item.ID, "stop")}>
                          停止
                        </button>
                      ) : (
                        <button className="ghost-button" onClick={() => void mutateInstance(item.ID, "start")}>
                          启动
                        </button>
                      )}
                      <button className="danger-button" onClick={() => void mutateInstance(item.ID, "delete")}>
                        删除
                      </button>
                    </div>
                  </article>
                ))}
              </div>
            )}
          </section>
        </>
      )}

      {activePanel === "backtesting" && (
        <section className="card wide-card">
          <div className="card-heading">
            <div>
              <h2>回测工作台</h2>
              <p className="muted">当前冠军 / 指定 challenger / 自定义参数包</p>
            </div>
            <LabStatePill enabled={labEnabled} />
          </div>

          {!labEnabled ? (
            <p className="empty-state">当前 app_role 不是 `lab/dev`，回测接口处于关闭状态。</p>
          ) : (
            <div className="stack">
              <form className="stack" onSubmit={handleBacktestSubmit}>
                <div className="two-col">
                  <label className="field">
                    <span>策略模板</span>
                    <select value={labTemplateID} onChange={(event) => setLabTemplateID(event.target.value)}>
                      <option value="core-btc-v1">core-btc-v1</option>
                      <option value="core-eth-v1">core-eth-v1</option>
                    </select>
                  </label>
                  <label className="field">
                    <span>参数来源</span>
                    <select value={backtestSource} onChange={(event) => setBacktestSource(event.target.value as BacktestSource)}>
                      <option value="champion">当前 champion</option>
                      <option value="gene">指定 challenger</option>
                      <option value="custom">自定义 JSON</option>
                    </select>
                  </label>
                </div>

                <div className="two-col">
                  <label className="field">
                    <span>初始资金 USDT</span>
                    <input value={backtestCapital} onChange={(event) => setBacktestCapital(event.target.value)} />
                  </label>
                  <label className="field">
                    <span>初始冷封存资产</span>
                    <input value={backtestColdAsset} onChange={(event) => setBacktestColdAsset(event.target.value)} />
                  </label>
                </div>

                {backtestSource === "gene" && (
                  <label className="field">
                    <span>选择 challenger</span>
                    <select value={backtestGeneID} onChange={(event) => setBacktestGeneID(event.target.value)}>
                      <option value="">请选择 challenger</option>
                      {challengers.map((record) => (
                        <option key={record.ID} value={record.ID}>
                          #{record.ID} · score {formatNumber(record.ScoreTotal)} · dd {formatPct(record.MaxDrawdown)}
                        </option>
                      ))}
                    </select>
                  </label>
                )}

                {backtestSource === "custom" && (
                  <label className="field">
                    <span>自定义 ParamPack JSON</span>
                    <textarea
                      className="code-input"
                      value={customParamPack}
                      onChange={(event) => setCustomParamPack(event.target.value)}
                    />
                  </label>
                )}

                <button className="primary-button" type="submit" disabled={loading}>
                  开始回测
                </button>
              </form>

              {backtestTask && (
                <div className="stack">
                  <div className="metric-grid">
                    <MetricCard label="任务状态" value={backtestTask.Status} detail={`#${backtestTask.ID}`} />
                    <MetricCard
                      label="最终权益"
                      value={formatMoney(backtestResult?.final_equity)}
                      detail={`注资 ${formatMoney(backtestResult?.total_injected)}`}
                    />
                    <MetricCard
                      label="收益率"
                      value={formatPct(backtestResult?.roi)}
                      detail={`回撤 ${formatPct(backtestResult?.max_drawdown)}`}
                    />
                    <MetricCard
                      label="成交次数"
                      value={String(backtestResult?.trade_count || 0)}
                      detail={backtestTask.ErrorText || "latest run"}
                    />
                  </div>

                  <article className="chart-card">
                    <div className="chart-header">
                      <h3>NAV 曲线</h3>
                      <span className="muted">{backtestResult?.nav?.length || 0} points</span>
                    </div>
                    <SimpleLineChart values={backtestResult?.nav || []} />
                  </article>
                </div>
              )}
            </div>
          )}
        </section>
      )}

      {activePanel === "evolution" && (
        <section className="card wide-card">
          <div className="card-heading">
            <div>
              <h2>进化实验室</h2>
              <p className="muted">任务队列、当前 champion、challenger 晋升</p>
            </div>
            <LabStatePill enabled={labEnabled} />
          </div>

          {!labEnabled ? (
            <p className="empty-state">当前 app_role 不是 `lab/dev`，进化接口处于关闭状态。</p>
          ) : (
            <div className="stack">
              <form className="stack" onSubmit={handleEvolutionSubmit}>
                <div className="two-col">
                  <label className="field">
                    <span>策略模板</span>
                    <select value={labTemplateID} onChange={(event) => setLabTemplateID(event.target.value)}>
                      <option value="core-btc-v1">core-btc-v1</option>
                      <option value="core-eth-v1">core-eth-v1</option>
                    </select>
                  </label>
                  <label className="field">
                    <span>Spawn 模式</span>
                    <select value={spawnMode} onChange={(event) => setSpawnMode(event.target.value as SpawnMode)}>
                      <option value="inherit">继承当前最优</option>
                      <option value="random_once">随机探索</option>
                      <option value="manual">手动指定</option>
                    </select>
                  </label>
                </div>

                <div className="two-col">
                  <label className="field">
                    <span>种群大小</span>
                    <input value={evolutionPopSize} onChange={(event) => setEvolutionPopSize(event.target.value)} />
                  </label>
                  <label className="field">
                    <span>最大代数</span>
                    <input value={evolutionGenerations} onChange={(event) => setEvolutionGenerations(event.target.value)} />
                  </label>
                </div>

                {spawnMode === "manual" && (
                  <label className="field">
                    <span>手动 SpawnPoint JSON</span>
                    <textarea
                      className="code-input"
                      value={manualSpawnPoint}
                      onChange={(event) => setManualSpawnPoint(event.target.value)}
                    />
                  </label>
                )}

                <button className="primary-button" type="submit" disabled={loading || !!runningEvolutionTask}>
                  {runningEvolutionTask ? "有任务运行中" : "启动新一轮进化"}
                </button>
              </form>

              {runningEvolutionTask && (
                <article className="chart-card">
                  <div className="chart-header">
                    <h3>当前运行任务</h3>
                    <span className="muted">task #{runningEvolutionTask.ID}</span>
                  </div>
                  <div className="metric-grid">
                    <MetricCard
                      label="当前代"
                      value={String(runningProgress?.generation || 0)}
                      detail={`max ${runningConfig?.max_generations || 0}`}
                    />
                    <MetricCard
                      label="最佳评分"
                      value={formatNumber(runningProgress?.best_score)}
                      detail={`mutation ${formatNumber(runningProgress?.mutation_prob)}`}
                    />
                    <MetricCard
                      label="当前回撤"
                      value={formatPct(runningProgress?.current_drawdown)}
                      detail={`scale ${formatNumber(runningProgress?.mutation_scale)}`}
                    />
                  </div>
                </article>
              )}

              <div className="workspace-grid">
                <article className="card nested-card">
                  <div className="card-heading">
                    <h3>历史任务</h3>
                    <span className="muted">{evolutionTasks.length} tasks</span>
                  </div>
                  <div className="stack compact-stack">
                    {evolutionTasks.map((task) => (
                      <div className="row-card" key={task.ID}>
                        <div>
                          <strong>#{task.ID}</strong>
                          <p className="muted">
                            {task.Status} · {formatTimestamp(task.CreatedAt ? Date.parse(task.CreatedAt) : undefined)}
                          </p>
                        </div>
                        <span className={`badge badge-${task.Status.toLowerCase()}`}>{task.Status}</span>
                      </div>
                    ))}
                  </div>
                </article>

                <article className="card nested-card">
                  <div className="card-heading">
                    <h3>当前 Champion</h3>
                    <span className="muted">{labTemplateID}</span>
                  </div>
                  {champion ? (
                    <div className="stack compact-stack">
                      <MetricCard label="Score" value={formatNumber(champion.ScoreTotal)} detail={`#${champion.ID}`} />
                      <MetricCard
                        label="Max Drawdown"
                        value={formatPct(champion.MaxDrawdown)}
                        detail={champion.Role}
                      />
                      <pre className="code-block">{prettyJSON(champion.ParamPack)}</pre>
                    </div>
                  ) : (
                    <p className="empty-state">当前模板还没有 champion。</p>
                  )}
                </article>
              </div>

              <article className="card nested-card">
                <div className="card-heading">
                  <h3>基因库</h3>
                  <span className="muted">{genomes.length} records</span>
                </div>
                <div className="gene-grid">
                  {genomes.map((record) => {
                    const sourceTask = evolutionTasks.find((task) => task.ResultGeneID === record.ID);
                    return (
                      <div
                        className={`gene-card ${record.Role === "champion" ? "gene-card-champion" : ""}`}
                        key={record.ID}
                      >
                        <div className="card-heading">
                          <strong>#{record.ID}</strong>
                          <span className={`badge badge-${roleToBadge(record.Role)}`}>{record.Role}</span>
                        </div>
                        <p className="muted">
                          score {formatNumber(record.ScoreTotal)} · dd {formatPct(record.MaxDrawdown)}
                        </p>
                        <p className="muted">{formatTimestamp(record.CreatedAt ? Date.parse(record.CreatedAt) : undefined)}</p>
                        <pre className="code-block compact-code">{prettyJSON(record.ParamPack)}</pre>
                        {record.Role === "challenger" && sourceTask && (
                          <button className="ghost-button" onClick={() => void handlePromote(sourceTask.ID)}>
                            晋升为 champion
                          </button>
                        )}
                      </div>
                    );
                  })}
                </div>
              </article>
            </div>
          )}
        </section>
      )}

      {activePanel === "datalab" && (
        <section className="card wide-card">
          <div className="card-heading">
            <div>
              <h2>Data Lab</h2>
              <p className="muted">Bitget 同步、1h CSV 导入、覆盖范围和最近数据预览</p>
            </div>
            <LabStatePill enabled={labEnabled} />
          </div>

          {!labEnabled ? (
            <p className="empty-state">当前 app_role 不是 `lab/dev`，Data Lab 接口处于关闭状态。</p>
          ) : (
            <div className="stack">
              <div className="two-col">
                <label className="field">
                  <span>数据标的</span>
                  <select value={dataLabSymbol} onChange={(event) => setDataLabSymbol(event.target.value)}>
                    <option value="BTCUSDT">BTCUSDT</option>
                    <option value="ETHUSDT">ETHUSDT</option>
                  </select>
                </label>
                <label className="field">
                  <span>同步 K 线数量</span>
                  <input value={syncLimit} onChange={(event) => setSyncLimit(event.target.value)} />
                </label>
              </div>

              <div className="action-row">
                <button className="primary-button" onClick={() => void handleSyncMarketData()} disabled={loading}>
                  从 Bitget 同步历史 K 线
                </button>
              </div>

              <form className="stack" onSubmit={handleImportCSV}>
                <label className="field">
                  <span>导入 1h CSV</span>
                  <input type="file" accept=".csv,text/csv" onChange={handleCSVChange} />
                </label>
                <button className="ghost-button" type="submit" disabled={loading || !csvFile}>
                  导入 CSV
                </button>
              </form>

              <div className="coverage-grid">
                {coverage.map((item) => (
                  <article className="coverage-card" key={`${item.symbol}-${item.interval}`}>
                    <p>{item.symbol}</p>
                    <strong>{item.count.toLocaleString()} bars</strong>
                    <span>{item.interval}</span>
                    <span>
                      {formatTimestamp(item.first_open_time)} → {formatTimestamp(item.last_open_time)}
                    </span>
                    <span>last close {formatMoney(item.last_close)}</span>
                  </article>
                ))}
              </div>

              <article className="chart-card">
                <div className="chart-header">
                  <h3>最近 24 根 K 线</h3>
                  <span className="muted">{dataLabSymbol}</span>
                </div>
                <SimpleLineChart values={recentBars.map((bar) => bar.Close)} />
                <div className="data-table">
                  <div className="data-row data-head">
                    <span>Open Time</span>
                    <span>Open</span>
                    <span>High</span>
                    <span>Low</span>
                    <span>Close</span>
                  </div>
                  {recentBars.slice(-8).map((bar) => (
                    <div className="data-row" key={bar.OpenTime}>
                      <span>{formatTimestamp(bar.OpenTime)}</span>
                      <span>{formatMoney(bar.Open)}</span>
                      <span>{formatMoney(bar.High)}</span>
                      <span>{formatMoney(bar.Low)}</span>
                      <span>{formatMoney(bar.Close)}</span>
                    </div>
                  ))}
                </div>
              </article>
            </div>
          )}
        </section>
      )}
    </main>
  );

  function handleCSVChange(event: ChangeEvent<HTMLInputElement>) {
    const nextFile = event.target.files?.[0] || null;
    setCSVFile(nextFile);
  }
}

function StatusCard({ label, value, detail }: { label: string; value: string; detail: string }) {
  return (
    <article className="status-card">
      <p>{label}</p>
      <strong>{value}</strong>
      <span>{detail}</span>
    </article>
  );
}

function MetricCard({ label, value, detail }: { label: string; value: string; detail: string }) {
  return (
    <article className="metric-card">
      <span>{label}</span>
      <strong>{value}</strong>
      <p>{detail}</p>
    </article>
  );
}

function LabStatePill({ enabled }: { enabled: boolean }) {
  return <span className={`lab-pill ${enabled ? "lab-pill-on" : "lab-pill-off"}`}>{enabled ? "lab/dev enabled" : "lab disabled"}</span>;
}

function SimpleLineChart({ values }: { values: number[] }) {
  if (values.length < 2) {
    return <div className="chart-empty">等待更多数据点…</div>;
  }

  const minValue = Math.min(...values);
  const maxValue = Math.max(...values);
  const range = maxValue - minValue || 1;
  const points = values
    .map((value, index) => {
      const x = (index / Math.max(values.length - 1, 1)) * 100;
      const y = 100 - ((value - minValue) / range) * 100;
      return `${x},${y}`;
    })
    .join(" ");

  return (
    <svg className="sparkline" viewBox="0 0 100 100" preserveAspectRatio="none">
      <polyline points={points} fill="none" stroke="currentColor" strokeWidth="2.2" strokeLinejoin="round" strokeLinecap="round" />
    </svg>
  );
}

function parseObject(value: unknown): Record<string, unknown> | null {
  if (value == null) {
    return null;
  }
  if (typeof value === "string") {
    try {
      return JSON.parse(value) as Record<string, unknown>;
    } catch {
      return null;
    }
  }
  if (typeof value === "object") {
    return value as Record<string, unknown>;
  }
  return null;
}

function parseBacktestResult(value: unknown): BacktestResult | null {
  const objectValue = parseObject(value);
  if (!objectValue) {
    return null;
  }
  return objectValue as BacktestResult;
}

function prettyJSON(value: unknown) {
  if (value == null) {
    return "{}";
  }
  if (typeof value === "string") {
    try {
      return JSON.stringify(JSON.parse(value), null, 2);
    } catch {
      return value;
    }
  }
  return JSON.stringify(value, null, 2);
}

function formatTimestamp(value?: number) {
  if (!value) {
    return "never";
  }
  return new Date(value).toLocaleString();
}

function formatMoney(value?: number) {
  if (typeof value !== "number" || Number.isNaN(value)) {
    return "--";
  }
  return new Intl.NumberFormat("en-US", {
    style: "currency",
    currency: "USD",
    maximumFractionDigits: value >= 1000 ? 0 : 2,
  }).format(value);
}

function formatPct(value?: number) {
  if (typeof value !== "number" || Number.isNaN(value)) {
    return "--";
  }
  return `${(value * 100).toFixed(2)}%`;
}

function formatNumber(value?: number) {
  if (typeof value !== "number" || Number.isNaN(value)) {
    return "--";
  }
  return value.toFixed(4);
}

function formatAsset(value: number | undefined, symbol: string) {
  if (typeof value !== "number" || Number.isNaN(value)) {
    return "--";
  }
  return `${value.toFixed(4)} ${baseAssetFromSymbol(symbol)}`;
}

function roleToBadge(role: string) {
  switch (role) {
    case "champion":
      return "running";
    case "challenger":
      return "stopped";
    default:
      return "deleted";
  }
}

function baseAssetFromSymbol(symbol: string) {
  return symbol.endsWith("USDT") ? symbol.slice(0, -4) : symbol;
}

function readInstanceIDFromURL() {
  if (typeof window === "undefined") {
    return null;
  }
  const raw = new URLSearchParams(window.location.search).get("instance");
  if (!raw) {
    return null;
  }
  const parsed = Number(raw);
  if (!Number.isFinite(parsed) || parsed <= 0) {
    return null;
  }
  return parsed;
}

function writeInstanceIDToURL(instanceID: number | null) {
  if (typeof window === "undefined") {
    return;
  }

  const url = new URL(window.location.href);
  if (instanceID) {
    url.searchParams.set("instance", String(instanceID));
  } else {
    url.searchParams.delete("instance");
  }
  window.history.replaceState({}, "", url.toString());
}
