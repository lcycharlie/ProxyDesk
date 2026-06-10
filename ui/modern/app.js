const pages = {
  dashboard: document.querySelector("#page-dashboard"),
  config: document.querySelector("#page-config"),
  routes: document.querySelector("#page-routes"),
  settings: document.querySelector("#page-settings"),
};

document.querySelectorAll(".nav-item").forEach((button) => {
  button.addEventListener("click", () => {
    document.querySelectorAll(".nav-item").forEach((item) => item.classList.remove("active"));
    Object.values(pages).forEach((page) => page.classList.remove("active"));
    button.classList.add("active");
    pages[button.dataset.page].classList.add("active");
    document.querySelector("h1").textContent = button.textContent.replace(/[0-9]/g, "").trim();
  });
});

document.querySelectorAll(".settings-tab").forEach((button) => {
  button.addEventListener("click", () => {
    document.querySelectorAll(".settings-tab").forEach((item) => item.classList.remove("active"));
    document.querySelectorAll(".settings-panel").forEach((panel) => panel.classList.remove("active"));
    button.classList.add("active");
    document.querySelector(`#settings-${button.dataset.settings}`).classList.add("active");
  });
});

const backend = window.go?.main?.App;
let selectedRouteIndex = -1;

const fallbackState = {
  appName: "ProxyDesk",
  localIP: "192.168.31.140",
  environmentExit: "点击刷新检测",
  portStart: "10000",
  portEnd: "10099",
  portOptions: Array.from({ length: 100 }, (_, index) => String(10000 + index)),
  countries: ["美国 (US)", "日本 (JP)", "尼日利亚 (NG)"],
  cities: ["全部城市", "California", "Tokyo", "Lagos"],
  localProtocols: ["HTTP/HTTPS", "SOCKS5"],
  upstreamProtocols: ["HTTP", "SOCKS5"],
  routes: [],
};

function fillSelect(select, values) {
  select.textContent = "";
  values.forEach((value) => {
    const option = document.createElement("option");
    option.value = value;
    option.textContent = value;
    select.appendChild(option);
  });
}

function applyInitialState(state) {
  document.querySelector("#envExit").textContent = state.environmentExit || "检测中...";
  document.querySelector("#localIP").textContent = state.localIP || "-";
  document.querySelector("#listenHost").value = state.localIP || "-";
  document.querySelector("#portStartInput").value = state.portStart || "10000";
  document.querySelector("#portEndInput").value = state.portEnd || "10099";
  fillSelect(document.querySelector("#portSelect"), state.portOptions || []);
  fillSelect(document.querySelector("#apiPortSelect"), state.portOptions || []);
  fillSelect(document.querySelector("#countrySelect"), state.countries || []);
  fillSelect(document.querySelector("#citySelect"), state.cities || []);
  fillSelect(document.querySelector("#localProtocolSelect"), state.localProtocols || []);
  fillSelect(document.querySelector("#upstreamProtocolSelect"), state.upstreamProtocols || []);
  fillSelect(document.querySelector("#apiLocalProtocolSelect"), state.localProtocols || []);
  fillSelect(document.querySelector("#apiUpstreamProtocolSelect"), state.upstreamProtocols || []);
  document.querySelector("#localProtocolSelect").value = "SOCKS5";
  document.querySelector("#upstreamProtocolSelect").value = "SOCKS5";
  document.querySelector("#apiLocalProtocolSelect").value = "SOCKS5";
  document.querySelector("#apiUpstreamProtocolSelect").value = "HTTP";
  renderRoutes(state.routes || []);
}

function selectedOrRunningRoute(routes) {
  if (!routes.length) {
    return null;
  }
  return (
    routes.find((route) => route.index === selectedRouteIndex) ||
    routes.find((route) => route.running) ||
    routes[0]
  );
}

function updateSummary(routes) {
  const selectedRoute = selectedOrRunningRoute(routes || []);
  const hasRunning = (routes || []).some((route) => route.running);
  const status = document.querySelector("#summaryStatus");
  status.textContent = hasRunning ? "运行中" : "未启动";
  status.classList.toggle("badge-running", hasRunning);
  status.classList.toggle("badge-idle", !hasRunning);

  document.querySelector("#summaryExitIP").textContent = selectedRoute?.exitDisplay || "-";
  document.querySelector("#summaryLocalProtocol").textContent = selectedRoute?.localProtocol || "-";
  document.querySelector("#summaryUpstreamProtocol").textContent = selectedRoute?.upstreamProtocol || "-";
  document.querySelector("#dashboardExit").textContent = selectedRoute?.exitDisplay || "-";
  document.querySelector("#dashboardUpstream").textContent = selectedRoute?.upstreamAddress || "-";
  document.querySelector("#dashboardError").textContent = "-";
}

function renderRoutes(routes) {
  const body = document.querySelector("#routeTableBody");
  body.textContent = "";
  if (!routes.length) {
    selectedRouteIndex = -1;
    updateSummary([]);
    const row = document.createElement("tr");
    row.innerHTML = `<td colspan="6" class="empty-row">暂无转发配置，请先在线路配置里新增。</td>`;
    body.appendChild(row);
    return;
  }
  let selectedApplied = false;
  routes.forEach((route, index) => {
    const row = document.createElement("tr");
    row.dataset.index = String(route.index);
    if (route.index === selectedRouteIndex || (!selectedApplied && selectedRouteIndex < 0 && index === 0)) {
      row.classList.add("selected");
      selectedRouteIndex = route.index;
      selectedApplied = true;
    }
    row.innerHTML = `
      <td><span class="badge ${route.running ? "badge-running" : "badge-idle"}">${route.status}</span></td>
      <td>${route.localAddress}</td>
      <td>${route.localProtocol}</td>
      <td>${route.upstreamProtocol}</td>
      <td>${route.upstreamAddress}</td>
      <td>${route.exitDisplay}</td>
    `;
    row.addEventListener("click", () => {
      selectedRouteIndex = route.index;
      document.querySelectorAll("#routeTableBody tr").forEach((item) => item.classList.remove("selected"));
      row.classList.add("selected");
      updateSummary(routes);
    });
    body.appendChild(row);
  });
  if (!selectedApplied && body.firstElementChild) {
    body.firstElementChild.classList.add("selected");
    selectedRouteIndex = Number(body.firstElementChild.dataset.index);
  }
  updateSummary(routes);
}

async function refreshPorts() {
  if (!backend?.GetPortOptionsForRange) {
    return;
  }
  const ports = await backend.GetPortOptionsForRange(
    document.querySelector("#portStartInput").value,
    document.querySelector("#portEndInput").value,
  );
  fillSelect(document.querySelector("#portSelect"), ports);
  fillSelect(document.querySelector("#apiPortSelect"), ports);
}

async function refreshLogs() {
  if (!backend?.GetLogs) {
    return;
  }
  const logs = await backend.GetLogs();
  document.querySelector("#logBox").textContent = logs || "暂无运行日志";
}

async function afterRouteAction(routes) {
  renderRoutes(routes || []);
  await refreshPorts();
  await refreshLogs();
}

async function runRouteAction(action, emptyMessage) {
  if (selectedRouteIndex < 0) {
    window.alert(emptyMessage || "请先选择一条转发配置");
    return;
  }
  try {
    await afterRouteAction(await action(selectedRouteIndex));
  } catch (error) {
    window.alert(error);
    await refreshLogs();
  }
}

async function loadInitialState() {
  if (!backend?.GetInitialState) {
    applyInitialState(fallbackState);
    return;
  }
  try {
    applyInitialState(await backend.GetInitialState());
    await refreshLogs();
    await refreshEnvironmentExit(false);
  } catch (error) {
    console.error(error);
    applyInitialState(fallbackState);
  }
}

document.querySelector("#countrySelect").addEventListener("change", async (event) => {
  if (!backend?.CitiesForCountry) {
    return;
  }
  fillSelect(document.querySelector("#citySelect"), await backend.CitiesForCountry(event.target.value));
});

document.querySelector("#countrySearchInput").addEventListener("input", async (event) => {
  if (!backend?.FilterCountries) {
    return;
  }
  const countries = await backend.FilterCountries(event.target.value);
  fillSelect(document.querySelector("#countrySelect"), countries);
  if (countries.length && backend.CitiesForCountry) {
    fillSelect(document.querySelector("#citySelect"), await backend.CitiesForCountry(countries[0]));
  }
});

async function refreshEnvironmentExit(keepOldValue = true) {
  const envExit = document.querySelector("#envExit");
  const oldText = envExit.textContent;
  envExit.textContent = "检测中...";
  if (!backend?.RefreshEnvironmentExit) {
    setTimeout(() => {
      envExit.textContent = oldText;
    }, 650);
    return;
  }
  try {
    envExit.textContent = await backend.RefreshEnvironmentExit();
  } catch (error) {
    console.error(error);
    envExit.textContent = keepOldValue ? oldText : "检测失败";
  }
}

document.querySelector("#refreshEnv").addEventListener("click", () => refreshEnvironmentExit(true));

document.querySelector("#windowMinimise")?.addEventListener("click", () => {
  window.runtime?.WindowMinimise?.();
});

document.querySelector("#windowToggleMaximise")?.addEventListener("click", () => {
  window.runtime?.WindowToggleMaximise?.();
});

document.querySelector("#windowClose")?.addEventListener("click", () => {
  window.runtime?.Hide?.();
});

document.querySelector("#addRouteBtn").addEventListener("click", async (event) => {
  event.preventDefault();
  if (!backend?.AddManualRoute) {
    renderRoutes([
      {
        status: "未启动",
        running: false,
        localAddress: `${document.querySelector("#listenHost").value}:${document.querySelector("#portSelect").value}`,
        localProtocol: document.querySelector("#localProtocolSelect").value,
        upstreamProtocol: document.querySelector("#upstreamProtocolSelect").value,
        upstreamAddress: "global.rpip.lokiproxy.com:35001",
        exitDisplay: "-",
      },
    ]);
    return;
  }
  try {
    const routes = await backend.AddManualRoute({
      localProtocol: document.querySelector("#localProtocolSelect").value,
      upstreamProtocol: document.querySelector("#upstreamProtocolSelect").value,
      localPort: document.querySelector("#portSelect").value,
      proxyLine: document.querySelector("#proxyLineInput").value,
      portStart: document.querySelector("#portStartInput").value,
      portEnd: document.querySelector("#portEndInput").value,
    });
    await afterRouteAction(routes);
    document.querySelector('[data-page="routes"]').click();
  } catch (error) {
    window.alert(`新增配置失败：${error}`);
  }
});

document.querySelector("#startRouteBtn").addEventListener("click", () => {
  runRouteAction((index) => backend.StartRoute(index), "请先新增一条转发配置");
});

document.querySelector("#stopRouteBtn").addEventListener("click", () => {
  runRouteAction((index) => backend.StopRoute(index));
});

document.querySelector("#deleteRouteBtn").addEventListener("click", () => {
  runRouteAction((index) => backend.DeleteRoute(index));
});

document.querySelector("#testRouteBtn").addEventListener("click", () => {
  runRouteAction((index) => backend.TestRouteExit(index));
});

document.querySelector("#stopAllBtn").addEventListener("click", async () => {
  if (!backend?.StopAllRoutes) {
    return;
  }
  await afterRouteAction(await backend.StopAllRoutes());
});

document.querySelector("#enableSystemProxyBtn").addEventListener("click", async () => {
  if (selectedRouteIndex < 0) {
    window.alert("请先选择一条转发配置");
    return;
  }
  try {
    await backend.EnableSystemProxy(selectedRouteIndex);
    await refreshLogs();
  } catch (error) {
    window.alert(error);
  }
});

document.querySelector("#disableSystemProxyBtn").addEventListener("click", async () => {
  try {
    await backend.DisableSystemProxy();
    await refreshLogs();
  } catch (error) {
    window.alert(error);
  }
});

document.querySelector("#fetchProviderBtn").addEventListener("click", async () => {
  if (!backend?.FetchProviderIP) {
    return;
  }
  try {
    const routes = await backend.FetchProviderIP({
      countryLabel: document.querySelector("#countrySelect").value,
      city: document.querySelector("#citySelect").value,
      localProtocol: document.querySelector("#apiLocalProtocolSelect").value,
      upstreamProtocol: document.querySelector("#apiUpstreamProtocolSelect").value,
      localPort: document.querySelector("#apiPortSelect").value,
      endpoint: document.querySelector("#apiEndpointInput").value,
      countryParam: document.querySelector("#apiCountryParamInput").value,
      cityParam: document.querySelector("#apiCityParamInput").value,
      jsonKey: document.querySelector("#apiJSONKeyInput").value,
      portStart: document.querySelector("#portStartInput").value,
      portEnd: document.querySelector("#portEndInput").value,
    });
    await afterRouteAction(routes);
    window.alert("API 获取成功，已加入转发列表。");
    document.querySelector('[data-page="routes"]').click();
  } catch (error) {
    window.alert(`API 获取失败：${error}`);
    await refreshLogs();
  }
});

document.querySelector("#clearLogsBtn").addEventListener("click", async () => {
  if (!backend?.ClearLogs) {
    document.querySelector("#logBox").textContent = "暂无运行日志";
    return;
  }
  document.querySelector("#logBox").textContent = await backend.ClearLogs() || "暂无运行日志";
});

document.querySelector("#portStartInput").addEventListener("change", refreshPorts);
document.querySelector("#portEndInput").addEventListener("change", refreshPorts);

loadInitialState();
