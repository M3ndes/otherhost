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
  codex: '<svg class="brand-icon codex-icon" viewBox="0 0 24 24" aria-hidden="true"><g class="codex-mark"><circle cx="12" cy="6.2" r="4.1"/><circle cx="17" cy="9" r="4.1"/><circle cx="17" cy="15" r="4.1"/><circle cx="12" cy="17.8" r="4.1"/><circle cx="7" cy="15" r="4.1"/><circle cx="7" cy="9" r="4.1"/><circle cx="12" cy="12" r="5.2"/></g><path class="codex-terminal" d="m8.8 9.7 2 2.3-2 2.3M13 14.3h2.8"/></svg>',
  claude: '<img class="brand-icon claude-icon" src="/claude.svg" alt="">',
  vscode: '<img class="brand-icon vscode-icon" src="/vscode.svg" alt="">',
  copy: '<svg viewBox="0 0 24 24" aria-hidden="true"><rect x="8" y="8" width="12" height="12" rx="2"/><path d="M16 8V6a2 2 0 0 0-2-2H6a2 2 0 0 0-2 2v8a2 2 0 0 0 2 2h2"/></svg>',
  trash: '<svg viewBox="0 0 24 24" aria-hidden="true"><path d="M4 7h16M9 7V4h6v3m3 0-1 13H7L6 7m4 4v5m4-5v5"/></svg>',
  sun: '<svg viewBox="0 0 24 24" aria-hidden="true"><circle cx="12" cy="12" r="4"/><path d="M12 2v2m0 16v2M4.9 4.9l1.4 1.4m11.4 11.4 1.4 1.4M2 12h2m16 0h2M4.9 19.1l1.4-1.4M17.7 6.3l1.4-1.4"/></svg>',
  moon: '<svg viewBox="0 0 24 24" aria-hidden="true"><path d="M20.5 14.3A8.5 8.5 0 0 1 9.7 3.5 8.5 8.5 0 1 0 20.5 14.3Z"/></svg>'
};

const state = {
  snapshot: null,
  actionToken: '',
  query: '',
  loading: true,
  terminal: null,
  terminalSocket: null,
  terminalDisposables: [],
  terminalResizeObserver: null,
  terminalGeneration: 0,
  terminalProjectPath: '',
  projectDeletionEnabled: false,
  pendingDeletion: null,
  deleteTrigger: null,
  deletingProject: false,
  initialNavigationDone: false
};

const terminalStartupMarker = new Uint8Array([27, 91, 50, 74, 27, 91, 72]);
const terminalStartupLimit = 64 * 1024;
const terminalStartupTimeout = 10_000;

function findByteSequence(buffer, sequence) {
  if (sequence.length === 0 || sequence.length > buffer.length) return -1;
  for (let offset = 0; offset <= buffer.length - sequence.length; offset += 1) {
    let matches = true;
    for (let index = 0; index < sequence.length; index += 1) {
      if (buffer[offset + index] !== sequence[index]) {
        matches = false;
        break;
      }
    }
    if (matches) return offset;
  }
  return -1;
}

function appendBytes(left, right) {
  const combined = new Uint8Array(left.length + right.length);
  combined.set(left);
  combined.set(right, left.length);
  return combined;
}

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
  state.projectDeletionEnabled = Boolean(snapshot.projectDeletionEnabled);
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
    const editorActions = document.createElement('div');
    editorActions.className = 'project-editor-actions';
    const codexButton = document.createElement('button');
    codexButton.className = 'editor-project codex-project';
    codexButton.type = 'button';
    codexButton.title = 'Connect this host to Codex';
    codexButton.setAttribute('aria-label', `Set up ${project.name} in Codex`);
    codexButton.innerHTML = `${icons.codex}<span>Codex</span>`;
    codexButton.addEventListener('click', () => openCodexProject(project));
    const claudeButton = document.createElement('button');
    claudeButton.className = 'editor-project claude-project';
    claudeButton.type = 'button';
    claudeButton.title = 'Open in Claude Desktop';
    claudeButton.setAttribute('aria-label', `Open ${project.name} in Claude Desktop`);
    claudeButton.innerHTML = `${icons.claude}<span>Claude</span>`;
    claudeButton.addEventListener('click', () => openClaudeProject(project));
    const openButton = document.createElement('button');
    openButton.className = 'editor-project vscode-project';
    openButton.type = 'button';
    openButton.title = 'Open in VS Code';
    openButton.setAttribute('aria-label', `Open ${project.name} in VS Code`);
    openButton.innerHTML = `${icons.vscode}<span>VS Code</span>`;
    openButton.addEventListener('click', () => openProject(project, openButton));
    editorActions.append(codexButton, claudeButton, openButton);
    const utilityActions = document.createElement('div');
    utilityActions.className = 'project-utility-actions';
    const copyButton = document.createElement('button');
    copyButton.className = 'copy-project';
    copyButton.type = 'button';
    copyButton.title = 'Copy remote path';
    copyButton.setAttribute('aria-label', `Copy path for ${project.name}`);
    copyButton.innerHTML = icons.copy;
    copyButton.addEventListener('click', () => copyPath(project.path));
    const terminalButton = document.createElement('button');
    terminalButton.className = 'terminal-project';
    terminalButton.type = 'button';
    terminalButton.title = 'Open in terminal';
    terminalButton.setAttribute('aria-label', `Open ${project.name} in terminal`);
    terminalButton.innerHTML = icons.terminal;
    terminalButton.addEventListener('click', () => openProjectTerminal(project));
    utilityActions.append(terminalButton, copyButton);
    if (state.projectDeletionEnabled) {
      const deleteButton = document.createElement('button');
      deleteButton.className = 'delete-project';
      deleteButton.type = 'button';
      deleteButton.title = 'Delete project';
      deleteButton.setAttribute('aria-label', `Delete ${project.name} permanently`);
      deleteButton.innerHTML = icons.trash;
      deleteButton.addEventListener('click', () => openDeleteProject(project, deleteButton));
      utilityActions.append(deleteButton);
    }
    actions.append(editorActions, utilityActions);

    card.append(top, title, projectPath, tags, actions);
    grid.append(card);
  });
}

function openDeleteProject(project, trigger) {
  state.pendingDeletion = project;
  state.deleteTrigger = trigger;
  state.deletingProject = false;
  setText('[data-delete-name]', project.name);
  setText('[data-delete-confirmation-name]', project.name);
  setText('[data-delete-path]', project.path);
  setText('[data-delete-submit-label]', 'Delete permanently');
  const input = document.querySelector('[data-delete-confirmation]');
  input.value = '';
  input.disabled = false;
  document.querySelector('[data-delete-submit]').disabled = true;
  document.querySelector('[data-delete-cancel]').disabled = false;
  document.querySelector('[data-delete-modal]').classList.remove('hidden');
  document.body.classList.add('modal-open');
  window.setTimeout(() => input.focus(), 0);
}

function closeDeleteProject(force = false) {
  if (state.deletingProject && !force) return;
  document.querySelector('[data-delete-modal]').classList.add('hidden');
  document.body.classList.remove('modal-open');
  const trigger = state.deleteTrigger;
  state.pendingDeletion = null;
  state.deleteTrigger = null;
  state.deletingProject = false;
  if (trigger?.isConnected) trigger.focus();
}

function updateDeleteConfirmation() {
  const project = state.pendingDeletion;
  const input = document.querySelector('[data-delete-confirmation]');
  document.querySelector('[data-delete-submit]').disabled = state.deletingProject || !project || input.value !== project.name;
}

async function deleteProject() {
  const project = state.pendingDeletion;
  const input = document.querySelector('[data-delete-confirmation]');
  if (!project || state.deletingProject || input.value !== project.name) return;

  state.deletingProject = true;
  input.disabled = true;
  document.querySelector('[data-delete-cancel]').disabled = true;
  document.querySelector('[data-delete-submit]').disabled = true;
  setText('[data-delete-submit-label]', 'Deleting…');
  try {
    const response = await fetch('/api/projects/delete', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json', 'X-Otherhost-Token': state.actionToken },
      body: JSON.stringify({ path: project.path, confirmation: project.name })
    });
    if (!response.ok) throw new Error((await response.text()).trim() || 'Could not delete the project.');
    if (state.terminalProjectPath === project.path) stopTerminal(true);
    state.snapshot.projects = (state.snapshot.projects || []).filter((candidate) => candidate.path !== project.path);
    renderProjects();
    closeDeleteProject(true);
    showToast(`${project.name} was permanently deleted.`);
    refresh();
  } catch (error) {
    state.deletingProject = false;
    input.disabled = false;
    document.querySelector('[data-delete-cancel]').disabled = false;
    setText('[data-delete-submit-label]', 'Delete permanently');
    updateDeleteConfirmation();
    showToast(error.message, true);
    input.focus();
    refresh();
  }
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
      headers: { 'Content-Type': 'application/json', 'X-Otherhost-Token': state.actionToken },
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

function openCodexProject(project) {
  const sshAlias = state.snapshot?.sshAlias;
  if (!sshAlias) {
    showToast('The Codex SSH connection is unavailable.', true);
    return;
  }
  window.location.href = `codex://settings/connections/ssh/add?name=${encodeURIComponent(sshAlias)}`;
  showToast(`Opening Codex SSH setup. Select ${project.path} on ${sshAlias}.`);
}

function terminalTheme() {
  if (document.documentElement.dataset.theme === 'dark') {
    return {
      background: '#15151c', foreground: '#ececf2', cursor: '#9b8cff', cursorAccent: '#15151c',
      selectionBackground: '#50469099', black: '#15151c', brightBlack: '#6f6f7c', red: '#ff858b',
      green: '#69d7aa', yellow: '#e4bf72', blue: '#78afff', magenta: '#a999ff', cyan: '#64ced3', white: '#ececf2'
    };
  }
  return {
    background: '#f7f7fa', foreground: '#24242b', cursor: '#6d5dfc', cursorAccent: '#f7f7fa',
    selectionBackground: '#6d5dfc44', black: '#24242b', brightBlack: '#696973', red: '#b4232c',
    green: '#0c7550', yellow: '#805d00', blue: '#1f6fb2', magenta: '#6848d8', cyan: '#00747c', white: '#5f5f69',
    brightRed: '#c93640', brightGreen: '#18845c', brightYellow: '#8f6800', brightBlue: '#2878c7',
    brightMagenta: '#765de0', brightCyan: '#0a7f88', brightWhite: '#24242b'
  };
}

function setTerminalState(kind, label) {
  const indicator = document.querySelector('[data-terminal-state]');
  indicator.className = `terminal-state ${kind}`;
  indicator.querySelector('span').textContent = label;
}

function stopTerminal(showEmpty = true) {
  state.terminalGeneration += 1;
  if (state.terminalSocket) {
    state.terminalSocket.onclose = null;
    state.terminalSocket.close(1000, 'Terminal closed');
    state.terminalSocket = null;
  }
  state.terminalDisposables.forEach((disposable) => disposable.dispose());
  state.terminalDisposables = [];
  if (state.terminalResizeObserver) {
    state.terminalResizeObserver.disconnect();
    state.terminalResizeObserver = null;
  }
  if (state.terminal) {
    state.terminal.dispose();
    state.terminal = null;
  }
  state.terminalProjectPath = '';
  document.querySelector('[data-terminal-mount]').replaceChildren();
  document.querySelector('[data-terminal-empty]').classList.toggle('hidden', !showEmpty);
  document.querySelector('[data-terminal-close]').disabled = true;
  if (showEmpty) {
    setText('[data-terminal-title]', 'Remote shell');
    setText('[data-terminal-location]', 'WSL home');
    setTerminalState('idle', 'Not connected');
  }
}

async function startTerminal(project = null) {
  if (!window.Terminal || !window.FitAddon?.FitAddon) {
    showToast('The terminal component could not be loaded.', true);
    return;
  }
  if (!state.actionToken) {
    showToast('Wait for the remote inventory before opening a terminal.', true);
    return;
  }

  stopTerminal(false);
  const generation = state.terminalGeneration;
  const mount = document.querySelector('[data-terminal-mount]');
  const empty = document.querySelector('[data-terminal-empty]');
  empty.classList.add('hidden');
  setText('[data-terminal-title]', project ? project.name : 'Remote shell');
  setText('[data-terminal-location]', project ? project.path : 'WSL home');
  state.terminalProjectPath = project?.path || '';
  setTerminalState('connecting', 'Connecting');
  document.querySelector('[data-terminal-close]').disabled = false;

  const terminal = new window.Terminal({
    cursorBlink: true,
    cursorStyle: 'bar',
    fontFamily: '"SFMono-Regular", Menlo, Monaco, Consolas, monospace',
    fontSize: 13,
    lineHeight: 1.25,
    scrollback: 5000,
    macOptionIsMeta: true,
    theme: terminalTheme()
  });
  const fitAddon = new window.FitAddon.FitAddon();
  terminal.loadAddon(fitAddon);
  terminal.open(mount);
  fitAddon.fit();
  state.terminal = terminal;

  const resizeObserver = new ResizeObserver(() => {
    if (state.terminal === terminal) fitAddon.fit();
  });
  resizeObserver.observe(mount);
  state.terminalResizeObserver = resizeObserver;

  let socket;
  let terminalReady = false;
  let startupOutput = new Uint8Array();
  let startupTimer;
  const encoder = new TextEncoder();
  state.terminalDisposables.push(terminal.onData((data) => {
    if (terminalReady && socket?.readyState === WebSocket.OPEN) socket.send(encoder.encode(data));
  }));
  state.terminalDisposables.push(terminal.onResize(({ cols, rows }) => {
    if (socket?.readyState === WebSocket.OPEN) {
      socket.send(JSON.stringify({ type: 'resize', columns: cols, rows }));
    }
  }));

  try {
    const response = await fetch('/api/terminals', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json', 'X-Otherhost-Token': state.actionToken },
      body: JSON.stringify({ path: project?.path || '', columns: terminal.cols, rows: terminal.rows })
    });
    if (!response.ok) throw new Error((await response.text()).trim() || 'Could not create the terminal session.');
    if (state.terminalGeneration !== generation) return;
    const session = await response.json();
    const socketURL = new URL(session.socketPath, window.location.href);
    socketURL.protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
    socket = new WebSocket(socketURL, session.protocol);
    socket.binaryType = 'arraybuffer';
    state.terminalSocket = socket;

    socket.onopen = () => {
      if (state.terminalGeneration !== generation) return;
      setTerminalState('connecting', 'Preparing terminal');
      socket.send(JSON.stringify({ type: 'resize', columns: terminal.cols, rows: terminal.rows }));
      startupTimer = window.setTimeout(() => {
        if (state.terminalGeneration !== generation || terminalReady) return;
        terminal.write('\r\nOtherhost: terminal initialization timed out.\r\n');
        setTerminalState('unavailable', 'Initialization failed');
        socket.close(1011, 'Terminal initialization timed out');
      }, terminalStartupTimeout);
    };
    socket.onmessage = (event) => {
      if (state.terminalGeneration !== generation) return;
      const output = new Uint8Array(event.data);
      if (terminalReady) {
        terminal.write(output);
        return;
      }
      startupOutput = appendBytes(startupOutput, output);
      if (startupOutput.length > terminalStartupLimit) {
        window.clearTimeout(startupTimer);
        terminal.write('\r\nOtherhost: terminal initialization produced too much output.\r\n');
        setTerminalState('unavailable', 'Initialization failed');
        socket.close(1011, 'Terminal initialization output limit exceeded');
        return;
      }
      const markerIndex = findByteSequence(startupOutput, terminalStartupMarker);
      if (markerIndex < 0) return;
      const visibleOutput = startupOutput.subarray(markerIndex + terminalStartupMarker.length);
      startupOutput = new Uint8Array();
      terminalReady = true;
      window.clearTimeout(startupTimer);
      setTerminalState('connected', 'Connected');
      if (visibleOutput.length > 0) terminal.write(visibleOutput);
      terminal.focus();
    };
    socket.onerror = () => {
      if (state.terminalGeneration === generation) setTerminalState('unavailable', 'Connection error');
    };
    socket.onclose = (event) => {
      if (state.terminalGeneration !== generation) return;
      window.clearTimeout(startupTimer);
      state.terminalSocket = null;
      if (!terminalReady && startupOutput.length > 0) {
        terminal.write('\r\nOtherhost: terminal initialization did not complete.\r\n');
      }
      setTerminalState('idle', event.code === 1000 ? 'Session ended' : 'Disconnected');
    };
  } catch (error) {
    if (state.terminalGeneration === generation) {
      stopTerminal(true);
      showToast(error.message, true);
    }
  }
}

function openProjectTerminal(project) {
  window.location.hash = 'terminal';
  document.getElementById('terminal').scrollIntoView();
  window.setTimeout(() => startTerminal(project), 120);
}

function openClaudeProject(project) {
  const sshAlias = state.snapshot?.sshAlias;
  if (!sshAlias) {
    showToast('The Claude SSH connection is unavailable.', true);
    return;
  }
  window.location.href = 'claude://code/new';
  showToast(`Opening Claude Desktop. Select ${sshAlias} over SSH, then ${project.path}.`);
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
    if (!state.initialNavigationDone) {
      state.initialNavigationDone = true;
      const target = document.getElementById(window.location.hash.slice(1) || 'overview');
      if (target) window.requestAnimationFrame(() => target.scrollIntoView());
    }
  } catch (error) {
    renderSnapshot({ status: 'unavailable', message: error.message, host: {}, environment: {}, projects: [], updatedAt: new Date().toISOString() });
  } finally {
    state.loading = false;
    document.querySelectorAll('[data-refresh]').forEach((button) => { button.disabled = false; });
  }
}

function configureTheme() {
  const storedTheme = localStorage.getItem('otherhost-theme');
  const preferredTheme = window.matchMedia('(prefers-color-scheme: dark)').matches ? 'dark' : 'light';
  const theme = storedTheme || preferredTheme;
  document.documentElement.dataset.theme = theme;
  const button = document.querySelector('[data-theme-toggle]');
  const updateButton = () => {
    const dark = document.documentElement.dataset.theme === 'dark';
    button.innerHTML = dark ? icons.sun : icons.moon;
    button.setAttribute('aria-label', dark ? 'Switch to light theme' : 'Switch to dark theme');
    if (state.terminal) state.terminal.options.theme = terminalTheme();
  };
  updateButton();
  button.addEventListener('click', () => {
    const next = document.documentElement.dataset.theme === 'dark' ? 'light' : 'dark';
    document.documentElement.dataset.theme = next;
    localStorage.setItem('otherhost-theme', next);
    updateButton();
  });
}

function configureNavigation() {
  const links = Array.from(document.querySelectorAll('[data-section-link]'));
  const sections = links.map((link) => document.getElementById(link.dataset.sectionLink));
  let updateScheduled = false;
  const update = () => {
    updateScheduled = false;
    const marker = window.scrollY + (window.innerHeight * .28);
    let active = sections[0];
    sections.forEach((section) => {
      if (section.offsetTop <= marker) active = section;
    });
    links.forEach((link) => link.classList.toggle('active', link.dataset.sectionLink === active.id));
  };
  window.addEventListener('scroll', () => {
    if (updateScheduled) return;
    updateScheduled = true;
    window.requestAnimationFrame(update);
  }, { passive: true });
  links.forEach((link) => link.addEventListener('click', () => {
    links.forEach((candidate) => candidate.classList.toggle('active', candidate === link));
    if (link.dataset.sectionLink === 'projects') refresh();
  }));
  update();
}

installIcons();
configureTheme();
configureNavigation();
document.querySelectorAll('[data-refresh]').forEach((button) => button.addEventListener('click', refresh));
document.querySelector('[data-terminal-start]').addEventListener('click', () => startTerminal());
document.querySelector('[data-terminal-new]').addEventListener('click', () => startTerminal());
document.querySelector('[data-terminal-close]').addEventListener('click', () => stopTerminal(true));
document.querySelector('[data-delete-confirmation]').addEventListener('input', updateDeleteConfirmation);
document.querySelector('[data-delete-form]').addEventListener('submit', (event) => {
  event.preventDefault();
  deleteProject();
});
document.querySelector('[data-delete-cancel]').addEventListener('click', () => closeDeleteProject());
document.querySelector('[data-delete-modal]').addEventListener('click', (event) => {
  if (event.target === event.currentTarget) closeDeleteProject();
});
document.addEventListener('keydown', (event) => {
  if (event.key === 'Escape' && state.pendingDeletion) closeDeleteProject();
});
document.querySelector('[data-project-search]').addEventListener('input', (event) => {
  state.query = event.target.value.trim().toLowerCase();
  renderProjects();
});
state.loading = false;
refresh();
setInterval(() => { if (!document.hidden) refresh(); }, 30000);
document.addEventListener('visibilitychange', () => { if (!document.hidden) refresh(); });
window.addEventListener('beforeunload', () => stopTerminal(false));
