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
  document.querySelector("#envExit").textContent = state.environmentExit || "-";
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
  document.querySelector("#localProtocolSelect").value = "SOCKS5";
  document.querySelector("#upstreamProtocolSelect").value = "SOCKS5";
  renderRoutes(state.routes || []);
}

function renderRoutes(routes) {
  const body = document.querySelector("#routeTableBody");
  body.textContent = "";
  if (!routes.length) {
    const row = document.createElement("tr");
    row.innerHTML = `<td colspan="6" class="empty-row">暂无转发配置，请先在线路配置里新增。</td>`;
    body.appendChild(row);
    return;
  }
  routes.forEach((route, index) => {
    const row = document.createElement("tr");
    if (index === 0) {
      row.classList.add("selected");
    }
    row.innerHTML = `
      <td><span class="badge ${route.running ? "badge-running" : "badge-idle"}">${route.status}</span></td>
      <td>${route.localAddress}</td>
      <td>${route.localProtocol}</td>
      <td>${route.upstreamProtocol}</td>
      <td>${route.upstreamAddress}</td>
      <td>${route.exitDisplay}</td>
    `;
    body.appendChild(row);
  });
}

async function loadInitialState() {
  if (!backend?.GetInitialState) {
    applyInitialState(fallbackState);
    return;
  }
  try {
    applyInitialState(await backend.GetInitialState());
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

document.querySelector("#refreshEnv").addEventListener("click", async () => {
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
    envExit.textContent = oldText;
  }
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
    });
    renderRoutes(routes);
    if (backend.GetPortOptions) {
      const ports = await backend.GetPortOptions();
      fillSelect(document.querySelector("#portSelect"), ports);
      fillSelect(document.querySelector("#apiPortSelect"), ports);
    }
    document.querySelector('[data-page="routes"]').click();
  } catch (error) {
    window.alert(`新增配置失败：${error}`);
  }
});

loadInitialState();
