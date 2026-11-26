import http from 'k6/http';
import exec from 'k6/execution';
import { check, sleep } from 'k6';
import { Counter, Rate, Trend } from 'k6/metrics';

const BASE = __ENV.BASE_URL || 'http://localhost:8080';
const QUICK = __ENV.QUICK === '1';
const SCALE = QUICK ? 0.1 : 1;
const ROWS = Number(__ENV.ROWS || 1000000);
const JOB_TIMEOUT_S = Number(__ENV.JOB_TIMEOUT_S || 180);
const SLICES = 4;
const DATABASES = ['loadtest_a', 'loadtest_b', 'loadtest_c', 'loadtest_d'];
const SAME_DB = 'loadtest_a';
const SPREAD_DBS = ['loadtest_b', 'loadtest_c', 'loadtest_d'];

const NORMAL_VUS = Number(__ENV.NORMAL_VUS || 10);
const NORMAL_JOBS_PER_MIN = Number(__ENV.NORMAL_JOBS_PER_MIN || 6);

const seconds = (s) => Math.max(1, Math.round(s * SCALE));
const PHASES = [
  { name: 'ramp', duration: seconds(30), factor: 1 },
  { name: 'normal', duration: seconds(60), factor: 1 },
  { name: 'ramp', duration: seconds(30), factor: 3 },
  { name: 'high', duration: seconds(60), factor: 3 },
  { name: 'ramp', duration: seconds(30), factor: 10 },
  { name: 'peak', duration: seconds(60), factor: 10 },
  { name: 'ramp', duration: seconds(30), factor: 30 },
  { name: 'stress', duration: seconds(60), factor: 30 },
  { name: 'ramp', duration: seconds(30), factor: 1 },
  { name: 'recovery', duration: seconds(90), factor: 1 },
  { name: 'ramp', duration: seconds(20), factor: 0 },
];
const MEASURED = ['normal', 'high', 'peak', 'stress', 'recovery'];

function stageAt(ms) {
  let edge = 0;
  for (const p of PHASES) {
    edge += p.duration * 1000;
    if (ms < edge) return p.name;
  }
  return 'ramp';
}

function stage() {
  try {
    return stageAt(Date.now() - exec.scenario.startTime);
  } catch (e) {
    return 'setup';
  }
}

const vuStages = PHASES.map((p) => ({ duration: `${p.duration}s`, target: Math.round(NORMAL_VUS * p.factor) }));
const jobStages = PHASES.map((p) => ({ duration: `${p.duration}s`, target: Math.round(NORMAL_JOBS_PER_MIN * p.factor) }));

const jobsDone = new Counter('jobs_done');
const jobSuccess = new Rate('job_success');
const jobQueueWait = new Trend('job_queue_wait', true);
const jobRunDuration = new Trend('job_run_duration', true);
const jobEndToEnd = new Trend('job_end_to_end', true);

function perStage(metric, expression) {
  const out = {};
  for (const s of MEASURED) {
    out[`${metric}{stage:${s}}`] = [expression];
  }
  return out;
}

function perStageAndTag(metric, tag, expression) {
  const out = {};
  for (const s of MEASURED) {
    out[`${metric}{${tag},stage:${s}}`] = [expression];
  }
  return out;
}

const THRESHOLDS = {
  'http_req_duration{type:api}': ['p(95)<500'],
  'http_req_failed{type:api}': ['rate<0.01'],
  'http_req_duration{type:submit}': ['p(95)<1000'],
  'http_req_failed{type:submit}': ['rate<0.01'],
  job_success: ['rate>0.95'],
};

export const options = {
  scenarios: {
    api: {
      executor: 'ramping-vus',
      exec: 'apiUser',
      startVUs: 0,
      stages: vuStages,
      gracefulRampDown: '10s',
    },
    jobs_same_db: {
      executor: 'ramping-arrival-rate',
      exec: 'jobSameDatabase',
      timeUnit: '1m',
      startRate: 0,
      preAllocatedVUs: 10,
      maxVUs: 300,
      stages: jobStages,
      gracefulStop: `${JOB_TIMEOUT_S}s`,
    },
    jobs_spread: {
      executor: 'ramping-arrival-rate',
      exec: 'jobSpreadDatabases',
      timeUnit: '1m',
      startRate: 0,
      preAllocatedVUs: 10,
      maxVUs: 300,
      stages: jobStages,
      gracefulStop: `${JOB_TIMEOUT_S}s`,
    },
  },
  setupTimeout: '5m',
  summaryTrendStats: ['avg', 'min', 'med', 'p(90)', 'p(95)', 'max', 'count'],
  thresholds: Object.assign(
    {},
    THRESHOLDS,
    perStageAndTag('http_req_duration', 'type:api', 'p(95)<500'),
    perStageAndTag('http_req_failed', 'type:api', 'rate<0.01'),
    perStageAndTag('http_reqs', 'type:api', 'count>=0'),
    perStageAndTag('http_req_duration', 'type:submit', 'p(95)<1000'),
    perStageAndTag('job_queue_wait', 'db:same', 'max>=0'),
    perStageAndTag('job_queue_wait', 'db:spread', 'max>=0'),
    perStageAndTag('job_run_duration', 'db:same', 'max>=0'),
    perStageAndTag('job_run_duration', 'db:spread', 'max>=0'),
    perStageAndTag('job_end_to_end', 'db:same', 'max>=0'),
    perStageAndTag('job_end_to_end', 'db:spread', 'max>=0'),
    perStageAndTag('jobs_done', 'db:same', 'count>=0'),
    perStageAndTag('jobs_done', 'db:spread', 'count>=0'),
    perStage('job_success', 'rate>=0'),
    { 'dropped_iterations{scenario:jobs_same_db}': ['count>=0'], 'dropped_iterations{scenario:jobs_spread}': ['count>=0'] },
  ),
};

class Session {
  constructor(username, password) {
    this.jar = new http.CookieJar();
    const res = http.post(`${BASE}/api/auth/login`, JSON.stringify({ username, password }), {
      jar: this.jar,
      headers: { 'Content-Type': 'application/json', Origin: BASE },
      tags: { type: 'login', name: 'POST /api/auth/login' },
    });
    if (res.status !== 200) {
      throw new Error(`login ${username}: ${res.status} ${res.body}`);
    }
    this.csrf = res.json('csrfToken');
  }

  params(type, name, extraTags) {
    return {
      jar: this.jar,
      headers: { 'Content-Type': 'application/json', Origin: BASE, 'X-CSRF-Token': this.csrf },
      tags: Object.assign({ type, name, stage: stage() }, extraTags || {}),
    };
  }

  get(path, type, name, extraTags) {
    return http.get(`${BASE}${path}`, this.params(type, name, extraTags));
  }

  post(path, body, type, name, extraTags) {
    return http.post(`${BASE}${path}`, JSON.stringify(body), this.params(type, name, extraTags));
  }

  put(path, body, type, name) {
    return http.put(`${BASE}${path}`, JSON.stringify(body), this.params(type, name));
  }
}

function must(res, what) {
  if (res.status < 200 || res.status >= 300) {
    throw new Error(`${what}: ${res.status} ${res.body}`);
  }
  return res.status === 204 ? null : res.json();
}

function terminal(status) {
  return ['success', 'failed', 'partial', 'rejected', 'canceled'].includes(status);
}

function waitForExecution(session, id, tags) {
  const deadline = Date.now() + JOB_TIMEOUT_S * 1000;
  while (Date.now() < deadline) {
    const res = session.get(`/api/executions/${id}`, 'poll', 'GET /api/executions/:id', tags);
    if (res.status === 200 && terminal(res.json('status'))) {
      return { view: res.json(), seenAt: Date.now() };
    }
    sleep(0.5);
  }
  return null;
}

function submitApproved(dev, rev, databaseId, recipe, tags) {
  const migration = must(dev.post(`/api/databases/${databaseId}/migrations`, recipe, 'submit', 'POST /api/databases/:id/migrations', tags), 'create migration');
  const submittedAt = Date.now();
  const execution = must(dev.post(`/api/migrations/${migration.id}/executions`, { kind: 'run' }, 'submit', 'POST /api/migrations/:id/executions', tags), 'submit');
  for (const who of [dev, rev]) {
    must(who.post(`/api/executions/${execution.id}/approve`, {}, 'submit', 'POST /api/executions/:id/approve', tags), 'approve');
  }
  return { id: execution.id, submittedAt };
}

export function setup() {
  const admin = new Session('demo-admin', 'demoadmin123');
  const groups = Object.fromEntries(must(admin.get('/api/admin/groups', 'setup', 'setup'), 'groups').map((g) => [g.name, g.id]));
  const existing = Object.fromEntries(must(admin.get('/api/admin/databases', 'setup', 'setup'), 'databases').map((d) => [d.name, d.id]));
  const ids = {};
  for (const name of DATABASES) {
    ids[name] = existing[name] || must(admin.post('/api/admin/databases', {
      name, host: 'localhost', port: 15433, dbName: name, username: 'platform_app', password: 'platform_app', sslMode: 'disable',
    }, 'setup', 'setup'), `create ${name}`).id;
    must(admin.put(`/api/admin/groups/${groups['Tim Developer']}/grants/${ids[name]}`, { level: 'EDIT' }, 'setup', 'setup'), 'grant edit');
    must(admin.put(`/api/admin/groups/${groups['Tim Reviewer']}/grants/${ids[name]}`, { level: 'VIEW' }, 'setup', 'setup'), 'grant view');
  }

  const dev = new Session('developer', 'developer123');
  const rev = new Session('reviewer', 'reviewer123');
  const seeded = DATABASES.map((name) => submitApproved(dev, rev, ids[name], {
    sqlUp: `DROP TABLE IF EXISTS lt_items; CREATE TABLE lt_items AS SELECT g AS id, 0 AS qty FROM generate_series(1, ${ROWS}) g; ALTER TABLE lt_items ADD PRIMARY KEY (id)`,
    sqlDown: 'DROP TABLE lt_items',
    mode: 'normal',
  }, { phase: 'setup' }));
  for (const s of seeded) {
    const done = waitForExecution(dev, s.id, { phase: 'setup' });
    if (!done || done.view.status !== 'success') {
      throw new Error(`seeding execution ${s.id} did not succeed: ${done ? done.view.status : 'timeout'}`);
    }
  }
  return { ids };
}

let reader;
const readerAccounts = [['developer', 'developer123'], ['reviewer', 'reviewer123']];

export function apiUser(data) {
  if (!reader) {
    const [u, p] = readerAccounts[exec.vu.idInTest % readerAccounts.length];
    reader = new Session(u, p);
  }
  const name = DATABASES[Math.floor(Math.random() * DATABASES.length)];
  const id = data.ids[name];
  const calls = [
    ['/api/databases', 'GET /api/databases'],
    [`/api/databases/${id}/schemas/public/tables`, 'GET /api/databases/:id/schemas/:schema/tables'],
    [`/api/databases/${id}/schemas/public/tables/lt_items/rows?page=${1 + Math.floor(Math.random() * 20)}&pageSize=50`, 'GET /api/databases/:id/schemas/:schema/tables/:table/rows'],
    [`/api/databases/${id}/executions`, 'GET /api/databases/:id/executions'],
    ['/api/auth/me', 'GET /api/auth/me'],
  ];
  const [path, label] = calls[Math.floor(Math.random() * calls.length)];
  const res = reader.get(path, 'api', label);
  check(res, { 'api status is 200': (r) => r.status === 200 });
  sleep(1);
}

let developer;
let reviewer;
let counter = 0;

function runJob(databaseId, dbTag) {
  if (!developer) {
    developer = new Session('developer', 'developer123');
    reviewer = new Session('reviewer', 'reviewer123');
  }
  const at = stage();
  const tags = { db: dbTag, stage: at };
  counter += 1;
  const width = Math.floor(ROWS / SLICES);
  const slice = (exec.vu.idInTest * 7919 + counter) % SLICES;
  const lower = slice * width;
  const upper = lower + width;
  let submitted;
  try {
    submitted = submitApproved(developer, reviewer, databaseId, {
      sqlUp: `UPDATE lt_items SET qty = qty + 1 WHERE id > ${lower} AND id <= ${upper}`,
      sqlDown: `UPDATE lt_items SET qty = qty - 1 WHERE id > ${lower} AND id <= ${upper}`,
      mode: 'normal',
    }, tags);
  } catch (e) {
    jobSuccess.add(false, tags);
    return;
  }
  const done = waitForExecution(developer, submitted.id, tags);
  const ok = !!done && done.view.status === 'success';
  jobSuccess.add(ok, tags);
  check(done, { 'job reached success': () => ok }, tags);
  if (!done) return;
  jobsDone.add(1, tags);
  const queued = done.view.queuedAt ? Date.parse(done.view.queuedAt) : null;
  const started = done.view.startedAt ? Date.parse(done.view.startedAt) : null;
  const finished = done.view.finishedAt ? Date.parse(done.view.finishedAt) : null;
  if (queued && started) jobQueueWait.add(Math.max(0, started - queued), tags);
  if (started && finished) jobRunDuration.add(finished - started, tags);
  jobEndToEnd.add(done.seenAt - submitted.submittedAt, tags);
}

export function jobSameDatabase(data) {
  runJob(data.ids[SAME_DB], 'same');
}

export function jobSpreadDatabases(data) {
  runJob(data.ids[SPREAD_DBS[exec.scenario.iterationInTest % SPREAD_DBS.length]], 'spread');
}

function metricValues(data, key) {
  return data.metrics[key] ? data.metrics[key].values : null;
}

function fmtMs(v, values) {
  if (values && !values.count) return '-';
  return v === undefined || v === null ? '-' : `${Math.round(v)} ms`;
}

function phaseSeconds(name) {
  return PHASES.filter((p) => p.name === name).reduce((sum, p) => sum + p.duration, 0);
}

export function handleSummary(data) {
  const rows = [];
  rows.push('| Tahap | VU API | Job/menit per skenario | API p50 | API p95 | API req/s | API error | Submit p95 | Job selesai (same / spread) | Job sukses | Queue wait p95 (same / spread) | Durasi job p95 (same / spread) | Submit sampai selesai p95 (same / spread) |');
  rows.push('| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |');
  for (const s of MEASURED) {
    const phase = PHASES.find((p) => p.name === s);
    const api = metricValues(data, `http_req_duration{type:api,stage:${s}}`) || {};
    const reqs = metricValues(data, `http_reqs{type:api,stage:${s}}`) || {};
    const failed = metricValues(data, `http_req_failed{type:api,stage:${s}}`) || {};
    const submit = metricValues(data, `http_req_duration{type:submit,stage:${s}}`) || {};
    const v = (m, tag) => metricValues(data, `${m}{db:${tag},stage:${s}}`) || {};
    const success = metricValues(data, `job_success{stage:${s}}`) || {};
    rows.push([
      s,
      Math.round(NORMAL_VUS * phase.factor),
      Math.round(NORMAL_JOBS_PER_MIN * phase.factor),
      fmtMs(api.med, api),
      fmtMs(api['p(95)'], api),
      reqs.count !== undefined ? (reqs.count / phaseSeconds(s)).toFixed(1) : '-',
      failed.rate !== undefined ? `${(failed.rate * 100).toFixed(2)}%` : '-',
      fmtMs(submit['p(95)'], submit),
      `${v('jobs_done', 'same').count || 0} / ${v('jobs_done', 'spread').count || 0}`,
      success.rate !== undefined && (success.passes + success.fails) > 0 ? `${(success.rate * 100).toFixed(1)}%` : '-',
      `${fmtMs(v('job_queue_wait', 'same')['p(95)'], v('job_queue_wait', 'same'))} / ${fmtMs(v('job_queue_wait', 'spread')['p(95)'], v('job_queue_wait', 'spread'))}`,
      `${fmtMs(v('job_run_duration', 'same')['p(95)'], v('job_run_duration', 'same'))} / ${fmtMs(v('job_run_duration', 'spread')['p(95)'], v('job_run_duration', 'spread'))}`,
      `${fmtMs(v('job_end_to_end', 'same')['p(95)'], v('job_end_to_end', 'same'))} / ${fmtMs(v('job_end_to_end', 'spread')['p(95)'], v('job_end_to_end', 'spread'))}`,
    ].join(' | ').replace(/^/, '| ').concat(' |'));
  }
  const overall = [];
  for (const [name, t] of Object.entries(THRESHOLDS)) {
    const m = data.metrics[name];
    const ok = m && m.thresholds ? Object.values(m.thresholds).every((x) => x.ok) : false;
    overall.push(`| \`${name}\` | \`${t.join(', ')}\` | ${ok ? 'lulus' : 'gagal'} |`);
  }
  const dropped = (name) => (metricValues(data, `dropped_iterations{scenario:${name}}`) || {}).count || 0;
  const vusMax = (metricValues(data, 'vus_max') || {}).max;
  const totalReqs = (metricValues(data, 'http_reqs') || {}).count;
  const iterations = (metricValues(data, 'iterations') || {}).count;
  const text = [
    '## Hasil per tahap',
    '',
    ...rows,
    '',
    '## Threshold',
    '',
    '| Metrik | Syarat | Hasil |',
    '| --- | --- | --- |',
    ...overall,
    '',
    `Total request HTTP: ${totalReqs}, iterasi: ${iterations}, VU maksimum: ${vusMax}. Iterasi job yang tidak sempat dimulai (dropped): same ${dropped('jobs_same_db')}, spread ${dropped('jobs_spread')}.`,
    '',
  ].join('\n');
  const out = { stdout: `${text}\n` };
  if (__ENV.SUMMARY_DIR) {
    out[`${__ENV.SUMMARY_DIR}/summary.md`] = text;
    out[`${__ENV.SUMMARY_DIR}/summary.json`] = JSON.stringify(data, null, 2);
  }
  return out;
}
