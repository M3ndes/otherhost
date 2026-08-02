const icons = {
  overview: '<svg viewBox="0 0 24 24" aria-hidden="true"><rect x="3" y="3" width="7" height="7" rx="2"/><rect x="14" y="3" width="7" height="7" rx="2"/><rect x="3" y="14" width="7" height="7" rx="2"/><rect x="14" y="14" width="7" height="7" rx="2"/></svg>',
  projects: '<svg viewBox="0 0 24 24" aria-hidden="true"><path d="M4 7.5A2.5 2.5 0 0 1 6.5 5h3l2 2h6A2.5 2.5 0 0 1 20 9.5v7a2.5 2.5 0 0 1-2.5 2.5h-11A2.5 2.5 0 0 1 4 16.5v-9Z"/></svg>',
  machine: '<svg viewBox="0 0 24 24" aria-hidden="true"><rect x="3" y="4" width="18" height="13" rx="2"/><path d="M8 21h8M12 17v4"/></svg>',
  arrow: '<svg viewBox="0 0 24 24" aria-hidden="true"><path d="M5 12h14m-5-5 5 5-5 5"/></svg>',
  refresh: '<svg viewBox="0 0 24 24" aria-hidden="true"><path d="M20 7v5h-5M4 17v-5h5"/><path d="M6.1 8a7 7 0 0 1 11.5-1L20 12M4 12l2.4 5a7 7 0 0 0 11.5-1"/></svg>',
  search: '<svg viewBox="0 0 24 24" aria-hidden="true"><circle cx="11" cy="11" r="7"/><path d="m20 20-4-4"/></svg>',
  alert: '<svg viewBox="0 0 24 24" aria-hidden="true"><path d="M10.3 4.1 2.7 17a2 2 0 0 0 1.7 3h15.2a2 2 0 0 0 1.7-3L13.7 4.1a2 2 0 0 0-3.4 0Z"/><path d="M12 9v4m0 3h.01"/></svg>',
  cpu: '<svg viewBox="0 0 24 24" aria-hidden="true"><rect x="7" y="7" width="10" height="10" rx="2"/><path d="M9 1v3m6-3v3M9 20v3m6-3v3M20 9h3m-3 6h3M1 9h3m-3 6h3"/></svg>',
  memory: '<svg viewBox="0 0 24 24" aria-hidden="true"><rect x="4" y="6" width="16" height="12" rx="2"/><path d="M8 10v4m4-4v4m4-4v4M8 3v3m4-3v3m4-3v3M8 18v3m4-3v3m4-3v3"/></svg>',
  gpu: '<svg viewBox="0 0 24 24" aria-hidden="true"><rect x="3" y="5" width="18" height="14" rx="2"/><circle cx="12" cy="12" r="4"/><path d="M7 9H5m2 6H5m14-6h-2m2 6h-2"/></svg>',
  disk: '<svg viewBox="0 0 24 24" aria-hidden="true"><ellipse cx="12" cy="6" rx="8" ry="3"/><path d="M4 6v6c0 1.7 3.6 3 8 3s8-1.3 8-3V6M4 12v6c0 1.7 3.6 3 8 3s8-1.3 8-3v-6"/></svg>',
  terminal: '<svg viewBox="0 0 24 24" aria-hidden="true"><rect x="3" y="4" width="18" height="16" rx="2"/><path d="m7 9 3 3-3 3m6 0h4"/></svg>',
  branch: '<svg viewBox="0 0 24 24" aria-hidden="true"><circle cx="6" cy="5" r="2"/><circle cx="18" cy="6" r="2"/><circle cx="6" cy="19" r="2"/><path d="M6 7v10m2-6h4a6 6 0 0 0 6-3"/></svg>',
  external: '<svg viewBox="0 0 24 24" aria-hidden="true"><path d="M15 4h5v5m0-5-9 9"/><path d="M18 13v5a2 2 0 0 1-2 2H6a2 2 0 0 1-2-2V8a2 2 0 0 1 2-2h5"/></svg>',
  copy: '<svg viewBox="0 0 24 24" aria-hidden="true"><rect x="8" y="8" width="12" height="12" rx="2"/><path d="M16 8V6a2 2 0 0 0-2-2H6a2 2 0 0 0-2 2v8a2 2 0 0 0 2 2h2"/></svg>',
  sun: '<svg viewBox="0 0 24 24" aria-hidden="true"><circle cx="12" cy="12" r="4"/><path d="M12 2v2m0 16v2M4.9 4.9l1.4 1.4m11.4 11.4 1.4 1.4M2 12h2m16 0h2M4.9 19.1l1.4-1.4M17.7 6.3l1.4-1.4"/></svg>',
  moon: '<svg viewBox="0 0 24 24" aria-hidden="true"><path d="M20.5 14.3A8.5 8.5 0 0 1 9.7 3.5 8.5 8.5 0 1 0 20.5 14.3Z"/></svg>'
};

const state = { snapshot: null, actionToken: '', query: '', loading: true };

function installIcons() {
  document.querySelectorAll('[data-icon]').forEach((element) => {
    element.innerHTML = icons[element.dataset.icon] || '';
  });
}

function setText(selector, value) {
  document.querySelectorAll(selector).forEach((element) => { element.textContent = value; });
}

function formatBytes(value, decimals = 0) {
  if (!Number.isFinite(value) || value <= 0) return '—';
  const units = ['B', 'KB', 'MB', 'GB', 'TB'];
  const index = Math.min(Math.floor(Math.log(value) / Math.log(1024)), units.length - 1);
  return `${(value / (1024 ** index)).toFixed(index === 0 ? 0 : decimals)} ${units[index]}`;
}

function formatDuration(milliseconds) {
  if (!Number.isFinite(milliseconds) || milliseconds <= 0) return '—';
  if (milliseconds < 1000) return `${milliseconds} ms`;
  return `${(milliseconds / 1000).toFixed(1)} s`;
}

function shortCPU(model) {
  if (!model) return '—';
  return model
    .replace(/\(R\)|\(TM\)/gi, '')
    .replace(/CPU\s*/gi, '')
    .replace(/\s+@\s+[\d.]+GHz/i, '')
    .replace(/\s+/g, ' ')
    .trim();
}

function shortGPU(model) {
  if (!model) return '—';
  return model.replace(/^NVIDIA\s+(GeForce\s+)?/i, '').trim();
}

function setConnection(status) {
  const unavailable = status !== 'connected';
  document.querySelectorAll('[data-status-dot]').forEach((dot) => {
    dot.classList.remove('loading', 'unavailable');
    if (unavailable) dot.classList.add('unavailable');
  });
  document.querySelectorAll('[data-connection-pill]').forEach((pill) => {
    pill.classList.remove('loading', 'unavailable');
    if (unavailable) pill.classList.add('unavailable');
  });
  setText('[data-connection-label]', unavailable ? 'Unavailable' : 'Connected');
  setText('[data-connection-mini]', unavailable ? 'Remote host unavailable' : 'Private SSH connection');
}

function renderSnapshot(snapshot) {
  state.snapshot = snapshot;
  state.loading = false;
  const connected = snapshot.status === 'connected';
  const host = snapshot.host || {};
  const cpu = host.cpu || {};
  const gpu = host.gpu || {};
  const environment = snapshot.environment || {};

  setConnection(snapshot.status);
  setText('[data-host-name]', host.name || 'Remote host');
  setText('[data-host-mini]', host.name || 'Remote host');
  setText('[data-host-os]', host.os || (connected ? 'Windows development host' : 'Waiting for connection'));
  setText('[data-live-badge]', connected ? 'Online' : 'Offline');
  document.querySelector('[data-live-badge]').classList.toggle('unavailable', !connected);

  const cpuName = shortCPU(cpu.model);
  const gpuName = shortGPU(gpu.model);
  setText('[data-host-processor]', cpuName);
  setText('[data-host-memory]', formatBytes(host.memoryBytes));
  setText('[data-host-gpu]', gpuName);
  setText('[data-host-latency]', connected ? formatDuration(snapshot.sshResponseMs) : '—');

  setText('[data-spec-cpu]', cpuName);
  setText('[data-spec-cpu-detail]', cpu.physicalCores && cpu.logicalProcessors ? `${cpu.physicalCores} cores · ${cpu.logicalProcessors} threads` : 'Processor details unavailable');
  setText('[data-spec-memory]', formatBytes(host.memoryBytes));
  setText('[data-spec-memory-detail]', host.memoryBytes ? 'Total host capacity' : 'Host capacity unavailable');
  setText('[data-spec-gpu]', gpuName);
  setText('[data-spec-gpu-detail]', gpu.memoryBytes ? `${formatBytes(gpu.memoryBytes)} graphics memory` : 'Graphics memory unavailable');

  setText('[data-environment-distro]', environment.distribution || '—');
  setText('[data-environment-cpu]', environment.processors ? `${environment.processors} logical processors` : '—');
  setText('[data-environment-memory]', formatBytes(environment.memoryBytes));
  setText('[data-environment-disk]', formatBytes(environment.disk?.availableBytes));

  const updatedAt = snapshot.updatedAt ? new Date(snapshot.updatedAt) : new Date();
  setText('[data-last-updated]', connected ? `Updated ${updatedAt.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' })}` : 'Inventory unavailable');

  const notice = document.querySelector('[data-notice]');
  notice.classList.toggle('hidden', connected);
  if (!connected) setText('[data-notice-message]', snapshot.message || 'Check that the Windows desktop and WSL are running.');

  renderProjects();
  renderDisks(host.disks || []);
}

function renderProjects() {
  const grid = document.querySelector('[data-project-grid]');
  const empty = document.querySelector('[data-project-empty]');
  const projects = (state.snapshot?.projects || []).filter((project) => project.name.toLowerCase().includes(state.query));
  grid.replaceChildren();
  grid.classList.toggle('hidden', projects.length === 0);
  empty.classList.toggle('hidden', projects.length !== 0);

  projects.forEach((project) => {
    const card = document.createElement('article');
    card.className = 'project-card';

    const top = document.createElement('div');
    top.className = 'project-card-top';
    const glyph = document.createElement('span');
    glyph.className = 'project-glyph';
    glyph.innerHTML = icons.projects;
    const badge = document.createElement('span');
    badge.className = 'ready-badge';
    badge.textContent = 'Available';
    top.append(glyph, badge);

    const title = document.createElement('h3');
    title.textContent = project.name;
    const projectPath = document.createElement('p');
    projectPath.className = 'project-path';
    projectPath.textContent = project.path;
    projectPath.title = project.path;

    const tags = document.createElement('div');
    tags.className = 'tag-list';
    (project.technologies || []).forEach((technology) => {
      const tag = document.createElement('span');
      tag.className = 'tech-tag';
      tag.textContent = technology;
      tags.append(tag);
    });
    if (project.branch) {
      const branch = document.createElement('span');
      branch.className = 'branch-tag';
      branch.innerHTML = icons.branch;
      const branchName = document.createElement('span');
      branchName.textContent = project.branch;
      branch.append(branchName);
      tags.append(branch);
    }

    const actions = document.createElement('div');
    actions.className = 'project-actions';
    const openButton = document.createElement('button');
    openButton.className = 'open-project';
    openButton.type = 'button';
    openButton.innerHTML = `${icons.external}<span>Open project</span>`;
    openButton.addEventListener('click', () => openProject(project, openButton));
    const copyButton = document.createElement('button');
    copyButton.className = 'copy-project';
    copyButton.type = 'button';
    copyButton.title = 'Copy remote path';
    copyButton.setAttribute('aria-label', `Copy path for ${project.name}`);
    copyButton.innerHTML = icons.copy;
    copyButton.addEventListener('click', () => copyPath(project.path));
    actions.append(openButton, copyButton);

    card.append(top, title, projectPath, tags, actions);
    grid.append(card);
  });
}

function renderDisks(disks) {
  const list = document.querySelector('[data-disk-list]');
  list.replaceChildren();
  if (!disks.length) {
    const message = document.createElement('span');
    message.className = 'last-updated';
    message.textContent = 'Storage inventory is unavailable.';
    list.append(message);
    return;
  }
  disks.forEach((disk) => {
    const used = Math.max(0, disk.totalBytes - disk.availableBytes);
    const percentage = disk.totalBytes ? Math.min(100, (used / disk.totalBytes) * 100) : 0;
    const row = document.createElement('div');
    const header = document.createElement('div');
    header.className = 'disk-row-header';
    const name = document.createElement('strong');
    name.textContent = disk.name;
    const detail = document.createElement('span');
    detail.textContent = `${formatBytes(disk.availableBytes)} available of ${formatBytes(disk.totalBytes)}`;
    header.append(name, detail);
    const track = document.createElement('div');
    track.className = 'progress-track';
    const progress = document.createElement('span');
    progress.style.width = `${percentage}%`;
    track.append(progress);
    row.append(header, track);
    list.append(row);
  });
}

async function openProject(project, button) {
  const label = button.querySelector('span');
  const original = label.textContent;
  label.textContent = 'Opening…';
  button.disabled = true;
  try {
    const response = await fetch('/api/projects/open', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json', 'X-Devbox-Token': state.actionToken },
      body: JSON.stringify({ path: project.path })
    });
    if (!response.ok) throw new Error((await response.text()).trim() || 'Could not open the project.');
    showToast(`Opening ${project.name} in VS Code.`);
  } catch (error) {
    showToast(error.message, true);
  } finally {
    label.textContent = original;
    button.disabled = false;
  }
}

async function copyPath(path) {
  try {
    await navigator.clipboard.writeText(path);
    showToast('Remote path copied.');
  } catch (_) {
    showToast('Could not copy the remote path.', true);
  }
}

let toastTimer;
function showToast(message, error = false) {
  const toast = document.querySelector('[data-toast]');
  toast.textContent = message;
  toast.classList.toggle('error', error);
  toast.classList.add('visible');
  clearTimeout(toastTimer);
  toastTimer = setTimeout(() => toast.classList.remove('visible'), 3500);
}

async function refresh() {
  if (state.loading) return;
  state.loading = true;
  document.querySelectorAll('[data-refresh]').forEach((button) => { button.disabled = true; });
  try {
    const response = await fetch('/api/snapshot', { cache: 'no-store' });
    if (!response.ok) throw new Error('Could not load the remote inventory.');
    const snapshot = await response.json();
    state.actionToken = snapshot.actionToken;
    renderSnapshot(snapshot);
  } catch (error) {
    renderSnapshot({ status: 'unavailable', message: error.message, host: {}, environment: {}, projects: [], updatedAt: new Date().toISOString() });
  } finally {
    state.loading = false;
    document.querySelectorAll('[data-refresh]').forEach((button) => { button.disabled = false; });
  }
}

function configureTheme() {
  const storedTheme = localStorage.getItem('devbox-theme');
  const preferredTheme = window.matchMedia('(prefers-color-scheme: dark)').matches ? 'dark' : 'light';
  const theme = storedTheme || preferredTheme;
  document.documentElement.dataset.theme = theme;
  const button = document.querySelector('[data-theme-toggle]');
  const updateButton = () => {
    const dark = document.documentElement.dataset.theme === 'dark';
    button.innerHTML = dark ? icons.sun : icons.moon;
    button.setAttribute('aria-label', dark ? 'Switch to light theme' : 'Switch to dark theme');
  };
  updateButton();
  button.addEventListener('click', () => {
    const next = document.documentElement.dataset.theme === 'dark' ? 'light' : 'dark';
    document.documentElement.dataset.theme = next;
    localStorage.setItem('devbox-theme', next);
    updateButton();
  });
}

function configureNavigation() {
  const links = Array.from(document.querySelectorAll('[data-section-link]'));
  const sections = links.map((link) => document.getElementById(link.dataset.sectionLink));
  const observer = new IntersectionObserver((entries) => {
    const visible = entries.filter((entry) => entry.isIntersecting).sort((a, b) => b.intersectionRatio - a.intersectionRatio)[0];
    if (!visible) return;
    links.forEach((link) => link.classList.toggle('active', link.dataset.sectionLink === visible.target.id));
  }, { rootMargin: '-20% 0px -65% 0px', threshold: [0, .25, .5] });
  sections.forEach((section) => observer.observe(section));
}

installIcons();
configureTheme();
configureNavigation();
document.querySelectorAll('[data-refresh]').forEach((button) => button.addEventListener('click', refresh));
document.querySelector('[data-project-search]').addEventListener('input', (event) => {
  state.query = event.target.value.trim().toLowerCase();
  renderProjects();
});
state.loading = false;
refresh();
setInterval(() => { if (!document.hidden) refresh(); }, 30000);
