# jc (Jenkins CLI) - God Mode Manual

`jc` is the comprehensive, 100% GUI-alternative terminal interface for the Jenkins-Firecracker ecosystem. This manual provides a complete reference for all features, workflows, and "superpowers" available to developers and automated agents.

## 🚀 Quick Start
```bash
# Build the tool
cd jenkins-cli && GOWORK=off go build -mod=vendor -o jc main.go

# Initial Login (saved to ~/.jenkins-cli.yaml)
./jc login --url http://141.105.65.227:8080 --user admin --token admin

# Verify connection
./jc system info
```

---

## 🏛 1. Infrastructure & Contexts
Manage multiple Jenkins clusters and physical hardware agents.

| Command | Description | Example |
| :--- | :--- | :--- |
| `jc context list` | List saved cluster profiles. | `jc context list` |
| `jc context use <name>` | Switch the active cluster. | `jc context use lab-prod` |
| `jc node list` | Audit all build agents/nodes. | `jc node list` |
| `jc node logs <name>` | Stream the agent's stdout/stderr. | `jc node logs agent-01` |
| `jc node create` | Provision a new permanent node. | `jc node create agent-02` |
| `jc sync jobs` | Reconcile job configs from one context to one or more targets. | `jc sync jobs --from lab-main --to lab-backup` |
| `jc cluster sync` | Automatically reconcile source jobs across selected cluster replicas. | `jc cluster sync --from replica-01 --all-contexts --watch` |
| `jc cluster ha` | Plan, preflight, and bundle OSS Jenkins HA-like recovery evidence. | `jc cluster ha preflight --plan-file ha-plan.json` |

---

## 🛠 2. Pipeline Development (Shared Libraries)
The ultimate toolkit for writing and debugging complex shared code.

| Command | Description | Example |
| :--- | :--- | :--- |
| `jc library list` | Find libs at Global and Folder levels. | `jc library list` |
| `jc library vars <lib>` | Discovery: list custom steps. | `jc library vars common-lib` |
| `jc library signatures <lib>` | Technical: extract `def call` params. | `jc library signatures e2e-lib` |
| `jc library doc <lib> <var>` | Documentation: read `.txt` help files. | `jc library doc utils myStep` |
| `jc library usage <lib>` | Impact Analysis: see who uses this lib. | `jc library usage pipeline-helpers` |
| `jc library edit <var>` | Live-edit global scripts in `$EDITOR`. | `jc library edit debugLogger` |
| `jc pipeline lint <file>` | Validate Declarative syntax locally. | `jc pipeline lint Jenkinsfile` |
| `jc pipeline run <file>` | Upload local source and run a temporary Jenkins job. | `jc pipeline run Jenkinsfile --source-ref b7a79fb --source-host 10.0.2.2 --logs` |
| `jc job script <job> <num>` | Get exact Jenkinsfile from history. | `jc job script test-job 42` |

---

## 🕹 3. Operational Control
Manage builds, artifacts, and interactive gates.

| Command | Description | Example |
| :--- | :--- | :--- |
| `jc job build <job> --wait`| Trigger build and wait for result. | `jc job build my-pipeline --wait` |
| `jc job info <job> [build]` | Show build details; without a build number, resolve the latest build or report that the job has no builds. | `jc job info my-pipeline` |
| `jc job logs <job> <build>` | Print the current console log snapshot. | `jc job logs my-job 42` |
| `jc job logs <job> <build> --follow` | Stream logs until the build finishes. | `jc job logs my-job 42 --follow` |
| `jc job stages <job>` | Visualize stage progress & status. | `jc job stages build-artifacts` |
| `jc job tests <job>` | Audit JUnit failure stack traces. | `jc job tests verify-results` |
| `jc job restart <j> <n> <s>`| Resume a pipeline from a specific stage. | `jc job restart job 10 Deploy` |
| `jc job branches <job>` | List branches of a Multibranch project. | `jc job branches web-app` |
| `jc exec <vm>` | Open or run a command in a Firecracker VM through NATS. | `jc --nats-url nats://127.0.0.1:4222 exec vm-01 --command 'uname -a'` |
| `jc trace <job> <build>` | Replay structured NATS build, VM, cleanup, test, and artifact evidence. | `jc --nats-url nats://127.0.0.1:4222 trace swe-bench 42 --logs --vm --tests --artifacts` |
| `jc fleet cleanup` | Dry-run or execute lease-aware Firecracker orphan cleanup on all fleet hosts. | `jc fleet cleanup --older-than 5m` |
| `jc job scan <job>` | Trigger Multibranch/Folder indexing. | `jc job scan web-app` |
| `jc job scan-logs <job>` | View indexing/repository scan logs. | `jc job scan-logs web-app` |
| `jc input list` | Audit all builds blocked on input. | `jc input list` |
| `jc input proceed <j> <n>` | Approve a manual gate/prompt. | `jc input proceed deployment 42` |
| `jc job clean <job>` | Wipe workspace and `@libs` cache. | `jc job clean buggy-pipeline` |

### Source Upload Pipeline Runs

Problem: lab VMs and Firecracker agents may not have GitHub credentials, and
forcing every experiment through a pushed remote branch slows down tight CI
loops. `jc pipeline run` creates a temporary Jenkins Pipeline job, uploads the
codebase as a tar archive served from the machine running `jc`, injects a
checksum-verified source bootstrap stage, and streams the build logs. Jenkins
agents fetch `JC_SOURCE_URL` over the route you provide; they never clone
GitHub.

Use `--source-ref` when the build must use a clean committed tree instead of
dirty local files:

```bash
jc pipeline run Jenkinsfile \
  --source-ref b7a79fb \
  --source-host 10.0.2.2 \
  --logs --timeout 15m
```

`--source-host` must be an address reachable from the Jenkins agent or VM. In
Firecracker labs this is usually the private host route or tap gateway. You can
also set `JENKINS_SOURCE_HOST` for repeated runs.

For large or pre-hosted archives, serve the tarball yourself and let Jenkins
fetch it directly:

```bash
jc pipeline run Jenkinsfile \
  --source-url http://10.0.2.2:8088/source-b7a79fb.tar.gz \
  --source-sha256 8c0d... \
  --logs
```

The generated bootstrap stage downloads the archive with `curl` or `wget`,
verifies `JC_SOURCE_SHA256` when present, extracts it into the workspace, and
then runs the original Jenkinsfile. The default archive excludes `.git`,
`node_modules`, `target`, `build`, common editor folders, and likely secret
files such as `.env.*`, `*.pem`, and `*.key`; add more patterns with
`--source-exclude`.

Verification workflow:

```bash
# Local proof that the archive path and injection are valid.
cd jenkins-cli
GOWORK=off go test ./cmd -run 'Test(CreatePipeline|InjectPipelineSource|ServePipeline)'

# Remote proof against the lab Jenkins, using the private route agents can reach.
jc pipeline run Jenkinsfile \
  --source-ref b7a79fb \
  --source-host 10.0.2.2 \
  --job-name jc-source-upload-smoke \
  --logs --timeout 15m
```

Use `--no-source-inject` only for custom Jenkinsfiles that call the
`JC_SOURCE_*` parameters themselves, or `--no-source` for a pure inline
Jenkinsfile run with no codebase upload.

### Smooth Log Streaming

Problem: Jenkins progressive console output includes hidden console annotations, which can leak as `ha:////...` blobs in raw terminals and make parallel Pipeline logs hard to read.

`jc` prints a clean console snapshot by default. Add `--follow` to keep the
progressive log stream attached until Jenkins reports the build is complete. It
strips Jenkins hidden annotations, preserves normal shell output, and
line-buffers chunks so parallel branches read like normal terminal output. If
Jenkins restarts or briefly returns its boot page while a followed log stream is
attached, `jc` retries the progressive log endpoint and resumes from the last
byte offset instead of dumping HTML into the terminal.

`jc job logs` always follows the Jenkins progressive console stream, even when
`--nats-url` is configured. Use `jc trace --logs --vm --tests --artifacts` for
NATS-backed replay of structured VM, test, artifact, cleanup, and log-plane
evidence.

```bash
jc job build firecracker-linux-kernel-10way-latest --logs --timeout 15m
jc job logs firecracker-linux-kernel-10way-latest 1
jc job logs firecracker-linux-kernel-10way-latest 1 --follow
```

The top-level `jc logs` command is the live attach shortcut. With a job target
it resolves the latest build when needed and follows Jenkins progressive logs:

```bash
jc logs firecracker-linux-kernel-10way-latest
jc logs firecracker-linux-kernel-10way-latest 42 --follow=false
```

For Firecracker VM logs, force the NATS VM log plane:

```bash
jc logs --vm firecracker-lab-pipeline-70dedb0c \
  --nats-url nats://141.105.65.227:4222
```

To inspect the NATS build log stream instead of Jenkins HTTP, use `--source
nats`; add `--replay` to read durable JetStream history before live tailing:

```bash
jc logs swe-bench-parallel-eval 12 \
  --source nats --replay \
  --nats-url nats://141.105.65.227:4222
```

Use `--tail`, `--since`, and `--until` for bounded JetStream replay. `--tail`
starts near the JetStream tail by stream sequence and then keeps the last N
matching rendered lines, so it avoids scanning the full retained log history in
the common case. Duration values are relative to the current `jc` process time;
RFC3339 timestamps are also accepted:

```bash
jc logs swe-bench-parallel-eval 12 --tail 200 --source nats
jc logs swe-bench-parallel-eval 12 --since 10m --source nats
jc logs swe-bench-parallel-eval 12 \
  --since 2026-07-04T10:00:00Z \
  --until 2026-07-04T10:15:00Z \
  --source nats
```

For agent and script consumers, `-o json` emits one structured record per log
line with timestamp, source, subject, stream, job, build, node, VM, stage,
line, and JetStream stream sequence when available:

```bash
jc logs swe-bench-parallel-eval 12 --source nats --tail 50 -o json
```

To merge Jenkins build logs with correlated Firecracker VM logs, pass `--vm`
with a Jenkins job and build number. `jc` resolves VM lease IDs from the NATS
event fabric, prefixes merged text by source, and `--no-prefix` restores plain
line output:

```bash
jc logs swe-bench-parallel-eval 12 --vm --source nats
jc logs swe-bench-parallel-eval 12 --vm --source nats --no-prefix
```

Filters run before rendering and before tail trimming:

```bash
jc logs swe-bench-parallel-eval 12 --source nats --grep 'ERROR|OOM|Exception'
jc logs swe-bench-parallel-eval 12 --source nats --level error --node firecracker-host-2
jc logs swe-bench-parallel-eval 12 --source nats --stage Test
```

For long-running sessions, store a cursor so a later attach resumes after the
last observed JetStream stream sequence for each matching subject. Legacy
timestamp-only cursor files are still accepted, but new cursor files prefer
sequence resume:

```bash
jc logs swe-bench-parallel-eval 12 \
  --source nats --cursor .jc/cursors/swe-bench-parallel-eval-12.json
```

When JetStream retention or compaction advances past the saved cursor, `jc`
prints a gap warning before continuing from the next available record:

```text
log gap detected: stream advanced from 1204 to 1211
```

If no live NATS messages arrive within `--idle-timeout` (10s by default), `jc`
prints the subscribed target and the Jenkins HTTP fallback command.

Restart-resume example:

```text
 make -C /tmp/jh/firecracker-cache/kernel-sources/linux-7.0.12 O=/tmp/jh/firecracker-agent/workspace/firecracker-linux-kernel-10way-latest/out-kernel-10 -j2 bzImage
 CC      drivers/gpu/drm/i915/gem/i915_gem_lmem.o
 CC      net/ipv6/protocol.o
```

Expected showcase output for parallel Firecracker builds:

```text
Running on Firecracker agent firecracker-lab-pipeline-70dedb0c using image ubuntu-jammy
+ VARIANT=kernel-10
+ OUT=/tmp/jh/firecracker-agent/workspace/firecracker-linux-kernel-10way-latest/out-kernel-10
+ make -C /tmp/jh/firecracker-cache/kernel-sources/linux-7.0.12 O=/tmp/jh/firecracker-agent/workspace/firecracker-linux-kernel-10way-latest/out-kernel-10 -j2 bzImage
```

Use `--raw` only when diagnosing Jenkins console-note behavior:

```bash
jc job logs firecracker-linux-kernel-10way-latest 1 --raw
```

### NATS Trace And Cleanup Evidence

`jc trace` replays the structured event fabric instead of only reading console
text. Use the detail flags when debugging Firecracker agents or SWE-style
evaluation shards. Event and log subjects are replayed in parallel, so a build
with many Firecracker leases does not wait serially on every possible VM log
subject before printing the timeline:

```bash
jc --nats-url nats://127.0.0.1:4222 \
  trace swe-bench-parallel-eval 12 \
  --logs --vm --tests --artifacts --limit 500
```

The default replay wait is `500ms` per subject. Increase it only when replaying
from a slow or remote broker:

```bash
jc trace JOB BUILD --logs --wait 2s
```

The flags are additive:

```bash
jc trace JOB BUILD --vm        # include VM, lease, and cleanup fields
jc trace JOB BUILD --tests     # include test and failure-tail fields
jc trace JOB BUILD --artifacts # include artifact path fields
jc trace JOB BUILD --proc-ai   # include Proc-AI process graph and anomaly fields
```

Useful hostd cleanup events include:

```text
lease.cleanup.started
lease.cleanup.finished
lease.cleanup.failed
lease.orphan_process.detected
lease.orphan_process.cleaned
lease.zombie.detected
```

Fleet cleanup is report-only by default. It calls each configured hostd through
Jenkins, using the fleet host shared secret already stored in the cloud config:

```bash
jc fleet cleanup --older-than 5m
```

To terminate old live orphan Firecracker processes after reviewing the dry-run:

```bash
jc fleet cleanup --execute --older-than 5m --kill-wait 5s --emit-nats
```

Zombie processes are never killed. They are reported as
`lease.zombie.detected`; the parent process must reap them with `wait()`.

### Proc-AI Process Graph

`hostd` records a lease-scoped Proc-AI snapshot for each Firecracker agent. The
phase-1 implementation samples the host-visible Firecracker and virtiofsd
process tree through `/proc`, computes thread, FD, RSS, and I/O delta totals,
and emits anomaly events such as `process.gone`, `process.zombie`, `fd.high`,
and `io.write_spike`.

This hostd snapshot baseline is available for Firecracker Linux leases. The
kernel-backed `/proc/ai`, eBPF, runtime adapter, and AI-debugging roadmap phases
are experimental and outside the public v1 guarantee.

Replay those fields with the regular trace command:

```bash
jc trace swe-aero-ci-confirm 42 \
  --vm --proc-ai --logs --tests --artifacts \
  --nats-url nats://141.105.65.227:4222
```

Tail one live lease after its instance ID appears in `jc trace --vm`:

```bash
jc debug inspect firecracker-lab-pipeline-abc123 \
  --nats-url nats://141.105.65.227:4222
```

### Cloud Transport Configuration

Use `jc cloud configure` to switch an existing Firecracker cloud between the
host-side vsock remoting bridge and NATS-backed Jenkins remoting:

```bash
jc cloud configure firecracker \
  --agent-transport nats \
  --nats-url nats://127.0.0.1:4222 \
  --nats-ready-timeout 120

jc job build swe-aero-ci-confirm --wait --logs
jc trace swe-aero-ci-confirm last --vm --logs --tests --artifacts

jc cloud configure firecracker --agent-transport vsock
```

For automation, omit flags and set `JENKINS_AGENT_TRANSPORT`,
`JENKINS_NATS_URL`, and `JENKINS_NATS_READY_TIMEOUT_SECONDS`; the command also
accepts the `FIRECRACKER_*` and `AERO_*` variants.

### Experimental SWE-Bench-Style OpenCode Matrix

The repository includes a Jenkins job recipe that runs SWE-bench-style
OpenCode evaluation shards on Firecracker agents over the NATS transport. The
job has a safe synthetic dry-run mode for proving VM fan-out, prediction
artifacts, JUnit, Allure, NATS trace replay, and cleanup without calling paid
model APIs.

```bash
jc cloud configure a \
  --agent-transport nats \
  --nats-url nats://141.105.65.227:4222 \
  --nats-ready-timeout 120

jc script examples/groovy-scripts/swe-opencode-firecracker-nats.groovy

jc job build swe-opencode-firecracker-nats \
  -p BENCH_MODE=synthetic \
  -p DRY_RUN=true \
  -p TASK_LIMIT=12 \
  -p SHARDS=2 \
  -p PROVIDERS=deepseek,zai,kimi \
  -p VM_CPUS=4 \
  -p VM_MEMORY=4096m \
  --wait --logs

jc trace swe-opencode-firecracker-nats last \
  --logs --vm --tests --artifacts
```

For real OpenCode runs, set `DRY_RUN=false` and store rotated API keys in
Jenkins string credentials with these IDs:

- `opencode-deepseek-api-key`
- `opencode-zai-api-key`
- `opencode-kimi-api-key`

The job writes per-provider `predictions-*.jsonl` files, per-shard
`results.ndjson`, JUnit XML, Allure results, patches, logs, and
`evidence.tgz` bundles. Use `BENCH_MODE=swe-bench` with `DATASET_NAME`,
`DATASET_SPLIT`, and `TASK_LIMIT=0` to load the full selected Hugging Face
SWE-bench split.

---

## 🔐 4. Security & Administration
Low-level control over the Jenkins JVM and configuration.

| Command | Description | Example |
| :--- | :--- | :--- |
| `jc approval list` | See scripts blocked by Sandbox. | `jc approval list` |
| `jc approval approve <id>` | Whitelist a blocked script/signature. | `jc approval approve "method java.io.File ..."` |
| `jc credential list` | Audit secret IDs and types globally. | `jc credential list` |
| `jc system env set <k> <v>` | Set Global Environment Variables. | `jc system env set PROD_URL "https://..."` |
| `jc plugin upload <hpi>` | Direct install of local plugin files. | `jc plugin upload my-plugin.hpi -r` |
| `jc plugin disable <id>` | Prevent a plugin from loading. | `jc plugin disable script-security` |
| `jc shell` | **Interactive REPL** to the Jenkins JVM. | `jc shell` |
| `jc doctor <job> <num>` | Explain failed builds from pipeline, logs, agents, and artifacts. | `jc doctor failing-job 5` |
| `jc cluster controllers` | Audit one or more Jenkins controllers from saved contexts. | `jc cluster controllers --all-contexts` |
| `jc cluster backup` | Create a portable controller backup zip. | `jc cluster backup --context lab -f lab-backup.zip` |
| `jc cluster restart` | Trigger Jenkins safeRestart with confirmation or dry-run. | `jc cluster restart --context lab --dry-run` |
| `jc cluster plugin install` | Install plugins across selected controllers. | `jc cluster plugin install warnings-ng --context lab --dry-run` |
| `jc cluster plugin upgrade` | Upgrade named plugins or all plugins with updates. | `jc cluster plugin upgrade --all-contexts --dry-run` |

### Explainable Build Doctor

Problem: Jenkins usually tells you only that a build is red. Operators still have to manually correlate the Jenkinsfile, stage view, console output, agent provisioning, and archived evidence bundles before they can say what failed and how to rerun it.

`jc doctor` turns those signals into one report:

- Likely cause
- Evidence
- Exact stage/branch
- Relevant log lines
- Suggested fix
- Rerun command
- Pipeline static model
- Stage timing
- Agent lifecycle events
- Artifacts/evidence bundles

Showcase commands:

```bash
jc doctor firecracker-linux-kernel-10way-latest 4
jc doctor firecracker-linux-kernel-10way-latest 4 -o json
jc doctor firecracker-linux-kernel-10way-latest 4 --bundle --bundle-output doctor-build-4.zip
```

Example output:

```text
Likely cause:
  [pipeline-script-error/high] A pipeline shell/Python script failed with SyntaxError: unterminated string literal

Exact stage/branch:
  Stage:  Build kernel-01
  Branch: kernel-01

Evidence:
  - Line 349 matched pipeline-script-error: SyntaxError: unterminated string literal
  - Build result is FAILURE after 11m56s.
  - Static pipeline model found 11 stage(s).
  - Build published 10 artifact(s), including 10 evidence-related file(s).

Suggested fix:
  1. Fix the generated script or heredoc/quote escaping in the failing stage.
  2. Run the embedded script locally or with a lightweight syntax check before rerunning the Jenkins job.

Rerun command:
  jc job build firecracker-linux-kernel-10way-latest --logs --timeout 30m
```

Portable bundle example:

```bash
unzip -l doctor-build-4.zip
```

Expected bundle files:

```text
manifest.json
diagnosis.json
diagnosis.txt
log-excerpt.txt
Jenkinsfile
stage-graph.json
artifact-manifest.json
agent-events.json
README.txt
```

### Cluster Operations

Problem: Jenkins controller operations are usually spread across GUI pages, Groovy snippets, plugin-manager forms, and ad hoc file copies. That makes backups, restarts, and plugin rollouts hard to repeat safely across lab, staging, and production controllers.

`jc cluster` turns saved contexts into a controller operations plane:

- Inventory selected controllers and plugin update counts.
- Produce portable backup zips containing controller metadata, system config, plugin inventory, job manifests, and job config XML.
- Dry-run controller restarts before triggering Jenkins `safeRestart`.
- Install or upgrade plugins on one controller or every saved context.
- Return JSON/YAML for automation and table output for humans.

Showcase commands:

```bash
jc cluster controllers --all-contexts
jc cluster controllers --context lab -o json

jc cluster backup --context lab -f lab-controller-backup.zip
unzip -l lab-controller-backup.zip

jc cluster restart --context lab --dry-run
jc cluster restart --context lab --yes

jc cluster plugin install warnings-ng --context lab --dry-run
jc cluster plugin install git@5.2.1 --context lab --force --restart

jc cluster plugin upgrade --all-contexts --dry-run
jc cluster plugin upgrade git workflow-job --context lab --restart
```

Cluster job sync:

Problem: OSS Jenkins controllers do not share live `$JENKINS_HOME` state. In a lab or warm-standby cluster, job definitions drift unless every config change is copied and verified on each replica. Manual XML copy is error-prone, and it usually leaves no evidence of what changed.

`jc sync` and `jc cluster sync` reconcile job config XML from a source context to target contexts:

- `jc sync job <name>` copies one job config, including `folder/job` full names.
- `jc sync jobs` recursively copies all job and folder configs from the source.
- `jc cluster sync` uses cluster target selection via `--context` or `--all-contexts`.
- `--prune` deletes target jobs that no longer exist in the source.
- `--watch --interval <duration>` keeps reconciling automatically.
- `--manifest-file <path>` writes an evidence JSON manifest with per-target create, update, unchanged, delete, and failure counts.

Showcase commands:

```bash
jc sync job release-smoke --from lab-main --to lab-backup
jc sync jobs --from lab-main --to lab-backup --prune --manifest-file sync-once.json

jc cluster sync --from replica-01 --context replica-02 --context replica-03 --prune
jc cluster sync --from replica-01 --all-contexts --watch --interval 10s --manifest-file cluster-sync.json
```

What sync currently covers: job and folder `config.xml`, recursive folder contents, create/update/delete reconciliation, dry-run output, JSON/YAML/table rendering, and evidence manifests. It intentionally does not claim to synchronize build history, running builds, queue state, workspaces, artifacts, credentials, secrets, plugins, nodes, or CloudBees HA runtime state.

OSS Jenkins HA recovery:

Problem: OSS Jenkins can be made highly recoverable, but it is unsafe to pretend a standby controller is ready unless jobs, plugins, agents, queue intent, artifact storage, secret mapping, and running-build boundaries have been checked together. Operators need a repeatable plan, a preflight gate, and a portable evidence bundle before flipping traffic or rerunning interrupted builds.

`jc cluster ha` turns those checks into a command family:

- `jc cluster ha plan` writes `ha-plan.json` and operator notes for the selected recovery level.
- `jc cluster ha inventory` collects job hashes, plugin versions, nodes, queue items, executor count, artifact-manager detection, and running builds from selected controllers.
- `jc cluster ha preflight` evaluates reachability, duplicate endpoints, job drift, plugin drift, external event/artifact/secret stores, built-in executors, and running-build limits.
- `jc cluster ha status` returns a compact readiness summary for automation.
- `jc cluster ha doctor` creates a portable support bundle with the plan, inventory, preflight result, and human summary.
- `jc cluster ha fork-spec` generates the Jenkins core/workflow-cps fork contract and destructive acceptance suite required before HA4 live migration can be claimed.

Showcase commands:

```bash
jc cluster ha plan \
  --primary replica-01 \
  --standby replica-02 \
  --standby replica-03 \
  --level HA3 \
  --artifact-store s3://ci-artifacts/jenkins \
  --event-store nats://nats.service.consul:4222 \
  --secret-store vault://secret/jenkins \
  --proxy-type nginx \
  --proxy-endpoint https://jenkins.example.com \
  --output-dir ha-plan

jc cluster ha inventory --all-contexts -o json
jc cluster ha preflight --plan-file ha-plan/ha-plan.json
jc cluster ha status --plan-file ha-plan/ha-plan.json -o json
jc cluster ha doctor --plan-file ha-plan/ha-plan.json --bundle ha-doctor.zip

jc cluster ha fork-spec \
  --primary replica-01 \
  --standby replica-02 \
  --jenkins-ref jenkinsci/jenkins:ha4-runtime \
  --workflow-cps-ref jenkinsci/workflow-cps:ha4-runtime \
  --state-store nats-jetstream://jenkins-ha4-flow-state \
  --lease-store etcd://jenkins-ha4-leases \
  --artifact-store s3://ci-artifacts/jenkins \
  --output-dir ha4-fork-spec
```

Expected HA doctor bundle files:

```text
manifest.json
ha/plan.json
ha/inventory.json
ha/preflight.json
ha/summary.txt
```

Expected HA4 fork-spec files:

```text
ha4-fork-spec.json
README.txt
architecture.md
fork-task-list.md
acceptance-tests.md
implementation-map.md
```

Example status output:

```text
READY level=HA3 pass=12 warn=3 fail=0
```

Boundary: `jc cluster ha` implements deterministic OSS Jenkins recovery and rerun readiness. It deliberately rejects `--level HA4` because live migration of running Pipeline CPS runtime state is not an OSS Jenkins plugin feature; that requires a Jenkins core/workflow runtime fork or CloudBees HA-class runtime support. The fork contract lives in `docs/jenkins-ha4-live-migration-spec.md` and can be regenerated as an operator/task bundle with `jc cluster ha fork-spec`.

CloudBees-style active-active plan:

```bash
jc cluster active-active plan \
  --name cb-controller-ha \
  --namespace cloudbees \
  --replicas 3 \
  --max-replicas 5 \
  --storage-class nfs-rwx \
  --storage-size 250Gi \
  --host jenkins.example.com \
  --output-dir active-active-plan

jc cluster active-active preflight --context replica-01 --context replica-02 --context replica-03
jc cluster active-active preflight --all-contexts -o json
```

Expected active-active plan files:

```text
00-namespace.yaml
10-pvc.yaml
20-deployment.yaml
30-service.yaml
40-pdb.yaml
50-hpa.yaml
60-ingress.yaml
README.txt
```

The active-active planner follows the CloudBees CI HA shape: Deployment-based controller replicas, shared ReadWriteMany `$JENKINS_HOME`, sticky load balancing, Hazelcast replica communication, `/health/` checks, anti-affinity, PodDisruptionBudget, and per-replica cache space outside shared home. It intentionally rejects `--platform oss-jenkins` for a single shared-home active-active controller because ordinary Jenkins controllers do not provide the CloudBees HA state synchronization layer.

Expected backup zip files:

```text
manifest.json
controllers/<context>/controller.json
controllers/<context>/system/config.xml
controllers/<context>/plugins/plugins.json
controllers/<context>/jobs/manifest.json
controllers/<context>/jobs/<job>/config.xml
```

Safety rules:

- `jc cluster restart` refuses to run unless `--yes` or `--dry-run` is present.
- Plugin install skips already installed plugins unless `--force` is set.
- Plugin upgrade with no names targets only plugins where Jenkins reports `hasUpdate=true`.
- Use `--context <name>` for a single controller and `--all-contexts` only after checking `jc context list`.
- `jc cluster ha preflight` treats running builds, built-in executors, local artifact storage, missing event stores, and non-externalized credentials as warnings unless they make deterministic recovery impossible.
- `jc cluster ha plan --level HA4` is intentionally rejected for OSS Jenkins.
- `jc cluster ha fork-spec` is a fork implementation contract, not a runtime toggle.
- `jc cluster active-active plan` is a CloudBees-style implementation plan. It is not a license, Operations Center policy, or proof that a storage class is actually RWX.

Comprehensive validation:

```bash
cd jenkins-cli
GOWORK=off go build -mod=vendor -o ../target/jc-cluster-test main.go
cd ..
JC_BIN="$PWD/target/jc-cluster-test" scripts/cluster-solution-e2e.sh
```

Remote Docker host validation:

```bash
scp target/jc-cluster-test scripts/cluster-solution-e2e.sh root@<host>:/tmp/
ssh root@<host> \
  'JC_BIN=/tmp/jc-cluster-test JC_CLUSTER_E2E_ROOT=/tmp/jc-cluster-solution-e2e JC_CLUSTER_E2E_PORT_BASE=19180 JC_CLUSTER_E2E_PREFIX=jc-cluster-solution-e2e /tmp/cluster-solution-e2e.sh'
```

The E2E harness creates three disposable Jenkins controllers in Docker, seeds jobs, installs `command-launcher` and `cloudbees-folder`, restarts controllers, verifies backup payloads, creates stale target jobs, runs dry-run/apply/idempotent/watch sync, builds synced jobs, streams latest logs, runs job list/disable/enable, generates an HA recovery plan, inventories all controllers, verifies HA3 preflight/status, creates an HA doctor bundle, checks HA4 rejection, generates an HA4 fork-spec bundle, exercises NATS-backed `jc exec`, checks active-active planning, tests expected failures, stops one controller, verifies failure detection, restarts it, and verifies the cluster becomes healthy again.

Expected final output:

```text
CLUSTER_SOLUTION_E2E_PASS cases=49
evidence=/tmp/jc-cluster-solution-e2e/evidence
<sha256>  /tmp/jc-cluster-solution-e2e/evidence/cluster-backup.zip
```

---

## 🧠 5. The "Escape Hatches" (API & Script)
For when you need logic not covered by standard commands.

1. **Direct API Access:**
   `./jc api /pluginManager/plugins` (GET raw JSON)
2. **Groovy Injections:**
   `./jc script "println Jenkins.instance.pluginManager.plugins"`
3. **Bulk Edits:**
   `./jc job edit my-job` (Opens job XML in local `$EDITOR`)

## 🏁 Workflow Summary for AI Agents:
1. **Always** check `jc context list` to ensure you are on the correct server.
2. **Before** changing a Shared Library, run `jc library usage` to see the blast radius.
3. **If** a build fails, run `jc doctor` first to get the likely cause, exact stage/branch, relevant log lines, evidence bundles, suggested fix, and rerun command.
4. **Never** use the GUI. If functionality is missing, use `jc script` or `jc shell` to perform the action via Groovy.
5. **Every** user-facing command or workflow change must update this manual with the problem it solves, runnable examples, and at least one expected showcase output or artifact layout.
