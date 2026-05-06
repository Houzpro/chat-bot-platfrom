package database

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
)

// SeedBaseModels scans modelsDir for *.gguf files and registers them as
// type='base' rows. Idempotent — files already in the table (matched by
// file_path) are skipped, so this is safe to run on every backend boot.
//
// `defaultEndpoint` is the in-network URL of the platform-wide llama-cpp
// container (typically http://llama-cpp:8080). All seeded base models share
// it, since the default container loads exactly one GGUF at a time chosen
// via the GGUF_MODEL_FILE env var. The "active" base model is the one
// whose filename matches activeFilename — for the rest we still seed a row
// (so the dropdown lists what files exist), but we mark them as 'stopped'
// so the UI can show that they aren't currently being served.
func SeedBaseModels(repo *ModelRepository, modelsDir, defaultEndpoint, activeFilename string) error {
	if modelsDir == "" {
		return nil
	}
	entries, err := os.ReadDir(modelsDir)
	if err != nil {
		// Missing dir is not fatal — the caller may run the backend without
		// any local models (e.g. in a CI image). Log and move on.
		log.Printf("[SeedBaseModels] cannot read %s: %v", modelsDir, err)
		return nil
	}

	seeded := 0
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if !strings.HasSuffix(strings.ToLower(name), ".gguf") {
			continue
		}
		filePath := filepath.ToSlash(filepath.Join(modelsDir, name))
		existing, err := repo.FindByFilePath(filePath)
		if err != nil {
			log.Printf("[SeedBaseModels] lookup %s: %v", filePath, err)
			continue
		}
		if existing != nil {
			// Already registered — refresh status/endpoint in case
			// GGUF_MODEL_FILE changed since the last boot. The container
			// only serves the currently-active file; the rest live as
			// "stopped" rows the user can see but not chat with yet.
			status := "stopped"
			endpoint := ""
			if name == activeFilename {
				status = "running"
				endpoint = defaultEndpoint
			}
			if existing.Status != status || existing.EndpointURL != endpoint {
				existing.Status = status
				existing.EndpointURL = endpoint
				_ = repo.Update(existing)
			}
			continue
		}

		status := "stopped"
		endpoint := ""
		containerName := ""
		if name == activeFilename {
			status = "running"
			endpoint = defaultEndpoint
			containerName = "chatbot-llama-cpp"
		}

		m := &Model{
			Name:          friendlyModelName(name),
			Type:          "base",
			FilePath:      filePath,
			GGUFPath:      filePath,
			ContainerName: containerName,
			EndpointURL:   endpoint,
			Status:        status,
		}
		if _, err := repo.Create(m); err != nil {
			log.Printf("[SeedBaseModels] create %s: %v", filePath, err)
			continue
		}
		seeded++
	}
	if seeded > 0 {
		log.Printf("[SeedBaseModels] registered %d base model(s) from %s", seeded, modelsDir)
	}
	return nil
}

// friendlyModelName turns "Qwen3.5-4B-Q4_K_M.gguf" into "Qwen3.5-4B-Q4_K_M".
// Kept dumb on purpose — operators can rename the row in DB if they want a
// nicer label, but we don't try to be clever here.
func friendlyModelName(filename string) string {
	base := filename
	if idx := strings.LastIndex(filename, "."); idx > 0 {
		base = filename[:idx]
	}
	if base == "" {
		return filename
	}
	return fmt.Sprintf("%s (base)", base)
}
