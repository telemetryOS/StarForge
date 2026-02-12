package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"

	"github.com/RobertWHurst/navaros"

	"github.com/telemetryos/starforge/installer"
	"github.com/telemetryos/starforge/installer/diskutil"
	"github.com/telemetryos/starforge/installer/server/routes/installations"
	"github.com/telemetryos/starforge/installer/server/routes/payloads"
)

func main() {
	port := flag.Int("port", 8100, "HTTP listen port")
	payloadDir := flag.String("payload-dir", "/payloads", "directory containing payload images")
	flag.Parse()

	if _, err := os.Stat(*payloadDir); err != nil {
		log.Fatalf("payload directory %q: %v", *payloadDir, err)
	}

	manager := installations.NewManager(*payloadDir)

	router := navaros.NewRouter()

	// Inject dependencies into request context.
	router.Use(func(ctx *navaros.Context) {
		ctx.Set("manager", manager)
		ctx.Set("payloadDir", *payloadDir)

		// JSON response marshaller.
		ctx.SetResponseBodyMarshaller(func(from any) (io.Reader, error) {
			data, err := json.Marshal(from)
			if err != nil {
				return nil, err
			}
			ctx.Headers.Set("Content-Type", "application/json")
			return bytes.NewReader(data), nil
		})

		ctx.Next()
	})

	// Payload routes.
	router.Use(payloadRouter(*payloadDir))

	// Disk routes.
	router.Use(diskRouter())

	// System routes.
	router.Use(systemRouter())

	// Installation routes.
	router.Use(installations.Router())

	addr := fmt.Sprintf(":%d", *port)
	log.Printf("starforge-install-server listening on %s (payloads: %s)", addr, *payloadDir)
	if err := http.ListenAndServe(addr, router); err != nil {
		log.Fatal(err)
	}
}

func payloadRouter(payloadDir string) *navaros.Router {
	r := navaros.NewRouter()

	r.Get("/payloads", func(ctx *navaros.Context) {
		entries, err := os.ReadDir(payloadDir)
		if err != nil {
			ctx.Status = http.StatusInternalServerError
			ctx.Body = map[string]string{"error": "reading payload directory"}
			return
		}

		var manifests []installer.PayloadManifest

		// Check for flat layout (single manifest.json at root).
		if flatManifest, err := os.ReadFile(filepath.Join(payloadDir, "manifest.json")); err == nil {
			var m installer.PayloadManifest
			if json.Unmarshal(flatManifest, &m) == nil {
				manifests = append(manifests, m)
			}
		}

		// Check for nested layout (subdirectories with manifest.json).
		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}
			manifestPath := filepath.Join(payloadDir, entry.Name(), "manifest.json")
			data, err := os.ReadFile(manifestPath)
			if err != nil {
				continue
			}
			var m installer.PayloadManifest
			if json.Unmarshal(data, &m) == nil {
				manifests = append(manifests, m)
			}
		}

		ctx.Status = http.StatusOK
		ctx.Body = manifests
	})

	r.Get("/payloads/:name", func(ctx *navaros.Context) {
		name := ctx.Params().Get("name")
		resolved, err := payloads.ResolvePayloadDir(payloadDir, name)
		if err != nil {
			ctx.Status = http.StatusNotFound
			ctx.Body = map[string]string{"error": fmt.Sprintf("payload %q not found", name)}
			return
		}

		data, err := os.ReadFile(filepath.Join(resolved, "manifest.json"))
		if err != nil {
			ctx.Status = http.StatusNotFound
			ctx.Body = map[string]string{"error": fmt.Sprintf("payload %q not found", name)}
			return
		}

		var m installer.PayloadManifest
		if err := json.Unmarshal(data, &m); err != nil {
			ctx.Status = http.StatusInternalServerError
			ctx.Body = map[string]string{"error": "parsing manifest"}
			return
		}

		ctx.Status = http.StatusOK
		ctx.Body = m
	})

	return r
}

func diskRouter() *navaros.Router {
	r := navaros.NewRouter()

	r.Get("/disks", func(ctx *navaros.Context) {
		disks, err := diskutil.ListDisks()
		if err != nil {
			ctx.Status = http.StatusInternalServerError
			ctx.Body = map[string]string{"error": fmt.Sprintf("listing disks: %v", err)}
			return
		}
		ctx.Status = http.StatusOK
		ctx.Body = disks
	})

	return r
}

func systemRouter() *navaros.Router {
	r := navaros.NewRouter()

	r.Get("/system", func(ctx *navaros.Context) {
		hostname, _ := os.Hostname()

		var memTotal uint64
		if data, err := os.ReadFile("/proc/meminfo"); err == nil {
			fmt.Sscanf(string(data), "MemTotal: %d", &memTotal)
			memTotal *= 1024 // /proc/meminfo reports kB
		}

		ctx.Status = http.StatusOK
		ctx.Body = map[string]any{
			"hostname": hostname,
			"arch":     runtime.GOARCH,
			"memory":   memTotal,
		}
	})

	r.Post("/system/reboot", func(ctx *navaros.Context) {
		ctx.Status = http.StatusOK
		ctx.Body = map[string]string{"status": "rebooting"}

		go func() {
			exec.Command("systemctl", "reboot").Run()
		}()
	})

	return r
}

