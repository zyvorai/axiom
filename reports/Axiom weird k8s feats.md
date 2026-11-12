# Features almost nobody ships in a k8s console

Research date: 2026-11-12. Goal: fill Axiom with **private-cloud console feats people forget exist** — grounded in seams already in zyvor-axiom (Still/Seal, PacketWolf, Atlas, fleet search, agents, presence tracking, intents, upgrade risk), not another nodes table.

## Why these are rare

Most consoles invent inventory, YAML editors, and Prometheus tiles. Axiom already has containment vocabulary (Still/Seal), operator presence counters, PacketWolf flow brain, Atlas volumes, ProposeOnly agents, and fleet search — half-wired. The unusual product is finishing those into **ops theatre** competitors do not have a word for.

## Pack A — Containment theatre (Axiom-native, highest uniqueness)

| # | Feature | Why nobody thinks of it | Build on |
|---|---------|-------------------------|----------|
| A1 | **Live operator blast radius** — before seal/sleep/end, show “3 terminals + 2 pod windows are open on these pods *right now*” | Consoles know objects, not that a human is mid-exec into the thing you are about to isolate | `blast.go`, `track.go` CountPresence/CountWindows, Still preview |
| A2 | **Finish the Still ladder** — hush → sleep → seal → end as one preview UX (today only seal/unseal execute) | Delete/restart are everywhere; staged “quiet the app” vocabulary is not | `StillAction`, `stillPreview`, SealWorkload |
| A3 | **PacketWolf seal receipt** — after Seal, re-query flows and write Timeline “flows before → after” | NetworkPolicy UIs stop at “policy created”; flow verification in the same console is rare | PacketWolf proxy, seal, timeline/audit |
| A4 | **Network Guard on the workload row** — live `flowsCount` + Isolate next to Seal | Policies live on Networking pages; “isolate *this* noisy app” belongs on the workload | `network/guard`, WorkloadDetail |
| A5 | **Atlas volumes in Still preview** — “these volumes survive sleep/end” (GAPS: `WorkloadVolumes` missing from Provider) | Storage fabric + containment preview in one dialog is private-cloud specific | blast VolumeLister, Atlas APIs |

## Pack B — Fleet ghosts & twins

| # | Feature | Why nobody thinks of it | Build on |
|---|---------|-------------------------|----------|
| B1 | **Sealed twin search** — fleet search filter `sealed:` / intent annotation | Cross-cluster name search exists; “find every network-isolated twin” does not | `fleet_search.go`, seal annotations |
| B2 | **Twin diff panel** — same workload name across clusters: image/replicas/status side-by-side | Search usually navigates to one hit; twin-diff is a fleet-console move | FleetSearchHit, cluster switcher |
| B3 | **Outage narrative** — fleet incident window × Timeline × AgentRun plans into one story | Tools have alerts *or* agents *or* health history; few compose all three | fleet history, timeline, agentruns |

## Pack C — Operator presence & memory

| # | Feature | Why nobody thinks of it | Build on |
|---|---------|-------------------------|----------|
| C1 | **“Who poked it” on Timeline** — merge pod-window / terminal presence into the event stream | Timelines show cluster events; almost none show console operator presence | track.go, pod proxy IncWindow, Timeline |
| C2 | **Place Intent ↔ Platform Intent drift** — NL launch annotation vs Platform Intent CR vs live replicas | Dual intent models are already Axiom-specific; competitors have neither to reconcile | intent.go, PlatformIntents |
| C3 | **Health finding → Seal ProposeOnly** — agent allowlist `seal_workload` after blast preview | Agents restart/delete; network containment as approved action is distinctive | agent_actions, HealthCheck |
| C4 | **Upgrade-risk codes → prep actions** — map `pdb-blocks-drain` → cordon/scale allowlisted steps | Lite assessors exist; code→human-approved prep pack is uncommon in OSS | upgrade_risk, agent_actions |

## Pack D — Data-plane truth (not “can the console reach X”)

| # | Feature | Why nobody thinks of it | Build on |
|---|---------|-------------------------|----------|
| D1 | **Tenant-pod reachability matrix** — exec curl/wget from chosen pods to Atlas / IdP / URL | Consoles test console→service; “can *this tenant pod* reach X” is ops gold | `TestPodProbe`, Integrations |
| D2 | **MCP tools for Still + fleet + guard** — external agents call Axiom semantics, not raw k8s | Cluster MCP is usually kubectl-shaped; wrapping blast/fleet is a different product | mcp.go |

## Pack E — Weird inventory nobody puts on a dashboard

| # | Feature | Why nobody thinks of it | Notes |
|---|---------|-------------------------|-------|
| E1 | **Finalizer ghosts** — objects stuck Terminating > N minutes with who owns the finalizer | Operators discover these by accident in `kubectl get` | List + age filter; Timeline card |
| E2 | **Image tag float alarm** — running digest vs `:latest` / mutable tag on Deployments | “What’s actually running” vs what YAML claims | Pod status containerIDs / imageID |
| E3 | **HPA thrash detector** — scale-up then scale-down within a window | Autoscaling UIs show desired; few show oscillation as a finding | HPA + Events |
| E4 | **Service with Ingress but 0 ready endpoints** — north-south lie detector | Ingress green, Service empty = silent outage class | Endpoints/EndpointSlice + Ingress |
| E5 | **Cert “will break at”** — cert-manager Certificate notAfter on Timeline + Alerts | Often a Grafana board, rarely a first-class console deadline | Detect cert-manager CRD |
| E6 | **Secret never-rotated age** — Secret creationTimestamp / annotation age heatmap (metadata only, never values) | Security pages show CVE scanners; rotation hygiene is ignored | ListSecrets age |
| E7 | **Node clock skew hint** — compare node Ready heartbeat skew / metrics time offset | Causes TLS and lease weirdness; almost never surfaced | Node conditions + optional Prom |
| E8 | **GPU wasted-FLOPS card** — requested GPU pods with near-zero DCGM util (idle money) | Cost shows dollars; “paid silicon doing nothing” is the operator gut punch | Accelerators + DCGM (from GPU pack) |

## Pack F — GPU console (from prior research; still ship)

Immediate + P0 + P1: accelerators API/UI, gpuUsd, GFD sharing/MIG, quotas, FailedScheduling, DCGM PromQL, DRA detect — Portainer-class, not Run:ai.

## Explicitly still out

- Browser TCP port-forward, ClusterRole write UI
- Run:ai fairshare / model catalogs / notebook MLOps
- Full chaos studio (Krkn) — deep-link only if CRDs present
- Automatic mute of production without ProposeOnly / confirm

## Suggested ship order when Agent mode is on

1. **GPU Immediate** (unblocks commit of existing nvidia_gpu*.go)  
2. **A1–A5 Still/Seal/PacketWolf/Atlas** (maximum “no one thought of this”)  
3. **C1 + C3 + B1** (presence timeline, seal-from-agent, sealed search)  
4. **D1 + E1–E4** (reachability + weird findings)  
5. **GPU P0/P1 + E8**  
6. **B2–B3, C2, C4, D2, E5–E7**  
7. Commit + push entire dirty tree

## Sources

- In-repo exploration of Still/blast/PacketWolf/fleet/agents/upgrade-risk seams (2026-11-12)
- Prior report: `reports/Axiom GPU console additions.md`
- External contrast only: Portainer GPU, OpenShift DCGM, Radar/Komodor-style root-cause, Periscope Karpenter pending reasons, Headlamp activities — used to show what *others* emphasize so Axiom can lean into what they miss
