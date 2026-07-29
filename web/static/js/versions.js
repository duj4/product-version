let loadedProducts = []
let isLoading = false
let pendingRenderFrame = 0

const viewState = {
  query: "",
  status: "all"
}

document.addEventListener("DOMContentLoaded", () => {
  document.getElementById("refreshButton").addEventListener("click", () => loadVersions(true))
  document.getElementById("searchInput").addEventListener("input", event => {
    viewState.query = normalizeText(event.target.value)
    scheduleProductTableRender()
  })
  document.getElementById("statusFilter").addEventListener("change", event => {
    viewState.status = event.target.value
    renderProductTable()
  })

  renderLoading()
  loadVersions(false)
})

async function loadVersions(isRefresh) {
  if (isLoading) return

  setLoadingState(true)
  hideError()

  try {
    const response = await fetch("/api/versions", {
      cache: "no-store",
      headers: {
        Accept: "application/json"
      }
    })
    const data = await response.json().catch(() => ({}))

    if (!response.ok) {
      throw new Error(data.error || `Request failed (${response.status})`)
    }

    loadedProducts = Array.isArray(data?.products) ? data.products : []
    renderDashboard()

    if (isRefresh) {
      showToast("Version inventory refreshed", "success")
    }
  } catch (error) {
    const message = error?.message || "Failed to load versions"
    renderError(message)
    if (isRefresh) {
      showToast(message, "error")
    }
  } finally {
    setLoadingState(false)
  }
}

function renderDashboard() {
  renderStats()
  renderProductTable()
  document.getElementById("lastUpdated").textContent = new Intl.DateTimeFormat(undefined, {
    hour: "2-digit",
    minute: "2-digit",
    second: "2-digit"
  }).format(new Date())
}

function renderStats() {
  const deployments = loadedProducts.flatMap(runtimeDeployments)
  const healthyRuntimes = deployments.filter(item => item?.status === "ok").length
  const attentionProducts = loadedProducts.filter(productHasAttention).length
  const sourceErrors = loadedProducts.reduce((count, product) => count + countSourceErrors(product), 0)

  document.getElementById("productCountStat").textContent = loadedProducts.length
  document.getElementById("runtimeHealthyStat").textContent = healthyRuntimes
  document.getElementById("runtimeHealthySub").textContent = `of ${deployments.length} deployments`
  document.getElementById("attentionStat").textContent = attentionProducts
  document.getElementById("sourceErrorsStat").textContent = sourceErrors
}

function renderProductTable() {
  const products = filteredProducts()
  const total = loadedProducts.length
  const summary = products.length === total
    ? `${total} ${total === 1 ? "product" : "products"}`
    : `${products.length} of ${total} products`

  document.getElementById("resultSummary").textContent = summary

  if (products.length === 0) {
    renderEmptyState(total)
    return
  }

  document.getElementById("versionsBody").innerHTML = products
    .map(renderProductRow)
    .join("")
}

function scheduleProductTableRender() {
  if (pendingRenderFrame) {
    window.cancelAnimationFrame(pendingRenderFrame)
  }

  pendingRenderFrame = window.requestAnimationFrame(() => {
    pendingRenderFrame = 0
    renderProductTable()
  })
}

function filteredProducts() {
  return loadedProducts
    .filter(product => {
      if (viewState.status === "attention" && !productHasAttention(product)) return false
      if (viewState.status === "healthy" && productHasAttention(product)) return false
      if (!viewState.query) return true
      return productSearchText(product).includes(viewState.query)
    })
    .sort((left, right) => productName(left).localeCompare(productName(right), undefined, {
      sensitivity: "base"
    }))
}

function renderProductRow(product) {
  const metadata = product?.metadata || {}
  const sources = product?.sources || {}
  const qa = deploymentFor(product, "qa")
  const prod = deploymentFor(product, "prod")
  const name = productName(product)
  const initial = (name || "?").trim().charAt(0).toUpperCase() || "?"
  const hue = hashHue(product?.key || name)

  return `
<tr class="product-row align-top">
  <td class="px-5 py-5">
    <div class="flex items-start gap-3">
      <div class="product-monogram flex h-10 w-10 shrink-0 items-center justify-center rounded-xl text-sm font-black text-white"
           style="--hue:${hue}" aria-hidden="true">
        ${escapeHtml(initial)}
      </div>
      <div class="min-w-0">
        <div class="truncate font-bold text-slate-900" title="${escapeHtml(name)}">${escapeHtml(name)}</div>
        <div class="mt-1 flex flex-wrap items-center gap-1.5">
          <span class="rounded-md bg-slate-100 px-1.5 py-0.5 font-mono text-[10px] font-semibold text-slate-500">${escapeHtml(product?.key || "-")}</span>
          <span class="rounded-md bg-blue-50 px-1.5 py-0.5 text-[10px] font-bold uppercase tracking-wider text-blue-600">${escapeHtml(metadata.application_type || "unknown")}</span>
        </div>
      </div>
    </div>
  </td>
  <td class="px-4 py-5">${renderCMDB(sources.cmdb)}</td>
  <td class="px-4 py-5">${renderRuntime(qa, "qa")}</td>
  <td class="px-4 py-5">${renderRuntime(prod, "prod")}</td>
  <td class="px-4 py-5">${renderLifecycle(qa, prod)}</td>
  <td class="px-4 py-5">${renderLatestOverall(sources.eol)}</td>
  <td class="px-4 py-5">${renderSignals(product, qa, prod)}</td>
</tr>
`
}

function renderCMDB(cmdb = {}) {
  if (cmdb.status === "disabled") {
    return sourcePlaceholder("Not configured", "CMDB source is disabled")
  }

  if (cmdb.status === "error") {
    return sourceError("Unavailable", cmdb.error)
  }

  const versions = Array.isArray(cmdb.versions) ? cmdb.versions : []
  const primary = cmdb.version || versions[0]?.version || "-"

  return `
<div class="space-y-2">
  <div class="flex items-center gap-2">
    <span class="font-mono text-base font-black text-slate-900">${escapeHtml(primary)}</span>
    ${sourceStatusPill("ok")}
  </div>
  ${versions.length > 1
    ? `<div class="space-y-1">${versions.slice(0, 3).map(renderCMDBVersion).join("")}</div>`
    : versions.length === 1
      ? `<div class="text-[11px] text-slate-400">${escapeHtml(versions[0]?.lifecycle_state || "")}</div>`
      : ""}
</div>
`
}

function renderCMDBVersion(item = {}) {
  const state = String(item.lifecycle_state || "").trim()
  const normalized = state.toLowerCase()
  const classes = normalized === "general availability"
    ? "bg-emerald-50 text-emerald-700"
    : normalized === "end of life"
      ? "bg-rose-50 text-rose-700"
      : "bg-slate-100 text-slate-500"

  return `
<div class="flex items-center gap-2 text-[11px]">
  <span class="font-mono font-bold text-slate-600">${escapeHtml(item.version || "-")}</span>
  <span class="rounded-md px-1.5 py-0.5 font-bold ${classes}">${escapeHtml(shortLifecycle(state))}</span>
</div>
`
}

function renderRuntime(deployment, env) {
  if (!deployment) {
    return sourceError("Missing deployment", `${env.toUpperCase()} deployment is missing from the API response`)
  }

  if (deployment.status === "disabled") {
    return sourcePlaceholder("Not configured", `${env.toUpperCase()} runtime source is disabled`)
  }

  if (deployment.status === "error") {
    return sourceError("Unavailable", deployment.error)
  }

  if (deployment.status !== "ok") {
    return sourcePlaceholder("Unknown", deployment.error || "Runtime status could not be determined")
  }

  const candidates = Array.isArray(deployment.candidates) ? deployment.candidates : []
  const candidateTitle = candidates.length > 1
    ? `Detected versions: ${candidates.join(", ")}`
    : ""

  return `
<div class="space-y-2" title="${escapeHtml(candidateTitle)}">
  <div class="flex items-center gap-2">
    <span class="font-mono text-base font-black text-slate-900">${escapeHtml(deployment.version || "-")}</span>
    <span class="h-1.5 w-1.5 rounded-full bg-emerald-500 text-emerald-500 status-dot"></span>
  </div>
  <div class="flex flex-wrap items-center gap-1.5">
    <span class="rounded-md bg-slate-100 px-1.5 py-0.5 text-[10px] font-black uppercase tracking-wider text-slate-500">${escapeHtml(deployment.type || "http")}</span>
    ${candidates.length > 1
      ? `<span class="rounded-md bg-indigo-50 px-1.5 py-0.5 text-[10px] font-bold text-indigo-600">${candidates.length} candidates</span>`
      : ""}
  </div>
</div>
`
}

function renderLifecycle(qa, prod) {
  return `
<div class="space-y-2">
  ${renderLifecycleLine("QA", qa?.assessment, "sky")}
  ${renderLifecycleLine("Prod", prod?.assessment, "violet")}
</div>
`
}

function renderLifecycleLine(label, assessment = {}, color) {
  const envClasses = color === "sky"
    ? "bg-sky-50 text-sky-700"
    : "bg-violet-50 text-violet-700"

  if (!assessment || assessment.status === "disabled") {
    return lifecycleShell(label, envClasses, "Not configured", "text-slate-400", "")
  }

  if (assessment.status === "error") {
    return lifecycleShell(label, envClasses, "Unavailable", "text-rose-600", assessment.error)
  }

  if (assessment.status === "partial") {
    const cycle = assessment.current_cycle ? `Cycle ${assessment.current_cycle}` : "Unresolved"
    return lifecycleShell(label, envClasses, cycle, "text-amber-600", assessment.error)
  }

  const state = assessment.is_eol
    ? "EOL"
    : assessment.is_maintained
      ? "Maintained"
      : "Not maintained"
  const stateClasses = assessment.is_eol
    ? "text-rose-600"
    : assessment.is_maintained
      ? "text-emerald-600"
      : "text-slate-500"
  const title = [
    assessment.current_cycle_label || assessment.current_cycle,
    assessment.eol_from ? `EOL from ${assessment.eol_from}` : "",
    assessment.latest_in_current_cycle ? `Latest ${assessment.latest_in_current_cycle}` : ""
  ].filter(Boolean).join(" · ")

  return `
<div class="flex items-center gap-2 rounded-xl border border-slate-100 bg-white/70 px-2.5 py-2" title="${escapeHtml(title)}">
  <span class="w-9 shrink-0 rounded-md px-1.5 py-1 text-center text-[9px] font-black uppercase tracking-wider ${envClasses}">${label}</span>
  <div class="min-w-0">
    <div class="flex items-center gap-1.5">
      <span class="font-mono text-xs font-black text-slate-800">${escapeHtml(assessment.current_cycle || "-")}</span>
      ${assessment.is_lts ? `<span class="rounded bg-purple-50 px-1 py-0.5 text-[8px] font-black text-purple-600">LTS</span>` : ""}
    </div>
    <div class="mt-0.5 text-[10px] font-bold ${stateClasses}">${escapeHtml(state)}</div>
  </div>
</div>
`
}

function lifecycleShell(label, envClasses, value, valueClasses, title) {
  return `
<div class="flex items-center gap-2 rounded-xl border border-slate-100 bg-white/70 px-2.5 py-2" title="${escapeHtml(title || value)}">
  <span class="w-9 shrink-0 rounded-md px-1.5 py-1 text-center text-[9px] font-black uppercase tracking-wider ${envClasses}">${label}</span>
  <span class="truncate text-[11px] font-bold ${valueClasses}">${escapeHtml(value)}</span>
</div>
`
}

function renderLatestOverall(eol = {}) {
  if (eol.status === "disabled") {
    return sourcePlaceholder("Not configured", "EOL source is disabled")
  }

  if (eol.status === "error") {
    return sourceError("Unavailable", eol.error)
  }

  return `
<div class="space-y-1.5">
  <div class="flex flex-wrap items-center gap-1.5">
    <span class="font-mono text-base font-black text-slate-900">${escapeHtml(eol.latest_overall || "-")}</span>
    ${eol.latest_overall_is_lts
      ? `<span class="rounded-md bg-purple-50 px-1.5 py-0.5 text-[9px] font-black text-purple-600">LTS</span>`
      : ""}
  </div>
  <div class="text-[11px] text-slate-400">
    ${eol.latest_overall_cycle ? `Cycle ${escapeHtml(eol.latest_overall_cycle)}` : "Cycle unavailable"}
  </div>
  ${eol.latest_overall_date
    ? `<div class="text-[10px] font-semibold text-slate-400">${escapeHtml(eol.latest_overall_date)}</div>`
    : ""}
</div>
`
}

function renderSignals(product, qa, prod) {
  const signals = []
  const cmdb = product?.sources?.cmdb || {}
  const eol = product?.sources?.eol || {}

  if (cmdb.status === "error") signals.push(signalBadge("CMDB unavailable", "rose", cmdb.error))
  if (eol.status === "error") signals.push(signalBadge("EOL unavailable", "rose", eol.error))

  for (const [env, deployment] of [["QA", qa], ["Prod", prod]]) {
    if (!deployment || deployment.status === "disabled") {
      signals.push(signalBadge(`${env} not configured`, "slate", "Runtime source is disabled"))
      continue
    }

    if (deployment.status === "error") {
      signals.push(signalBadge(`${env} runtime error`, "rose", deployment.error))
      continue
    }

    const assessment = deployment.assessment || {}
    if (assessment.is_eol) {
      signals.push(signalBadge(`${env} is EOL`, "rose", assessment.eol_from ? `EOL from ${assessment.eol_from}` : "Current cycle is EOL"))
    }
    if (assessment.cmdb_mismatch) {
      signals.push(signalBadge(`${env} mismatch`, "amber", `CMDB ${cmdb.version || "-"} · Runtime ${deployment.version || "-"}`))
    }
    if (assessment.patch_available) {
      signals.push(signalBadge(`${env} patch available`, "blue", `Latest in cycle: ${assessment.latest_in_current_cycle || "-"}`))
    }
    if (assessment.status === "partial") {
      signals.push(signalBadge(`${env} unresolved`, "amber", assessment.error))
    }
  }

  if (signals.length === 0) {
    return `
<div class="inline-flex items-center gap-2 rounded-xl bg-emerald-50 px-3 py-2 text-xs font-bold text-emerald-700">
  <svg viewBox="0 0 24 24" class="h-3.5 w-3.5" fill="none" stroke="currentColor" stroke-width="2.4">
    <path d="m5 12 4 4L19 6" stroke-linecap="round" stroke-linejoin="round" />
  </svg>
  Aligned
</div>
`
  }

  return `<div class="flex flex-wrap gap-1.5">${signals.join("")}</div>`
}

function signalBadge(text, color, title) {
  const classes = {
    rose: "bg-rose-50 text-rose-700 ring-rose-100",
    amber: "bg-amber-50 text-amber-700 ring-amber-100",
    blue: "bg-blue-50 text-blue-700 ring-blue-100",
    slate: "bg-slate-100 text-slate-600 ring-slate-200"
  }[color] || "bg-slate-100 text-slate-600 ring-slate-200"

  return `<span class="rounded-lg px-2 py-1 text-[10px] font-bold ring-1 ring-inset ${classes}" title="${escapeHtml(title || text)}">${escapeHtml(text)}</span>`
}

function sourceStatusPill(status) {
  if (status !== "ok") return ""
  return `<span class="rounded-md bg-emerald-50 px-1.5 py-0.5 text-[9px] font-black uppercase tracking-wider text-emerald-600">Registered</span>`
}

function sourcePlaceholder(label, title) {
  return `
<div title="${escapeHtml(title || label)}">
  <div class="text-xs font-bold text-slate-400">${escapeHtml(label)}</div>
  <div class="mt-1 h-1 w-10 rounded-full bg-slate-100"></div>
</div>
`
}

function sourceError(label, message) {
  return `
<div title="${escapeHtml(message || label)}">
  <div class="inline-flex items-center gap-1.5 text-xs font-bold text-rose-600">
    <span class="h-1.5 w-1.5 rounded-full bg-rose-500"></span>
    ${escapeHtml(label)}
  </div>
  <div class="mt-1 max-w-[170px] truncate text-[10px] text-slate-400">${escapeHtml(message || "Source request failed")}</div>
</div>
`
}

function runtimeDeployments(product) {
  const deployments = product?.sources?.runtime?.deployments
  return Array.isArray(deployments) ? deployments : []
}

function deploymentFor(product, env) {
  return runtimeDeployments(product).find(item => normalizeText(item?.env) === env)
}

function productHasAttention(product) {
  const sources = product?.sources || {}
  if (sources.cmdb?.status === "error" || sources.eol?.status === "error") return true

  return runtimeDeployments(product).some(deployment => {
    if (deployment?.status !== "ok") return true
    const assessment = deployment?.assessment || {}
    return assessment.status === "error" ||
      assessment.status === "partial" ||
      assessment.is_eol === true ||
      assessment.cmdb_mismatch === true ||
      assessment.patch_available === true
  })
}

function countSourceErrors(product) {
  const sources = product?.sources || {}
  let count = 0

  if (sources.cmdb?.status === "error") count += 1
  if (sources.eol?.status === "error") count += 1
  count += runtimeDeployments(product).filter(item => item?.status === "error").length

  return count
}

function productSearchText(product) {
  const metadata = product?.metadata || {}
  const sources = product?.sources || {}
  const runtimeValues = runtimeDeployments(product).flatMap(item => [
    item?.env,
    item?.type,
    item?.version,
    ...(Array.isArray(item?.candidates) ? item.candidates : [])
  ])

  return [
    product?.key,
    metadata.display_name,
    metadata.application_type,
    sources.cmdb?.version,
    sources.eol?.product,
    sources.eol?.latest_overall,
    ...runtimeValues
  ].map(normalizeText).join(" ")
}

function productName(product) {
  return product?.metadata?.display_name || product?.key || "Unknown product"
}

function shortLifecycle(value) {
  const normalized = normalizeText(value)
  if (normalized === "general availability") return "GA"
  if (normalized === "end of life") return "EOL"
  return value || "Unknown"
}

function hashHue(value) {
  let hash = 0
  const text = String(value || "")
  for (let index = 0; index < text.length; index += 1) {
    hash = ((hash << 5) - hash + text.charCodeAt(index)) | 0
  }
  return Math.abs(hash) % 300 + 20
}

function normalizeText(value) {
  return String(value || "").trim().toLowerCase()
}

function renderLoading() {
  document.getElementById("versionsBody").innerHTML = Array.from({ length: 5 }).map(() => `
<tr>
  <td class="px-5 py-5"><div class="loading-shimmer h-10 w-44 rounded-xl"></div></td>
  <td class="px-4 py-5"><div class="loading-shimmer h-10 w-32 rounded-xl"></div></td>
  <td class="px-4 py-5"><div class="loading-shimmer h-10 w-32 rounded-xl"></div></td>
  <td class="px-4 py-5"><div class="loading-shimmer h-10 w-32 rounded-xl"></div></td>
  <td class="px-4 py-5"><div class="loading-shimmer h-16 w-44 rounded-xl"></div></td>
  <td class="px-4 py-5"><div class="loading-shimmer h-10 w-28 rounded-xl"></div></td>
  <td class="px-4 py-5"><div class="loading-shimmer h-14 w-44 rounded-xl"></div></td>
</tr>
`).join("")
}

function renderEmptyState(hasProducts) {
  const filtered = hasProducts > 0
  document.getElementById("versionsBody").innerHTML = `
<tr>
  <td colspan="7" class="px-6 py-20 text-center">
    <div class="mx-auto flex h-12 w-12 items-center justify-center rounded-2xl bg-slate-100 text-slate-400">
      <svg viewBox="0 0 24 24" class="h-5 w-5" fill="none" stroke="currentColor" stroke-width="1.8">
        <circle cx="11" cy="11" r="7" /><path d="m20 20-4-4" stroke-linecap="round" />
      </svg>
    </div>
    <div class="mt-3 font-bold text-slate-700">${filtered ? "No matching products" : "No products configured"}</div>
    <div class="mt-1 text-xs text-slate-400">${filtered ? "Try a different search or status filter." : "Add products to products.yaml to populate the inventory."}</div>
  </td>
</tr>
`
}

function renderError(message) {
  const errorBox = document.getElementById("errorBox")
  errorBox.textContent = message
  errorBox.classList.remove("hidden")

  if (loadedProducts.length === 0) {
    document.getElementById("resultSummary").textContent = "Unable to load products"
    document.getElementById("versionsBody").innerHTML = `
<tr>
  <td colspan="7" class="px-6 py-20 text-center">
    <div class="font-bold text-rose-600">Failed to load version inventory</div>
    <div class="mt-2 text-xs text-slate-400">${escapeHtml(message)}</div>
  </td>
</tr>
`
  }
}

function hideError() {
  const errorBox = document.getElementById("errorBox")
  errorBox.textContent = ""
  errorBox.classList.add("hidden")
}

function setLoadingState(loading) {
  isLoading = loading
  const button = document.getElementById("refreshButton")
  const icon = document.getElementById("refreshIcon")
  const label = document.getElementById("refreshLabel")

  button.disabled = loading
  button.classList.toggle("cursor-wait", loading)
  button.classList.toggle("opacity-70", loading)
  icon.classList.toggle("spin", loading)
  label.textContent = loading ? "Refreshing" : "Refresh"
}

function showToast(message, type) {
  const toast = document.createElement("div")
  const classes = type === "error"
    ? "border-rose-200 bg-rose-50 text-rose-700"
    : "border-emerald-200 bg-emerald-50 text-emerald-700"

  toast.className = `max-w-sm rounded-2xl border px-4 py-3 text-sm font-semibold shadow-xl ${classes}`
  toast.textContent = message
  document.getElementById("toastContainer").appendChild(toast)

  window.setTimeout(() => toast.remove(), 3200)
}

function escapeHtml(value) {
  return String(value ?? "")
    .replaceAll("&", "&amp;")
    .replaceAll("<", "&lt;")
    .replaceAll(">", "&gt;")
    .replaceAll('"', "&quot;")
    .replaceAll("'", "&#039;")
}
