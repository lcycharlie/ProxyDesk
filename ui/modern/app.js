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

const portSelect = document.querySelector("#portSelect");
const usedPorts = new Set([10000, 10002]);

for (let port = 10000; port <= 10099; port += 1) {
  if (usedPorts.has(port)) {
    continue;
  }
  const option = document.createElement("option");
  option.value = String(port);
  option.textContent = String(port);
  portSelect.appendChild(option);
}

document.querySelector("#refreshEnv").addEventListener("click", () => {
  const envExit = document.querySelector("#envExit");
  const oldText = envExit.textContent;
  envExit.textContent = "检测中...";
  setTimeout(() => {
    envExit.textContent = oldText;
  }, 650);
});
