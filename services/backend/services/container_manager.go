package services

import (
	"backend/database"
	"context"
	"fmt"
	"log"
	"strings"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/mount"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/client"
)

// ContainerManager owns the lifecycle of per-model llama.cpp server
// containers. It is the only place in the codebase that calls into the
// Docker daemon — handlers route requests through ManagerAPI so we can
// swap implementations later (e.g. a sidecar orchestrator) without
// touching call sites.
//
// Concurrency: a single mutex guards the port-pool scan. The Docker
// daemon serializes its own requests, so we don't need finer-grained
// locking around individual container ops.
type ContainerManager struct {
	cli           *client.Client
	repo          *database.ModelRepository
	image         string
	network       string
	modelsHostDir string // host path mounted into the container, e.g. /models
	portMin       int
	portMax       int
	nCtx          int
	nThreads      int
	nGPULayers    int
	parallel      int
	useGPU        bool
	cacheTypeK    string
	cacheTypeV    string
}

// ManagerConfig collects the env-driven knobs in one place. Zero values
// fall back to sane defaults inside NewContainerManager.
type ManagerConfig struct {
	Image         string
	Network       string
	ModelsHostDir string
	PortMin       int
	PortMax       int
	NCtx          int
	NThreads      int
	NGPULayers    int
	Parallel      int
	UseGPU        bool
	// KV cache quantization. Empty string keeps llama.cpp's default (f16).
	// Use "q8_0" to halve VRAM/RAM for the KV cache at minor quality cost.
	CacheTypeK string
	CacheTypeV string
}

func NewContainerManager(repo *database.ModelRepository, cfg ManagerConfig) (*ContainerManager, error) {
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return nil, fmt.Errorf("docker client init: %w", err)
	}

	if cfg.Image == "" {
		// Pick the right default image based on UseGPU so operators don't
		// have to remember to flip both flags. Override LLAMA_IMAGE to
		// pin a specific tag.
		if cfg.UseGPU {
			cfg.Image = "ghcr.io/ggml-org/llama.cpp:server-cuda"
		} else {
			cfg.Image = "ghcr.io/ggml-org/llama.cpp:server"
		}
	}
	if cfg.Network == "" {
		cfg.Network = "chat-bot-platfrom_default"
	}
	if cfg.PortMin == 0 {
		cfg.PortMin = 8100
	}
	if cfg.PortMax == 0 {
		cfg.PortMax = 8199
	}
	if cfg.NCtx == 0 {
		cfg.NCtx = 8192
	}
	if cfg.NThreads == 0 {
		cfg.NThreads = 6
	}
	if cfg.Parallel == 0 {
		cfg.Parallel = 1
	}
	// On GPU, default to offloading every layer (-1) unless the operator
	// explicitly capped it. CPU keeps NGPULayers=0 by default.
	if cfg.UseGPU && cfg.NGPULayers == 0 {
		cfg.NGPULayers = -1
	}

	return &ContainerManager{
		cli:           cli,
		repo:          repo,
		image:         cfg.Image,
		network:       cfg.Network,
		modelsHostDir: cfg.ModelsHostDir,
		portMin:       cfg.PortMin,
		portMax:       cfg.PortMax,
		nCtx:          cfg.NCtx,
		nThreads:      cfg.NThreads,
		nGPULayers:    cfg.NGPULayers,
		parallel:      cfg.Parallel,
		useGPU:        cfg.UseGPU,
		cacheTypeK:    cfg.CacheTypeK,
		cacheTypeV:    cfg.CacheTypeV,
	}, nil
}

// Ping verifies the daemon is reachable. Called once on startup so the
// operator sees a clear log line if the socket isn't mounted, instead of
// the first deploy click failing with an opaque error.
func (m *ContainerManager) Ping(ctx context.Context) error {
	_, err := m.cli.Ping(ctx)
	return err
}

// pickFreePort scans the registry for ports already claimed by running
// finetuned containers and returns the smallest unused port in our pool.
// We don't probe the Docker daemon directly — the registry is the source
// of truth, and stale rows are reconciled by reconcile() at startup.
func (m *ContainerManager) pickFreePort(ctx context.Context, models []database.Model) (int, error) {
	used := make(map[int]struct{}, len(models))
	for _, mm := range models {
		if mm.ContainerPort > 0 {
			used[mm.ContainerPort] = struct{}{}
		}
	}
	for p := m.portMin; p <= m.portMax; p++ {
		if _, taken := used[p]; !taken {
			return p, nil
		}
	}
	return 0, fmt.Errorf("no free ports in range %d-%d", m.portMin, m.portMax)
}

// Deploy starts a llama.cpp container for the given model. Idempotent: if
// a container with the same name is already running the existing endpoint
// is returned without recreating it.
//
// Returns the (containerName, port, endpointURL) that the caller should
// persist on the model row.
func (m *ContainerManager) Deploy(ctx context.Context, model *database.Model, allModels []database.Model) (string, int, string, error) {
	if model.GGUFPath == "" && model.FilePath == "" {
		return "", 0, "", fmt.Errorf("model has no GGUF path")
	}
	ggufPath := model.GGUFPath
	if ggufPath == "" {
		ggufPath = model.FilePath
	}

	containerName := containerNameFor(model.ID)

	// If a container of this name already exists, decide what to do based
	// on its state. The common case here is a backend restart while the
	// container is still up — we just reuse it.
	existing, err := m.findContainerByName(ctx, containerName)
	if err == nil && existing != nil {
		if existing.State == "running" {
			log.Printf("[ContainerManager] %s already running, reusing", containerName)
			return containerName, model.ContainerPort, model.EndpointURL, nil
		}
		// Container exists but is not running — remove and recreate so we
		// don't carry over a broken state (e.g. crash loop, stale config).
		log.Printf("[ContainerManager] %s exists in state=%s, removing before redeploy", containerName, existing.State)
		_ = m.cli.ContainerRemove(ctx, existing.ID, container.RemoveOptions{Force: true})
	}

	port, err := m.pickFreePort(ctx, allModels)
	if err != nil {
		return "", 0, "", err
	}

	// Map the gguf path that's stored on disk (e.g. "/models/foo.gguf") to
	// the path inside the new llama.cpp container. We mount the same host
	// dir at /models, so the path translation is a 1:1 substitution.
	mountedPath := ggufPath
	if !strings.HasPrefix(ggufPath, "/models/") {
		// The seed function uses "/models/<filename>"; if a future code path
		// stores something else, fall back to the basename under /models.
		// This avoids passing a backend-internal path that doesn't exist
		// inside the new container.
		idx := strings.LastIndexAny(ggufPath, "/\\")
		basename := ggufPath
		if idx >= 0 {
			basename = ggufPath[idx+1:]
		}
		mountedPath = "/models/" + basename
	}

	cmd := []string{
		"--model", mountedPath,
		"--host", "0.0.0.0",
		"--port", "8080",
		"--ctx-size", fmt.Sprintf("%d", m.nCtx),
		"--threads", fmt.Sprintf("%d", m.nThreads),
		"--n-gpu-layers", fmt.Sprintf("%d", m.nGPULayers),
		"--parallel", fmt.Sprintf("%d", m.parallel),
	}
	// KV cache quant — matches what the static llama-cpp service uses on
	// GPU mode (q8_0/q8_0). Skipped when empty so CPU defaults stay f16.
	if m.cacheTypeK != "" {
		cmd = append(cmd, "--cache-type-k", m.cacheTypeK)
	}
	if m.cacheTypeV != "" {
		cmd = append(cmd, "--cache-type-v", m.cacheTypeV)
	}

	hostCfg := &container.HostConfig{
		RestartPolicy: container.RestartPolicy{Name: "unless-stopped"},
		Mounts: []mount.Mount{
			{
				Type:   mount.TypeBind,
				Source: m.modelsHostDir,
				Target: "/models",
			},
		},
	}
	// Request the NVIDIA GPU when configured. This is the SDK equivalent
	// of compose's `deploy.resources.reservations.devices` block. Without
	// this, the cuda image would still start but llama.cpp would log
	// "no usable GPU found" and silently fall back to CPU.
	if m.useGPU {
		hostCfg.Resources.DeviceRequests = []container.DeviceRequest{
			{
				Driver:       "nvidia",
				Count:        -1, // all available GPUs
				Capabilities: [][]string{{"gpu"}},
			},
		}
	}

	createResp, err := m.cli.ContainerCreate(ctx,
		&container.Config{
			Image: m.image,
			Cmd:   cmd,
			Labels: map[string]string{
				"chatbot.managed":  "true",
				"chatbot.model_id": model.ID,
			},
		},
		hostCfg,
		&network.NetworkingConfig{
			EndpointsConfig: map[string]*network.EndpointSettings{
				m.network: {},
			},
		},
		nil,
		containerName,
	)
	if err != nil {
		return "", 0, "", fmt.Errorf("create container: %w", err)
	}

	if err := m.cli.ContainerStart(ctx, createResp.ID, container.StartOptions{}); err != nil {
		// Best-effort cleanup so we don't leak a broken container row.
		_ = m.cli.ContainerRemove(ctx, createResp.ID, container.RemoveOptions{Force: true})
		return "", 0, "", fmt.Errorf("start container: %w", err)
	}

	endpoint := fmt.Sprintf("http://%s:8080", containerName)
	log.Printf("[ContainerManager] deployed %s (port %d) endpoint=%s", containerName, port, endpoint)
	return containerName, port, endpoint, nil
}

// Stop stops + removes the model's container. Safe to call when the
// container doesn't exist — no-op in that case so the user can recover
// from a manual `docker rm`.
func (m *ContainerManager) Stop(ctx context.Context, containerName string) error {
	if containerName == "" {
		return nil
	}
	existing, err := m.findContainerByName(ctx, containerName)
	if err != nil {
		return err
	}
	if existing == nil {
		return nil
	}
	timeout := 10
	_ = m.cli.ContainerStop(ctx, existing.ID, container.StopOptions{Timeout: &timeout})
	if err := m.cli.ContainerRemove(ctx, existing.ID, container.RemoveOptions{Force: true}); err != nil {
		return fmt.Errorf("remove container: %w", err)
	}
	log.Printf("[ContainerManager] stopped %s", containerName)
	return nil
}

// Status returns the Docker state ("running" / "exited" / ...) for the
// model's container, or "" if the container is gone.
func (m *ContainerManager) Status(ctx context.Context, containerName string) (string, error) {
	if containerName == "" {
		return "", nil
	}
	existing, err := m.findContainerByName(ctx, containerName)
	if err != nil {
		return "", err
	}
	if existing == nil {
		return "", nil
	}
	return existing.State, nil
}

// findContainerByName returns nil, nil when the container is not present.
// Docker's name filter requires a leading "/" but we hide that detail
// here so callers pass the bare name they wrote on creation.
func (m *ContainerManager) findContainerByName(ctx context.Context, name string) (*containerSummary, error) {
	f := filters.NewArgs()
	f.Add("name", "^/?"+name+"$")
	list, err := m.cli.ContainerList(ctx, container.ListOptions{All: true, Filters: f})
	if err != nil {
		return nil, fmt.Errorf("list containers: %w", err)
	}
	for _, c := range list {
		for _, n := range c.Names {
			if n == "/"+name || n == name {
				return &containerSummary{ID: c.ID, State: c.State}, nil
			}
		}
	}
	return nil, nil
}

type containerSummary struct {
	ID    string
	State string
}

// Reconcile syncs DB rows with what Docker actually reports. Called once
// on backend startup: if a model row claims status='running' but its
// container is gone (e.g. compose down between deploys), we flip the row
// back to 'stopped' so the next /deploy is a fresh start, not a stale
// "already running" reuse.
func (m *ContainerManager) Reconcile(ctx context.Context, models []database.Model) {
	for i := range models {
		mm := &models[i]
		if mm.ContainerName == "" {
			continue
		}
		// Skip the platform-default base container — its lifecycle is owned
		// by docker-compose, not by us. We can't see compose-managed
		// containers reliably from inside the backend, so treat
		// docker-compose-named entries as "not ours to manage".
		if !strings.HasPrefix(mm.ContainerName, "chatbot-llama-ft-") {
			continue
		}
		state, err := m.Status(ctx, mm.ContainerName)
		if err != nil {
			log.Printf("[ContainerManager] reconcile %s: %v", mm.ContainerName, err)
			continue
		}
		desired := mm.Status
		if state == "" {
			// Container is gone.
			desired = "stopped"
			mm.ContainerName = ""
			mm.ContainerPort = 0
			mm.EndpointURL = ""
		} else if state == "running" {
			desired = "running"
		} else {
			desired = "stopped"
		}
		if desired != mm.Status {
			mm.Status = desired
			if err := m.repo.Update(mm); err != nil {
				log.Printf("[ContainerManager] reconcile update %s: %v", mm.ID, err)
			}
		}
	}
}

// containerNameFor returns the deterministic Docker container name for a
// model. Short prefix of the UUID keeps it readable in `docker ps` output
// without risking collisions across users.
func containerNameFor(modelID string) string {
	short := strings.ReplaceAll(modelID, "-", "")
	if len(short) > 12 {
		short = short[:12]
	}
	return "chatbot-llama-ft-" + short
}

