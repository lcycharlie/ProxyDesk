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

loadInitialState();
