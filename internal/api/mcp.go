package api

// MCP (Model Context Protocol) HTTP endpoint for Axiom.
// JSON-RPC 2.0 over POST /api/v1/mcp — read tools plus HITL writes
// (create_agent_run / approve / deny / propose_seal). No ephemeral ClusterRole
// sandbox yet; writes stay ProposeOnly or human-approved allowlisted actions.
// Auth is the same session/API-key guard as the rest of the API.

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/zyvorai/axiom/internal/platform"
)

type mcpRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params"`
}

type mcpResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  any             `json:"result,omitempty"`
	Error   *mcpError       `json:"error,omitempty"`
}

type mcpError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type mcpTool struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"inputSchema"`
}

func (s *Server) registerMCP(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/v1/mcp", s.mcpRPC)
	mux.HandleFunc("GET /api/v1/mcp", s.mcpInfo)
	mux.HandleFunc("GET /api/v1/mcp/tools", s.mcpToolsHTTP)
}

func (s *Server) mcpInfo(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"protocol": "mcp",
		"transport": "http+jsonrpc",
		"endpoint": "/api/v1/mcp",
		"server": map[string]string{
			"name":    "zyvor-axiom",
			"version": buildVersion,
		},
		"capabilities": map[string]any{"tools": map[string]any{}},
		"instructions": "POST JSON-RPC 2.0 to /api/v1/mcp. Auth with the same session cookie or API key as the Axiom console. Writes are HITL only: create_agent_run / propose_seal (ProposeOnly) and approve_agent_run / deny_agent_run.",
	})
}

func (s *Server) mcpToolsHTTP(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"tools": s.mcpToolDefs()})
}

func (s *Server) mcpRPC(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, mcpResponse{JSONRPC: "2.0", Error: &mcpError{Code: -32700, Message: "parse error"}})
		return
	}
	var req mcpRequest
	if err := json.Unmarshal(body, &req); err != nil {
		writeJSON(w, http.StatusOK, mcpResponse{JSONRPC: "2.0", Error: &mcpError{Code: -32700, Message: "parse error"}})
		return
	}
	if req.JSONRPC == "" {
		req.JSONRPC = "2.0"
	}
	res := s.mcpDispatch(r, req)
	writeJSON(w, http.StatusOK, res)
}

func (s *Server) mcpDispatch(r *http.Request, req mcpRequest) mcpResponse {
	out := mcpResponse{JSONRPC: "2.0", ID: req.ID}
	switch req.Method {
	case "initialize":
		out.Result = map[string]any{
			"protocolVersion": "2024-11-05",
			"capabilities":    map[string]any{"tools": map[string]any{}},
			"serverInfo":      map[string]string{"name": "zyvor-axiom", "version": buildVersion},
			"instructions":    "Axiom MCP: investigate a k3s/Kubernetes cluster and create ProposeOnly agent runs for human approval.",
		}
	case "notifications/initialized", "ping":
		out.Result = map[string]any{}
	case "tools/list":
		out.Result = map[string]any{"tools": s.mcpToolDefs()}
	case "tools/call":
		result, err := s.mcpCallTool(r, req.Params)
		if err != nil {
			out.Error = &mcpError{Code: -32000, Message: err.Error()}
		} else {
			out.Result = result
		}
	default:
		out.Error = &mcpError{Code: -32601, Message: "method not found: " + req.Method}
	}
	return out
}

func (s *Server) mcpToolDefs() []mcpTool {
	str := map[string]any{"type": "string"}
	obj := func(props map[string]any, required ...string) map[string]any {
		schema := map[string]any{"type": "object", "properties": props}
		if len(required) > 0 {
			schema["required"] = required
		}
		return schema
	}
	return []mcpTool{
		{Name: "list_pods", Description: "List pods in a namespace", InputSchema: obj(map[string]any{"namespace": str}, "namespace")},
		{Name: "list_events", Description: "List recent Warning events in a namespace", InputSchema: obj(map[string]any{"namespace": str}, "namespace")},
		{Name: "list_nodes", Description: "List cluster nodes", InputSchema: obj(map[string]any{})},
		{Name: "list_workloads", Description: "List workloads (Deployments/StatefulSets/DaemonSets) in a namespace", InputSchema: obj(map[string]any{"namespace": str})},
		{Name: "diagnose_pod", Description: "Plain-English diagnosis of why a pod is not ready", InputSchema: obj(map[string]any{"namespace": str, "name": str}, "namespace", "name")},
		{Name: "cluster_health", Description: "Run the deterministic cluster health check", InputSchema: obj(map[string]any{"namespace": str})},
		{Name: "upgrade_risk", Description: "Assess cluster upgrade readiness (nodes, PDBs, warnings)", InputSchema: obj(map[string]any{})},
		{Name: "list_agent_runs", Description: "List agentic remediation runs", InputSchema: obj(map[string]any{"namespace": str})},
		{Name: "create_agent_run", Description: "Create a ProposeOnly agent run (human must approve before changes)", InputSchema: obj(map[string]any{
			"namespace": str, "goal": str, "kind": str, "name": str, "dryRun": map[string]any{"type": "boolean"},
		}, "goal")},
		{Name: "get_workload", Description: "Get a single workload resource detail", InputSchema: obj(map[string]any{"namespace": str, "kind": str, "name": str}, "namespace", "kind", "name")},
		{Name: "list_firing_alerts", Description: "List firing Alertmanager alerts when AXIOM_ALERTMANAGER_URL is set", InputSchema: obj(map[string]any{})},
		{Name: "list_accelerators", Description: "NVIDIA GPU inventory (nodes, pods, operator, quotas, DRA)", InputSchema: obj(map[string]any{})},
		{Name: "still_preview", Description: "Preview Still ladder blast radius (hush/sleep/seal/end)", InputSchema: obj(map[string]any{
			"namespace": str, "kind": str, "name": str, "action": str,
		}, "namespace", "kind", "name")},
		{Name: "fleet_search", Description: "Cross-cluster search; prefix sealed: to find network-isolated twins", InputSchema: obj(map[string]any{"q": str}, "q")},
		{Name: "network_guard", Description: "PacketWolf / isolation summary for a workload", InputSchema: obj(map[string]any{
			"namespace": str, "kind": str, "name": str,
		}, "namespace", "name")},
		{Name: "list_dra", Description: "DRA inventory (DeviceClass / ResourceSlice / ResourceClaim)", InputSchema: obj(map[string]any{})},
		{Name: "outage_narrative", Description: "Compose fleet outage narrative (who poked it + agent runs)", InputSchema: obj(map[string]any{
			"cluster": str, "from": str, "state": str,
		}, "cluster")},
		{Name: "propose_seal", Description: "Create a ProposeOnly agent run to Seal a workload (HITL)", InputSchema: obj(map[string]any{
			"namespace": str, "kind": str, "name": str, "goal": str,
		}, "namespace", "name")},
		{Name: "approve_agent_run", Description: "Approve a pending ProposeOnly agent run (HITL write)", InputSchema: obj(map[string]any{
			"namespace": str, "name": str,
		}, "namespace", "name")},
		{Name: "deny_agent_run", Description: "Deny a pending ProposeOnly agent run", InputSchema: obj(map[string]any{
			"namespace": str, "name": str,
		}, "namespace", "name")},
	}
}

func (s *Server) mcpCallTool(r *http.Request, params json.RawMessage) (map[string]any, error) {
	var p struct {
		Name      string         `json:"name"`
		Arguments map[string]any `json:"arguments"`
	}
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, fmt.Errorf("invalid params")
	}
	args := p.Arguments
	if args == nil {
		args = map[string]any{}
	}
	str := func(k string) string {
		v, _ := args[k].(string)
		return strings.TrimSpace(v)
	}
	ctx := r.Context()
	var payload any
	var err error
	switch p.Name {
	case "list_pods":
		payload, err = s.provider.ListPods(ctx, str("namespace"))
	case "list_events":
		payload, err = s.provider.ListEvents(ctx, str("namespace"))
	case "list_nodes":
		payload, err = s.provider.ListNodes(ctx)
	case "list_workloads":
		ns := str("namespace")
		if ns == "" {
			ns = "all"
		}
		payload, err = s.provider.ListWorkloads(ctx, ns)
	case "diagnose_pod":
		payload, err = s.provider.DiagnosePod(ctx, str("namespace"), str("name"))
	case "cluster_health":
		payload, err = s.provider.ClusterHealth(ctx, str("namespace"))
	case "upgrade_risk":
		payload, err = platform.AssessUpgradeRisk(ctx, s.provider)
	case "list_firing_alerts":
		if c := platform.AlertmanagerFromEnv(); c != nil {
			payload, err = c.ListFiring(ctx)
		} else {
			payload = map[string]any{"available": false, "note": "Set AXIOM_ALERTMANAGER_URL"}
		}
	case "list_accelerators":
		payload, err = s.provider.ListAccelerators(ctx)
	case "still_preview":
		action := platform.StillAction(str("action"))
		if action == "" {
			action = platform.StillSeal
		}
		payload, err = platform.StillPreview(ctx, s.provider, str("namespace"), str("kind"), str("name"), action, "")
	case "fleet_search":
		if sp, ok := s.provider.(*platform.SwitchableProvider); ok {
			payload = sp.SearchFleet(ctx, str("q"), nil, 0)
		} else {
			payload = platform.EmptyFleetSearch(str("q"))
		}
	case "network_guard":
		kind := str("kind")
		if kind == "" {
			kind = "Deployment"
		}
		payload, err = s.provider.WorkloadNetworkGuard(ctx, str("namespace"), kind, str("name"))
	case "list_agent_runs":
		payload, err = s.provider.ListAgentRuns(ctx, str("namespace"))
	case "get_workload":
		payload, err = s.provider.GetResource(ctx, str("namespace"), str("kind"), str("name"))
	case "list_dra":
		payload, err = platform.ListDRA(ctx, s.provider)
	case "outage_narrative":
		payload = platform.BuildOutageNarrative(ctx, s.provider, str("cluster"), str("from"), str("state"))
	case "propose_seal":
		ns := str("namespace")
		if ns == "" {
			ns = "default"
		}
		kind := str("kind")
		if kind == "" {
			kind = "Deployment"
		}
		goal := str("goal")
		if goal == "" {
			goal = "Seal workload after network finding — isolate egress"
		}
		dry := true
		req := platform.CreateAgentRunRequest{
			Namespace: ns, Goal: goal, DryRun: &dry, Source: platform.AgentSourceMCP,
			ExtraContext: "seal isolate packetwolf unexpected egress",
			Target:       &platform.AgentObjectRef{APIVersion: "apps/v1", Kind: kind, Namespace: ns, Name: str("name")},
		}
		run, cerr := s.provider.CreateAgentRun(ctx, req)
		if cerr != nil {
			err = cerr
			break
		}
		planned, _ := platform.PlanAgentRun(ctx, s.provider, s.aiSettings(), s.mode == "demo", run, req.ExtraContext)
		if updated, uerr := s.provider.UpdateAgentRun(ctx, planned); uerr == nil {
			run = updated
		} else {
			run = planned
		}
		payload = run
	case "approve_agent_run":
		payload, err = s.provider.ApproveAgentRun(ctx, str("namespace"), str("name"))
	case "deny_agent_run":
		payload, err = s.provider.DenyAgentRun(ctx, str("namespace"), str("name"))
	case "create_agent_run":
		dry := true
		if v, ok := args["dryRun"].(bool); ok {
			dry = v
		}
		req := platform.CreateAgentRunRequest{
			Namespace: str("namespace"),
			Goal:      str("goal"),
			DryRun:    &dry,
			Source:    platform.AgentSourceMCP,
		}
		if req.Namespace == "" {
			req.Namespace = "default"
		}
		if kind, name := str("kind"), str("name"); name != "" {
			req.Target = &platform.AgentObjectRef{Kind: kind, Name: name, Namespace: req.Namespace, APIVersion: "v1"}
			if kind == "" {
				req.Target.Kind = "Pod"
			}
		}
		run, cerr := s.provider.CreateAgentRun(ctx, req)
		if cerr != nil {
			err = cerr
			break
		}
		planned, _ := platform.PlanAgentRun(ctx, s.provider, s.aiSettings(), s.mode == "demo", run, "mcp")
		if updated, uerr := s.provider.UpdateAgentRun(ctx, planned); uerr == nil {
			run = updated
		} else {
			run = planned
		}
		platform.RecordAxiomAudit("agentrun.create", run.Namespace+"/"+run.Name, "mcp:"+run.Goal, s.auditUser(r))
		EmitStateChanged("agentrun", "create")
		payload = run
	default:
		return nil, fmt.Errorf("unknown tool %q", p.Name)
	}
	if err != nil {
		return map[string]any{
			"content": []map[string]any{{"type": "text", "text": err.Error()}},
			"isError": true,
		}, nil
	}
	raw, _ := json.MarshalIndent(payload, "", "  ")
	if len(raw) > 100_000 {
		raw = raw[:100_000]
	}
	platform.RecordAxiomAudit("mcp.tools_call", p.Name, "", s.auditUser(r))
	return map[string]any{
		"content": []map[string]any{{"type": "text", "text": string(raw)}},
	}, nil
}
